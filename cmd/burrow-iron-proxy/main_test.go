package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRefusesWhatIronDoesNotEnforce(t *testing.T) {
	for _, args := range [][]string{
		{"-print", "-ports", "22"},
		{"-print", "-ports", "443,8443"},
		{"-print", "-allow", "1.1.1.1"},
		{"-print", "-allow", "pypi.org:22"},
		{"-print", "pypi.org"}, // a stray argument is not a host
	} {
		if err := run(args); err == nil {
			t.Errorf("%v: want error", args)
		}
	}
}

// The configuration is written for the proxy's user only, then iron-proxy is started from it.
func TestRunWritesPrivateConfig(t *testing.T) {
	dir := t.TempDir()
	err := run([]string{"-dir", dir, "-iron", filepath.Join(dir, "missing"), "-allow", "pypi.org"})
	if err == nil || !strings.Contains(err.Error(), "exec") {
		t.Fatalf("want the exec of the missing binary to fail, got %v", err)
	}
	st, err := os.Stat(filepath.Join(dir, "iron-proxy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("config mode %v, want 0600", st.Mode().Perm())
	}
}
