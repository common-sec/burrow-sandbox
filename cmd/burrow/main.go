// Command burrow manages sandboxes for AI agents and their tools on rootless Podman.
//
// PROTOTYPE (0.1): see docs/ROADMAP.md for what is in and out.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"

	"github.com/common-sec/burrow-sandbox/internal/config"
	"github.com/common-sec/burrow-sandbox/internal/podman"
	"github.com/common-sec/burrow-sandbox/internal/sandbox"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

const usage = `burrow: sandboxes for AI agents and their tools, on rootless Podman (prototype)

Usage:
  burrow [flags] up ENV_FILE                 create an environment: internal network, proxy, sandbox
  burrow [flags] exec NAME [-- CMD ARGS...]  run a command in the sandbox (default: sh)
  burrow [flags] run NAME PRESET_FILE [-- ARGS...]
                                             run a tool preset as an ephemeral sibling
  burrow [flags] port NAME PORT [HOST_PORT]  publish a sandbox port on 127.0.0.1 after start
  burrow [flags] down NAME [--purge]         remove the environment (--purge: also its home and definition)
  burrow [flags] ls                          list environments
  burrow version

Flags:
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
	state, err := stateDir()
	if err != nil {
		return err
	}
	m := &sandbox.Manager{Podman: runner, StateDir: state, TTY: isTerminal(os.Stdin)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "up":
		if len(rest) != 1 {
			return errors.New("usage: burrow up ENV_FILE")
		}
		e, err := config.LoadEnv(rest[0])
		if err != nil {
			return err
		}
		if err := m.Up(ctx, e); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "%s is up (engine %s). Next: burrow exec %s\n", e.Name, e.Engine, e.Name)
		return nil
	case "exec":
		if len(rest) < 1 {
			return errors.New("usage: burrow exec NAME [-- CMD ARGS...]")
		}
		return m.Exec(ctx, rest[0], afterDash(rest[1:]))
	case "run":
		if len(rest) < 2 {
			return errors.New("usage: burrow run NAME PRESET_FILE [-- ARGS...]")
		}
		p, err := config.LoadPreset(rest[1])
		if err != nil {
			return err
		}
		return m.Run(ctx, rest[0], p, afterDash(rest[2:]))
	case "port":
		if len(rest) < 2 || len(rest) > 3 {
			return errors.New("usage: burrow port NAME PORT [HOST_PORT]")
		}
		p, err := strconv.Atoi(rest[1])
		if err != nil {
			return fmt.Errorf("PORT: %w", err)
		}
		hp := p
		if len(rest) == 3 {
			if hp, err = strconv.Atoi(rest[2]); err != nil {
				return fmt.Errorf("HOST_PORT: %w", err)
			}
		}
		if err := m.Port(ctx, rest[0], p, hp); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "sandbox port %d is on http://127.0.0.1:%d\n", p, hp)
		return nil
	case "down":
		if len(rest) < 1 || len(rest) > 2 || (len(rest) == 2 && rest[1] != "--purge") {
			return errors.New("usage: burrow down NAME [--purge]")
		}
		return m.Down(ctx, rest[0], len(rest) == 2)
	case "ls":
		out, err := m.List(ctx)
		if err != nil {
			return err
		}
		if out != "" {
			fmt.Println(out)
		}
		return nil
	case "version":
		fmt.Println("burrow", version)
		return nil
	default:
		fs.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func afterDash(a []string) []string {
	if len(a) > 0 && a[0] == "--" {
		return a[1:]
	}
	return a
}

// stateDir holds stored environment definitions: ~/.config/burrow/envs on Linux,
// %AppData%\burrow\envs on Windows, ~/Library/Application Support/burrow/envs on macOS.
func stateDir() (string, error) {
	if d := os.Getenv("BURROW_STATE_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "burrow", "envs"), nil
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
