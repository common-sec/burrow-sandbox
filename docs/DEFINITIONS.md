# Environment and preset definitions

One `KEY=VALUE` per line, `#` comments, optional quotes around the whole value. No expansion, no
substitution: a definition is data. Unknown keys and duplicates are errors.

## Environment (`burrow up FILE`)

| Key | Default | Meaning |
|---|---|---|
| `NAME` | required | 2 to 32 of `a-z`, `0-9`, `-` |
| `IMAGE` | required | pinned by digest (`registry/repo[:tag]@sha256:...`) or a `localhost/` image |
| `WORKSPACE` | required unless `WORKSPACE_MODE=none` | host folder mounted at `/work` (Windows: `C:\path`) |
| `WORKSPACE_MODE` | `rw` | `none`, `ro`, `rw` |
| `ALLOW` | none | hosts the proxy lets through (port 443): `pypi.org`, or `*.github.com` for subdomains |
| `ENGINE` | `crun` | `crun`, or `krun` (experimental microVM; Linux with KVM) |
| `RUN_AS` | `1000:1000` | uid:gid inside; the host user is mapped to it; uid 0 refused |
| `PROXY_IMAGE` | `localhost/burrow-proxy:dev` | the proxy image |
| `FORWARD_IMAGE` | `localhost/burrow-forward:dev` | the port forwarder image |
| `MEMORY` | none | e.g. `4g` |

## Preset (`burrow run NAME FILE [-- ARGS]`)

| Key | Default | Meaning |
|---|---|---|
| `NAME`, `IMAGE`, `ENGINE`, `MEMORY` | | as above |
| `ARGS` | none | arguments to the image's entrypoint; `-- ARGS` on the command line are appended |
| `WORKSPACE` | `ro` | the environment's workspace: `none`, `ro`, `rw` (never wider than the environment's) |
| `NETWORK` | `proxy` | `proxy` (own internal network and proxy with `ALLOW`) or `none` |
| `ALLOW` | none | the preset's own allowlist (needs `NETWORK=proxy`) |
| `RUN_AS` | `65532:65532` | the preset's own user |
| `CACHE` | none | `volume:/path:mode`, mode `rw`, `ro` or `O` (overlay: writes discarded); volume `burrow-cache-<volume>` |
