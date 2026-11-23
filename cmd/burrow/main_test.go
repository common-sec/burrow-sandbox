package main

import (
	"errors"
	"flag"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
