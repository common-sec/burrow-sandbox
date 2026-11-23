// Package consent decides whether a run may go ahead with a setting that weakens one of
// burrow's defaults (decision 0013).
//
// A weakening is accepted only by the person running burrow: a flag that names it (--accept
// root, or --user 0:0, which is typed and therefore explicit), or an answer at the controlling
// terminal. Without a terminal, or in CI mode, nobody can answer, so it is refused. Environment
// variables never accept anything: every child process inherits them silently and they are the
// cheapest thing for an agent to set.
package consent

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Names are the weakenings burrow knows. Each new one is added by the decision that allows it.
var Names = []string{Root}

// Root is the weakening of a sandbox, tool or shell that runs as uid 0 inside (decision 0011).
const Root = "root"

// Weakening is one setting of a run that weakens a default.
type Weakening struct {
	Name     string // one of Names
	From     string // where it was asked for: "--user", "RUN_AS in preset kali (/p/kali.preset)"
	Explicit bool   // typed by the person on the command line (a flag): accepted as is
}

// ParseAccept reads the value of --accept: a comma-separated list of Names.
func ParseAccept(s string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, n := range strings.Split(s, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if !slices.Contains(Names, n) {
			return nil, fmt.Errorf("--accept %q: unknown setting (known: %s)", n, strings.Join(Names, ", "))
		}
		out[n] = true
	}
	return out, nil
}

// Terminal is the controlling terminal, for the question. Open returns an error when there is
// none (an agent, a script, CI).
type Terminal interface {
	Open() (io.ReadWriteCloser, error)
}

// Policy is how this run answers weakenings.
type Policy struct {
	Accepted map[string]bool // from --accept
	CI       bool            // CI mode: never ask
	DryRun   bool            // --dry-run: report, neither ask nor refuse
	Terminal Terminal
	Report   io.Writer // where --dry-run says what a real run would need
}

// ErrRefused is returned when a weakening is neither accepted nor confirmed.
var ErrRefused = errors.New("not accepted")

// Check returns nil when every weakening is accepted: explicit, named in --accept, or confirmed
// at the terminal. Otherwise it returns an error that names the flag to add.
func (p Policy) Check(ws []Weakening) error {
	var open []Weakening
	for _, w := range ws {
		if !slices.Contains(Names, w.Name) {
			return fmt.Errorf("internal: unknown weakening %q", w.Name)
		}
		if !w.Explicit && !p.Accepted[w.Name] {
			open = append(open, w)
		}
	}
	if len(open) == 0 {
		return nil
	}
	if p.DryRun {
		if p.Report != nil {
			for _, w := range open {
				_, _ = fmt.Fprintf(p.Report, "# burrow: a real run asks to confirm %s (%s), or needs --accept %s\n",
					w.Name, w.From, w.Name)
			}
		}
		return nil
	}
	if p.CI {
		return refused(open, "CI mode never asks")
	}
	if p.Terminal == nil {
		return refused(open, "no terminal to ask on")
	}
	tty, err := p.Terminal.Open()
	if err != nil {
		return refused(open, "no terminal to ask on")
	}
	defer func() { _ = tty.Close() }() // the answer is already read; a close error changes nothing
	in := bufio.NewReader(tty)
	for _, w := range open {
		_, _ = fmt.Fprintf(tty, "burrow: %s asks for %s. %s\nRun anyway? [y/N] ", printable(w.From), w.Name, explain(w.Name))
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return refused([]Weakening{w}, "no answer")
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
		default:
			return refused([]Weakening{w}, "answered no")
		}
	}
	return nil
}

// printable replaces control characters, so a crafted file name cannot rewrite the question
// with terminal escape sequences.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}

func explain(name string) string {
	if name == Root {
		return "Root keeps no capabilities and no new privileges, but can change everything in the " +
			"container, and the workspace files it writes are yours on the host."
	}
	return ""
}

func refused(ws []Weakening, why string) error {
	names := map[string]bool{}
	var from []string
	for _, w := range ws {
		names[w.Name] = true
		from = append(from, w.Name+" ("+printable(w.From)+")")
	}
	list := make([]string, 0, len(names))
	for n := range names {
		list = append(list, n)
	}
	sort.Strings(list)
	return fmt.Errorf("%w: %s, %s; to run it, add --accept %s before the command",
		ErrRefused, strings.Join(from, ", "), why, strings.Join(list, ","))
}

// CIFromEnv says whether the CI variable switches CI mode on. CI mode only tightens burrow, so
// trusting a variable anyone can set is safe: setting it gains nothing.
func CIFromEnv(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("CI"))) {
	case "1", "true", "yes":
		return true
	}
	return false
}
