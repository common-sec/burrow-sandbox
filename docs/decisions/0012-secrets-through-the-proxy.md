# 0012: Secrets through the proxy: placeholders in the sandbox, values fetched from the user's store

**Status:** proposed (2026-11-23). **Owner:** J.
**Design, diagrams and flows:** [docs/SECRETS.md](../SECRETS.md).

## Context

Agents and tools need credentials (a model API key, a GitHub token, a registry login). Today
burrow has none: `ENV` is for plain settings only, and the roadmap says secrets come "only through
the vault/proxy system, never as variables" (0.1 and 0.3). Two requirements from J:

1. The real secret is put into requests **at the proxy**; the sandbox only sees an alias.
2. burrow **does not store secrets**. It fetches them from a store that already exists, with a
   simple first version and a path to more stores later. Reuse what exists where we can.

Serves R7 (credential and network proxy) and D4 (credentials short-lived, scoped, never at rest in
plaintext), O4 (integrate before you build), O7 (usable). Controls from the organisation's matrix:
No Embedded Unencrypted Static Authenticators (SCF IAC-14.3), Credential Sharing (IAC-23), Least
Privilege (IAC-30), Data Flow Enforcement (NET-04), AI Agent Privileged Role & Delegation
Boundaries (AAT-39.6), Sandboxing AI Agents (AAT-39.11), AI Agent Logging (AAT-40.1), and it uses
Password Managers (IAC-15.2) as the store. SCF is credited as in secure-standard-platforms'
NOTICE.md.

## Prior art (checked 2026-11-23)

