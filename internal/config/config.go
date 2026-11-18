// Package config reads sandbox and preset definitions.
//
// burrow runs presets, not images. A preset names a pinned image and the settings it runs with.
// It is used two ways: `burrow run PRESET` starts a persistent sandbox from it, and
// `burrow exec PRESET` runs it as a one-off tool next to a sandbox, inheriting what the preset
// allows (workspace, proxy) and never more than the sandbox has. A sandbox file (`burrow up`) is a
// preset reference plus local settings, like a compose file.
//
// The format is deliberately small: one KEY=VALUE per line, '#' comments, optional single or
// double quotes around the value. No variable expansion, no command substitution, no includes:
// a definition is data, never code. The format is provisional (see docs/decisions/0003).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Engine is the OCI runtime that runs a container.
type Engine string

// Engines.
const (
	EngineCrun Engine = "crun" // default: plain container (R11)
	EngineKrun Engine = "krun" // experimental: libkrun microVM, own kernel (R11)
)

// WorkspaceMode is how the workspace folder is mounted.
type WorkspaceMode string

// Workspace modes.
const (
	WorkspaceNone    WorkspaceMode = "none"
	WorkspaceRO      WorkspaceMode = "ro"
	WorkspaceRW      WorkspaceMode = "rw"
	WorkspaceInherit WorkspaceMode = "inherit" // exec only: the sandbox's mode
)

// NetworkMode is how a preset reaches the network.
type NetworkMode string

// Network modes.
const (
	NetworkNone  NetworkMode = "none"  // no interface but loopback
	NetworkProxy NetworkMode = "proxy" // internal network whose only exit is its own proxy (SB3)
	// NetworkInherit (exec only): its own proxy with the sandbox's allowlist. The tool never joins
	// the sandbox's network, so it cannot reach the sandbox nor the sandbox it.
	NetworkInherit NetworkMode = "inherit"
)

// Env is a persistent sandbox: one agent or developer environment. It is what `up` and `run`
// start and what burrow stores, resolved: no preset reference, no relative paths.
type Env struct {
	Name          string
	Image         string
	Workspace     string        // host folder; mounted at /work
	WorkspaceMode WorkspaceMode // rw by default for a sandbox
	Allow         []string      // hosts the sandbox's proxy lets through
	Engine        Engine
	RunAs         string // uid:gid inside the sandbox; the host user is mapped to it (keep-id)
	ProxyImage    string
	ForwardImage  string
	Memory        string // podman --memory, optional
	// Definition is the sandbox file `up` read. When it is inside the workspace it is mounted
	// read-only over itself, so the agent cannot widen its own sandbox for the next `up`.
	Definition string
}

// Preset is a named, pinned image with the settings it runs with (see the package comment).
type Preset struct {
	Name          string
	Image         string
	Args          []string
	WorkspaceMode WorkspaceMode // ro by default for a preset
	Network       NetworkMode
	Allow         []string
	Engine        Engine
	RunAs         string
	Cache         *Cache
	Memory        string
	Source        string            // file path, or "built-in"
	raw           map[string]string // the keys the preset sets, for NewSandbox
}

// Cache is a named volume shared by runs of the same tool (e.g. the Trivy database).
type Cache struct {
	Volume string // without the burrow-cache- prefix
	Path   string // mount point inside the container
	Mode   string // rw, ro, or O (overlay: writes are discarded with the container)
}

var (
	keyRe   = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	nameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}[a-z0-9]$`)
	runAsRe = regexp.MustCompile(`^[0-9]{1,10}:[0-9]{1,10}$`)
	hostRe  = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)
	// An image must be pinned by digest (D1); a tag is allowed only next to the digest, and local
	// images (built by `make images`) are allowed by name.
	imageRe = regexp.MustCompile(`^(localhost/[a-z0-9._/-]+(:[A-Za-z0-9._-]+)?|[a-z0-9.-]+(:[0-9]+)?/[a-z0-9._/-]+(:[A-Za-z0-9._-]+)?@sha256:[a-f0-9]{64})$`)
	volRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,62}$`)
)

