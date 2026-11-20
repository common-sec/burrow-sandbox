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
  burrow up [-f FILE] [--image IMAGE]
        start the persistent sandbox defined by a sandbox file (default ./` + SandboxFile + `)
  burrow run PRESET [--name NAME] [--image IMAGE] [--workspace DIR]
        start an ad-hoc persistent sandbox from a preset (workspace: current folder)
  burrow exec [-s SANDBOX] [--strict] [--workspace DIR] PRESET [-- ARGS...]
        run a preset as a one-off tool next to the sandbox, with what the preset asks for.
        Where the preset says inherit, it takes the sandbox's workspace mode and allowlist;
        without a sandbox (-s, or a sandbox file here) it takes the preset's fallback.
        --strict (or STRICT=true in the sandbox file): never more than the sandbox has
  burrow shell [NAME] [-- CMD ARGS...]
        run a command inside the sandbox (default: sh)
  burrow port NAME PORT [HOST_PORT]
        publish a sandbox port on 127.0.0.1 after start
  burrow down [NAME] [--purge]
        remove the sandbox (--purge: also its home volume and stored definition)
  burrow ls        list sandboxes
  burrow presets [list | search TERM | info PRESET]
        the presets in your presets folder
  burrow version

NAME defaults to the NAME in ./` + SandboxFile + `. Flags go before "--".

Global flags (before the command):
  --dry-run       print the podman commands instead of running them
  -v              print every podman command before running it
  --podman PATH   podman binary (default: podman from PATH)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "burrow:", err)
		var ee interface{ ExitCode() int }
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("burrow", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	dry := fs.Bool("dry-run", false, "")
	verbose := fs.Bool("v", false, "")
	bin := fs.String("podman", "", "")
	if err := fs.Parse(argv); err != nil {
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
	m := &sandbox.Manager{Podman: runner, StateDir: state, TTY: isTerminal(os.Stdin)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	find := func(ref string) (*config.Preset, error) { return config.FindPreset(ref, userPresets) }

	cmd, rest := args[0], args[1:]
	sub := flag.NewFlagSet("burrow "+cmd, flag.ContinueOnError)
	sub.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	switch cmd {
	case "up":
		file := sub.String("f", SandboxFile, "")
		image := sub.String("image", "", "")
		pos, _, err := parseArgs(sub, rest)
		if err != nil || len(pos) != 0 {
			return errors.New("usage: burrow up [-f FILE] [--image IMAGE]")
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
		e, err := config.NewSandbox(p, keys, filepath.Dir(*file), config.Overrides{Image: *image})
		if err != nil {
			return fmt.Errorf("%s: %w", *file, err)
		}
		e.Definition = *file
		return up(ctx, m, e)
	case "run":
		name := sub.String("name", "", "")
		image := sub.String("image", "", "")
		ws := sub.String("workspace", "", "")
		pos, _, err := parseArgs(sub, rest)
		if err != nil || len(pos) != 1 {
			return errors.New("usage: burrow run PRESET [--name NAME] [--image IMAGE] [--workspace DIR]")
		}
		p, err := find(pos[0])
		if err != nil {
			return err
		}
		dir, err := absOrCwd(*ws)
		if err != nil {
			return err
		}
		e, err := config.NewSandbox(p, nil, dir, config.Overrides{Name: *name, Image: *image, Workspace: dir})
		if err != nil {
			return err
		}
		return up(ctx, m, e)
	case "exec":
		name := sub.String("s", "", "")
		strict := sub.Bool("strict", false, "")
		ws := sub.String("workspace", "", "")
		pos, after, err := parseArgs(sub, rest)
		if err != nil || len(pos) != 1 {
			return errors.New("usage: burrow exec [-s SANDBOX] [--strict] [--workspace DIR] PRESET [-- ARGS...]")
		}
		if *name != "" && *ws != "" {
			return errors.New("exec: --workspace is for a tool without a sandbox; a sandbox brings its own")
		}
		p, err := find(pos[0])
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
		return m.Exec(ctx, *name, dir, p, after, sandbox.ExecOptions{Strict: *strict})
	case "shell":
		pos, after, err := parseArgs(sub, rest)
		if err != nil || len(pos) > 1 {
			return errors.New("usage: burrow shell [NAME] [-- CMD ARGS...]")
		}
		name, err := nameArg(pos)
		if err != nil {
			return err
		}
		return m.Shell(ctx, name, after)
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