| Project | What it does | Licence | What we take |
|---|---|---|---|
| [Anthropic sandbox-runtime](https://github.com/anthropic-experimental/sandbox-runtime) (`credentials` with `mode: "mask"`) | puts a sentinel in place of an environment variable or file inside the sandbox; the proxy substitutes the real value back on egress, only to the entry's `injectHosts` (default: the allowed domains); masking requires TLS termination | Apache-2.0 (LICENSE in repo) | the core model: **placeholder in, substitution at the proxy, per-secret host list, only over verified TLS** |
| [Docker Sandboxes](https://github.com/docker/docs/blob/main/content/manuals/ai/sandboxes/configuration/credentials.md) (`sbx secret`) | host-side proxy overwrites the auth header; the sandbox sees a sentinel (`proxy-managed`); values in the OS keychain, or a dynamic source: `--ref op://...` / AWS ARN, or `--command 'gh auth token'`, with `--refresh`; "providing a value and approving its use are separate steps" (credential bindings) | product; docs only used | **references and commands instead of stored values**; refresh; separating the value from the approval |
| [NVIDIA OpenShell](https://github.com/NVIDIA/OpenShell) providers | provider profiles in YAML: credential name, env var, placement (`auth_style: bearer`, `header_name`), endpoints with access level, allowed binaries; agents never see real credentials | Apache-2.0 (LICENSE in repo) | **the preset declares where a credential may go; values kept apart from the profile**; endpoint access levels for later |
| [Fly.io tokenizer](https://github.com/superfly/tokenizer) | Go proxy; the client sends a secret sealed to the proxy's public key, the proxy decrypts and injects it; processors: inject (header or query, with format), HMAC, SigV4 re-signing, JWT exchange whose access token is sealed again; `allowed_hosts` | Apache-2.0 (LICENSE in repo) | the **processor list** for v2 (query, HMAC, SigV4, JWT/OAuth); sealing is a v3 option for a shared proxy. Not usable as is: it needs the client to send a header and speak plain HTTP to the proxy, which agents do not do |
| [Envoy credential injector](https://github.com/envoyproxy/envoy/blob/main/docs/root/configuration/http/http_filters/credential_injector_filter.rst) | HTTP filter that injects a Basic or Bearer `Authorization` header from a generic secret (SDS) or OAuth2 client credentials, for workload identity | Apache-2.0 | a candidate for the 0.3 proxy's injection step; no placeholder substitution, so the sandbox would need no token at all (fine for some tools, not for clients that require one) |
| [fnox](https://github.com/jdx/fnox) | one CLI over ~30 stores (age, OS keychain, pass, 1Password, Bitwarden, Vault, AWS/GCP/Azure, Infisical...), short-lived "leases", `fnox get KEY` prints one value | MIT (LICENSE in repo) | **a ready multi-store resolver** we call through `cmd:`, so burrow needs no store code of its own |
| [Podman `shell` secret driver](https://github.com/containers/podman/blob/main/docs/source/markdown/podman-secret-create.1.md) | user scripts `list`/`lookup`/`store`/`delete`, `SECRET_ID` in the environment, values on stdin/stdout | Apache-2.0 | the **plugin shape** (with Docker and git credential helpers): an executable, a tiny stdin/stdout protocol |

Versions and maintainer counts were not checked for this record (GitHub's API is not reachable
from this session); they are checked when a component is adopted, as AGENTS.md requires.

## Decisions

### 1. The alias: placeholder substitution at the proxy

| Option | For | Against |
|---|---|---|
| **A. Random placeholder per secret and sandbox; the proxy substitutes it in request headers, only for the secret's hosts** (sandbox-runtime, Docker custom secrets) | works with every client that reads a token from an environment variable or file, unchanged; a placeholder seen going anywhere else is a clear, blockable leak signal; useless outside this proxy | needs TLS termination for those hosts (the 0.3 proxy); breaks signed requests (SigV4, HMAC) until re-signing processors exist |
| B. Fixed sentinel (`proxy-managed`) and header injection by rule (Docker built-ins, Envoy, OpenShell) | no placeholder to match; injection does not depend on the client | needs a rule per service (header name, scheme); a fixed sentinel cannot tell secrets apart or flag a leak; a client that validates the token's format fails |
| C. Real value in the sandbox, egress limited by the allowlist | nothing to build | the agent holds the credential: any allowed host, a log, a cache or a later run leaks it. Fails R7 and D4 |
| D. Client-sealed secrets (tokenizer) | the proxy needs no table; good for a shared proxy | the sandbox must add a header and speak plain HTTP to the proxy; agents and tools do not |

**Decision: A**, with B as a later addition for tools that take no token (v2). Placeholders are
`PREFIX` + `burrow-` + 32 random base62 characters, so clients that check a prefix (`sk-ant-`,
`ghp_`) still accept them. The proxy also looks inside `Authorization: Basic` (git over HTTPS,
registries). Query strings only when the secret opts in. Hosts without a secret are not
terminated.

### 2. Where values come from: the user's store, resolved by burrow on the host

| Option | For | Against |
|---|---|---|
| **A. burrow resolves on the host at start, from references in the user's own files; built-in `env:` and `cmd:`** | reaches whatever the user already has (password manager CLI, keychain, `gh`, CI variables) on every platform, including Windows and macOS where Podman runs in a VM that cannot see the host's keychain; no store code and no Go dependency in burrow | values are fetched once per start; a `cmd:` reference runs a program on the host, so where references may come from must be strict |
| B. burrow stores secrets (its own encrypted file or vault) | full control | J ruled it out; one more place to protect, back up and rotate (D4) |
| C. Podman secrets (`--secret`) for the proxy | Podman-native | the `file` driver keeps the value as a file in Podman's storage (inside the Podman machine on Windows and macOS: at rest, D4); the `shell`/`pass` drivers run inside the VM there, so they cannot reach the host's store |
| D. The proxy fetches values itself | values never pass through burrow; refresh without the CLI | the proxy needs its own identity to the store; that suits CI and a shared proxy (G5), not a laptop with a password manager unlocked by the user |
| E. fnox as burrow's only backend | ~30 stores and leases in one tool | a Rust binary on the host, one main maintainer; it reads `fnox.toml` from the project folder, which the agent can write. Better as one `cmd:` target than as the core |

**Decision: A for v1, D for v3.** Rules:

- References and the hosts a secret may reach live only in the user's secrets folder
  (`~/.config/burrow/secrets/NAME.secret`), never in a preset, a sandbox file or the workspace. A
  preset only says `SECRETS=NAME:VAR`. So a preset cannot run a command on the host or send a
  secret further than the user allowed (decision 0011: loosening only from the user).
- `cmd:` takes an argument list split on spaces: no shell, no expansion, no quoting. It runs with
  a minimal environment, a timeout, and the user's config folder as working directory (not the
  workspace, so a project's `fnox.toml` or `.envrc` cannot redirect it). An empty output is an
  error; stderr is not logged.
- Recommended first store: the organisation's password manager (Bitwarden, IAC-15.2) through
  `cmd:bw ...` or `cmd:bws ...`; 1Password, pass, the macOS and Linux keychains, `gh auth token`
  and fnox the same way; `env:` for CI. Examples in [SECRETS.md](../SECRETS.md).

### 3. How values reach the proxy

| Option | For | Against |
|---|---|---|
| **A. `podman exec -i PROXY burrow-proxy load`, bindings as JSON on stdin, kept in memory** | never in arguments, environment, files or podman secrets; works through the Podman machine's remote connection; the same path refreshes and revokes | needs a small control channel inside the proxy container (a socket on a tmpfs); to be verified with podman-remote on Windows and macOS |
| B. Environment variables of the proxy container | simplest | visible in `podman inspect`, stored in the container's config on disk |
| C. A file or volume mounted into the proxy | simple | at rest on disk (in the VM on Windows and macOS) |
| D. Podman secret | Podman-native | see 2C |

**Decision: A.**

### 4. Pluggable stores later

| Option | For | Against |
|---|---|---|
| **A. A resolver interface in Go; built-ins `env` and `cmd`; later plugins `burrow-secret-SCHEME` (JSON on stdin/stdout, optional `expires_at`) from the user's plugins folder only** | the shape Docker credential helpers, git credential helpers and Podman's `shell` driver already use; Vault/OpenBao, cloud stores and token exchanges without Go dependencies in burrow; organisations can write their own | each plugin is code on the host to review (it is chosen by the user, like a preset) |
| B. Link store SDKs into burrow | one binary | many dependencies (cloud SDKs are large), against "standard library first" and DR-0006's static, small binaries |
| C. Only `cmd:`, forever | nothing more to build | no expiry metadata, so no automatic refresh of short-lived tokens |

**Decision: A.** `cmd:` already covers most stores in v1; plugins come when expiry and refresh are
needed (v2). Proxy-side fetch (2D) is v3, with the shared proxy and central policy (roadmap 0.3).

## Consequences

- Building this depends on the 0.3 proxy choice (an organisation decision in
  secure-standard-platforms): it must meet the requirements listed in
  [SECRETS.md](../SECRETS.md#what-the-proxy-must-do-requirements-for-the-03-proxy-choice). The
  temporary `burrow-proxy` only tunnels and is not extended for this.
- New definition keys: `SECRETS` in presets and sandbox files; a new file kind, `*.secret`, in the
  user's config folder. New commands: `burrow secrets` (list names and status, never values),
  `burrow secrets refresh`, `burrow secrets revoke`.
- Under strict mode a tool may only use secrets its sandbox also uses.
- The threat model in ARCHITECTURE.md gains the rows in SECRETS.md when this is built; user stories
  and tests (placeholder never equals the value; a placeholder to a host outside `HOSTS` is blocked
  and the command ran; `cmd:` from a preset is refused; no value in `--dry-run`, `podman inspect`
  or logs) come with the code.

## Gaps

- HTTP(S) only: no SSH keys, raw TCP database passwords, or signed requests in v1.
- No automatic refresh in v1: a long-running sandbox keeps the value from its start until
  `burrow secrets refresh`.
- OAuth logins done inside the sandbox (agent CLIs) still put a real token there.
- An allowed host that reflects headers to a third party leaks the value: `HOSTS` must name API
  hosts only. Response redaction is best effort for bodies.
- Windows Credential Manager: no stock command that prints a stored password is known to us
  (unverified); Windows users go through Bitwarden, 1Password or fnox.
- Whether fnox can be pointed at a config outside the project folder by flag is unverified.
- Versions, release dates and maintainers of the projects above are not yet checked.

## Why the others were discarded

- **1B** is kept for tools that take no token, but cannot be the base: it needs a rule per service
  and cannot detect a leak. **1C** fails R7 and D4. **1D** needs the client's cooperation.
- **2B** is ruled out by J and by D4. **2C** puts values at rest or cannot reach the host's store
  from the Podman machine. **2D** is right for CI and a shared proxy, so it is v3, not discarded.
  **2E** trusts a config file in the workspace and makes one tool the core.
- **3B to 3D** put the value where `podman inspect` or the disk can show it.
- **4B** grows burrow's dependency tree for every store; **4C** cannot refresh short-lived tokens.
