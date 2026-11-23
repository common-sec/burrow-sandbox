package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultWorkdir is where the workspace is mounted, and the working directory, unless WORKDIR or
// --workdir says otherwise (decision 0012).
const DefaultWorkdir = "/work"

// Mount kinds (decision 0012). burrow builds every podman flag from these fields; no mount string
// from a definition reaches podman as written.
const (
	MountTmpfs  = "tmpfs"  // tmpfs:TARGET[:SIZE]
	MountVolume = "volume" // volume:NAME:TARGET:MODE, a named volume scoped to the sandbox
	MountWork   = "work"   // work:SUBDIR:TARGET:MODE, a folder inside the workspace
	MountHost   = "host"   // host:PATH:TARGET:MODE, a host folder; never from a preset
)

// Mount is one entry of MOUNT or --mount.
type Mount struct {
	Kind   string
	Source string // volume name, workspace subfolder (relative) or host path; empty for tmpfs
	Target string // absolute path inside the container
	Mode   string // ro or rw (O for volumes); rw for tmpfs
	Size   string // tmpfs only, optional
	From   string // where the entry came from, for warnings and errors (not stored)
}

// String is the entry in MOUNT syntax.
func (m Mount) String() string {
	switch m.Kind {
	case MountTmpfs:
		if m.Size != "" {
			return m.Kind + ":" + m.Target + ":" + m.Size
		}
		return m.Kind + ":" + m.Target
	default:
		return m.Kind + ":" + m.Source + ":" + m.Target + ":" + m.Mode
	}
}

var sizeRe = regexp.MustCompile(`^[0-9]{1,6}[kmg]$`)

// reservedTargets are container paths a mount may not cover: the system's own folders and the
// sandbox's home volume.
var reservedTargets = []string{"/proc", "/sys", "/dev", "/run", "/etc", "/home/sandbox"}

// ParseMounts reads a comma-separated MOUNT value. allowHost is false for presets: a preset, which
// can be shared, never chooses a folder on the user's machine (decision 0012).
func ParseMounts(s, from string, allowHost bool) ([]Mount, error) {
	var out []Mount
	for _, one := range commaList(s) {
		m, err := parseMount(one, from, allowHost)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, checkTargets(out)
}

func parseMount(one, from string, allowHost bool) (Mount, error) {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("MOUNT %q (%s): %s", one, from, fmt.Sprintf(format, a...))
	}
	if strings.ContainsAny(one, "\"'\n\r") {
		return Mount{}, bad("no quotes or newlines")
	}
	kind, rest, _ := strings.Cut(one, ":")
	m := Mount{Kind: kind, From: from}
	switch kind {
	case MountTmpfs:
		parts := strings.Split(rest, ":")
		if len(parts) > 2 {
			return m, bad("tmpfs:TARGET[:SIZE]")
		}
		m.Target, m.Mode = parts[0], "rw"
		if len(parts) == 2 {
			if m.Size = parts[1]; !sizeRe.MatchString(m.Size) {
				return m, bad("SIZE like 512m or 2g")
			}
		}
	case MountVolume, MountWork, MountHost:
		// Split from the right: a host path on Windows has a drive colon (C:\out).
		i := strings.LastIndex(rest, ":")
		j := -1
		if i > 0 {
			j = strings.LastIndex(rest[:i], ":")
		}
		if i < 0 || j < 0 {
			return m, bad("%s:SOURCE:TARGET:MODE", kind)
		}
		m.Source, m.Target, m.Mode = rest[:j], rest[j+1:i], rest[i+1:]
	default:
		return m, bad("kind tmpfs, volume, work or host")
	}
	if err := checkTarget(m.Target); err != nil {
		return m, bad("%v", err)
	}
	switch m.Kind {
	case MountVolume:
		if !volRe.MatchString(m.Source) {
			return m, bad("volume name: a-z, 0-9, '_', '.', '-'")
		}
		if m.Mode != "rw" && m.Mode != "ro" && m.Mode != "O" {
			return m, bad("mode rw, ro or O")
		}
	case MountWork:
		if m.Mode != "rw" && m.Mode != "ro" {
			return m, bad("mode rw or ro")
		}
		s := filepath.ToSlash(m.Source)
		if s == "" || strings.HasPrefix(s, "/") || filepath.IsAbs(m.Source) || filepath.VolumeName(m.Source) != "" ||
			path.Clean(s) != s || s == "." || s == ".." || strings.HasPrefix(s, "../") || strings.ContainsAny(s, ",:") {
			return m, bad("a clean folder inside the workspace, relative, without '..'")
		}
	case MountHost:
		if !allowHost {
			return m, bad("a preset cannot mount a host folder; put it in the sandbox file or use --mount (decision 0012)")
		}
		if m.Mode != "rw" && m.Mode != "ro" {
			return m, bad("mode rw or ro")
		}
		if !filepath.IsAbs(m.Source) || filepath.Clean(m.Source) != m.Source {
			return m, bad("an absolute, clean host path")
		}
		if err := CheckPath("MOUNT", m.Source); err != nil {
			return m, bad("%v", err)
		}
	}
	return m, nil
}

