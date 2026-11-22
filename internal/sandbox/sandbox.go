// Package sandbox turns sandbox and preset definitions into podman commands.
//
// Layout (see docs/ARCHITECTURE.md): every sandbox and every tool run (exec) sits on its own
// internal network (--internal --disable-dns: no route out, no DNS server). The only other member
// is its proxy, which also has a leg on a normal network. So the proxy is the only exit (SB3, R7),
// even for a tool that ignores HTTPS_PROXY. Tools are siblings of the sandbox, started by burrow,
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
	"slices"
	"strconv"
	"strings"

	"github.com/common-sec/burrow-sandbox/internal/config"
	"github.com/common-sec/burrow-sandbox/internal/podman"
	"github.com/common-sec/burrow-sandbox/internal/proxy"
)

const (
	proxyPort = "3128"
	labelEnv  = "burrow.env"
	labelKind = "burrow.kind"
)

// Hardening applied to everything burrow starts (SB2). --http-proxy=false: podman otherwise copies
// the host's proxy variables (and any credentials in them) into every container.
var hardening = []string{"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--http-proxy=false"}

// image returns the image argument with its pull policy. A localhost/ image is one the user
// builds (make test-images): never pull it, so a missing image is an error and not a request to
// whatever answers as a registry on localhost:443. Other images are pulled only when missing.
func image(ref string) []string {
	if strings.HasPrefix(ref, "localhost/") {
		return []string{"--pull=never", ref}
	}
	return []string{"--pull=missing", ref}
}

// requireImages fails before anything is created when a local (localhost/) image is not built,
// and names the make target that builds it. Registry images are not checked here: they are
// pinned by digest and pulled on first use, and a failed pull is cleaned up like any failure.
func (m *Manager) requireImages(ctx context.Context, refs ...string) error {
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "localhost/") {
			continue
		}
		if _, err := m.Podman.Output(ctx, "image", "exists", ref); err != nil {
			return fmt.Errorf("image %s is not built (burrow never pulls localhost/ images): run %s",
				ref, buildHint(ref))
		}
	}
	return nil
}

func buildHint(ref string) string {
	switch {
	case strings.HasPrefix(ref, "localhost/burrow-test-"):
		return "`make test-images` (alpine, curl, trivy) or `make sandbox-images` (agents, distributions); without make, see README \"Try it\""
	case strings.HasPrefix(ref, "localhost/burrow-"):
		return "`make images` (proxy, forwarder); without make, see README \"Try it\""
	default:
		return "the build for that image"
	}
}

// Manager runs sandboxes and tools.
type Manager struct {
	Podman   podman.Runner
	StateDir string // stored definitions, one file per sandbox
	TTY      bool   // attach a terminal to exec and preset runs
}

// Names of everything a sandbox owns.
func netName(env string) string     { return "burrow-" + env }
func sandboxName(env string) string { return "burrow-" + env }
func proxyName(base string) string  { return base + "-proxy" }

// egressName is the proxy's own way out: a normal network that only this proxy (and, for a
// sandbox, its port forwarders) join, so proxies of different sandboxes never share a network.
func egressName(base string) string { return base + "-egress" }
func homeVolume(env string) string  { return "burrow-" + env + "-home" }

