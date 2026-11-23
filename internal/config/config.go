// Package config reads sandbox and preset definitions.
//
// burrow runs presets, not images. A preset names a pinned image and the least privilege it needs
// to do its job. It is used two ways: `burrow run PRESET` starts a persistent sandbox from it, and
// `burrow exec PRESET` runs it as a one-off tool next to a sandbox (a sibling, with its own
// restrictions). Where the preset says inherit, the tool takes the sandbox's setting, or the
// preset's own fallback when there is no sandbox. Strict mode caps tools by their sandbox. A
// sandbox file (`burrow up`) is a preset reference plus local settings, like a compose file.
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
	WorkspaceInherit WorkspaceMode = "inherit" // exec only: the sandbox's mode, else the fallback
)

// NetworkMode is how a preset reaches the network.
type NetworkMode string

// Network modes.
const (
	NetworkNone  NetworkMode = "none"  // no interface but loopback
	NetworkProxy NetworkMode = "proxy" // internal network whose only exit is its own proxy (SB3)
	// NetworkInherit (exec only): its own proxy with the sandbox's allowlist, else the fallback.
	// The tool never joins the sandbox's network, so it cannot reach the sandbox nor the sandbox it.
	NetworkInherit NetworkMode = "inherit"
)

// ProxyKind is the proxy a sandbox (and the tools next to it) gets as its only exit.
type ProxyKind string

// Proxies.
const (
	// ProxyBurrow is burrow-proxy (internal/proxy): CONNECT to allowed hosts on allowed ports.
	// The default until iron-proxy has passed on every platform (decision 0013).
	ProxyBurrow ProxyKind = "burrow"
	// ProxyIron is iron-proxy (github.com/ironsh/iron-proxy) behind burrow's launcher
	// (cmd/burrow-iron-proxy): experimental, opt-in (decision 0013).
	ProxyIron ProxyKind = "iron"
)

// ToolCaps are the capabilities a tool preset may ask for with CAP_ADD: at most Podman's own
// default set plus NET_RAW. Nothing that administers the kernel, devices, namespaces or other
// processes (SYS_ADMIN, SYS_PTRACE, SYS_MODULE, NET_ADMIN, BPF...). no-new-privileges stays on.
// Sandboxes never get any; strict mode refuses tools that ask (decision 0007).
var ToolCaps = keys("CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL", "NET_BIND_SERVICE",
	"NET_RAW", "SETFCAP", "SETGID", "SETPCAP", "SETUID", "SYS_CHROOT")

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
	UserFlag      bool   // RunAs came from --user (may be root), not from a preset or file
	Proxy         ProxyKind
	ProxyImage    string // the image of Proxy, unless PROXY_IMAGE names another
	ForwardImage  string
	Memory        string   // podman --memory, optional
	Env           []string // KEY=VALUE, not secret
	// Strict caps every tool run next to this sandbox by the sandbox's own settings.
	Strict bool
	// Definition is the sandbox file `up` read. When it is inside the workspace it is mounted
	// read-only over itself, so the agent cannot widen its own sandbox for the next `up`.
	Definition string
}

// Preset is a named, pinned image with the settings it runs with (see the package comment).
type Preset struct {
	Name              string
	Description       string
	Image             string
	Args              []string
	WorkspaceMode     WorkspaceMode // ro by default for a tool
	WorkspaceFallback WorkspaceMode // for inherit without a sandbox; ro by default
	Network           NetworkMode   // none by default for a tool
	NetworkFallback   NetworkMode   // for inherit without a sandbox; none by default
	Allow             []string
	Engine            Engine
	RunAs             string
	Caches            []Cache
	CapAdd            []string
	Env               []string // KEY=VALUE, not secret
	Memory            string
	Source            string            // file path
	raw               map[string]string // the keys the preset sets, for NewSandbox
}

// Cache is a named volume shared by the tools of one sandbox (e.g. the Trivy database, written by
// trivy-db and read by trivy-scan). Its podman volume is burrow-cache-<sandbox>-<name>: tools of
// another sandbox (or run without one) never see it, so they cannot poison it.
type Cache struct {
	Volume string // the cache's name, without prefix or scope
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
	"PROXY", "PROXY_IMAGE", "FORWARD_IMAGE", "MEMORY", "ENV", "STRICT", "DEFINITION", "USER_FLAG")

