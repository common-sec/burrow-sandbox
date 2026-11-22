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

// pinned matches test/images/alpine/Dockerfile; Dependabot does not update it here: see issue #7.
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

// with returns the first call that contains seq anywhere.
func with(calls [][]string, seq ...string) []string {
	for _, c := range calls {
		if has(c, seq...) {
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
	if c := with(rec.Calls, "burrow-dev-egress"); c == nil || !has(c, "network", "create") || has(c, "--internal") {
		t.Fatal("no dedicated egress network for the proxy")
	}
	if !has(px, "--network", "burrow-dev-egress") || !has(px, "--cap-drop", "ALL") || !has(px, "--read-only") ||
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
		{"--http-proxy=false"}, // the host's proxy settings must not leak in
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

func TestShellKrunFallsBackToOneShotVM(t *testing.T) {
	m, rec := setup(t, config.EngineKrun)
	if err := m.Shell(context.Background(), "dev", []string{"id"}); err != nil {
		t.Fatal(err)
	}
	last := rec.Calls[len(rec.Calls)-1]
	if !has(last, "run", "--rm") || !has(last, "--runtime", "krun") || last[len(last)-1] != "id" {
		t.Fatalf("krun exec: %v", last)
	}
}

func TestExecPresetNoNetwork(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	p := &config.Preset{
		Name: "scan", Image: pinned, Args: []string{"fs", "/work"}, WorkspaceMode: config.WorkspaceRO,
		Network: config.NetworkNone, Engine: config.EngineCrun, RunAs: "65532:65532",
		Caches: []config.Cache{{Volume: "trivy", Path: "/cache", Mode: "O"}, {Volume: "java", Path: "/java", Mode: "rw"}},
		Env:    []string{"TRIVY_QUIET=true"},
	}
	n := len(rec.Calls)
	if err := m.Exec(context.Background(), "dev", "", p, []string{"--quiet"}, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	run := rec.Calls[n]
	if !has(run, "--network", "none") || !has(run, "--read-only") || !has(run, "-v", "burrow-cache-dev_trivy:/cache:O") ||
		!has(run, "-v", "burrow-cache-dev_java:/java:rw,U") || !has(run, "-e", "TRIVY_QUIET=true") ||
		run[len(run)-1] != "--quiet" || !strings.Contains(strings.Join(run, " "), ":/work:ro -w /work") || !strings.HasSuffix(strings.Join(run, " "), pinned+" fs /work --quiet") {
		t.Fatalf("preset: %v", run)
	}
}

func TestExecPresetOwnProxyAndCleanup(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	p := &config.Preset{
		Name: "db", Image: pinned, WorkspaceMode: config.WorkspaceNone, Network: config.NetworkProxy,
		Allow: []string{"mirror.gcr.io"}, Engine: config.EngineCrun, RunAs: "65532:65532",
	}
	n := len(rec.Calls)
	if err := m.Exec(context.Background(), "dev", "", p, nil, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	calls := rec.Calls[n:]
	px := with(calls, "-allow")
	if !has(px, "-allow", "mirror.gcr.io") {
		t.Fatalf("preset proxy allowlist: %v", px)
	}
	if !strings.HasSuffix(px[slices.Index(px, "--network")+1], "-egress") {
		t.Fatalf("tool proxy not on its own egress network: %v", px)
	}
	var removed []string
	for _, c := range calls {
		if has(c, "network", "rm", "-f") {
			removed = append(removed, c[len(c)-1])
		}
	}
	if len(removed) != 2 || !strings.HasSuffix(removed[1], "-egress") {
		t.Fatalf("tool networks not removed: %v", removed)
	}
}

func TestExecGetsWhatThePresetAsksStrictCaps(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	e, _ := m.loadEnv("dev")
	e.WorkspaceMode = config.WorkspaceRO
	if err := m.saveEnv(e); err != nil {
		t.Fatal(err)
	}
	p := &config.Preset{
		Name: "fmt", Image: pinned, WorkspaceMode: config.WorkspaceRW, Network: config.NetworkNone,
		Engine: config.EngineCrun, RunAs: "65532:65532", CapAdd: []string{"NET_RAW"},
	}
	// A sibling tool gets what its preset asks for, even more than the sandbox has...
	n := len(rec.Calls)
	if err := m.Exec(context.Background(), "dev", "", p, nil, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if run := rec.Calls[n]; !has(run, "-v", e.Workspace+":/work:rw") || !has(run, "--cap-add", "NET_RAW") {
		t.Fatalf("tool: %v", run)
	}
	// ...unless strict mode caps it by the sandbox: workspace, capabilities, allowlist.
	if err := m.Exec(context.Background(), "dev", "", p, nil, ExecOptions{Strict: true}); err == nil {
		t.Fatal("strict: rw tool with capabilities on a ro sandbox: want error")
	}
	p.WorkspaceMode, p.CapAdd = config.WorkspaceRO, nil
	p.Network, p.Allow = config.NetworkProxy, []string{"evil.example"}
	if err := m.Exec(context.Background(), "dev", "", p, nil, ExecOptions{Strict: true}); err == nil {
		t.Fatal("strict: host outside the sandbox's allowlist: want error")
	}
	p.Allow = []string{"pypi.org"}
	if err := m.Exec(context.Background(), "dev", "", p, nil, ExecOptions{Strict: true}); err != nil {
		t.Fatalf("strict: within the sandbox: %v", err)
	}
	e.Strict = true // STRICT=true in the sandbox file turns it on without the flag
	if err := m.saveEnv(e); err != nil {
		t.Fatal(err)
	}
	p.Allow = []string{"*.pypi.org"}
	if err := m.Exec(context.Background(), "dev", "", p, nil, ExecOptions{}); err == nil {
		t.Fatal("STRICT=true: wildcard wider than the sandbox: want error")
	}
}

func TestPortAndDown(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	if err := m.Port(context.Background(), "dev", 8080, 18080); err != nil {
		t.Fatal(err)
	}
	fwd := find(rec, "run", "-d", "--name", "burrow-dev-fwd-8080")
	if !has(fwd, "-p", "127.0.0.1:18080:8080") || !has(fwd, "-target", "10.89.0.2:8080") ||
		!has(fwd, "--network", "burrow-dev-egress") {
		t.Fatalf("forwarder: %v", fwd)
	}
	rec.Replies["volume ls"] = "burrow-dev-home\nburrow-cache-dev_trivy\nburrow-cache-dev-x_trivy\nburrow-cache-exec_trivy"
	if err := m.Port(context.Background(), "dev", 0, 1); err == nil {
		t.Fatal("port 0: want error")
	}
	if err := m.Down(context.Background(), "dev", true); err != nil {
		t.Fatal(err)
	}
	rm := with(rec.Calls, "volume", "rm")
	if !has(rm, "burrow-dev-home", "burrow-cache-dev_trivy") || slices.Contains(rm, "burrow-cache-dev-x_trivy") ||
		slices.Contains(rm, "burrow-cache-exec_trivy") {
		t.Fatalf("purge must remove this sandbox's volumes only: %v", rm)
	}
	if _, err := os.Stat(filepath.Join(m.StateDir, "dev.env")); !os.IsNotExist(err) {
		t.Fatal("purge kept the definition")
	}
}

func TestExecInheritsFromSandbox(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	e, _ := m.loadEnv("dev")
	p := &config.Preset{
		Name: "fmt", Image: pinned, WorkspaceMode: config.WorkspaceInherit, Network: config.NetworkInherit,
		WorkspaceFallback: config.WorkspaceRO, NetworkFallback: config.NetworkNone,
		Engine: config.EngineCrun, RunAs: "65532:65532",
	}
	n := len(rec.Calls)
	if err := m.Exec(context.Background(), "dev", "/ignored", p, nil, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	calls := rec.Calls[n:]
	// Its own proxy with the sandbox's allowlist, on its own network: never the sandbox's.
	if px := with(calls, "-allow"); !has(px, "-allow", "pypi.org") {
		t.Fatalf("inherited allowlist: %v", px)
	}
	var run []string
	for _, c := range calls {
		if has(c, "run", "--rm") {
			run = c
		}
	}
	if has(run, "--network", "burrow-dev") || !has(run, "-v", e.Workspace+":/work:rw") {
		t.Fatalf("inherit: %v", run)
	}
}

func TestExecWithoutSandbox(t *testing.T) {
	rec := &podman.Recorder{Replies: map[string]string{"inspect": "10.89.0.2"}}
	m := &Manager{Podman: rec, StateDir: t.TempDir()}
	ws := t.TempDir()
	p := &config.Preset{
		Name: "fmt", Image: pinned, WorkspaceMode: config.WorkspaceInherit, Network: config.NetworkInherit,
		WorkspaceFallback: config.WorkspaceRO, NetworkFallback: config.NetworkNone,
		Engine: config.EngineCrun, RunAs: "65532:65532", Caches: []config.Cache{{Volume: "c1", Path: "/c", Mode: "rw"}},
	}
	if err := m.Exec(context.Background(), "", ws, p, nil, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	run := rec.Calls[0]
	// inherit without a sandbox takes the preset's fallback (here the defaults: ro, no network),
	// and caches are not shared with any sandbox.
	if !has(run, "--network", "none") || !has(run, "-v", ws+":/work:ro") || slices.Contains(run, "burrow.env") ||
		!has(run, "-v", "burrow-cache-exec_c1:/c:rw,U") {
		t.Fatalf("standalone: %v", run)
	}
	// A fallback that asks for more: rw and its own proxy with the preset's allowlist.
	p.WorkspaceFallback, p.NetworkFallback, p.Allow = config.WorkspaceRW, config.NetworkProxy, []string{"ghcr.io"}
	n := len(rec.Calls)
	if err := m.Exec(context.Background(), "", ws, p, nil, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	calls := rec.Calls[n:]
	if !has(with(calls, "-allow"), "-allow", "ghcr.io") || with(calls, "-v", ws+":/work:rw") == nil {
		t.Fatalf("fallback: %v", calls)
	}
	if err := m.Exec(context.Background(), "", ws+"/a,b", p, nil, ExecOptions{}); err == nil {
		t.Fatal("comma in workspace: want error")
	}
}

func TestDefinitionReadOnlyInsideWorkspace(t *testing.T) {
	rec := &podman.Recorder{Replies: map[string]string{"inspect": "10.89.0.2"}}
	m := &Manager{Podman: rec, StateDir: t.TempDir()}
	ws := t.TempDir()
	def := filepath.Join(ws, "burrow.sandbox")
	e := &config.Env{
		Name: "dev", Image: pinned, Workspace: ws, WorkspaceMode: config.WorkspaceRW, Engine: config.EngineCrun,
		RunAs: "1000:1000", ProxyImage: config.DefaultProxyImage, ForwardImage: config.DefaultForwardImage,
		Definition: def,
	}
	if err := m.Up(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	sb := find(rec, "run", "-d", "--name", "burrow-dev")
	if !has(sb, "-v", def+":/work/burrow.sandbox:ro") {
		t.Fatalf("definition not read-only: %v", sb)
	}
}

func TestUpRefusesRunningSandbox(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	rec.Replies["ps -a -q"] = "abc123"
	e, _ := m.loadEnv("dev")
	if err := m.Up(context.Background(), e); err == nil {
		t.Fatal("second up of a running sandbox: want error")
	}
}

// U20: an image the user builds (localhost/) is never pulled: a missing one fails instead of
// asking whatever answers on localhost:443 for it. Registry images are pulled only if missing.
func TestImagePullPolicy(t *testing.T) {
	for _, c := range []struct{ ref, want string }{
		{"localhost/burrow-test-alpine:dev", "--pull=never"},
		{"docker.io/library/alpine:3@sha256:" + strings.Repeat("a", 64), "--pull=missing"},
		{"localhost.evil.example/burrow-test-alpine:dev", "--pull=missing"}, // a registry, not local
		{"localhost:5000/burrow-test-alpine:dev", "--pull=missing"},         // a registry on a port
	} {
		if got := image(c.ref); len(got) != 2 || got[0] != c.want || got[1] != c.ref {
			t.Errorf("image(%q) = %v, want [%s %s]", c.ref, got, c.want, c.ref)
		}
	}
}

// Every container burrow starts names its pull policy right before its image.
func TestEveryRunSetsPullPolicy(t *testing.T) {
	m, rec := setup(t, config.EngineCrun)
	p := &config.Preset{
		Name: "scan", Image: "localhost/burrow-test-trivy:dev", WorkspaceMode: config.WorkspaceRO,
		Network: config.NetworkNone, Engine: config.EngineCrun, RunAs: "65532:65532",
	}
	if err := m.Exec(context.Background(), "dev", "", p, nil, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := m.Port(context.Background(), "dev", 8080, 8080); err != nil {
		t.Fatal(err)
	}
	runs := 0
	for _, c := range rec.Calls {
		if len(c) == 0 || c[0] != "run" {
			continue
		}
		runs++
		ok := false
		for i := 0; i+1 < len(c); i++ {
			if (c[i] == "--pull=never" || c[i] == "--pull=missing") && image(c[i+1])[0] == c[i] {
				ok = true
			}
		}
		if !ok {
			t.Errorf("run without a pull policy before its image: %v", c)
		}
	}
	if runs < 4 { // proxy, sandbox, tool, forwarder
		t.Fatalf("expected the proxy, sandbox, tool and forwarder runs, got %d", runs)
	}
}