func checkTarget(t string) error {
	if !strings.HasPrefix(t, "/") || path.Clean(t) != t || strings.ContainsAny(t, ",:\\ ") {
		return fmt.Errorf("target %q: an absolute, clean container path without commas, colons or spaces", t)
	}
	if t == "/" {
		return errors.New("target cannot be /")
	}
	for _, r := range reservedTargets {
		if t == r || strings.HasPrefix(t, r+"/") {
			return fmt.Errorf("target %s is reserved (%s)", t, r)
		}
	}
	return nil
}

// CheckWorkdir checks WORKDIR or --workdir: a container path like a mount target.
func CheckWorkdir(w string) error {
	if err := checkTarget(w); err != nil {
		return fmt.Errorf("WORKDIR: %w", err)
	}
	return nil
}

func checkTargets(ms []Mount) error {
	seen := map[string]bool{}
	for _, m := range ms {
		if seen[m.Target] {
			return fmt.Errorf("MOUNT: target %s used twice", m.Target)
		}
		seen[m.Target] = true
	}
	return nil
}

// MergeMounts merges mount lists by target; a later list wins (preset, sandbox file, flags).
func MergeMounts(lists ...[]Mount) []Mount {
	var out []Mount
	idx := map[string]int{}
	for _, l := range lists {
		for _, m := range l {
			if i, ok := idx[m.Target]; ok {
				out[i] = m
				continue
			}
			idx[m.Target] = len(out)
			out = append(out, m)
		}
	}
	return out
}

// CheckMountLayout refuses mounts that collide with the workspace: on the working directory
// itself, or a work: mount without a workspace.
func CheckMountLayout(ms []Mount, workdir string, ws WorkspaceMode) error {
	for _, m := range ms {
		if m.Target == workdir {
			return fmt.Errorf("MOUNT %s: the target is the workspace (WORKDIR)", m)
		}
		if m.Kind == MountWork && (ws == WorkspaceNone || ws == "") {
			return fmt.Errorf("MOUNT %s: a work: mount needs a workspace (WORKSPACE_MODE is none)", m)
		}
	}
	return nil
}

// ResolveMountSources checks the host side of work: and host: mounts on this machine and returns
// the host path of each (empty for tmpfs and volumes). A work: folder must exist inside the
// workspace after symlinks are resolved. A host: folder must exist, be a folder, and must not be
// /, the home folder itself, or hold the Podman socket (/run, /var/run, $XDG_RUNTIME_DIR, or a
// parent of them), which stays impossible (decision 0011).
func ResolveMountSources(ms []Mount, workspace string) ([]string, error) {
	out := make([]string, len(ms))
	for i, m := range ms {
		switch m.Kind {
		case MountWork:
			ws, err := filepath.EvalSymlinks(workspace)
			if err != nil {
				return nil, fmt.Errorf("MOUNT %s: workspace: %w", m, err)
			}
			p, err := filepath.EvalSymlinks(filepath.Join(ws, filepath.FromSlash(m.Source)))
			if err != nil {
				return nil, fmt.Errorf("MOUNT %s: %w", m, err)
			}
			if rel, ok := within(ws, p); !ok || rel == "." {
				return nil, fmt.Errorf("MOUNT %s: resolves outside the workspace (%s)", m, p)
			}
			if err := isDir(p); err != nil {
				return nil, fmt.Errorf("MOUNT %s: %w", m, err)
			}
			if err := CheckPath("MOUNT", p); err != nil {
				return nil, err
			}
			out[i] = p
		case MountHost:
			p, err := filepath.EvalSymlinks(m.Source)
			if err != nil {
				return nil, fmt.Errorf("MOUNT %s: %w", m, err)
			}
			if err := isDir(p); err != nil {
				return nil, fmt.Errorf("MOUNT %s: %w", m, err)
			}
			if err := checkHostSource(p); err != nil {
				return nil, fmt.Errorf("MOUNT %s: %w", m, err)
			}
			if err := CheckPath("MOUNT", p); err != nil {
				return nil, err
			}
			out[i] = p
		}
	}
	return out, nil
}

func isDir(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a folder", p)
	}
	return nil
}

func checkHostSource(p string) error {
	if filepath.Dir(p) == p {
		return fmt.Errorf("%s: a whole drive or the root folder cannot be mounted", p)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if h, err := filepath.EvalSymlinks(home); err == nil {
			home = h
		}
		if p == home {
			return fmt.Errorf("%s: the whole home folder cannot be mounted; name a folder inside it", p)
		}
	}
	sockets := []string{"/run", "/var/run"}
	if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" {
		sockets = append(sockets, x)
	}
	for _, s := range sockets {
		if r, err := filepath.EvalSymlinks(s); err == nil {
			s = r
		}
		if _, ok := within(p, s); ok {
			return fmt.Errorf("%s holds %s, where the Podman socket lives (decision 0011)", p, s)
		}
		if _, ok := within(s, p); ok {
			return fmt.Errorf("%s is inside %s, where the Podman socket lives (decision 0011)", p, s)
		}
	}
	return nil
}

// within reports whether path is dir or inside it, and its relative path.
func within(dir, p string) (string, bool) {
	rel, err := filepath.Rel(dir, p)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

func mountsString(ms []Mount) string {
	s := make([]string, len(ms))
	for i, m := range ms {
		s[i] = m.String()
	}
	return strings.Join(s, ",")
}