// sandboxKeys are the keys of a sandbox file (`burrow up`).
var sandboxKeys = keys("NAME", "PRESET", "IMAGE", "WORKSPACE", "WORKSPACE_MODE", "NETWORK", "ALLOW",
	"ENGINE", "RUN_AS", "PROXY", "PROXY_IMAGE", "FORWARD_IMAGE", "MEMORY", "ENV", "STRICT")

var presetKeys = keys("NAME", "DESCRIPTION", "IMAGE", "ARGS", "WORKSPACE_MODE", "NETWORK", "ALLOW",
	"ENGINE", "RUN_AS", "CACHE", "CAP_ADD", "ENV", "MEMORY")

// toolOnly are preset keys that make sense only for exec.
var toolOnly = []string{"ARGS", "CACHE", "CAP_ADD"}

func keys(k ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range k {
		m[x] = true
	}
	return m
}

// Defaults for images built locally by `make images`.
const (
	DefaultProxyImage     = "localhost/burrow-proxy:dev"
	DefaultIronProxyImage = "localhost/burrow-iron-proxy:dev"
	DefaultForwardImage   = "localhost/burrow-forward:dev"
)

// ProxyImageFor is the image of a proxy kind (an unknown kind gets burrow-proxy's image; Validate
// refuses the kind itself).
func ProxyImageFor(k ProxyKind) string {
	if k == ProxyIron {
		return DefaultIronProxyImage
	}
	return DefaultProxyImage
}

