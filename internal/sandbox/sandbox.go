// Package sandbox turns environment and preset definitions into podman commands.
//
// Layout (see docs/ARCHITECTURE.md): every sandbox and every preset run sits on its own internal
// network (--internal --disable-dns: no route out, no DNS server). The only other member is its
// proxy, which also has a leg on a normal network. So the proxy is the only exit (SB3, R7), even
// for a tool that ignores HTTPS_PROXY. Presets are siblings of the sandbox, started by burrow,
// never by the sandbox itself (R3).
package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/common-sec/burrow-sandbox/internal/config"
	"github.com/common-sec/burrow-sandbox/internal/podman"
)

const (
	// EgressNetwork is the network the proxy (and only the proxy) uses to reach the outside.
	EgressNetwork = "podman"
	proxyPort     = "3128"
	labelEnv      = "burrow.env"
	labelKind     = "burrow.kind"
)

// Hardening applied to everything burrow starts (SB2).
var hardening = []string{"--cap-drop", "ALL", "--security-opt", "no-new-privileges"}

// Manager runs environments.
type Manager struct {
	Podman   podman.Runner
	StateDir string // stored definitions, one file per environment
	TTY      bool   // attach a terminal to exec and preset runs
}

// Names of everything an environment owns.
func netName(env string) string     { return "burrow-" + env }
func sandboxName(env string) string { return "burrow-" + env }
func proxyName(base string) string  { return base + "-proxy" }
func homeVolume(env string) string  { return "burrow-" + env + "-home" }

// Up creates the environment: internal network, proxy, persistent sandbox.
func (m *Manager) Up(ctx context.Context, e *config.Env) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.WorkspaceMode != config.WorkspaceNone {
		abs, err := filepath.Abs(e.Workspace)
		if err != nil {
			return err
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			return fmt.Errorf("WORKSPACE %s: not a folder", abs)
		}
		e.Workspace = abs
	}
	if err := m.saveEnv(e); err != nil {
		return err
	}
	net := netName(e.Name)
	proxyIP, err := m.startProxy(ctx, e.Name, net, e.ProxyImage, e.Allow)
	if err != nil {
		return err
	}
	args := append([]string{"run", "-d", "--name", sandboxName(e.Name)}, m.sandboxFlags(e, proxyIP)...)
	args = append(args, e.Image, "sleep", "infinity")
	if _, err := m.Podman.Output(ctx, args...); err != nil {
		return err
	}
	return nil
}

// startProxy creates an internal network and its proxy, and returns the proxy's address on it.
func (m *Manager) startProxy(ctx context.Context, env, net, image string, allow []string) (string, error) {
	if _, err := m.Podman.Output(ctx, "network", "create", "--internal", "--disable-dns",
		"--label", labelEnv+"="+env, net); err != nil {
		return "", err
	}
	name := proxyName(net)
	args := []string{
		"run", "-d", "--name", name, "--network", EgressNetwork,
		"--label", labelEnv + "=" + env, "--label", labelKind + "=proxy",
		"--read-only", "--user", "65532:65532", "--memory", "64m", "--pids-limit", "64",
	}
	args = append(args, hardening...)
	args = append(args, image, "-listen", "0.0.0.0:"+proxyPort, "-allow", strings.Join(allow, ","))
	if _, err := m.Podman.Output(ctx, args...); err != nil {
		return "", err
	}
	if _, err := m.Podman.Output(ctx, "network", "connect", net, name); err != nil {
		return "", err
	}
	return m.ipOn(ctx, name, net)
}