// Up creates a sandbox: internal network, proxy, persistent container. `burrow up` and
// `burrow run` both end here, with the definition already resolved (config.NewSandbox).
func (m *Manager) Up(ctx context.Context, e *config.Env) error {
	if err := e.Validate(); err != nil {
		return err
	}
	// A stopped sandbox (down without --purge) may be brought up again; a running one may not.
	if ids, err := m.Podman.Output(ctx, "ps", "-a", "-q", "--filter", "label="+labelEnv+"="+e.Name,
		"--filter", "label="+labelKind+"=sandbox"); err != nil {
		return err
	} else if ids != "" {
		return fmt.Errorf("sandbox %q exists (`burrow down %s` first, or choose another --name)", e.Name, e.Name)
	}
	if e.Definition != "" {
		abs, err := filepath.Abs(e.Definition)
		if err != nil {
			return err
		}
		e.Definition = abs
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
	if err := m.requireImages(ctx, e.Image, e.ProxyImage); err != nil {
		return err
	}
	if err := m.saveEnv(e); err != nil {
		return err
	}
	// No sandbox container exists (checked above), so anything else with this sandbox's label is
	// left from an earlier start that failed or was interrupted: remove it first.
	if err := m.removeAll(ctx, e.Name); err != nil {
		return err
	}
	if err := m.start(ctx, e); err != nil {
		// Leave nothing half-made behind (a network without its proxy blocks the next start). The
		// definition and the home volume stay, as after `burrow down`.
		if cerr := m.removeAll(context.WithoutCancel(ctx), e.Name); cerr != nil {
			return fmt.Errorf("%w (cleanup failed too: %w; run `burrow down %s`)", err, cerr, e.Name)
		}
		return err
	}
	return nil
}

func (m *Manager) start(ctx context.Context, e *config.Env) error {
	proxyIP, err := m.startProxy(ctx, e.Name, netName(e.Name), e.ProxyImage, e.Allow)
	if err != nil {
		return err
	}
	args := append([]string{"run", "-d", "--name", sandboxName(e.Name)}, m.sandboxFlags(e, proxyIP)...)
	args = append(append(args, image(e.Image)...), "sleep", "infinity")
	_, err = m.Podman.Output(ctx, args...)
	return err
}

// startProxy creates an internal network, a dedicated egress network and the proxy that joins
// both, and returns the proxy's address on the internal one. env is the owning sandbox ("" for a
// tool run without one).
func (m *Manager) startProxy(ctx context.Context, env, net, proxyImage string, allow []string) (string, error) {
	nargs := append(append([]string{"network", "create", "--internal", "--disable-dns"}, envLabel(env)...), net)
	if _, err := m.Podman.Output(ctx, nargs...); err != nil {
		return "", err
	}
	eargs := append(append([]string{"network", "create"}, envLabel(env)...), egressName(net))
	if _, err := m.Podman.Output(ctx, eargs...); err != nil {
		return "", err
	}
	name := proxyName(net)
	args := append([]string{"run", "-d", "--name", name, "--network", egressName(net)}, envLabel(env)...)
	args = append(args, "--label", labelKind+"=proxy",
		"--read-only", "--user", "65532:65532", "--memory", "64m", "--pids-limit", "64")
	args = append(args, hardening...)
	args = append(append(args, image(proxyImage)...), "-listen", "0.0.0.0:"+proxyPort, "-allow", strings.Join(allow, ","))
	if _, err := m.Podman.Output(ctx, args...); err != nil {
		return "", err
	}
	if _, err := m.Podman.Output(ctx, "network", "connect", net, name); err != nil {
		return "", err
	}
	return m.ipOn(ctx, name, net)
}

func envLabel(env string) []string {
	if env == "" {
		return nil
	}
	return []string{"--label", labelEnv + "=" + env}
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

// sandboxFlags are shared by `up` and by `shell` on krun (which cannot exec into a running VM).
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
	f = append(f, envFlags(e.Env)...)
	if e.WorkspaceMode != config.WorkspaceNone {
		f = append(f, "-v", e.Workspace+":/work:"+string(e.WorkspaceMode), "-w", "/work")
		// The sandbox file must not be writable from inside: the agent would widen its own
		// sandbox (ALLOW, mounts) at the next `up`. Mount it read-only over itself.
		if rel, ok := inside(e.Workspace, e.Definition); ok && e.WorkspaceMode == config.WorkspaceRW {
			f = append(f, "-v", e.Definition+":/work/"+filepath.ToSlash(rel)+":ro")
		}
	}
	if e.Memory != "" {
		f = append(f, "--memory", e.Memory)
	}
	return f
}

func envFlags(env []string) []string {
	var f []string
	for _, kv := range env {
		f = append(f, "-e", kv)
	}
	return f
}

// inside reports whether path is inside dir, and its relative path.
func inside(dir, path string) (string, bool) {
	if dir == "" || path == "" {
		return "", false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// Shell runs a command (default: sh) inside the persistent sandbox.
func (m *Manager) Shell(ctx context.Context, name string, cmd []string) error {
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
		args = append(append(args, m.sandboxFlags(e, ip)...), image(e.Image)...)
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

// ExecOptions change how a tool runs.
type ExecOptions struct {
	// Strict caps the tool by its sandbox: no more workspace access, no host outside the
	// sandbox's allowlist, no added capabilities. Also on when the sandbox file says STRICT=true.
	Strict bool
}

// Exec runs a preset as a one-off tool and waits for it. The tool is a sibling of the sandbox, not
// a process inside it: it gets exactly what its preset asks for (the preset is the least privilege
// the tool needs), with its own user, mounts and proxy. Where the preset says inherit, it takes the
// sandbox's setting; without a sandbox (name == ""), or when the sandbox has nothing to inherit,
// it takes the preset's fallback (inherit:MODE; ro and none by default). workspace is the folder
// to mount when there is no sandbox.
func (m *Manager) Exec(ctx context.Context, name, workspace string, p *config.Preset, extra []string, o ExecOptions) (err error) {
	r, err := m.resolveTool(name, workspace, p, o)
	if err != nil {
		return err
	}
	need := []string{p.Image}
	if r.network == config.NetworkProxy {
		need = append(need, r.proxyImage)
	}
	if err := m.requireImages(ctx, need...); err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	base := "burrow-" + def(r.env, "exec") + "-" + p.Name + "-" + id
	args := append([]string{"run", "--rm", "-i", "--name", base}, envLabel(r.env)...)
	args = append(args, "--label", labelKind+"=tool", "--user", p.RunAs, "--read-only", "--tmpfs", "/tmp",
		"--pids-limit", "512")
	args = append(args, m.tty()...)
	args = append(args, hardening...)
	for _, c := range p.CapAdd {
		args = append(args, "--cap-add", c)
	}
	args = append(args, engineFlags(p.Engine)...)
	switch r.network {
	case config.NetworkNone:
		args = append(args, "--network", "none")
	case config.NetworkProxy:
		defer func() { err = errors.Join(err, m.removeRun(context.WithoutCancel(ctx), base)) }()
		ip, perr := m.startProxy(ctx, r.env, base, r.proxyImage, r.allow)
		if perr != nil {
			return perr
		}
		args = append(args, "--network", base)
		args = append(args, proxyEnv(ip)...)
	default:
		return fmt.Errorf("preset %s: NETWORK %q", p.Name, r.network)
	}
	args = append(args, envFlags(p.Env)...)
	if r.wsMode != config.WorkspaceNone {
		// Map the host user to the tool's own user, so the tool can read the user's files (and
		// write them, if rw) without making them world-readable. Inside, it is still its own uid.
		uid, gid, _ := strings.Cut(p.RunAs, ":")
		args = append(args, "--userns", "keep-id:uid="+uid+",gid="+gid,
			"-v", r.workspace+":/work:"+string(r.wsMode), "-w", "/work")
	}
	for _, c := range p.Caches {
		mode := c.Mode
		if mode == "rw" {
			mode = "rw,U" // the volume belongs to the preset's user, so the tool can update it
		}
		args = append(args, "-v", CacheVolume(r.env, c.Volume)+":"+c.Path+":"+mode)
	}
	if p.Memory != "" {
		args = append(args, "--memory", p.Memory)
	}
	args = append(append(append(args, image(p.Image)...), p.Args...), extra...)
	return m.Podman.Attach(ctx, args...)
}

// CacheVolume is the podman volume of a cache: scoped to the sandbox, so tools of other sandboxes
// (or run without one: scope "exec", a name reserved for this) cannot read or poison it.
// The "_" separator cannot occur in sandbox names, so one sandbox's prefix never matches another's.
func CacheVolume(env, name string) string { return "burrow-cache-" + def(env, "exec") + "_" + name }

type toolRun struct {
	env, proxyImage, workspace string
	wsMode                     config.WorkspaceMode
	network                    config.NetworkMode
	allow                      []string
}

// resolveTool turns a preset's modes into what the tool gets (see Exec).
func (m *Manager) resolveTool(name, workspace string, p *config.Preset, o ExecOptions) (*toolRun, error) {
	r := &toolRun{
		proxyImage: config.DefaultProxyImage, workspace: workspace,
		wsMode: p.WorkspaceMode, network: p.Network, allow: p.Allow,
	}
	var e *config.Env
	if name != "" {
		var err error
		if e, err = m.loadEnv(name); err != nil {
			return nil, err
		}
		r.env, r.proxyImage = e.Name, e.ProxyImage
		o.Strict = o.Strict || e.Strict
		if e.WorkspaceMode != config.WorkspaceNone {
			r.workspace = e.Workspace
		}
	}
	if r.wsMode == config.WorkspaceInherit {
		r.wsMode = p.WorkspaceFallback
		if e != nil && e.WorkspaceMode != config.WorkspaceNone {
			r.wsMode = e.WorkspaceMode
		}
	}
	if r.network == config.NetworkInherit {
		r.network = p.NetworkFallback
		if e != nil {
			r.network, r.allow = config.NetworkProxy, e.Allow // a sandbox always has a proxy
		}
	}
	if o.Strict {
		if err := checkStrict(e, p, r); err != nil {
			return nil, fmt.Errorf("strict mode: preset %s: %w", p.Name, err)
		}
	}
	if r.wsMode != config.WorkspaceNone {
		if err := config.CheckPath("workspace", r.workspace); err != nil {
			return nil, err
		}
		if st, err := os.Stat(r.workspace); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("workspace %s: not a folder", r.workspace)
		}
	}
	return r, nil
}

// checkStrict refuses a tool that would get more than its sandbox (e is nil without a sandbox:
// then only the added capabilities are refused).
func checkStrict(e *config.Env, p *config.Preset, r *toolRun) error {
	if len(p.CapAdd) > 0 {
		return fmt.Errorf("asks for capabilities %v", p.CapAdd)
	}
	if e == nil {
		return nil
	}
	if r.wsMode == config.WorkspaceRW && e.WorkspaceMode != config.WorkspaceRW {
		return fmt.Errorf("wants the workspace read-write, sandbox %s does not have it", e.Name)
	}
	if r.wsMode != config.WorkspaceNone && e.WorkspaceMode == config.WorkspaceNone {
		return fmt.Errorf("needs the workspace, sandbox %s has none", e.Name)
	}
	if r.network == config.NetworkProxy {
		sb := proxy.Policy{Hosts: e.Allow}
		for _, h := range r.allow {
			if !coveredBy(sb, h) {
				return fmt.Errorf("allows %s, which sandbox %s does not", h, e.Name)
			}
		}
	}
	return nil
}

// coveredBy reports whether every host the pattern h matches is allowed by the sandbox's policy.
func coveredBy(sb proxy.Policy, h string) bool {
	if suffix, ok := strings.CutPrefix(h, "*."); ok {
		return slices.Contains(sb.Hosts, h) || slices.Contains(sb.Hosts, "*."+suffix)
	}
	return sb.Allowed(h, 443)
}

func def(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func (m *Manager) removeRun(ctx context.Context, base string) error {
	_, err1 := m.Podman.Output(ctx, "rm", "-f", "--ignore", proxyName(base))
	_, err2 := m.Podman.Output(ctx, "network", "rm", "-f", base)
	_, err3 := m.Podman.Output(ctx, "network", "rm", "-f", egressName(base))
	return errors.Join(err1, err2, err3)
}

// Port publishes a sandbox port on 127.0.0.1 of the host after the sandbox started (R9). Podman
// cannot add a published port to a running container, so a forwarder container does it: it is
// published with -p 127.0.0.1:HOST:PORT on the sandbox's egress network, joins the sandbox's
// internal network, and relays that one port to the sandbox's address, nothing else.
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
	if err := m.requireImages(ctx, e.ForwardImage); err != nil {
		return err
	}
	ip, err := m.ipOn(ctx, sandboxName(e.Name), netName(e.Name))
	if err != nil {
		return err
	}
	fwd := fmt.Sprintf("burrow-%s-fwd-%d", e.Name, ctrPort)
	cp := strconv.Itoa(ctrPort)
	args := []string{
		"run", "-d", "--name", fwd, "--network", egressName(netName(e.Name)),
		"-p", "127.0.0.1:" + strconv.Itoa(hostPort) + ":" + cp,
		"--label", labelEnv + "=" + e.Name, "--label", labelKind + "=forward",
		"--read-only", "--user", "65532:65532", "--memory", "32m", "--pids-limit", "32",
	}
	args = append(args, hardening...)
	args = append(append(args, image(e.ForwardImage)...), "-listen", "0.0.0.0:"+cp, "-target", ip+":"+cp)
	if _, err := m.Podman.Output(ctx, args...); err != nil {
		return err
	}
	_, err = m.Podman.Output(ctx, "network", "connect", netName(e.Name), fwd)
	return err
}

// Down removes the sandbox's containers and networks. The home volume and the stored
// definition stay unless purge is set, so `up` again brings back the user's state (W1).
func (m *Manager) Down(ctx context.Context, name string, purge bool) error {
	if _, err := m.loadEnv(name); err != nil {
		return err
	}
	if err := m.removeAll(ctx, name); err != nil {
		return err
	}
	if purge {
		vols, err := m.Podman.Output(ctx, "volume", "ls", "--format", "{{.Name}}")
		if err != nil {
			return err
		}
		rm := []string{homeVolume(name)}
		for _, v := range strings.Fields(vols) {
			if strings.HasPrefix(v, CacheVolume(name, "")) {
				rm = append(rm, v) // the sandbox's caches
			}
		}
		if _, err := m.Podman.Output(ctx, append([]string{"volume", "rm", "-f"}, rm...)...); err != nil {
			return err
		}
		return os.Remove(m.envPath(name))
	}
	return nil
}

// removeAll removes every container and network labelled with the sandbox's name.
func (m *Manager) removeAll(ctx context.Context, name string) error {
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
	return nil
}

// List prints the sandboxes and their state.
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
		return nil, fmt.Errorf("no sandbox %q (start it with `burrow up` or `burrow run` first)", name)
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
