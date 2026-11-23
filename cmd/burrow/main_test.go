package main

import (
	"errors"
	"flag"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/common-sec/burrow-sandbox/internal/consent"
)

func TestParseArgsFlagsAnywhereBeforeDash(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	name := fs.String("name", "", "")
	pos, after, err := parseArgs(fs, []string{"alpine", "--name", "x1", "--", "--name", "y"})
	if err != nil || *name != "x1" || !slices.Equal(pos, []string{"alpine"}) || !slices.Equal(after, []string{"--name", "y"}) {
		t.Fatalf("pos=%v after=%v name=%q err=%v", pos, after, *name, err)
	}
	if _, _, err := parseArgs(flag.NewFlagSet("t", flag.ContinueOnError), []string{"--nope"}); err == nil {
		t.Fatal("unknown flag: want error")
	}
}

func TestDryRunCommands(t *testing.T) {
	t.Setenv("BURROW_STATE_DIR", t.TempDir())
	presets, err := filepath.Abs("../../test/presets")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BURROW_PRESETS_DIR", presets)
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"--dry-run", "run", "alpine", "--name", "t1"},
		{"--dry-run", "exec", "--workspace", ".", "trivy-scan"},
		{"--dry-run", "exec", "--strict", "--workspace", ".", "trivy"},
		{"--dry-run", "presets", "info", "trivy"},
		{"--dry-run", "presets", "search", "trivy"},
		{"--dry-run", "presets"},
	} {
		if err := run(args); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	if err := run([]string{"--dry-run", "up"}); err == nil {
		t.Error("up without a sandbox file: want error")
	}
	if err := run([]string{"--dry-run", "presets", "info", "missing"}); err == nil {
		t.Error("info on a missing preset: want error")
	}
}

// U20: a missing local image ends with the command that builds it.
func TestHintForMissingLocalImage(t *testing.T) {
	for _, c := range []struct {
		msg  string
		want bool
	}{
		{"podman run: exit status 125: Error: localhost/burrow-test-alpine:dev: image not known", true},
		{"podman run: exit status 125: Error: docker.io/library/alpine: image not known", false},
		{"podman run: exit status 1: permission denied", false},
	} {
		if got := hint(errors.New(c.msg)) != ""; got != c.want {
			t.Errorf("hint(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

// U22: every run as root warns, and the warning names where root was asked for.
func TestRootWarning(t *testing.T) {
	for _, c := range []struct {
		user, from string
		warn       bool
	}{
		{"0:0", "--user", true},
		{"0:1000", "RUN_AS in preset build (/p/build.preset)", true},
		{"00:0", "RUN_AS in ./burrow.sandbox", true}, // leading zero is still root
		{"1000:1000", "--user", false},
		{"1000:0", "--user", false}, // gid 0 is not root
		{"", "", false},
		{"root", "--user", false}, // invalid, refused elsewhere
	} {
		w := rootWarning(c.user, c.from)
		if (w != "") != c.warn {
			t.Fatalf("rootWarning(%q, %q) = %q, want warning=%v", c.user, c.from, w, c.warn)
		}
		if c.warn && !strings.Contains(w, c.from) {
			t.Fatalf("warning does not name its source %q: %q", c.from, w)
		}
	}
}

type noTTY struct{}

func (noTTY) Open() (io.ReadWriteCloser, error) { return nil, errors.New("no terminal") }

// U23: root asked for by a file needs a person or a named flag; --user is the person's own.
func TestRootNeedsAcceptance(t *testing.T) {
	preset := "RUN_AS in preset kali (/p/kali.preset)"
	for _, c := range []struct {
		name       string
		u          ui
		user, from string
		ok         bool
	}{
		{"not root", ui{}, "1000:1000", preset, true},
		{"--user 0:0, CI mode", ui{policy: consent.Policy{CI: true}}, "0:0", "--user", true},
		{"preset root, no terminal", ui{policy: consent.Policy{Terminal: noTTY{}}}, "0:0", preset, false},
		{"preset root, CI mode", ui{policy: consent.Policy{CI: true}}, "0:0", preset, false},
		{"preset root, --accept root", ui{policy: consent.Policy{CI: true, Accepted: map[string]bool{"root": true}}}, "0:0", preset, true},
		{"sandbox file root, no terminal", ui{policy: consent.Policy{Terminal: noTTY{}}}, "0:1000", "RUN_AS in ./burrow.sandbox", false},
	} {
		err := c.u.allow(c.user, c.from)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// U24: CI mode only tightens: the CI variable switches it on, CI=false does not switch --ci off,
// CI mode never asks, and --accept takes known names only (no wildcard).
func TestCIModeOnlyTightens(t *testing.T) {
	preset := "RUN_AS in preset kali (/p/kali.preset)"
	for _, c := range []struct {
		name   string
		ci     bool
		accept string
		env    map[string]string
		user   string
		from   string
		ok     bool
	}{
		{"CI=true refuses root from a preset", false, "", map[string]string{"CI": "true"}, "0:0", preset, false},
		{"CI=false does not switch --ci off", true, "", map[string]string{"CI": "false"}, "0:0", preset, false},
		{"--accept root in CI", false, "root", map[string]string{"CI": "true"}, "0:0", preset, true},
		{"--user 0:0 in CI", false, "", map[string]string{"CI": "1"}, "0:0", "--user", true},
		{"no root in CI", true, "", nil, "1000:1000", preset, true},
	} {
		u, err := newUI(c.ci, c.accept, false, func(k string) string { return c.env[k] })
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		u.policy.Terminal = yesTTY{} // a terminal that would say yes: CI mode must not ask it
		if !u.ci {
			t.Fatalf("%s: CI mode is off", c.name)
		}
		if err := u.allow(c.user, c.from); (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
	if _, err := newUI(false, "all", false, func(string) string { return "" }); err == nil {
		t.Error("--accept all: want an error")
	}
	if u, _ := newUI(false, "", false, func(string) string { return "" }); u.ci {
		t.Error("no --ci, no CI: CI mode must be off")
	}
}

type yesTTY struct{}

func (yesTTY) Open() (io.ReadWriteCloser, error) {
	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{strings.NewReader("y\n"), io.Discard, io.NopCloser(nil)}, nil
}

// A preset path with a newline cannot start a GitHub Actions workflow command of its own.
func TestAnnotationEscapes(t *testing.T) {
	if got := annotation("RUN_AS in preset x (/p/a\n::add-mask::x%.preset)"); strings.ContainsAny(got, "\r\n") || !strings.Contains(got, "%0A") || !strings.Contains(got, "%25") {
		t.Fatalf("annotation = %q", got)
	}
}
