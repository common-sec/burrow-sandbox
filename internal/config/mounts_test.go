package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// U23: every MOUNT entry kind with a good value, a bad value and the value an attacker would try.
func TestParseMounts(t *testing.T) {
	for _, c := range []struct {
		in        string
		allowHost bool
		ok        bool
		want      string // String() of the first entry
	}{
		{"tmpfs:/tmp", false, true, "tmpfs:/tmp"},
		{"tmpfs:/tmp:2g", false, true, "tmpfs:/tmp:2g"},
		{"tmpfs:/tmp:lots", false, false, ""},
		{"tmpfs:/tmp:1g:exec", false, false, ""}, // no options of our own
		{"tmpfs:/tmp,exec", false, false, ""},    // a comma splits entries: "exec" is no entry
		{"volume:dist:/dist:rw", false, true, "volume:dist:/dist:rw"},
		{"volume:dist:/dist:O", false, true, "volume:dist:/dist:O"},
		{"volume:../x:/dist:rw", false, false, ""},
		{"volume:dist:/proc/x:rw", false, false, ""}, // reserved target
		{"volume:dist:/dist:rw,U", false, false, ""}, // podman options cannot ride along
		{"work:dist:/dist:rw", false, true, "work:dist:/dist:rw"},
		{"work:out/bin:/out:ro", false, true, "work:out/bin:/out:ro"},
		{"work:../..:/x:rw", false, false, ""},
		{"work:a/../../b:/x:rw", false, false, ""},
		{"work:/etc:/x:ro", false, false, ""},
		{"work:.:/x:ro", false, false, ""},
		{"work:dist:/dist:O", false, false, ""},
		{"host:/srv/out:/out:rw", true, true, "host:/srv/out:/out:rw"},
		{"host:/srv/out:/out:rw", false, false, ""}, // never from a preset
		{"host:srv/out:/out:rw", true, false, ""},   // relative
		{"host:/srv/../etc:/out:ro", true, false, ""},
		{"host:/srv/out:/out:z", true, false, ""}, // relabel or other modes are not ours to pass
		{"host:/srv/out:/:ro", true, false, ""},
		{"host:/srv/out:/run/podman:ro", true, false, ""},
		{"host:/srv/out:/home/sandbox:ro", true, false, ""},
		{"host:/srv/out:/out x:ro", true, false, ""},
		{"host:/srv/\"out:/out:ro", true, false, ""},
		{"bind:/srv:/srv:rw", true, false, ""}, // unknown kind
		{"tmpfs:relative", false, false, ""},
		{"tmpfs:/a/../etc", false, false, ""},
		{"tmpfs:/x,tmpfs:/x", false, false, ""}, // a target twice
	} {
		ms, err := ParseMounts(c.in, "test", c.allowHost)
		if (err == nil) != c.ok {
			t.Fatalf("%q (host %v): err %v, want ok=%v", c.in, c.allowHost, err, c.ok)
		}
		if c.ok && ms[0].String() != c.want {
			t.Fatalf("%q: %s, want %s", c.in, ms[0], c.want)
		}
	}
}

