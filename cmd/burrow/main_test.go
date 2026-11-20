package main

import (
	"flag"
	"path/filepath"
	"slices"
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