// CheckProxy accepts the known proxy kinds.
func CheckProxy(k ProxyKind) error {
	switch k {
	case ProxyBurrow, ProxyIron:
		return nil
	}
	return fmt.Errorf("PROXY %q: burrow or iron", k)
}

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
		Proxy:         ProxyKind(def(m["PROXY"], string(ProxyBurrow))),
		ForwardImage:  def(m["FORWARD_IMAGE"], DefaultForwardImage),
		Memory:        m["MEMORY"],
		Env:           commaList(m["ENV"]),
		Definition:    m["DEFINITION"],
	}
	e.ProxyImage = def(m["PROXY_IMAGE"], ProxyImageFor(e.Proxy))
	var err2 error
	e.Strict, err2 = parseBool("STRICT", m["STRICT"])
	if err2 != nil {
		return nil, err2
	}
	if e.UserFlag, err2 = parseBool("USER_FLAG", m["USER_FLAG"]); err2 != nil {
		return nil, err2
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
	RunAs     string // --user: uid:gid; unlike RUN_AS it may be root (exception in decision 0011)
	Proxy     ProxyKind
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
			if k != "DESCRIPTION" {
				m[k] = v
			}
		}
	}
	for k, v := range file {
		if k != "PRESET" {
			m[k] = v
		}
	}
	for k, v := range map[string]string{"NAME": o.Name, "IMAGE": o.Image, "WORKSPACE": o.Workspace, "PROXY": string(o.Proxy)} {
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
		Proxy:         ProxyKind(def(m["PROXY"], string(ProxyBurrow))),
		ForwardImage:  def(m["FORWARD_IMAGE"], DefaultForwardImage),
		Memory:        m["MEMORY"],
		Env:           commaList(m["ENV"]),
	}
	e.ProxyImage = def(m["PROXY_IMAGE"], ProxyImageFor(e.Proxy))
	var err error
	if e.Strict, err = parseBool("STRICT", m["STRICT"]); err != nil {
		return nil, err
	}
	if o.RunAs != "" {
		e.RunAs, e.UserFlag = o.RunAs, true
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
	if e.Proxy == "" {
		e.Proxy = ProxyBurrow // definitions stored before PROXY existed
	}
	var errs []error
	if !nameRe.MatchString(e.Name) {
		errs = append(errs, fmt.Errorf("NAME %q: 2 to 32 of a-z, 0-9 and '-'", e.Name))
	}
	if e.Name == "exec" {
		errs = append(errs, errors.New(`NAME "exec" is reserved (tools run without a sandbox)`))
	}
	errs = append(errs, checkImage("IMAGE", e.Image), CheckProxy(e.Proxy), checkImage("PROXY_IMAGE", e.ProxyImage),
		checkImage("FORWARD_IMAGE", e.ForwardImage), checkEngine(e.Engine), checkEnvUser(e),
		checkHosts(e.Allow), checkMemory(e.Memory), checkEnv(e.Env))
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
		Source:      src,
		raw:         m,
		Name:        m["NAME"],
		Description: m["DESCRIPTION"],
		Image:       m["IMAGE"],
		Args:        strings.Fields(m["ARGS"]),
		Allow:       list(m["ALLOW"]),
		Engine:      Engine(def(m["ENGINE"], string(EngineCrun))),
		RunAs:       def(m["RUN_AS"], "65532:65532"),
		CapAdd:      list(m["CAP_ADD"]),
		Env:         commaList(m["ENV"]),
		Memory:      m["MEMORY"],
	}
	var errs []error
	ws, wsFb, _ := strings.Cut(def(m["WORKSPACE_MODE"], string(WorkspaceRO)), ":")
	p.WorkspaceMode, p.WorkspaceFallback = WorkspaceMode(ws), WorkspaceMode(def(wsFb, string(WorkspaceRO)))
	nw, nwFb, _ := strings.Cut(def(m["NETWORK"], string(NetworkNone)), ":")
	p.Network, p.NetworkFallback = NetworkMode(nw), NetworkMode(def(nwFb, string(NetworkNone)))
	if c := m["CACHE"]; c != "" {
		p.Caches, err = parseCaches(c)
		errs = append(errs, err)
	}
	if !nameRe.MatchString(p.Name) {
		errs = append(errs, fmt.Errorf("NAME %q: 2 to 32 of a-z, 0-9 and '-'", p.Name))
	}
	errs = append(errs, checkImage("IMAGE", p.Image), checkEngine(p.Engine), CheckRunAs(p.RunAs),
		checkHosts(p.Allow), checkMemory(p.Memory), checkEnv(p.Env), checkCaps(p.CapAdd))
	if p.WorkspaceMode != WorkspaceInherit && wsFb != "" {
		errs = append(errs, errors.New("WORKSPACE_MODE: a fallback (:mode) only goes with inherit"))
	}
	if p.Network != NetworkInherit && nwFb != "" {
		errs = append(errs, errors.New("NETWORK: a fallback (:mode) only goes with inherit"))
	}
	for _, w := range []WorkspaceMode{p.WorkspaceMode, p.WorkspaceFallback} {
		switch w {
		case WorkspaceNone, WorkspaceRO, WorkspaceRW, WorkspaceInherit:
		default:
			errs = append(errs, fmt.Errorf("WORKSPACE_MODE %q: none, ro, rw or inherit[:none|ro|rw]", w))
		}
	}
	if p.WorkspaceFallback == WorkspaceInherit {
		errs = append(errs, errors.New("WORKSPACE_MODE: the fallback cannot be inherit"))
	}
	for _, n := range []NetworkMode{p.Network, p.NetworkFallback} {
		switch n {
		case NetworkNone, NetworkProxy, NetworkInherit:
		default:
			errs = append(errs, fmt.Errorf("NETWORK %q: none, proxy or inherit[:none|proxy]", n))
		}
	}
	if p.NetworkFallback == NetworkInherit {
		errs = append(errs, errors.New("NETWORK: the fallback cannot be inherit"))
	}
	// ALLOW is the preset's own allowlist: used with proxy, or as inherit's proxy fallback.
	if len(p.Allow) > 0 && p.Network != NetworkProxy && (p.Network != NetworkInherit || p.NetworkFallback != NetworkProxy) {
		errs = append(errs, errors.New("ALLOW needs NETWORK=proxy or NETWORK=inherit:proxy"))
	}
	if strings.ContainsAny(p.Description, "\r\n") {
		errs = append(errs, errors.New("DESCRIPTION: one line"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	return p, nil
}

// FindPreset resolves a preset reference. A reference with a path separator or a .preset suffix is
// a file. Otherwise it is a name in userDir (NAME.preset): the user's own presets, outside any
// workspace. Presets are never looked up in the workspace: an agent could write one there and have
// the host run it with wider rights. A shared index of published presets is planned (roadmap 0.4).
func FindPreset(ref, userDir string) (*Preset, error) {
	if strings.ContainsAny(ref, `/\`) || strings.HasSuffix(ref, ".preset") {
		return LoadPreset(ref)
	}
	if !nameRe.MatchString(ref) {
		return nil, fmt.Errorf("preset %q: a name (a-z, 0-9, '-') or a path to a .preset file", ref)
	}
	p, err := LoadPreset(filepath.Join(userDir, ref+".preset"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no preset %q in %s (see `burrow presets`)", ref, userDir)
	}
	return p, err
}

// ListPresets returns the valid presets in userDir, sorted by name. Invalid files are reported in
// the error but do not hide the others.
func ListPresets(userDir string) ([]*Preset, error) {
	files, _ := filepath.Glob(filepath.Join(userDir, "*.preset"))
	sort.Strings(files)
	var out []*Preset
	var errs []error
	for _, f := range files {
		p, err := LoadPreset(f)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, p)
	}
	return out, errors.Join(errs...)
}

// SearchPresets returns the presets whose name or description contains term (any case).
func SearchPresets(all []*Preset, term string) []*Preset {
	term = strings.ToLower(term)
	var out []*Preset
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Name+" "+p.Description), term) {
			out = append(out, p)
		}
	}
	return out
}

func parseCaches(s string) ([]Cache, error) {
	var out []Cache
	seen := map[string]bool{}
	for _, one := range list(s) {
		parts := strings.Split(one, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("CACHE %q: name:/path:rw|ro|O", one)
		}
		c := Cache{Volume: parts[0], Path: parts[1], Mode: parts[2]}
		if !volRe.MatchString(c.Volume) || !strings.HasPrefix(c.Path, "/") || strings.Contains(c.Path, "..") {
			return nil, fmt.Errorf("CACHE %q: invalid name or path", one)
		}
		switch c.Mode {
		case "rw", "ro", "O":
		default:
			return nil, fmt.Errorf("CACHE %q: mode rw, ro or O", one)
		}
		if seen[c.Volume] || seen[c.Path] {
			return nil, fmt.Errorf("CACHE %q: name or path used twice", one)
		}
		seen[c.Volume], seen[c.Path] = true, true
		out = append(out, c)
	}
	return out, nil
}

func parseBool(key, v string) (bool, error) {
	switch v {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	}
	return false, fmt.Errorf("%s %q: true or false", key, v)
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// checkEnv validates ENV entries. They are plain settings, never secrets (those will come from the
// proxy and a vault, roadmap 0.3). Proxy variables are burrow's own and cannot be overridden.
func checkEnv(env []string) error {
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !envKeyRe.MatchString(k) {
			return fmt.Errorf("ENV %q: KEY=value, comma separated", kv)
		}
		if strings.HasSuffix(strings.ToUpper(k), "_PROXY") {
			return fmt.Errorf("ENV %s: proxy variables are set by burrow", k)
		}
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("ENV %s: one line", k)
		}
	}
	return nil
}

func checkCaps(caps []string) error {
	for _, c := range caps {
		if !ToolCaps[c] {
			return fmt.Errorf("CAP_ADD %q: one of Podman's default capabilities or NET_RAW, without CAP_ (decision 0007)", c)
		}
	}
	return nil
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

// commaList splits on commas only (values may contain spaces), trimming each entry.
func commaList(v string) []string {
	var out []string
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
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

// checkEnvUser: RUN_AS from a preset or sandbox file is never root; one given with --user may be.
func checkEnvUser(e *Env) error {
	if e.UserFlag {
		return CheckUser(e.RunAs)
	}
	return CheckRunAs(e.RunAs)
}

// CheckUser checks the --user flag: uid:gid, root (uid 0) included. Root is only ever asked for
// on the command line by the person running burrow, never by a preset or a sandbox file (which
// the agent could write), and keeps no capabilities and no-new-privileges (decision 0011).
func CheckUser(s string) error {
	if !runAsRe.MatchString(s) {
		return fmt.Errorf("--user %q: uid:gid", s)
	}
	return nil
}

// IsRoot reports whether a checked uid:gid runs as uid 0.
func IsRoot(s string) bool {
	uid, _ := strconv.Atoi(strings.SplitN(s, ":", 2)[0])
	return uid == 0
}

// CheckRunAs accepts uid:gid with a uid other than 0: a preset or sandbox file never runs
// anything as root.
func CheckRunAs(s string) error {
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
	w("PROXY", string(e.Proxy))
	w("PROXY_IMAGE", e.ProxyImage)
	w("FORWARD_IMAGE", e.ForwardImage)
	w("MEMORY", e.Memory)
	w("ENV", strings.Join(e.Env, ","))
	if e.Strict {
		w("STRICT", "true")
	}
	if e.UserFlag {
		w("USER_FLAG", "true")
	}
	w("DEFINITION", e.Definition)
	return b.String()
}
