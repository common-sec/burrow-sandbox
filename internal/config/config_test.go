package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pinned = "docker.io/library/alpine:3.22@sha256:" + "49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f"

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "def")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadEnvDefaultsAndRoundTrip(t *testing.T) {
	ws := t.TempDir()
	e, err := LoadEnv(write(t, "# comment\nNAME=dev\nIMAGE="+pinned+"\nWORKSPACE=\""+ws+"\"\nALLOW=pypi.org, *.github.com\n"))
	if err != nil {
		t.Fatal(err)
	}
	if e.Engine != EngineCrun || e.WorkspaceMode != WorkspaceRW || e.RunAs != "1000:1000" || e.ProxyImage != DefaultProxyImage {
		t.Fatalf("defaults: %+v", e)
	}
	if strings.Join(e.Allow, "|") != "pypi.org|*.github.com" {
		t.Fatalf("allow: %v", e.Allow)
	}
	back, err := LoadEnv(write(t, e.String()))
	if err != nil {
		t.Fatal(err)
	}
	if back.String() != e.String() {
		t.Fatalf("round trip:\n%s\n%s", e, back)
	}
}

func TestLoadEnvRejects(t *testing.T) {
	base := "NAME=dev\nIMAGE=" + pinned + "\nWORKSPACE_MODE=none\n"
	cases := map[string]string{
		"unknown key":     base + "NETWORK=host\n",
		"duplicate":       base + "NAME=x\n",
		"unpinned image":  "NAME=dev\nIMAGE=docker.io/library/alpine:3.22\nWORKSPACE_MODE=none\n",
		"root":            base + "RUN_AS=0:0\n",
		"ip allow":        base + "ALLOW=1.1.1.1\n",
		"wildcard all":    base + "ALLOW=*\n",
		"port in allow":   base + "ALLOW=pypi.org:443\n",
		"bad engine":      base + "ENGINE=runc\n",
		"bad name":        "NAME=Dev_1\nIMAGE=" + pinned + "\nWORKSPACE_MODE=none\n",
		"no workspace":    "NAME=dev\nIMAGE=" + pinned + "\n",
		"comma workspace": "NAME=dev\nIMAGE=" + pinned + "\nWORKSPACE=/a,b\n",
		"colon workspace": "NAME=dev\nIMAGE=" + pinned + "\nWORKSPACE=/a:/etc:rw\n",
		"half quote":      base + "MEMORY=\"1g\n",
		"not key=value":   base + "oops\n",
		"bad memory":      base + "MEMORY=lots\n",
	}
	for name, body := range cases {
		if _, err := LoadEnv(write(t, body)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestLoadPreset(t *testing.T) {
	p, err := LoadPreset(write(t, "NAME=trivy-scan\nIMAGE="+pinned+"\nNETWORK=none\nCACHE=trivy:/cache:O\nARGS=fs --offline-scan /work\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.WorkspaceMode != WorkspaceRO || p.RunAs != "65532:65532" || p.Cache.Mode != "O" || len(p.Args) != 3 {
		t.Fatalf("%+v", p)
	}
	if _, err := LoadPreset(write(t, "NAME=x1\nIMAGE="+pinned+"\nNETWORK=none\nALLOW=pypi.org\n")); err == nil {
		t.Error("ALLOW with NETWORK=none: want error")
	}
	if _, err := LoadPreset(write(t, "NAME=x1\nIMAGE="+pinned+"\nCACHE=../x:/c:rw\n")); err == nil {
		t.Error("bad cache volume: want error")
	}
}
