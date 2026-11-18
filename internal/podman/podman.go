// Package podman runs the podman CLI.
//
// burrow drives the podman command, not its API: the same binary works on Linux and, as a remote
// client of a Podman machine, on macOS and Windows (D6). Arguments are always passed as a list,
// never through a shell.
package podman

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner runs podman. Tests and --dry-run use a Recorder instead of the real binary.
type Runner interface {
	// Output runs podman and returns its trimmed standard output.
	Output(ctx context.Context, args ...string) (string, error)
	// Attach runs podman with the caller's standard streams (interactive commands).
	Attach(ctx context.Context, args ...string) error
}

// CLI runs a podman binary.
type CLI struct {
	Path    string    // podman binary; "podman" from PATH by default
	Verbose io.Writer // when set, every command is printed here first
}

func (c *CLI) bin() string {
	if c.Path == "" {
		return "podman"
	}
	return c.Path
}

func (c *CLI) trace(args []string) {
	if c.Verbose != nil {
		_, _ = fmt.Fprintln(c.Verbose, "+", Quote(c.bin(), args))
	}
}

// Output implements Runner.
func (c *CLI) Output(ctx context.Context, args ...string) (string, error) {
	c.trace(args)
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, c.bin(), args...) //nolint:gosec // fixed binary, argument list
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("podman %s: %w: %s", first(args), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// Attach implements Runner.
func (c *CLI) Attach(ctx context.Context, args ...string) error {
	c.trace(args)
	cmd := exec.CommandContext(ctx, c.bin(), args...) //nolint:gosec // fixed binary, argument list
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// Recorder records commands instead of running them. Replies maps a command prefix to its output.
type Recorder struct {
	Calls   [][]string
	Replies map[string]string
	Out     io.Writer // when set (--dry-run), commands are printed
}

// Output implements Runner.
func (r *Recorder) Output(_ context.Context, args ...string) (string, error) {
	r.Calls = append(r.Calls, args)
	if r.Out != nil {
		_, _ = fmt.Fprintln(r.Out, Quote("podman", args))
	}
	joined := strings.Join(args, " ")
	best := ""
	for prefix := range r.Replies {
		if strings.HasPrefix(joined, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best != "" {
		return r.Replies[best], nil
	}
	return "", nil
}

// Attach implements Runner.
func (r *Recorder) Attach(ctx context.Context, args ...string) error {
	_, err := r.Output(ctx, args...)
	return err
}

// Quote renders a command for humans (logs, --dry-run); it is never executed by a shell.
func Quote(bin string, args []string) string {
	parts := []string{bin}
	for _, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'$`\\|&;<>(){}*?!#~") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

func first(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}
