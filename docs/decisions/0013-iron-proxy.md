# 0013: iron-proxy as an opt-in proxy (`PROXY=iron`): first integration

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

## Facts checked (2026-11-23, primary sources)

| What | Value | Source |
|---|---|---|
| Licence | Apache-2.0 | `LICENSE` in the repository at tag `v0.56.0` |
| Version | 0.56.0, tagged 2026-11-04: the newest past the 7-day cooldown (0.57.0 to 0.59.0 are from 2026-11-17 to 2026-11-19). It includes a fix that requires a patched Go toolchain (upstream #288) | `git log` of the tags |
| Image | `docker.io/ironsh/iron-proxy:0.56.0@sha256:727da0cc35a6d73ed2913a38d56c4b9b4476c075149492e4fbf75552e83361ff` (multi-arch index; the binary is static, the base is Alpine) | `podman pull`, `podman image inspect`; 0.53.0's digest matched the spike's |
| Maintainers | 13 authors in 199 commits since 2026-04-01; 78 of the commits since May by one maintainer | `git shortlog` at `v0.56.0` |
| Behaviour used here | `tls.mode: sni-only` (no termination; the pipeline runs on the CONNECT target **and** on the ClientHello SNI; the upstream port is always 443), `dns.enabled: false`, `upstream_deny_cidrs` checked at connect time against the resolved address (IPv4-mapped addresses unmapped), allowlist `domains` with `*.domain` globs | source at `v0.56.0`: `internal/proxy/tunnel.go`, `sni_passthrough.go`, `internal/dnsguard`, `internal/hostmatch`; and the runs below |

## Decisions

### 1. How burrow uses iron-proxy

| Option | For | Against |
|---|---|---|
| A. Keep only `burrow-proxy` until secrets are designed in full | nothing new to review | iron-proxy is never tried on Podman; the secrets work starts from zero |
| **B. iron-proxy behind a small launcher in its own image, opt-in with `PROXY=iron`, sni-only** | same layout and flags as today; no CA in the sandbox; tested now on Podman; the launcher is where secrets are loaded later | two known gaps against burrow-proxy (exceptions below); a second proxy image to keep pinned and scanned |
| C. The spike's full stack: iron-proxy + `egress-policy` sidecar (gRPC transforms) | deny rules, DoH, port and path checks the spike tested | a second container per proxy, gRPC and YAML dependencies in burrow, and `egress-policy` is not an accepted in-house tool yet (org DR-0006) |
| D. iron-proxy in MITM mode now | method and path rules, the base for secrets | every sandbox and tool must trust a per-sandbox CA (each runtime has its own trust store: Node, Python, Go, Java), for no feature that uses it yet |
| E. Envoy (DR-0009's fallback) | mature, foundation-backed | far more configuration for the same allowlist; no placeholder substitution |

**Decision: B.** `PROXY=iron` (sandbox file) or `--proxy iron` (`up`, `run`, and `exec` without a
sandbox) starts `localhost/burrow-iron-proxy:dev` instead of `burrow-proxy`. A tool next to a
sandbox gets the same kind of proxy as its sandbox (its own instance, as today). Presets cannot set
`PROXY`: the proxy is the user's choice for their environment (decision 0011). The default stays
`burrow` until iron-proxy has passed on the platforms in the roadmap (0.2) and the exceptions below
are closed.

### 2. How the configuration reaches iron-proxy

| Option | For | Against |
|---|---|---|
| **A. A launcher (`cmd/burrow-iron-proxy`, Go standard library) takes burrow-proxy's flags, renders the configuration into a tmpfs (`/run/burrow`, mode 0700, owned by the proxy's user) and `exec`s iron-proxy** | burrow starts both proxies with the same arguments; the configuration lives in memory only; the launcher is the place for `load` (secrets over stdin) and reloads later; the rendered file is unit-tested | one more binary in the image |
| B. A file from burrow's state folder, bind-mounted read-only | no launcher | with rootless Podman the proxy's uid 65532 cannot read the user's 0600 file without widening it; secrets would later sit on disk |
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
| only port 443 | sni-only mode always dials the SNI host on 443, whatever the CONNECT says; the launcher refuses `-ports` other than 443 |
| no private, loopback, link-local or CGNAT addresses after resolution | `upstream_deny_cidrs`: every non-public range (IPv4 and IPv6, including metadata and documentation ranges); a unit test checks the list covers everything `burrow-proxy` refuses |
| a JSON audit line per decision | iron-proxy's audit log (`"action"`, `"rejected_by"`, host, SNI, client address) |
| the SNI must match | iron-proxy runs the allowlist on the SNI too, so `CONNECT allowed:443` with another SNI is refused |

The container gets the same defaults as every proxy (read-only, uid 65532, no capabilities, no new
privileges, `--http-proxy=false`) plus the tmpfs and `--memory 128m --pids-limit 128`.

## Exceptions (O3, AGENTS.md: a weaker setting needs owner, reason, expiry)

The default does not change. With `PROXY=iron`, burrow prints a warning, because:

| What | Owner | Reason | Expiry |
|---|---|---|---|
| Plain HTTP to an allowed host is forwarded, on any port (`http://allowed:22/` is dialled; `burrow-proxy` refuses everything but CONNECT). Only allowed hosts are reachable, and only as HTTP | J | iron-proxy has no per-scheme or per-port rule; closing it needs a transform (the spike's policy sidecar), an upstream change, or a small gate in the launcher | when TLS termination lands (step 2 below), or 2027-02-23 |
| `*.example.com` also allows `example.com` itself (`burrow-proxy`: subdomains only) | J | iron-proxy's glob matches the apex too; the apex of a domain the user already trusts | the same |

## Path to secrets (decision 0012)

1. **This record:** iron-proxy as the exit, the launcher, the tmpfs, the tests.
2. **TLS termination and placeholders:** a per-sandbox CA created by the launcher (key in the
   tmpfs only; the certificate handed to the sandbox and its tools), `tls.mode: mitm`, and a
   `burrow-iron-proxy load` command that reads bindings from stdin (`podman exec -i`, as in
   SECRETS.md flow 1), writes iron-proxy's `secrets` transform (`replace.proxy_value`, the
   binding's hosts) and reloads through iron-proxy's management API on loopback.
3. **Open questions for that step** (to test, not yet verified): iron-proxy's TLS mode is global,
   so "terminate only for hosts with a binding" (SECRETS.md requirement 2) needs either MITM for
   every host or an upstream per-host passthrough; iron-proxy does not block a placeholder sent to
   a host outside its binding (requirement 3) without a transform; redaction of echoed values in
   responses is not built in.

## Results

Linux, Podman 4.9.3 (rootful, netavark), amd64, 2026-11-23:

- Unit tests: rendered configuration (listeners, modes, deny list against `burrow-proxy`'s rule,
  input that tries to inject JSON or globs), launcher flags, the exact podman command for
  sandboxes and tools with `PROXY=iron`, the `PROXY` key and `--proxy` flags.
- `make e2e`: all 14 new checks (X1 to X14) pass: allowed host, other host, IP literal, SNI not
  allowed, SOCKS5 to another host, direct IP without the proxy, metrics not reachable from the
  sandbox, a tool's own iron-proxy next to the sandbox and without one, the audit log.
- By hand: `CONNECT pypi.org:22` with TLS reached pypi.org on 443 (no port pivot); plain HTTP to
  pypi.org:22 was dialled (the exception above).

**Not run yet:** rootless Podman, macOS and Windows (Podman machine), arm64, a hosted CI runner.
The other e2e checks that failed in this environment failed for reasons unrelated to the proxy
(rootful Podman and workspace permissions; the forwarder image could not be built behind a TLS-
intercepting network); they need a run on J's machine.

## Gaps

- The two exceptions above.
- No method or path rules, no secrets, no TLS inspection yet (step 2).
- Allowed hosts remain exfiltration paths, as with `burrow-proxy`.
- Single-vendor upstream with one main maintainer (DR-0009); `burrow-proxy` stays as the default and
  the fallback.
- The image copies iron-proxy's release binary; it is not rebuilt from source here, and its
  signatures are not verified (D7: no signing solution yet). How it could be verified: upstream
  publishes a signing key (`public-key.asc`, `sign-release.sh`) for its release archives.

## Why the others were discarded

- **1A:** leaves the riskiest part (iron-proxy on Podman, inside burrow's layout) untested until the
  secrets design is complete.
- **1C:** brings the sidecar's code and dependencies into burrow before the organisation accepts it.
- **1D:** costs every user a CA in every trust store now, for a benefit that only arrives with secrets.
- **1E:** DR-0009's fallback, kept as such.
- **2B, 2C, 2D:** each puts the configuration (and later the secrets) somewhere the user, the agent
  or the disk can read, or cannot carry the allowlist.
