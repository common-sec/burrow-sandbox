package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/common-sec/burrow-sandbox/internal/config"
	"github.com/common-sec/burrow-sandbox/internal/podman"
)

const pinned = "docker.io/library/alpine:3.22@sha256:" + "49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f"

func setup(t *testing.T, engine config.Engine) (*Manager, *podman.Recorder) {
	t.Helper()
	rec := &podman.Recorder{Replies: map[string]string{"inspect": "10.89.0.2"}}
	m := &Manager{Podman: rec, StateDir: t.TempDir()}
	e := &config.Env{
		Name: "dev", Image: pinned, Workspace: t.TempDir(), WorkspaceMode: config.WorkspaceRW,
		Allow: []string{"pypi.org"}, Engine: engine, RunAs: "1000:1000",
		ProxyImage: config.DefaultProxyImage, ForwardImage: config.DefaultForwardImage,
	}
	if err := m.Up(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	return m, rec
}

func find(rec *podman.Recorder, prefix ...string) []string {
	for _, c := range rec.Calls {
		if len(c) >= len(prefix) && slices.Equal(c[:len(prefix)], prefix) {
			return c
		}
	}
	return nil
}

func has(args []string, seq ...string) bool {
	for i := range args {
		if i+len(seq) <= len(args) && slices.Equal(args[i:i+len(seq)], seq) {
			return true
		}
	}
	return false
}

func TestUpLayout(t *testing.T) {
	_, rec := setup(t, config.EngineCrun)
	net := find(rec, "network", "create")
	if !has(net, "--internal") || !has(net, "--disable-dns") || net[len(net)-1] != "burrow-dev" {
		t.Fatalf("internal network: %v", net)
	}
	px := find(rec, "run", "-d", "--name", "burrow-dev-proxy")
	if !has(px, "--network", EgressNetwork) || !has(px, "--cap-drop", "ALL") || !has(px, "--read-only") ||
		!has(px, "-allow", "pypi.org") {
		t.Fatalf("proxy: %v", px)
	}
	if find(rec, "network", "connect", "burrow-dev", "burrow-dev-proxy") == nil {
		t.Fatal("proxy not connected to the internal network")
	}
	sb := find(rec, "run", "-d", "--name", "burrow-dev")
	for _, want := range [][]string{
		{"--network", "burrow-dev"},
		{"--cap-drop", "ALL"},
		{"--security-opt", "no-new-privileges"},
		{"--userns", "keep-id:uid=1000,gid=1000"},
		{"-e", "HTTPS_PROXY=http://10.89.0.2:3128"},
	} {
		if !has(sb, want...) {
			t.Errorf("sandbox lacks %v: %v", want, sb)
		}
	}
	for _, bad := range []string{"--privileged", "podman", "host"} {
		if has(sb, "--network", bad) || slices.Contains(sb, bad) && bad == "--privileged" {
			t.Errorf("sandbox has %s", bad)
		}
	}
	if slices.Contains(sb, "/var/run/docker.sock") || strings.Contains(strings.Join(sb, " "), "podman.sock") {
		t.Error("engine socket mounted")
	}
}

func TestExecKrunFallsBackToOneShotVM(t *testing.T) {
	m, rec := setup(t, config.EngineKrun)
	if err := m.Exec(context.Background(), "dev", []string{"id"}); err != nil {
		t.Fatal(err)
	}
	last := rec.Calls[len(rec.Calls)-1]
	if !has(last, "run", "--rm") || !has(last, "--runtime", "krun") || last[len(last)-1] != "id" {
		t.Fatalf("krun exec: %v", last)
	}
}

func TestRunPresetNoNetwork(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	p := &config.Preset{
		Name: "scan", Image: pinned, Args: []string{"fs", "/work"}, WorkspaceMode: config.WorkspaceRO,
		Network: config.NetworkNone, Engine: config.EngineCrun, RunAs: "65532:65532",
		Cache: &config.Cache{Volume: "trivy", Path: "/cache", Mode: "O"},
	}
	n := len(rec.Calls)
	if err := m.Run(context.Background(), "dev", p, []string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	run := rec.Calls[n]
	if !has(run, "--network", "none") || !has(run, "--read-only") || !has(run, "-v", "burrow-cache-trivy:/cache:O") ||
		run[len(run)-1] != "--quiet" || !strings.Contains(strings.Join(run, " "), ":/work:ro -w /work") || !strings.HasSuffix(strings.Join(run, " "), pinned+" fs /work --quiet") {
		t.Fatalf("preset: %v", run)
	}
}

func TestRunPresetOwnProxyAndCleanup(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	p := &config.Preset{
		Name: "db", Image: pinned, WorkspaceMode: config.WorkspaceNone, Network: config.NetworkProxy,
		Allow: []string{"mirror.gcr.io"}, Engine: config.EngineCrun, RunAs: "65532:65532",
	}
	n := len(rec.Calls)
	if err := m.Run(context.Background(), "dev", p, nil); err != nil {
		t.Fatal(err)
	}
	calls := rec.Calls[n:]
	if !has(calls[1], "-allow", "mirror.gcr.io") {
		t.Fatalf("preset proxy allowlist: %v", calls[1])
	}
	last := calls[len(calls)-1]
	if !has(last, "network", "rm", "-f") {
		t.Fatalf("preset network not removed: %v", last)
	}
}

func TestRunPresetCannotWidenWorkspace(t *testing.T) {
	m, _ := setup(t, config.EngineCrun)
	e, _ := m.loadEnv("dev")
	e.WorkspaceMode = config.WorkspaceRO
	if err := m.saveEnv(e); err != nil {
		t.Fatal(err)
	}
	p := &config.Preset{
		Name: "fmt", Image: pinned, WorkspaceMode: config.WorkspaceRW, Network: config.NetworkNone,
		Engine: config.EngineCrun, RunAs: "65532:65532",
	}
	if err := m.Run(context.Background(), "dev", p, nil); err == nil {
		t.Fatal("rw preset on a ro environment: want error")
	}
}

func TestPortAndDown(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	if err := m.Port(context.Background(), "dev", 8080, 18080); err != nil {
		t.Fatal(err)
	}
	fwd := find(rec, "run", "-d", "--name", "burrow-dev-fwd-8080")
	if !has(fwd, "-p", "127.0.0.1:18080:8080") || !has(fwd, "-target", "10.89.0.2:8080") {
		t.Fatalf("forwarder: %v", fwd)
	}
	if err := m.Port(context.Background(), "dev", 0, 1); err == nil {
		t.Fatal("port 0: want error")
	}
	if err := m.Down(context.Background(), "dev", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(m.StateDir, "dev.env")); !os.IsNotExist(err) {
		t.Fatal("purge kept the definition")
	}
}