// U23: a preset can never mount a host folder; WORKDIR is checked like a target.
func TestPresetMounts(t *testing.T) {
	p, err := ParsePreset(strings.NewReader("NAME=build\nIMAGE="+pinned+"\nWORKDIR=/src\nMOUNT=work:dist:/dist:rw,tmpfs:/tmp:1g\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if p.Workdir != "/src" || len(p.Mounts) != 2 || p.Mounts[0].Kind != MountWork {
		t.Fatalf("%+v", p)
	}
	for _, bad := range []string{
		"MOUNT=host:/home/me/.ssh:/ssh:ro\n",
		"WORKDIR=/proc\n",
		"WORKDIR=src\n",
		"WORKDIR=/a,b\n",
	} {
		if _, err := ParsePreset(strings.NewReader("NAME=build\nIMAGE="+pinned+"\n"+bad), "test"); err == nil {
			t.Fatalf("%q: want error", bad)
		}
	}
}

// U23: preset, sandbox file and flags merge by target, the later winning; a stored sandbox keeps
// its mounts and WORKDIR.
func TestNewSandboxMounts(t *testing.T) {
	dir := t.TempDir()
	p, err := ParsePreset(strings.NewReader("NAME=dev\nIMAGE="+pinned+"\nWORKDIR=/src\nMOUNT=tmpfs:/tmp:1g,volume:dist:/dist:rw\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	flag, err := ParseMounts("volume:dist:/dist:ro,host:/srv/data:/data:ro", "--mount", true)
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewSandbox(p, map[string]string{"MOUNT": "tmpfs:/tmp:2g,work:out:/out:rw"}, dir,
		Overrides{Name: "tt", Mounts: flag, WorkspaceMode: WorkspaceRO})
	if err != nil {
		t.Fatal(err)
	}
	got := mountsString(e.Mounts)
	if got != "tmpfs:/tmp:2g,volume:dist:/dist:ro,work:out:/out:rw,host:/srv/data:/data:ro" ||
		e.Workdir != "/src" || e.WorkspaceMode != WorkspaceRO {
		t.Fatalf("merged: %s, workdir %s, mode %s", got, e.Workdir, e.WorkspaceMode)
	}
	path := filepath.Join(dir, "tt.env")
	if err := os.WriteFile(path, []byte(e.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	back, err := LoadEnv(path)
	if err != nil || mountsString(back.Mounts) != got || back.Workdir != "/src" {
		t.Fatalf("stored: %+v, %v", back, err)
	}
	// A mount on the workspace itself, or a work: mount without a workspace, is refused.
	if _, err := NewSandbox(p, map[string]string{"MOUNT": "tmpfs:/src"}, dir, Overrides{Name: "tt"}); err == nil {
		t.Fatal("mount on WORKDIR accepted")
	}
	if _, err := NewSandbox(p, map[string]string{"MOUNT": "work:out:/out:ro", "WORKSPACE_MODE": "none"}, dir, Overrides{Name: "tt"}); err == nil {
		t.Fatal("work: mount without a workspace accepted")
	}
}

// U23: on the host side, a work: folder must stay inside the workspace (symlinks included), and a
// host: folder must not be the root, the home folder or hold the Podman socket.
func TestResolveMountSources(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(ws, "dist"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	home := t.TempDir()
	t.Setenv("HOME", home)
	data := filepath.Join(home, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		m  Mount
		ok bool
	}{
		{Mount{Kind: MountWork, Source: "dist", Target: "/dist", Mode: "rw"}, true},
		{Mount{Kind: MountWork, Source: "escape", Target: "/x", Mode: "rw"}, false}, // symlink out
		{Mount{Kind: MountWork, Source: "missing", Target: "/x", Mode: "rw"}, false},
		{Mount{Kind: MountWork, Source: "file", Target: "/x", Mode: "ro"}, false},
		{Mount{Kind: MountHost, Source: data, Target: "/data", Mode: "ro"}, true},
		{Mount{Kind: MountHost, Source: home, Target: "/h", Mode: "ro"}, false},
		{Mount{Kind: MountHost, Source: "/", Target: "/r", Mode: "ro"}, false},
		{Mount{Kind: MountHost, Source: runtime, Target: "/s", Mode: "ro"}, false},
		{Mount{Kind: MountHost, Source: filepath.Dir(runtime), Target: "/s", Mode: "ro"}, false}, // a parent
		{Mount{Kind: MountHost, Source: "/run", Target: "/s", Mode: "ro"}, false},
		{Mount{Kind: MountTmpfs, Target: "/tmp", Mode: "rw"}, true},
	} {
		_, err := ResolveMountSources([]Mount{c.m}, ws)
		if (err == nil) != c.ok {
			t.Fatalf("%s: err %v, want ok=%v", c.m, err, c.ok)
		}
	}
}
