# 0013: iron-proxy as burrow's proxy engine: one proxy, two engines, iron by default

**Status:** proposed (2026-11-23). **Owner:** J.
**Builds on:** the organisation's egress-proxy spike and DR-0009 (proposed) in
[secure-standard-platforms PR #5](https://github.com/common-sec/secure-standard-platforms/pull/5),
and the secrets design in decision 0012 / [docs/SECRETS.md](../SECRETS.md) (draft, PR #9).

Serves R7 (credential and network proxy, roadmap 0.3) and SB3 (the proxy is the only exit). Controls
from the organisation's matrix: Data Flow Enforcement (SCF NET-04), Sandboxing AI Agents
(AAT-39.11), Secure Engineering Principles (TDA-15). SCF is credited as in secure-standard-platforms'
NOTICE.md.

## Context

`burrow-proxy` is a temporary stand-in (CONNECT, host allowlist, no private addresses). The
organisation's spike chose [iron-proxy](https://github.com/ironsh/iron-proxy) for the real proxy
(DR-0009: allowlist by host, method and path, placeholder secrets, OAuth, JSON audit log), with a
policy sidecar for what iron-proxy lacks. The spike ran on Docker Compose only; DR-0009 keeps
iron-proxy experimental until it passes on Podman. This record is the first step inside burrow:
iron-proxy as the sandbox's only exit, end to end, with the same guarantees as today and the
structure the secrets design needs.

**Review (J, PR #17):** iron-proxy is the default; one proxy with unified settings and one handler
that starts one engine or the other; a flag to choose the embedded proxy or to disable the proxy,
where disabling needs a warning and an explicit acceptance; the single-vendor risk is an
exception, as there is no better candidate now.

## Facts checked (2026-11-23, primary sources)

| What | Value | Source |
|---|---|---|
| Licence | Apache-2.0 | `LICENSE` in the repository at tag `v0.56.0` |
| Version | 0.56.0, tagged 2026-11-04: the newest past the 7-day cooldown (0.57.0 to 0.59.0 are from 2026-11-17 to 2026-11-19). It includes a fix that requires a patched Go toolchain (upstream #288) | `git log` of the tags |
| Release image | `docker.io/ironsh/iron-proxy:0.56.0@sha256:727da0cc35a6d73ed2913a38d56c4b9b4476c075149492e4fbf75552e83361ff`: its binary (built with Go 1.27.3) has 5 HIGH vulnerabilities in dependencies: go-jose 4.1.4 (CVE-2026-116587, fixed in 4.1.6), x/net 0.58.0 (CVE-2026-78661, CVE-2026-78669, fixed in 0.62.0 and 0.60.0), grpc 1.83.0 (CVE-2026-84304, CVE-2026-84445, fixed in 1.83.1 and 1.83.2). All are denial of service. 0.57.0 to 0.59.0 still ship go-jose 4.1.4 and x/net 0.58.0 | Trivy in `make scan-images` (CI run on this PR); `go.mod` at each tag |
| What we build | the module `github.com/ironsh/iron-proxy@v0.56.0` (sumdb-checked), built with Go 1.27.4 from `packaging/proxy/iron/go.mod`, which raises only go-jose to 4.1.6 (2026-10-14), x/net to 0.62.0 (2026-10-27) and grpc to 1.83.2 (2026-08-25), all past the cooldown; x/crypto, x/sys and x/text move with x/net. iron-proxy's own tests (`go test ./internal/...`) pass with these versions | `go list -m -json`, `go version -m` on both binaries |
| Maintainers | 13 authors in 199 commits since 2026-04-01; 78 of the commits since May by one maintainer | `git shortlog` at `v0.56.0` |
| Behaviour used here | `tls.mode: sni-only` (no termination; the pipeline runs on the CONNECT target **and** on the ClientHello SNI; the upstream port is always 443), `dns.enabled: false`, `upstream_deny_cidrs` checked at connect time against the resolved address (IPv4-mapped addresses unmapped), allowlist `domains` with `*.domain` globs | source at `v0.56.0`: `internal/proxy/tunnel.go`, `sni_passthrough.go`, `internal/dnsguard`, `internal/hostmatch`; and the runs below |

## Decisions

### 1. How burrow uses iron-proxy

| Option | For | Against |
|---|---|---|
| A. Keep only `burrow-proxy` until secrets are designed in full | nothing new to review | iron-proxy is never tried on Podman; the secrets work starts from zero |
| **B. One proxy, `burrow-proxy`, with two engines: iron-proxy (default) and the embedded CONNECT proxy; sni-only** | one image, one set of flags, one code path in burrow; no CA in the sandbox; tested now on Podman; the proxy binary is where secrets are loaded later | two known gaps of iron against the embedded engine (exceptions below); iron-proxy's binary in the image |
| B'. iron-proxy in its own image behind a launcher, opt-in (this PR's first version) | the default untouched | two images, two code paths in burrow, two sets of limits for the same job (review: unify) |
| C. The spike's full stack: iron-proxy + `egress-policy` sidecar (gRPC transforms) | deny rules, DoH, port and path checks the spike tested | a second container per proxy, gRPC and YAML dependencies in burrow, and `egress-policy` is not an accepted in-house tool yet (org DR-0006) |
| D. iron-proxy in MITM mode now | method and path rules, the base for secrets | every sandbox and tool must trust a per-sandbox CA (each runtime has its own trust store: Node, Python, Go, Java), for no feature that uses it yet |
| E. Envoy (DR-0009's fallback) | mature, foundation-backed | far more configuration for the same allowlist; no placeholder substitution |

**Decision: B.** `burrow-proxy` (one binary, one image `localhost/burrow-proxy:dev`) takes the
same flags for every engine and `-engine iron` (default) or `-engine embedded`. On the host,
`startProxy` is the one handler: same networks, same container flags, only `-engine` differs.

| Setting | Engine | How |
|---|---|---|
| default | `iron` | nothing to set |
| `PROXY=embedded` or `--proxy embedded` | burrow's own CONNECT proxy (`internal/proxy`) | `up`, `run`; `exec` without a sandbox |
| `PROXY=none` or `--proxy none` | **no proxy**: the sandbox gets a normal network with a route out and DNS | `up`, `run` only, and only with `--accept-risk` on the command line (a sandbox file cannot accept for the user); burrow prints a warning. Tools next to it still get their own proxy (iron): a tool never inherits "no proxy" |

A tool next to a sandbox gets the sandbox's engine (its own instance, as today). Presets cannot
set `PROXY`: the proxy is the user's choice for their environment (decision 0011).

### 2. How the configuration reaches iron-proxy

| Option | For | Against |
|---|---|---|
| **A. `burrow-proxy -engine iron` (Go standard library) renders the configuration from its flags into a tmpfs (`/run/burrow`, mode 0700, owned by the proxy's user) and `exec`s iron-proxy** | the same flags as the embedded engine; the configuration lives in memory only; the place for `load` (secrets over stdin) and reloads later; the rendered file is unit-tested | – |
| B. A file from burrow's state folder, bind-mounted read-only | nothing rendered in the container | with rootless Podman the proxy's uid 65532 cannot read the user's 0600 file without widening it; secrets would later sit on disk |
| C. iron-proxy's `IRON_*` environment variables only | no file | the allowlist cannot be set that way; values in the environment show in `podman inspect` |
| D. A Podman secret | Podman-native | stored on disk in Podman's secret store; anyone with the user's Podman access can read it (`podman secret inspect --showsecret`) |

**Decision: A.** The configuration is JSON (iron-proxy's YAML parser reads it), so burrow adds no
dependency. iron-proxy is started with an empty environment, so neither `IRON_*` overrides nor
`HTTP(S)_PROXY` can change it.

### 3. The rendered configuration (`internal/ironproxy`)

| burrow-proxy rule | iron-proxy setting |
|---|---|
| only the proxy port is reachable from the sandbox | `tunnel_listen` (CONNECT and SOCKS5) on the given address; the transparent HTTP and HTTPS listeners and metrics on `127.0.0.1`; management off; DNS server off (the sandbox network has no DNS anyway) |
| only allowed host names, no IP literals | `allowlist` with `domains` = ALLOW and `cidrs: []`; an empty ALLOW is an empty list (deny all) |
| only port 443 | sni-only mode always dials the SNI host on 443, whatever the CONNECT says; `-engine iron` refuses `-ports` other than 443 |
| no private, loopback, link-local or CGNAT addresses after resolution | `upstream_deny_cidrs`: every non-public range (IPv4 and IPv6, including metadata and documentation ranges); a unit test checks the list covers everything `burrow-proxy` refuses |
| a JSON audit line per decision | iron-proxy's audit log (`"action"`, `"rejected_by"`, host, SNI, client address) |
| the SNI must match | iron-proxy runs the allowlist on the SNI too, so `CONNECT allowed:443` with another SNI is refused |

Every proxy container, whatever its engine, gets read-only, uid 65532, no capabilities, no new
privileges, `--http-proxy=false`, the tmpfs and `--memory 128m --pids-limit 128`.

## Exceptions (O3, AGENTS.md: a weaker setting needs owner, reason, expiry)

| What | Owner | Reason | Expiry |
|---|---|---|---|
| iron-proxy, the default engine, is a young project with one main maintainer from one vendor (78 of its commits since May by one person) | J | no better candidate now (DR-0009: Envoy is the fallback; the embedded engine stays one flag away, `--proxy embedded`) | review at the next proxy decision in the organisation, or 2027-05-23 |
| `PROXY=none` removes the proxy (SB3) | J | some work cannot go through a proxy yet; the user accepts it explicitly with `--accept-risk` on the command line, with a warning, never from a file (decision 0011) | review with the audit log (roadmap), or 2027-05-23 |
| With iron, plain HTTP to an allowed host is forwarded, on any port (`http://allowed:22/` is dialled; the embedded engine refuses everything but CONNECT). Only allowed hosts are reachable, and only as HTTP | J | iron-proxy has no per-scheme or per-port rule; closing it needs a transform (the spike's policy sidecar), an upstream change, or a small gate in burrow-proxy | when TLS termination lands (step 2 below), or 2027-02-23 |
| With iron, `*.example.com` also allows `example.com` itself (embedded: subdomains only) | J | iron-proxy's glob matches the apex too; the apex of a domain the user already trusts | the same |

## Path to secrets (decision 0012)

1. **This record:** iron-proxy as the default exit, one proxy with two engines, the tmpfs, the tests.
2. **TLS termination and placeholders:** a per-sandbox CA created by `burrow-proxy` (key in the
   tmpfs only; the certificate handed to the sandbox and its tools), `tls.mode: mitm`, and a
   `burrow-proxy load` command that reads bindings from stdin (`podman exec -i`, as in
   SECRETS.md flow 1), writes iron-proxy's `secrets` transform (`replace.proxy_value`, the
   binding's hosts) and reloads through iron-proxy's management API on loopback.
3. **Open questions for that step** (to test, not yet verified): iron-proxy's TLS mode is global,
   so "terminate only for hosts with a binding" (SECRETS.md requirement 2) needs either MITM for
   every host or an upstream per-host passthrough; iron-proxy does not block a placeholder sent to
   a host outside its binding (requirement 3) without a transform; redaction of echoed values in
   responses is not built in.

## Results

Linux, Podman 4.9.3 (rootful, netavark), amd64, 2026-11-23:

- Unit tests: rendered configuration (listeners, modes, deny list against the embedded engine's
  rule, input that tries to inject JSON or globs), `burrow-proxy` flags and engines, the exact
  podman command for each engine and for `none` (sandboxes and tools), the `PROXY` key,
  `--proxy` and `--accept-risk`.
- `make e2e`: the existing checks now run through iron (the default); the new checks pass:
  X1 to X5 (iron is the engine, SNI not allowed, SOCKS5 to another host, metrics not reachable,
  audit log), M1 to M7 (embedded engine), O1 to O5 (`none` refused without `--accept-risk`,
  started with it, no proxy, direct route, a tool next to it keeps its own proxy).
- By hand: `CONNECT pypi.org:22` with TLS reached pypi.org on 443 (no port pivot); plain HTTP to
  pypi.org:22 was dialled (the exception above).

**Not run yet:** rootless Podman (CI's e2e job), macOS and Windows (Podman machine), arm64. The
other e2e checks that failed in this environment failed for reasons unrelated to the proxy
(rootful Podman and workspace permissions; the forwarder image could not be built behind a TLS-
intercepting network).

## Gaps

- The two exceptions above.
- No method or path rules, no secrets, no TLS inspection yet (step 2).
- Allowed hosts remain exfiltration paths, as with `burrow-proxy`.
- Single-vendor upstream with one main maintainer (exception above); the embedded engine is the
  fallback.
- iron-proxy is built from its Go module with newer dependencies than upstream released, so the
  binary is ours, not upstream's: its behaviour was checked with upstream's unit tests and our
  e2e, not upstream's integration tests. Drop the overrides when an upstream release carries the
  fixes (Dependabot watches `packaging/proxy/iron`).
- Nothing is signed yet (D7). How it could be verified: the module and its dependencies are
  pinned in `go.sum` and checked against the Go checksum database at build time.

## Why the others were discarded

- **B':** the review asked for one proxy with unified settings and one handler.
- **1A:** leaves the riskiest part (iron-proxy on Podman, inside burrow's layout) untested until the
  secrets design is complete.
- **1C:** brings the sidecar's code and dependencies into burrow before the organisation accepts it.
- **1D:** costs every user a CA in every trust store now, for a benefit that only arrives with secrets.
- **1E:** DR-0009's fallback, kept as such.
- **2B, 2C, 2D:** each puts the configuration (and later the secrets) somewhere the user, the agent
  or the disk can read, or cannot carry the allowlist.
