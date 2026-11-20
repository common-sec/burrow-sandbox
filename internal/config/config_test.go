package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pinned matches test/images/alpine/Dockerfile; Dependabot does not update it here: see issue #7.
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
	if p.WorkspaceMode != WorkspaceRO || p.RunAs != "65532:65532" || p.Caches[0].Mode != "O" || len(p.Args) != 3 {
		t.Fatalf("%+v", p)
	}
	if _, err := LoadPreset(write(t, "NAME=x1\nIMAGE="+pinned+"\nNETWORK=none\nALLOW=pypi.org\n")); err == nil {
		t.Error("ALLOW with NETWORK=none: want error")
	}
	if _, err := LoadPreset(write(t, "NAME=x1\nIMAGE="+pinned+"\nCACHE=../x:/c:rw\n")); err == nil {
		t.Error("bad cache volume: want error")
	}
}

func TestNewSandboxPrecedence(t *testing.T) {
	p, err := ParsePreset(strings.NewReader("NAME=base\nIMAGE="+pinned+"\nWORKSPACE_MODE=rw\nNETWORK=proxy\nALLOW=pypi.org\nMEMORY=1g\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := map[string]string{"NAME": "dev", "PRESET": "base", "ALLOW": "proxy.golang.org", "WORKSPACE": "src"}
	e, err := NewSandbox(p, file, dir, Overrides{Image: "localhost/mine:dev"})
	if err != nil {
		t.Fatal(err)
	}
	// preset < file < flags
	if e.Name != "dev" || e.Image != "localhost/mine:dev" || strings.Join(e.Allow, ",") != "proxy.golang.org" ||
		e.Memory != "1g" || e.Workspace != filepath.Join(dir, "src") || e.WorkspaceMode != WorkspaceRW {
		t.Fatalf("%+v", e)
	}
	tool, _ := ParsePreset(strings.NewReader("NAME=t1\nIMAGE="+pinned+"\nARGS=x\n"), "test")
	if _, err := NewSandbox(tool, nil, dir, Overrides{}); err == nil {
		t.Error("tool preset as sandbox: want error")
	}
	if _, err := NewSandbox(nil, map[string]string{"NAME": "x1", "IMAGE": pinned, "NETWORK": "none"}, dir, Overrides{}); err == nil {
		t.Error("sandbox with NETWORK=none: want error")
	}
}

func TestPresetInheritAndDefaults(t *testing.T) {
	p, err := ParsePreset(strings.NewReader("NAME=t1\nIMAGE="+pinned+"\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if p.WorkspaceMode != WorkspaceRO || p.Network != NetworkNone {
		t.Fatalf("defaults must be the narrowest: %+v", p)
	}
	p, err = ParsePreset(strings.NewReader("NAME=t1\nIMAGE="+pinned+"\nWORKSPACE_MODE=inherit\nNETWORK=inherit\n"), "test")
	if err != nil || p.WorkspaceFallback != WorkspaceRO || p.NetworkFallback != NetworkNone {
		t.Fatalf("bare inherit falls back to ro and none: %+v %v", p, err)
	}
	p, err = ParsePreset(strings.NewReader("NAME=t1\nIMAGE="+pinned+"\nWORKSPACE_MODE=inherit:rw\nNETWORK=inherit:proxy\nALLOW=ghcr.io\n"), "test")
	if err != nil || p.WorkspaceFallback != WorkspaceRW || p.NetworkFallback != NetworkProxy {
		t.Fatalf("inherit with fallbacks: %+v %v", p, err)
	}
	bad := map[string]string{
		"ALLOW with inherit:none":  "NETWORK=inherit\nALLOW=x.org\n",
		"ALLOW with none":          "NETWORK=none\nALLOW=x.org\n",
		"old WORKSPACE key":        "WORKSPACE=ro\n",
		"fallback without inherit": "WORKSPACE_MODE=ro:rw\n",
		"inherit as fallback":      "NETWORK=inherit:inherit\n",
		"bad fallback":             "WORKSPACE_MODE=inherit:all\n",
		"dangerous capability":     "CAP_ADD=SYS_ADMIN\n",
		"capability with prefix":   "CAP_ADD=CAP_NET_RAW\n",
		"proxy variable in ENV":    "ENV=HTTPS_PROXY=http://evil:3128\n",
		"ENV without value":        "ENV=FOO\n",
		"cache twice":              "CACHE=a:/c:rw,a:/d:rw\n",
		"cache path twice":         "CACHE=a:/c:rw,b:/c:rw\n",
	}
	for name, body := range bad {
		if _, err := ParsePreset(strings.NewReader("NAME=t1\nIMAGE="+pinned+"\n"+body), "test"); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	p, err = ParsePreset(strings.NewReader("NAME=t1\nIMAGE="+pinned+"\nCAP_ADD=NET_RAW\nENV=A=1 2,B=x=y\nCACHE=a:/c:rw,b:/d:O\n"), "test")
	if err != nil || len(p.Caches) != 2 || strings.Join(p.Env, "|") != "A=1 2|B=x=y" || p.CapAdd[0] != "NET_RAW" {
		t.Fatalf("caps, env, caches: %+v %v", p, err)
	}
}

func TestSandboxStrictAndEnv(t *testing.T) {
	dir := t.TempDir()
	e, err := NewSandbox(nil, map[string]string{"NAME": "dev", "IMAGE": pinned, "STRICT": "true", "ENV": "A=1"}, dir, Overrides{})
	if err != nil || !e.Strict || e.Env[0] != "A=1" {
		t.Fatalf("%+v %v", e, err)
	}
	back, err := LoadEnv(write(t, e.String()))
	if err != nil || !back.Strict || back.String() != e.String() {
		t.Fatalf("round trip: %+v %v", back, err)
	}
	for _, f := range []map[string]string{
		{"NAME": "dev", "IMAGE": pinned, "STRICT": "yes"},
		{"NAME": "exec", "IMAGE": pinned},
	} {
		if _, err := NewSandbox(nil, f, dir, Overrides{}); err == nil {
			t.Errorf("%v: want error", f)
		}
	}
	tool, _ := ParsePreset(strings.NewReader("NAME=t1\nIMAGE="+pinned+"\nCAP_ADD=NET_RAW\n"), "test")
	if _, err := NewSandbox(tool, nil, dir, Overrides{}); err == nil {
		t.Error("a sandbox never gets capabilities: want error")
	}
}

func TestFindAndListPresets(t *testing.T) {
	user := t.TempDir()
	mine := filepath.Join(user, "base.preset")
	if err := os.WriteFile(mine, []byte("NAME=base\nDESCRIPTION=Go build tools\nIMAGE=localhost/x:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "broken.preset"), []byte("NAME=broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, err := FindPreset("base", user); err != nil || p.Source != mine {
		t.Fatalf("user preset: %v %v", p, err)
	}
	if _, err := FindPreset("nope", user); err == nil {
		t.Error("missing preset: want error")
	}
	if _, err := FindPreset("../etc", user); err == nil {
		t.Error("path traversal as a name: want a file lookup error")
	}
	if _, err := FindPreset("Bad_Name", user); err == nil {
		t.Error("bad name: want error")
	}
	all, err := ListPresets(user)
	if len(all) != 1 || all[0].Name != "base" || err == nil {
		t.Fatalf("list keeps valid presets and reports broken ones: %v %v", all, err)
	}
	if got := SearchPresets(all, "GO BUILD"); len(got) != 1 {
		t.Fatalf("search by description: %v", got)
	}
	if got := SearchPresets(all, "trivy"); len(got) != 0 {
		t.Fatalf("search: %v", got)
	}
}