// Parse reads KEY=VALUE pairs. Unknown keys are an error: a typo must not silently weaken a setting.
func Parse(r io.Reader, known map[string]bool) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", n)
		}
		k = strings.TrimSpace(k)
		if !keyRe.MatchString(k) {
			return nil, fmt.Errorf("line %d: invalid key %q", n, k)
		}
		if !known[k] {
			return nil, fmt.Errorf("line %d: unknown key %q", n, k)
		}
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("line %d: %s set twice", n, k)
		}
		v, err := unquote(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		out[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func unquote(v string) (string, error) {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		if v[len(v)-1] != v[0] {
			return "", errors.New("unterminated quote")
		}
		return v[1 : len(v)-1], nil
	}
	if strings.ContainsAny(v, `"'`) {
		return "", errors.New("quotes only around the whole value")
	}
	return v, nil
}

// envKeys are the keys of a stored sandbox (state, written by burrow).
var envKeys = keys("NAME", "IMAGE", "WORKSPACE", "WORKSPACE_MODE", "ALLOW", "ENGINE", "RUN_AS",
	"PROXY_IMAGE", "FORWARD_IMAGE", "MEMORY", "DEFINITION")

// sandboxKeys are the keys of a sandbox file (`burrow up`).
var sandboxKeys = keys("NAME", "PRESET", "IMAGE", "WORKSPACE", "WORKSPACE_MODE", "NETWORK", "ALLOW",
	"ENGINE", "RUN_AS", "PROXY_IMAGE", "FORWARD_IMAGE", "MEMORY")

var presetKeys = keys("NAME", "IMAGE", "ARGS", "WORKSPACE_MODE", "NETWORK", "ALLOW", "ENGINE", "RUN_AS",
	"CACHE", "MEMORY")

// toolOnly are preset keys that make sense only for exec.
var toolOnly = []string{"ARGS", "CACHE"}

func keys(k ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range k {
		m[x] = true
	}
	return m
}

// Defaults for images built locally by `make images`.
const (
	DefaultProxyImage   = "localhost/burrow-proxy:dev"
	DefaultForwardImage = "localhost/burrow-forward:dev"
)

// LoadEnv reads and validates a stored sandbox (written by burrow).
func LoadEnv(path string) (*Env, error) {
	m, err := parseFile(path, envKeys)
	if err != nil {
		return nil, err
	}
	e := &Env{
		Name:          m["NAME"],
		Image:         m["IMAGE"],
		Workspace:     m["WORKSPACE"],
		WorkspaceMode: WorkspaceMode(def(m["WORKSPACE_MODE"], string(WorkspaceRW))),
		Allow:         list(m["ALLOW"]),
		Engine:        Engine(def(m["ENGINE"], string(EngineCrun))),
		RunAs:         def(m["RUN_AS"], "1000:1000"),
		ProxyImage:    def(m["PROXY_IMAGE"], DefaultProxyImage),
		ForwardImage:  def(m["FORWARD_IMAGE"], DefaultForwardImage),
		Memory:        m["MEMORY"],
		Definition:    m["DEFINITION"],
	}
	return e, e.Validate()
}

// LoadSandboxFile reads a sandbox file without resolving it (see NewSandbox).
func LoadSandboxFile(path string) (map[string]string, error) {
	return parseFile(path, sandboxKeys)
}

// Overrides are command-line settings; they win over the sandbox file and the preset.
type Overrides struct {
	Name      string
	Image     string
	Workspace string // absolute
}

// NewSandbox resolves a sandbox from a preset (may be nil), a sandbox file's keys (may be nil;
// relative paths are relative to baseDir) and overrides. Later sources win: preset, file, flags.
func NewSandbox(p *Preset, file map[string]string, baseDir string, o Overrides) (*Env, error) {
	m := map[string]string{}
	if p != nil {
		for _, k := range toolOnly {
			if p.raw[k] != "" {
				return nil, fmt.Errorf("preset %s sets %s: it is a tool preset, use `burrow exec`", p.Name, k)
			}
		}
		for k, v := range p.raw {
			m[k] = v
		}
	}
	for k, v := range file {
		if k != "PRESET" {
			m[k] = v
		}
	}
	for k, v := range map[string]string{"NAME": o.Name, "IMAGE": o.Image, "WORKSPACE": o.Workspace} {
		if v != "" {
			m[k] = v
		}
	}
	switch NetworkMode(m["NETWORK"]) {
	case "", NetworkProxy:
	default:
		return nil, fmt.Errorf("NETWORK %q: a sandbox always has its own proxy (an empty ALLOW blocks everything)", m["NETWORK"])
	}
	e := &Env{
		Name:          m["NAME"],
		Image:         m["IMAGE"],
		WorkspaceMode: WorkspaceMode(def(m["WORKSPACE_MODE"], string(WorkspaceRW))),
		Allow:         list(m["ALLOW"]),
		Engine:        Engine(def(m["ENGINE"], string(EngineCrun))),
		RunAs:         def(m["RUN_AS"], "1000:1000"),
		ProxyImage:    def(m["PROXY_IMAGE"], DefaultProxyImage),
		ForwardImage:  def(m["FORWARD_IMAGE"], DefaultForwardImage),
		Memory:        m["MEMORY"],
	}
	if e.WorkspaceMode != WorkspaceNone {
		ws := def(m["WORKSPACE"], ".")
		if !filepath.IsAbs(ws) {
			ws = filepath.Join(baseDir, ws)
		}
		e.Workspace = filepath.Clean(ws)
	}
	return e, e.Validate()
}

// Validate checks every field; it is also run on definitions stored by `burrow up`.
func (e *Env) Validate() error {
	var errs []error
	if !nameRe.MatchString(e.Name) {
		errs = append(errs, fmt.Errorf("NAME %q: 2 to 32 of a-z, 0-9 and '-'", e.Name))
	}
	errs = append(errs, checkImage("IMAGE", e.Image), checkImage("PROXY_IMAGE", e.ProxyImage),
		checkImage("FORWARD_IMAGE", e.ForwardImage), checkEngine(e.Engine), checkRunAs(e.RunAs),
		checkHosts(e.Allow), checkMemory(e.Memory))
	switch e.WorkspaceMode {
	case WorkspaceRW, WorkspaceRO:
		if e.Workspace == "" {
			errs = append(errs, errors.New("WORKSPACE is required unless WORKSPACE_MODE=none"))
		}
		errs = append(errs, CheckPath("WORKSPACE", e.Workspace))
	case WorkspaceNone:
	default:
		errs = append(errs, fmt.Errorf("WORKSPACE_MODE %q: none, ro or rw", e.WorkspaceMode))
	}
	errs = append(errs, CheckPath("DEFINITION", e.Definition))
	return errors.Join(errs...)
}

// CheckPath refuses host paths that would change the meaning of a podman -v argument
// (host:container:mode) or of a stored definition: quotes, commas, newlines, extra colons (one is
// allowed for a Windows drive letter).
func CheckPath(key, path string) error {
	if strings.ContainsAny(path, "\"',\n\r") || strings.Count(path, ":") > 1 {
		return fmt.Errorf("%s %q: no quotes, commas, newlines or extra colons", key, path)
	}
	return nil
}

// LoadPreset reads and validates a preset file.
func LoadPreset(path string) (*Preset, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the user's own preset file
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return ParsePreset(f, path)
}

// ParsePreset reads and validates a preset; src names it in errors.
func ParsePreset(r io.Reader, src string) (*Preset, error) {
	m, err := Parse(r, presetKeys)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	p := &Preset{
		Source:        src,
		raw:           m,
		Name:          m["NAME"],
		Image:         m["IMAGE"],
		Args:          strings.Fields(m["ARGS"]),
		WorkspaceMode: WorkspaceMode(def(m["WORKSPACE_MODE"], string(WorkspaceRO))),
		Network:       NetworkMode(def(m["NETWORK"], string(NetworkNone))),
		Allow:         list(m["ALLOW"]),
		Engine:        Engine(def(m["ENGINE"], string(EngineCrun))),
		RunAs:         def(m["RUN_AS"], "65532:65532"),
		Memory:        m["MEMORY"],
	}
	var errs []error
	if c := m["CACHE"]; c != "" {
		p.Cache, err = parseCache(c)
		errs = append(errs, err)
	}
	if !nameRe.MatchString(p.Name) {
		errs = append(errs, fmt.Errorf("NAME %q: 2 to 32 of a-z, 0-9 and '-'", p.Name))
	}
	errs = append(errs, checkImage("IMAGE", p.Image), checkEngine(p.Engine), checkRunAs(p.RunAs),
		checkHosts(p.Allow), checkMemory(p.Memory))
	switch p.WorkspaceMode {
	case WorkspaceNone, WorkspaceRO, WorkspaceRW, WorkspaceInherit:
	default:
		errs = append(errs, fmt.Errorf("WORKSPACE_MODE %q: none, ro, rw or inherit", p.WorkspaceMode))
	}
	switch p.Network {
	case NetworkNone, NetworkInherit:
		if len(p.Allow) > 0 {
			errs = append(errs, errors.New("ALLOW needs NETWORK=proxy"))
		}
	case NetworkProxy:
	default:
		errs = append(errs, fmt.Errorf("NETWORK %q: none, proxy or inherit", p.Network))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	return p, nil
}

// FindPreset resolves a preset reference. A reference with a path separator or a .preset suffix is
// a file. Otherwise it is a name: userDir/NAME.preset first (the user's own presets, outside any
// workspace), then the presets built into burrow. Presets are never looked up in the workspace:
// an agent could write one there and have the host run it with wider rights.
func FindPreset(ref, userDir string, builtin fs.FS) (*Preset, error) {
	if strings.ContainsAny(ref, `/\`) || strings.HasSuffix(ref, ".preset") {
		return LoadPreset(ref)
	}
	if !nameRe.MatchString(ref) {
		return nil, fmt.Errorf("preset %q: a name (a-z, 0-9, '-') or a path to a .preset file", ref)
	}
	if userDir != "" {
		p, err := LoadPreset(filepath.Join(userDir, ref+".preset"))
		if !errors.Is(err, os.ErrNotExist) {
			return p, err
		}
	}
	f, err := builtin.Open(ref + ".preset")
	if err != nil {
		return nil, fmt.Errorf("no preset %q (see `burrow presets`)", ref)
	}
	defer func() { _ = f.Close() }()
	return ParsePreset(f, "built-in")
}

// PresetInfo is one entry of ListPresets.
type PresetInfo struct{ Name, Source string }

// ListPresets lists the presets FindPreset can resolve by name; user presets hide built-ins.
func ListPresets(userDir string, builtin fs.FS) []PresetInfo {
	seen := map[string]string{}
	if userDir != "" {
		files, _ := filepath.Glob(filepath.Join(userDir, "*.preset"))
		for _, f := range files {
			seen[strings.TrimSuffix(filepath.Base(f), ".preset")] = f
		}
	}
	files, _ := fs.Glob(builtin, "*.preset")
	for _, f := range files {
		n := strings.TrimSuffix(f, ".preset")
		if _, ok := seen[n]; !ok {
			seen[n] = "built-in"
		}
	}
	out := make([]PresetInfo, 0, len(seen))
	for n, s := range seen {
		out = append(out, PresetInfo{n, s})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func parseCache(s string) (*Cache, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("CACHE %q: volume:/path:rw|ro|O", s)
	}
	c := &Cache{Volume: parts[0], Path: parts[1], Mode: parts[2]}
	if !volRe.MatchString(c.Volume) || !strings.HasPrefix(c.Path, "/") || strings.Contains(c.Path, "..") {
		return nil, fmt.Errorf("CACHE %q: invalid volume or path", s)
	}
	switch c.Mode {
	case "rw", "ro", "O":
	default:
		return nil, fmt.Errorf("CACHE %q: mode rw, ro or O", s)
	}
	return c, nil
}

func parseFile(path string, known map[string]bool) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the user's own definition file
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	m, err := Parse(f, known)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

func def(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func list(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
}

func checkImage(key, img string) error {
	if !imageRe.MatchString(img) {
		return fmt.Errorf("%s %q: pin by digest (registry/repo[:tag]@sha256:...) or use a localhost/ image", key, img)
	}
	return nil
}

func checkEngine(e Engine) error {
	if e != EngineCrun && e != EngineKrun {
		return fmt.Errorf("ENGINE %q: crun or krun", e)
	}
	return nil
}

func checkRunAs(s string) error {
	if !runAsRe.MatchString(s) {
		return fmt.Errorf("RUN_AS %q: uid:gid", s)
	}
	if uid, _ := strconv.Atoi(strings.SplitN(s, ":", 2)[0]); uid == 0 {
		return errors.New("RUN_AS: root (uid 0) is not allowed")
	}
	return nil
}

func checkHosts(hosts []string) error {
	for _, h := range hosts {
		if !hostRe.MatchString(h) {
			return fmt.Errorf("ALLOW %q: a lowercase host name or *.domain (no IPs, no ports)", h)
		}
	}
	return nil
}

var memRe = regexp.MustCompile(`^[0-9]{1,6}[kmg]$`)

func checkMemory(m string) error {
	if m != "" && !memRe.MatchString(m) {
		return fmt.Errorf("MEMORY %q: e.g. 512m or 2g", m)
	}
	return nil
}

// String renders the environment as a definition file that LoadEnv reads back unchanged.
func (e *Env) String() string {
	var b strings.Builder
	w := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s=\"%s\"\n", k, v)
		}
	}
	w("NAME", e.Name)
	w("IMAGE", e.Image)
	w("WORKSPACE", e.Workspace)
	w("WORKSPACE_MODE", string(e.WorkspaceMode))
	w("ALLOW", strings.Join(e.Allow, ","))
	w("ENGINE", string(e.Engine))
	w("RUN_AS", e.RunAs)
	w("PROXY_IMAGE", e.ProxyImage)
	w("FORWARD_IMAGE", e.ForwardImage)
	w("MEMORY", e.Memory)
	w("DEFINITION", e.Definition)
	return b.String()
}
