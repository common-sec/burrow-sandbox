// Command burrow manages sandboxes for AI agents and their tools on rootless Podman.
//
// PROTOTYPE (0.1): see docs/ROADMAP.md for what is in and out.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/common-sec/burrow-sandbox/internal/config"
	"github.com/common-sec/burrow-sandbox/internal/consent"
	"github.com/common-sec/burrow-sandbox/internal/podman"
	"github.com/common-sec/burrow-sandbox/internal/sandbox"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

// SandboxFile is the sandbox file `up` (and the other commands, for the sandbox name) look for in
// the current folder, like compose.yaml.
const SandboxFile = "burrow.sandbox"

const usage = `burrow: sandboxes for AI agents and their tools, on rootless Podman (prototype)

burrow runs tool and environment presets focused on security, not images: a preset is a pinned
image plus the least privilege it needs. Presets live in your presets folder (` + "`burrow presets`" + `).

Usage:
  burrow up [-f FILE] [--image IMAGE] [--user UID:GID] [MOUNT FLAGS]
        start the persistent sandbox defined by a sandbox file (default ./` + SandboxFile + `)
  burrow run PRESET [--name NAME] [--image IMAGE] [--workspace DIR] [--user UID:GID] [MOUNT FLAGS]
        start an ad-hoc persistent sandbox from a preset (workspace: current folder)
  burrow exec [-s SANDBOX] [--strict] [--workspace DIR] [--user UID:GID] [MOUNT FLAGS] PRESET [-- ARGS...]
        run a preset as a one-off tool next to the sandbox, with what the preset asks for.
        Where the preset says inherit, it takes the sandbox's workspace mode and allowlist;
        without a sandbox (-s, or a sandbox file here) it takes the preset's fallback.
        --strict (or STRICT=true in the sandbox file): never more than the sandbox has
  burrow shell [NAME] [--user UID:GID] [-- CMD ARGS...]
        run a command inside the sandbox (default: sh)
  burrow port NAME PORT [HOST_PORT]
        publish a sandbox port on 127.0.0.1 after start
  burrow down [NAME] [--purge]
        remove the sandbox (--purge: also its home volume and stored definition)
  burrow ls        list sandboxes
  burrow presets [list | search TERM | info PRESET]
        the presets in your presets folder
  burrow version

--user UID:GID runs as that user instead of the preset's RUN_AS. Root (0:0) is allowed from the
flag, a preset or a sandbox file; burrow warns every time, and root still has no capabilities.
Root asked for by a preset or sandbox file must be confirmed: burrow asks on the terminal, and
without one (an agent, a script, CI) it refuses unless --accept root is given.
MOUNT FLAGS (decision 0012) override the preset and the sandbox file:
  --workdir PATH          where the workspace is mounted inside, and the working dir (default /work)
  --workspace-mode MODE   none, ro or rw (exec: also inherit)
  --mount ENTRY           repeatable: tmpfs:TARGET[:SIZE], volume:NAME:TARGET:MODE,
                          work:SUBDIR:TARGET:MODE (a folder inside the workspace),
                          host:PATH:TARGET:MODE (a host folder: only here or in the sandbox file;
                          burrow warns every time)
NAME defaults to the NAME in ./` + SandboxFile + `. Flags go before "--".

Global flags (before the command):
  --dry-run       print the podman commands instead of running them
  -v              print every podman command before running it
  --podman PATH   podman binary (default: podman from PATH)
  --accept LIST   accept these weakened settings without asking (comma-separated: root)
  --ci            CI mode (also when CI=true): never ask (refuse what is not accepted), no TTY,
                  warnings also as GitHub annotations; it only tightens burrow, never loosens it
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "burrow:", err)
		if h := hint(err); h != "" {
			fmt.Fprintln(os.Stderr, "burrow:", h)
		}
		var ee interface{ ExitCode() int }
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		os.Exit(1)
	}
}

// hint explains the errors people hit first. burrow never pulls localhost/ images (they are built
// locally), so a missing one ends in Podman's "image not known".
func hint(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "image not known") && strings.Contains(msg, "localhost/") {
		return "the image is not built yet: run `make test-images` (alpine, curl, trivy) or " +
			"`make sandbox-images` (agents, distributions); without make, see README \"Try it\""
	}
	return ""
}

// ui is how burrow talks to the person for this run: warnings, and the policy for settings
// that weaken a default (decision 0013).
type ui struct {
	policy  consent.Policy
	ci      bool
	actions bool // GitHub Actions: warnings also as annotations, so they show in the run summary
	stderr  io.Writer
}

// warnRoot says it plainly whenever a sandbox, tool or shell runs as root, and where root was
// asked for: --user, a preset or a sandbox file (decision 0011).
func (u *ui) warnRoot(user, from string) {
	if w := rootWarning(user, from); w != "" {
		// A warning that cannot be written to stderr has nowhere else to go.
		_, _ = fmt.Fprintln(u.stderr, w)
		if u.actions {
			_, _ = fmt.Fprintln(u.stderr, "::warning title=burrow::"+annotation(strings.TrimPrefix(w, "burrow: warning: ")))
		}
	}
}

// annotation escapes a GitHub Actions workflow command message, so a preset path with a newline
// cannot start a command of its own (GitHub's escaping: %, CR, LF).
func annotation(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// allow checks a root user against the policy (decision 0013): --user is typed by the person
// and accepted as is; root from a preset or a sandbox file must be accepted or confirmed.
func (u *ui) allow(user, from string) error {
	// A malformed user is refused by config before anything starts; only a valid root is checked.
	if valid := user != "" && config.CheckUser(user) == nil; !valid || !config.IsRoot(user) {
		return nil
	}
	if from == "" {
		from = "RUN_AS"
	}
	return u.policy.Check([]consent.Weakening{{Name: consent.Root, From: from, Explicit: from == "--user"}})
}

// root warns about and checks a root user before anything starts.
func (u *ui) root(user, from string) error {
	if err := u.allow(user, from); err != nil {
		return err
	}
	u.warnRoot(user, from)
	return nil
}

func rootWarning(user, from string) string {
	if user == "" || config.CheckUser(user) != nil || !config.IsRoot(user) {
		return ""
	}
	if from == "" {
		from = "RUN_AS"
	}
	return "burrow: warning: " + from + " " + user + " runs as root inside: no capabilities and no " +
		"new privileges, but root can change everything in the container, and the workspace files it " +
		"writes are yours on the host"
}

// newUI reads the global --ci and --accept flags. CI mode only tightens, so the CI variable may
// switch it on; nothing switches it off.
func newUI(ci bool, accept string, dry bool, getenv func(string) string) (*ui, error) {
	accepted, err := consent.ParseAccept(accept)
	if err != nil {
		return nil, err
	}
	ci = ci || consent.CIFromEnv(getenv)
	policy := consent.Policy{
		Accepted: accepted,
		CI:       ci,
		DryRun:   dry,
		Terminal: consent.ControllingTerminal{},
		Report:   os.Stdout,
	}
	return &ui{
		policy:  policy,
		ci:      ci,
		actions: ci && getenv("GITHUB_ACTIONS") == "true",
		stderr:  os.Stderr,
	}, nil
}

// listFlag collects a repeatable flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// mountOpts are --workdir, --workspace-mode and --mount (decision 0012).
type mountOpts struct {
	workdir, wsMode string
	mounts          listFlag
}

func addMountFlags(fs *flag.FlagSet) *mountOpts {
	o := &mountOpts{}
	fs.StringVar(&o.workdir, "workdir", "", "")
	fs.StringVar(&o.wsMode, "workspace-mode", "", "")
	fs.Var(&o.mounts, "mount", "")
	return o
}

// resolve checks the flags; tool allows inherit as a workspace mode.
func (o *mountOpts) resolve(tool bool) (string, config.WorkspaceMode, []config.Mount, error) {
	if o.workdir != "" {
		if err := config.CheckWorkdir(o.workdir); err != nil {
			return "", "", nil, fmt.Errorf("--workdir: %w", err)
		}
	}
	mode := config.WorkspaceMode(o.wsMode)
	switch mode {
	case "", config.WorkspaceNone, config.WorkspaceRO, config.WorkspaceRW:
	case config.WorkspaceInherit:
		if !tool {
			return "", "", nil, errors.New("--workspace-mode inherit is for exec only")
		}
	default:
		return "", "", nil, fmt.Errorf("--workspace-mode %q: none, ro or rw", o.wsMode)
	}
	ms, err := config.ParseMounts(strings.Join(o.mounts, ","), "--mount", true)
	if err != nil {
		return "", "", nil, err
	}
	for _, m := range o.mounts {
		if strings.Contains(m, ",") {
			return "", "", nil, fmt.Errorf("--mount %q: one entry per flag", m)
		}
	}
	return o.workdir, mode, ms, nil
}

func mountList(ms []config.Mount) string {
	s := make([]string, len(ms))
	for i, m := range ms {
		s[i] = m.String()
	}
	return strings.Join(s, ", ")
}

// warnHostMounts says it plainly for every host folder mounted (decision 0012).
func warnHostMounts(ms []config.Mount) {
	for _, w := range hostMountWarnings(ms) {
		fmt.Fprintln(os.Stderr, w)
	}
}

func hostMountWarnings(ms []config.Mount) []string {
	var out []string
	for _, m := range ms {
		if m.Kind == config.MountHost {
			out = append(out, "burrow: warning: host folder "+m.Source+" mounted at "+m.Target+" ("+m.Mode+
				") from "+m.From+": everything in the container can "+map[string]string{"ro": "read", "rw": "read and change"}[m.Mode]+" it")
		}
	}
	return out
}

func run(argv []string) error {
	fs := flag.NewFlagSet("burrow", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	dry := fs.Bool("dry-run", false, "")
	verbose := fs.Bool("v", false, "")
	bin := fs.String("podman", "", "")
	ci := fs.Bool("ci", false, "")
	accept := fs.String("accept", "", "")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	u, err := newUI(*ci, *accept, *dry, os.Getenv)
	if err != nil {
		return err
	}
	args := fs.Args()
	if len(args) == 0 {
		fs.Usage()
		return errors.New("missing command")
	}

	var runner podman.Runner = &podman.CLI{Path: *bin}
	if *verbose {
		runner = &podman.CLI{Path: *bin, Verbose: os.Stderr}
	}
	if *dry {
		runner = &podman.Recorder{Out: os.Stdout, Replies: map[string]string{"inspect": "10.89.0.2"}}
	}
	state, err := configDir("BURROW_STATE_DIR", "envs")
	if err != nil {
		return err
	}
	userPresets, err := configDir("BURROW_PRESETS_DIR", "presets")
	if err != nil {
		return err
	}
	m := &sandbox.Manager{Podman: runner, StateDir: state, TTY: !u.ci && isTerminal(os.Stdin)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	find := func(ref string) (*config.Preset, error) { return config.FindPreset(ref, userPresets) }

	cmd, rest := args[0], args[1:]
	// Commands that create containers or networks first check the Podman they talk to.
	if !*dry {
		switch cmd {
		case "up", "run", "exec", "port":
			if err := sandbox.Preflight(ctx, runner); err != nil {
				return err
			}
		}
	}
	sub := flag.NewFlagSet("burrow "+cmd, flag.ContinueOnError)
	sub.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	switch cmd {
	case "up":
		file := sub.String("f", SandboxFile, "")
		image := sub.String("image", "", "")
		user := sub.String("user", "", "")
		mo := addMountFlags(sub)
		pos, _, err := parseArgs(sub, rest)
		if err != nil || len(pos) != 0 {
			return errors.New("usage: burrow up [-f FILE] [--image IMAGE] [--user UID:GID] [--workdir PATH] [--workspace-mode MODE] [--mount ENTRY]")
		}
		wd, wsm, ms, err := mo.resolve(false)
		if err != nil {
			return err
		}
		keys, err := config.LoadSandboxFile(*file)
		if err != nil {
			return err
		}
		var p *config.Preset
		if ref := keys["PRESET"]; ref != "" {
			if p, err = find(ref); err != nil {
				return err
			}
		}
		e, err := config.NewSandbox(p, keys, filepath.Dir(*file), config.Overrides{
			Image: *image, RunAs: *user,
			Workdir: wd, WorkspaceMode: wsm, Mounts: ms,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", *file, err)
		}
		e.Definition = *file
		if err := u.root(e.RunAs, strings.Replace(e.RunAsFrom, "the sandbox file", *file, 1)); err != nil {
			return err
		}
		warnHostMounts(e.Mounts)
		return up(ctx, m, e)
	case "run":
		name := sub.String("name", "", "")
		image := sub.String("image", "", "")
		ws := sub.String("workspace", "", "")
		user := sub.String("user", "", "")
		mo := addMountFlags(sub)
		pos, _, err := parseArgs(sub, rest)
		if err != nil || len(pos) != 1 {
			return errors.New("usage: burrow run PRESET [--name NAME] [--image IMAGE] [--workspace DIR] [--user UID:GID] [--workdir PATH] [--workspace-mode MODE] [--mount ENTRY]")
		}
		wd, wsm, ms, err := mo.resolve(false)
		if err != nil {
			return err
		}
		p, err := find(pos[0])
		if err != nil {
			return err
		}
		dir, err := absOrCwd(*ws)
		if err != nil {
			return err
		}
		e, err := config.NewSandbox(p, nil, dir, config.Overrides{
			Name: *name, Image: *image, Workspace: dir, RunAs: *user,
			Workdir: wd, WorkspaceMode: wsm, Mounts: ms,
		})
		if err != nil {
			return err
		}
		if err := u.root(e.RunAs, e.RunAsFrom); err != nil {
			return err
		}
		warnHostMounts(e.Mounts)
		return up(ctx, m, e)
	case "exec":
		name := sub.String("s", "", "")
		strict := sub.Bool("strict", false, "")
		ws := sub.String("workspace", "", "")
		user := sub.String("user", "", "")
		mo := addMountFlags(sub)
		pos, after, err := parseArgs(sub, rest)
		if err != nil || len(pos) != 1 {
			return errors.New("usage: burrow exec [-s SANDBOX] [--strict] [--workspace DIR] [--user UID:GID] [--workdir PATH] [--workspace-mode MODE] [--mount ENTRY]... PRESET [-- ARGS...]")
		}
		wd, wsm, ms, err := mo.resolve(true)
		if err != nil {
			return err
		}
		if *name != "" && *ws != "" {
			return errors.New("exec: --workspace is for a tool without a sandbox; a sandbox brings its own")
		}
		p, err := find(pos[0])
		if err != nil {
			return err
		}
		if *user != "" {
			err = u.root(*user, "--user")
		} else {
			err = u.root(p.RunAs, "RUN_AS in preset "+p.Name+" ("+p.Source+")")
		}
		if err != nil {
			return err
		}
		if *name == "" && *ws == "" {
			if *name, err = nameFromFile(); err != nil {
				return err
			}
		}
		dir, err := absOrCwd(*ws)
		if err != nil {
			return err
		}
		warnHostMounts(ms)
		return m.Exec(ctx, *name, dir, p, after, sandbox.ExecOptions{
			Strict: *strict, User: *user,
			Workdir: wd, WorkspaceMode: wsm, Mounts: ms,
		})
	case "shell":
		user := sub.String("user", "", "")
		pos, after, err := parseArgs(sub, rest)
		if err != nil || len(pos) > 1 {
			return errors.New("usage: burrow shell [NAME] [--user UID:GID] [-- CMD ARGS...]")
		}
		if err := u.root(*user, "--user"); err != nil {
			return err
		}
		name, err := nameArg(pos)
		if err != nil {
			return err
		}
		return m.Shell(ctx, name, *user, after)
	case "port":
		pos, _, err := parseArgs(sub, rest)
		if err != nil || len(pos) < 2 || len(pos) > 3 {
			return errors.New("usage: burrow port NAME PORT [HOST_PORT]")
		}
		p, err := strconv.Atoi(pos[1])
		if err != nil {
			return fmt.Errorf("PORT: %w", err)
		}
		hp := p
		if len(pos) == 3 {
			if hp, err = strconv.Atoi(pos[2]); err != nil {
				return fmt.Errorf("HOST_PORT: %w", err)
			}
		}
		if err := m.Port(ctx, pos[0], p, hp); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "sandbox port %d is on http://127.0.0.1:%d\n", p, hp)
		return nil
	case "down":
		purge := sub.Bool("purge", false, "")
		pos, _, err := parseArgs(sub, rest)
		if err != nil || len(pos) > 1 {
			return errors.New("usage: burrow down [NAME] [--purge]")
		}
		name, err := nameArg(pos)
		if err != nil {
			return err
		}
		return m.Down(ctx, name, *purge)
	case "ls":
		out, err := m.List(ctx)
		if err != nil {
			return err
		}
		if out != "" {
			fmt.Println(out)
		}
		return nil
	case "presets":
		return presetsCmd(rest, userPresets, find)
	case "version":
		fmt.Println("burrow", version)
		return nil
	default:
		fs.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// presetsCmd lists, searches and describes the user's presets. A shared index of published
// presets is planned (roadmap 0.4); until then this is the user's presets folder.
func presetsCmd(args []string, dir string, find func(string) (*config.Preset, error)) error {
	sub := "list"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	all, listErr := config.ListPresets(dir)
	switch {
	case sub == "list" && len(args) == 0:
	case sub == "search" && len(args) == 1:
		all = config.SearchPresets(all, args[0])
	case sub == "info" && len(args) == 1:
		p, err := find(args[0])
		if err != nil {
			return err
		}
		printPreset(os.Stdout, p)
		return nil
	default:
		return errors.New("usage: burrow presets [list | search TERM | info PRESET]")
	}
	for _, p := range all {
		fmt.Printf("%-20s %s\n", p.Name, p.Description)
	}
	fmt.Fprintf(os.Stderr, "(presets folder: %s)\n", dir)
	return listErr
}

func printPreset(w io.Writer, p *config.Preset) {
	ws := string(p.WorkspaceMode)
	if p.WorkspaceMode == config.WorkspaceInherit {
		ws += " (without a sandbox: " + string(p.WorkspaceFallback) + ")"
	}
	nw := string(p.Network)
	if p.Network == config.NetworkInherit {
		nw += " (without a sandbox: " + string(p.NetworkFallback) + ")"
	}
	rows := [][2]string{
		{"name", p.Name},
		{"description", p.Description},
		{"source", p.Source},
		{"image", p.Image},
		{"workspace", ws},
		{"workdir", p.Workdir},
		{"mounts", mountList(p.Mounts)},
		{"network", nw},
		{"allow", strings.Join(p.Allow, ", ")},
		{"run as", p.RunAs},
		{"engine", string(p.Engine)},
		{"capabilities", strings.Join(p.CapAdd, ", ")},
		{"env", strings.Join(p.Env, ", ")},
		{"args", strings.Join(p.Args, " ")},
		{"memory", p.Memory},
	}
	for _, c := range p.Caches {
		rows = append(rows, [2]string{"cache", c.Volume + " at " + c.Path + " (" + c.Mode + ")"})
	}
	for _, r := range rows {
		if r[1] != "" {
			_, _ = fmt.Fprintf(w, "%-13s %s\n", r[0]+":", r[1])
		}
	}
}

func up(ctx context.Context, m *sandbox.Manager, e *config.Env) error {
	if err := m.Up(ctx, e); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s is up (engine %s). Next: burrow shell %s\n", e.Name, e.Engine, e.Name)
	return nil
}

// parseArgs parses flags anywhere before "--" (Go's flag package stops at the first positional
// argument) and returns the positional arguments and everything after "--".
func parseArgs(fs *flag.FlagSet, args []string) (pos, after []string, err error) {
	for i, a := range args {
		if a == "--" {
			args, after = args[:i], args[i+1:]
			break
		}
	}
	fs.SetOutput(io.Discard)
	for {
		if err := fs.Parse(args); err != nil {
			return nil, nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, after, nil
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

// nameArg is the sandbox name given, or the NAME in ./burrow.sandbox.
func nameArg(pos []string) (string, error) {
	if len(pos) == 1 {
		return pos[0], nil
	}
	n, err := nameFromFile()
	if err == nil && n == "" {
		err = errors.New("no NAME given and no " + SandboxFile + " here")
	}
	return n, err
}

// nameFromFile returns the NAME in ./burrow.sandbox, or "" when there is no such file.
func nameFromFile() (string, error) {
	keys, err := config.LoadSandboxFile(SandboxFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if keys["NAME"] == "" {
		return "", errors.New(SandboxFile + ": NAME is required")
	}
	return keys["NAME"], nil
}

func absOrCwd(dir string) (string, error) {
	if dir == "" {
		return os.Getwd()
	}
	return filepath.Abs(dir)
}

// configDir is a folder under burrow's configuration folder (~/.config/burrow on Linux,
// %AppData%\burrow on Windows, ~/Library/Application Support/burrow on macOS), or the value of
// the environment variable env. "envs" holds stored sandboxes, "presets" the user's presets.
func configDir(env, sub string) (string, error) {
	if d := os.Getenv(env); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "burrow", sub), nil
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
