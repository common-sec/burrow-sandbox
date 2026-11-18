# Presets and sandbox files

burrow runs **presets**, not images ([decision 0005](decisions/0005-presets-not-images.md)). Both
file kinds use one `KEY=VALUE` per line, `#` comments, optional quotes around the whole value. No
expansion, no substitution: a definition is data. Unknown keys and duplicates are errors.

## Preset (`*.preset`)

A pinned image and the settings it runs with. Used two ways:

- `burrow run PRESET`: a persistent sandbox (keys marked *tool only* are refused).
- `burrow exec [-s SANDBOX] PRESET [-- ARGS]`: a one-off tool next to a sandbox.

| Key | Default | Meaning |
|---|---|---|
| `NAME` | required | 2 to 32 of `a-z`, `0-9`, `-` |
| `IMAGE` | required | pinned by digest (`registry/repo[:tag]@sha256:...`) or a `localhost/` image |
| `WORKSPACE_MODE` | `ro` (tool), `rw` (sandbox) | `none`, `ro`, `rw`, or `inherit` (tool only: the sandbox's mode). A tool never gets more than its sandbox. |
| `NETWORK` | `none` (tool), `proxy` (sandbox) | `none`; `proxy` (own internal network and proxy with `ALLOW`); `inherit` (tool only: own proxy with the sandbox's `ALLOW`). A sandbox is always `proxy`. |
| `ALLOW` | none | hosts the proxy lets through (port 443): `pypi.org`, or `*.github.com` for subdomains. Needs `NETWORK=proxy`. |
| `ENGINE` | `crun` | `crun`, or `krun` (experimental microVM; Linux with KVM) |
| `RUN_AS` | `65532:65532` (tool), `1000:1000` (sandbox) | uid:gid inside; the host user is mapped to it when the workspace is mounted; uid 0 refused |
| `MEMORY` | none | e.g. `4g` |
| `ARGS` | none | *tool only*: arguments to the image's entrypoint; `-- ARGS` on the command line are appended |
| `CACHE` | none | *tool only*: `volume:/path:mode`, mode `rw`, `ro` or `O` (overlay: writes discarded); volume `burrow-cache-<volume>` |

Without a sandbox (no `-s` and no `./burrow.sandbox`), a tool runs on its own: the workspace is
the current folder (or `--workspace DIR`), and `inherit` means `ro` and `none`.

**Where presets come from**, first match wins: a path (contains `/` or ends in `.preset`); the
user's presets folder (`burrow presets` prints it; `BURROW_PRESETS_DIR` overrides it); the presets
built into burrow ([presets/](../presets/)). Never the workspace, which the agent can write.

Built in today: `alpine` (sandbox), `trivy-db` and `trivy-scan` (tools).

## Sandbox file (`burrow.sandbox`)

Like a compose file: names a preset and sets local values. `burrow up` reads `./burrow.sandbox`
(or `-f FILE`); `shell`, `down` and `exec` use its `NAME` when none is given.

| Key | Default | Meaning |
|---|---|---|
| `NAME` | required | the sandbox's name |
| `PRESET` | none | a preset to start from; every key below that is set here wins over the preset's |
| `IMAGE` | the preset's | as above; `--image` on the command line wins over both |
| `WORKSPACE` | `.` | host folder mounted at `/work`, relative to the sandbox file (Windows: `C:\path`) |
| `WORKSPACE_MODE` | `rw` | `none`, `ro`, `rw` |
| `NETWORK` | `proxy` | only `proxy` for a sandbox |
| `ALLOW`, `ENGINE`, `RUN_AS`, `MEMORY` | | as for a preset |
| `PROXY_IMAGE` | `localhost/burrow-proxy:dev` | the proxy image |
| `FORWARD_IMAGE` | `localhost/burrow-forward:dev` | the port forwarder image |

When the sandbox file is inside a read-write workspace, burrow mounts it read-only over itself:
the agent cannot widen its own sandbox for the next `up`.

Example: [examples/burrow.sandbox](../examples/burrow.sandbox).
