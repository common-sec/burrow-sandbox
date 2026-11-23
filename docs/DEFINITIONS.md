# Presets and sandbox files

burrow runs **presets**, not images ([decision 0005](decisions/0005-presets-not-images.md)). Both
file kinds use one `KEY=VALUE` per line, `#` comments, optional quotes around the whole value. No
expansion, no substitution: a definition is data. Unknown keys and duplicates are errors.

## Preset (`*.preset`)

A pinned image and the least-privilege settings its tool needs ([decision
0007](decisions/0007-preset-security-model.md)). Used two ways:

- `burrow run PRESET`: a persistent sandbox (keys marked *tool only* are refused).
- `burrow exec [-s SANDBOX] PRESET [-- ARGS]`: a one-off tool next to a sandbox.

| Key | Default | Meaning |
|---|---|---|
| `NAME` | required | 2 to 32 of `a-z`, `0-9`, `-` |
| `DESCRIPTION` | none | one line for `burrow presets list`, `search` and `info` |
| `IMAGE` | required | pinned by digest (`registry/repo[:tag]@sha256:...`) or a `localhost/` image |
| `WORKSPACE_MODE` | `ro` (tool), `rw` (sandbox) | `none`, `ro`, `rw`, or (tool only) `inherit` / `inherit:FALLBACK`: the sandbox's mode, or `FALLBACK` (`none`, `ro`, `rw`) when there is no sandbox; plain `inherit` falls back to `ro` |
| `NETWORK` | `none` (tool), `proxy` (sandbox) | `none`; `proxy` (own internal network and proxy with `ALLOW`); or (tool only) `inherit` / `inherit:FALLBACK`: own proxy with the sandbox's `ALLOW`, or `FALLBACK` (`none`, `proxy` with this preset's `ALLOW`) without a sandbox; plain `inherit` falls back to `none`. A sandbox is always `proxy`. |
| `ALLOW` | none | hosts the proxy lets through (port 443): `pypi.org`, or `*.github.com` for subdomains. Needs `NETWORK=proxy` or `inherit:proxy`. |
| `ENV` | none | plain settings, `KEY=VALUE,KEY=VALUE`. Not for secrets (those come through the proxy, roadmap 0.3); `*_PROXY` keys are refused (burrow sets them). |
| `ENGINE` | `crun` | `crun`, or `krun` (experimental microVM; Linux with KVM) |
| `RUN_AS` | `65532:65532` (tool), `1000:1000` (sandbox) | uid:gid inside; the host user is mapped to it when the workspace is mounted; uid 0 refused |
| `MEMORY` | none | e.g. `4g` |
| `ARGS` | none | *tool only*: arguments to the image's entrypoint; `-- ARGS` on the command line are appended |
| `CACHE` | none | *tool only*: one or more `name:/path:mode`, comma-separated, mode `rw`, `ro` or `O` (overlay: writes discarded). See "Caches" below. |
| `CAP_ADD` | none | *tool only*: capabilities to add back after `--cap-drop ALL`, comma-separated, only from Podman's default set plus `NET_RAW` (`CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `FSETID`, `KILL`, `NET_BIND_SERVICE`, `NET_RAW`, `SETFCAP`, `SETGID`, `SETPCAP`, `SETUID`, `SYS_CHROOT`). `no-new-privileges` stays on. Refused under strict mode. |

By default a tool gets what its preset asks for, even more than its sandbox has: the preset is
trusted to ask only for what its tool needs. With `burrow exec --strict`, or `STRICT=true` in the
sandbox file, a tool is capped by its sandbox: its workspace mode no wider, its allowlist covered
by the sandbox's, no `CAP_ADD`. A tool that asks for more is refused before it starts.

Without a sandbox (no `-s` and no `./burrow.sandbox`), a tool runs on its own: the workspace is
the current folder (or `--workspace DIR`), and `inherit` uses the preset's fallback.

### Caches

A cache is a named volume that keeps a tool's downloads (a vulnerability database, a module
cache) between runs. A preset can have several, each on its own path.

Caches are **scoped to the sandbox**: the volume is `burrow-cache-<sandbox>_<name>`, or
`burrow-cache-exec_<name>` for tools run without a sandbox. So presets next to the same sandbox
share a cache by using the same name (`trivy-db` writes `trivy`, `trivy-scan` reads it), and a
tool next to another sandbox never sees it: an agent that poisons the cache of its own sandbox
does not reach any other. Within one sandbox, a preset that must not be affected by what others
wrote can mount the cache `ro` or `O`, or use a name of its own. Per-preset scoping inside a
sandbox is not done yet (*gap*). `burrow down NAME --purge` removes the sandbox's caches.

### Volumes and secrets

Not yet: today a container gets only the workspace (at `/work`), its caches, and, for a sandbox,
its home volume. Extra volumes and mounts, and secrets (through the vault/proxy system, never as
variables), are on the [roadmap](ROADMAP.md).

**Where presets come from**, first match wins: a path (contains `/` or ends in `.preset`); the
user's presets folder (`burrow presets` prints it; `BURROW_PRESETS_DIR` overrides it). Never the
workspace, which the agent can write. `burrow presets list`, `presets search TERM` and
`presets info PRESET` show what is there. There are no presets built into burrow; prebuilt,
published presets with an index are planned (roadmap 0.4). Test presets: [test/presets](../test/presets/):
sandboxes `alpine`, `curl`, `ubuntu`, `kali`, and with an agent `claude`, `codex`, `opencode`,
`pi`, `hermes` (images: `make sandbox-images`); tools `trivy`, `trivy-db`, `trivy-scan`.

## Sandbox file (`burrow.sandbox`)

Like a compose file: names a preset and sets local values. `burrow up` reads `./burrow.sandbox`
(or `-f FILE`); `shell`, `down` and `exec` use its `NAME` when none is given.

| Key | Default | Meaning |
|---|---|---|
| `NAME` | required | the sandbox's name (`exec` is reserved) |
| `PRESET` | none | a preset to start from; every key below that is set here wins over the preset's |
| `IMAGE` | the preset's | as above; `--image` on the command line wins over both |
| `WORKSPACE` | `.` | host folder mounted at `/work`, relative to the sandbox file (Windows: `C:\path`) |
| `WORKSPACE_MODE` | `rw` | `none`, `ro`, `rw` |
| `NETWORK` | `proxy` | only `proxy` for a sandbox |
| `ALLOW`, `ENGINE`, `RUN_AS`, `MEMORY`, `ENV` | | as for a preset |
| `STRICT` | `false` | `true`: every tool run next to this sandbox is capped by it (as `exec --strict`) |
| `PROXY_IMAGE` | `localhost/burrow-proxy:dev` | the proxy image |
| `FORWARD_IMAGE` | `localhost/burrow-forward:dev` | the port forwarder image |

When the sandbox file is inside a read-write workspace, burrow mounts it read-only over itself:
the agent cannot widen its own sandbox for the next `up`.

Example: [examples/burrow.sandbox](../examples/burrow.sandbox).