var ipRe = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,3}){3}$`)

func (m *Manager) ipOn(ctx context.Context, container, net string) (string, error) {
	ip, err := m.Podman.Output(ctx, "inspect", "--format",
		`{{(index .NetworkSettings.Networks "`+net+`").IPAddress}}`, container)
	if err != nil {
		return "", err
	}
	if !ipRe.MatchString(ip) {
		return "", fmt.Errorf("%s has no IPv4 address on %s (got %q)", container, net, ip)
	}
	return ip, nil
}

func proxyEnv(ip string) []string {
	u := "http://" + ip + ":" + proxyPort
	return []string{
		"-e", "HTTPS_PROXY=" + u, "-e", "HTTP_PROXY=" + u, "-e", "https_proxy=" + u,
		"-e", "http_proxy=" + u, "-e", "NO_PROXY=localhost,127.0.0.1", "-e", "no_proxy=localhost,127.0.0.1",
	}
}

func engineFlags(e config.Engine) []string {
	if e == config.EngineKrun {
		return []string{"--runtime", "krun"}
	}
	return nil
}

// sandboxFlags are shared by `up` and by `exec` on krun (which cannot exec into a running VM).
func (m *Manager) sandboxFlags(e *config.Env, proxyIP string) []string {
	uid, gid, _ := strings.Cut(e.RunAs, ":")
	f := []string{
		"--network", netName(e.Name), "--label", labelEnv + "=" + e.Name, "--label", labelKind + "=sandbox",
		"--userns", "keep-id:uid=" + uid + ",gid=" + gid, "--user", e.RunAs,
		"--pids-limit", "2048", "-v", homeVolume(e.Name) + ":/home/sandbox:U", "-e", "HOME=/home/sandbox",
	}
	f = append(f, hardening...)
	f = append(f, engineFlags(e.Engine)...)
	f = append(f, proxyEnv(proxyIP)...)
	if e.WorkspaceMode != config.WorkspaceNone {
		f = append(f, "-v", e.Workspace+":/work:"+string(e.WorkspaceMode), "-w", "/work")
	}
	if e.Memory != "" {
		f = append(f, "--memory", e.Memory)
	}
	return f
}

// Exec runs a command in the persistent sandbox.
func (m *Manager) Exec(ctx context.Context, name string, cmd []string) error {
	e, err := m.loadEnv(name)
	if err != nil {
		return err
	}
	if len(cmd) == 0 {
		cmd = []string{"sh"}
	}
	if e.Engine == config.EngineKrun {
		// krun has no exec. Run a short-lived VM with the sandbox's exact settings: files in the
		// workspace and home persist, processes do not.
		ip, err := m.ipOn(ctx, proxyName(netName(e.Name)), netName(e.Name))
		if err != nil {
			return err
		}
		args := append([]string{"run", "--rm", "-i"}, m.tty()...)
		args = append(append(args, m.sandboxFlags(e, ip)...), e.Image)
		return m.Podman.Attach(ctx, append(args, cmd...)...)
	}
	args := append([]string{"exec", "-i"}, m.tty()...)
	return m.Podman.Attach(ctx, append(append(args, sandboxName(e.Name)), cmd...)...)
}

func (m *Manager) tty() []string {
	if m.TTY {
		return []string{"-t"}
	}
	return nil
}

// Run starts a preset as an ephemeral sibling of the environment's sandbox and waits for it.
func (m *Manager) Run(ctx context.Context, name string, p *config.Preset, extra []string) (err error) {
	e, err := m.loadEnv(name)
	if err != nil {
		return err
	}
	if p.WorkspaceMode == config.WorkspaceRW && e.WorkspaceMode != config.WorkspaceRW {
		return errors.New("preset wants the workspace read-write, the environment does not allow it")
	}
	if p.WorkspaceMode != config.WorkspaceNone && e.WorkspaceMode == config.WorkspaceNone {
		return errors.New("preset needs the workspace, the environment has none")
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	base := "burrow-" + e.Name + "-" + p.Name + "-" + id
	args := []string{
		"run", "--rm", "-i", "--name", base, "--label", labelEnv + "=" + e.Name,
		"--label", labelKind + "=preset", "--user", p.RunAs, "--read-only", "--tmpfs", "/tmp",
		"--pids-limit", "512",
	}
	args = append(args, m.tty()...)
	args = append(args, hardening...)
	args = append(args, engineFlags(p.Engine)...)
	switch p.Network {
	case config.NetworkNone:
		args = append(args, "--network", "none")
	case config.NetworkProxy:
		defer func() { err = errors.Join(err, m.removeRun(context.WithoutCancel(ctx), base)) }()
		ip, perr := m.startProxy(ctx, e.Name, base, e.ProxyImage, p.Allow)
		if perr != nil {
			return perr
		}
		args = append(args, "--network", base)
		args = append(args, proxyEnv(ip)...)
	}
	if p.WorkspaceMode != config.WorkspaceNone {
		// Map the host user to the preset's own user, so the tool can read the user's files (and
		// write them, if rw) without making them world-readable. Inside, it is still its own uid.
		uid, gid, _ := strings.Cut(p.RunAs, ":")
		args = append(args, "--userns", "keep-id:uid="+uid+",gid="+gid,
			"-v", e.Workspace+":/work:"+string(p.WorkspaceMode), "-w", "/work")
	}
	if c := p.Cache; c != nil {
		mode := c.Mode
		if mode == "rw" {
			mode = "rw,U" // the volume belongs to the preset's user, so the tool can update it
		}
		args = append(args, "-v", "burrow-cache-"+c.Volume+":"+c.Path+":"+mode)
	}
	if p.Memory != "" {
		args = append(args, "--memory", p.Memory)
	}
	args = append(append(append(args, p.Image), p.Args...), extra...)
	return m.Podman.Attach(ctx, args...)
}

func (m *Manager) removeRun(ctx context.Context, base string) error {
	_, err1 := m.Podman.Output(ctx, "rm", "-f", "--ignore", proxyName(base))
	_, err2 := m.Podman.Output(ctx, "network", "rm", "-f", base)
	return errors.Join(err1, err2)
}

// Port publishes a sandbox port on 127.0.0.1 of the host after the sandbox started (R9). A
// forwarder container joins the sandbox's internal network and relays one port, nothing else.
func (m *Manager) Port(ctx context.Context, name string, ctrPort, hostPort int) error {
	e, err := m.loadEnv(name)
	if err != nil {
		return err
	}
	for _, p := range []int{ctrPort, hostPort} {
		if p < 1 || p > 65535 {
			return fmt.Errorf("port %d out of range", p)
		}
	}
	ip, err := m.ipOn(ctx, sandboxName(e.Name), netName(e.Name))
	if err != nil {
		return err
	}
	fwd := fmt.Sprintf("burrow-%s-fwd-%d", e.Name, ctrPort)
	cp := strconv.Itoa(ctrPort)
	args := []string{
		"run", "-d", "--name", fwd, "--network", EgressNetwork,
		"-p", "127.0.0.1:" + strconv.Itoa(hostPort) + ":" + cp,
		"--label", labelEnv + "=" + e.Name, "--label", labelKind + "=forward",
		"--read-only", "--user", "65532:65532", "--memory", "32m", "--pids-limit", "32",
	}
	args = append(args, hardening...)
	args = append(args, e.ForwardImage, "-listen", "0.0.0.0:"+cp, "-target", ip+":"+cp)
	if _, err := m.Podman.Output(ctx, args...); err != nil {
		return err
	}
	_, err = m.Podman.Output(ctx, "network", "connect", netName(e.Name), fwd)
	return err
}

// Down removes the environment's containers and networks. The home volume and the stored
// definition stay unless purge is set, so `up` again brings back the user's state (W1).
func (m *Manager) Down(ctx context.Context, name string, purge bool) error {
	if _, err := m.loadEnv(name); err != nil {
		return err
	}
	ids, err := m.Podman.Output(ctx, "ps", "-a", "-q", "--filter", "label="+labelEnv+"="+name)
	if err != nil {
		return err
	}
	if ids != "" {
		if _, err := m.Podman.Output(ctx, append([]string{"rm", "-f", "-t", "2"}, strings.Fields(ids)...)...); err != nil {
			return err
		}
	}
	nets, err := m.Podman.Output(ctx, "network", "ls", "-q", "--filter", "label="+labelEnv+"="+name)
	if err != nil {
		return err
	}
	for _, n := range strings.Fields(nets) {
		if _, err := m.Podman.Output(ctx, "network", "rm", "-f", n); err != nil {
			return err
		}
	}
	if purge {
		if _, err := m.Podman.Output(ctx, "volume", "rm", "-f", homeVolume(name)); err != nil {
			return err
		}
		return os.Remove(m.envPath(name))
	}
	return nil
}

// List prints the environments' sandboxes and their state.
func (m *Manager) List(ctx context.Context) (string, error) {
	return m.Podman.Output(ctx, "ps", "-a", "--filter", "label="+labelKind+"=sandbox",
		"--format", `{{index .Labels "`+labelEnv+`"}}	{{.State}}	{{.Image}}`)
}

func (m *Manager) envPath(name string) string { return filepath.Join(m.StateDir, name+".env") }

func (m *Manager) saveEnv(e *config.Env) error {
	if err := os.MkdirAll(m.StateDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(m.envPath(e.Name), []byte(e.String()), 0o600)
}

func (m *Manager) loadEnv(name string) (*config.Env, error) {
	e, err := config.LoadEnv(m.envPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no environment %q (run `burrow up` first)", name)
	}
	return e, err
}

func randomID() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
