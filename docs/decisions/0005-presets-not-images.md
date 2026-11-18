# 0005: burrow runs presets, not images

**Status:** proposed (2026-11-18). **Asked by:** J (review of PR #1): "burrow runs presets, not
images", "up and run should run a persistent sandbox", "exec should run a preset", "allow the
preset to inherit configs like fs mounts (rw/ro/inherit)... maybe proxy settings".

## Context

The first prototype copied Docker's verbs: `up ENV_FILE` started a sandbox from an environment
file, `exec NAME` ran a command inside it, `run NAME PRESET_FILE` started a tool. An image was the
unit, and every command needed a file path. J wants the unit to be the **preset**: a pinned image
plus the settings it is safe to run with, shipped with burrow or written by the user, so people
pick "trivy-scan", not an image and twelve flags (O7).

## Options

| Option | For | Against |
|---|---|---|
| **A. Presets as the unit; a sandbox file like compose; tools inherit from the sandbox, capped by it** | one concept for sandboxes and tools; built-ins make the secure path the easy one (O7); inheritance removes copy-paste of mounts and allowlists | a new file kind (sandbox file); inheritance rules must be clear and tested |
| B. Keep Docker's verbs (`run IMAGE`, `exec CONTAINER`) | familiar | every run repeats the security settings by hand; images are not policies; "run an image" is exactly what we want to make harder |
| C. Only compose-like files, no presets | one file explains everything | ad-hoc use (`burrow run alpine`) needs a file; tools cannot be shared between projects |

## Decision

**A.**

| Command | Does |
|---|---|
| `burrow up [-f FILE]` | starts the persistent sandbox described by a **sandbox file** (`./burrow.sandbox` by default). The file names a preset (`PRESET=`) and overrides its keys, like compose. |
| `burrow run PRESET [--name] [--image] [--workspace]` | starts an **ad-hoc persistent sandbox** from a preset, on the current folder. |
| `burrow exec [-s SANDBOX] PRESET [-- ARGS]` | runs a preset as a **one-off tool** next to the sandbox (from `-s`, or from `./burrow.sandbox`). |
| `burrow shell [NAME] [-- CMD]` | runs a command inside the sandbox (what `exec` did before). |

- **Where the image comes from**, first match wins: `--image`, the sandbox file's `IMAGE`, the
  preset's `IMAGE`. Every image is still pinned by digest or `localhost/`.
- **Where presets come from:** a path (contains `/` or ends in `.preset`), then the user's
  presets folder (`burrow presets` prints it), then the presets built into the binary
  (`presets/*.preset`, embedded). **Never from the workspace**: the agent can write there, and a
  preset found there would run with whatever rights it asks for.
- **Inheritance (exec):** `WORKSPACE_MODE=inherit` takes the sandbox's mode; `NETWORK=inherit`
  gives the tool its **own** proxy with the sandbox's allowlist (it never joins the sandbox's
  network, so neither can reach the other). Explicit `ro`/`rw` stay possible, but a tool never gets
  more than the sandbox: `rw` needs a read-write sandbox. Without a sandbox, `inherit` falls back to
  the narrowest setting (read-only, no network). Tool defaults are the narrowest too:
  `WORKSPACE_MODE=ro`, `NETWORK=none`.
- **The sandbox file is protected from the agent:** when it is inside a read-write workspace, it
  is mounted read-only over itself, so the agent cannot widen `ALLOW` or mounts for the next `up`
  (checked end to end: write and rename both fail).
- One key set for both files: the preset key `WORKSPACE` became `WORKSPACE_MODE`, the same name
  as in the sandbox file, where `WORKSPACE` is the folder.

## Why the others were discarded

- **B** keeps the security settings in the user's head; that is the insecure-is-easier trap O7
  warns about.
- **C** makes one-off use heavier than `podman run`, so people would bypass burrow.

## Consequences and gaps

- `shell` is a new verb for the old "command inside the sandbox"; J confirmed the name on PR #1.
- Only workspace and proxy are inherited for now. Caches, memory and engine are not; J named
  "fs mounts" and "proxy settings" as the first two.
- A sandbox's `NETWORK` must be `proxy` (the default): a sandbox always has its own proxy, and an
  empty `ALLOW` blocks everything. `none` for sandboxes is not implemented.
- The format stays KEY=VALUE ([0003](0003-definition-format.md)), provisional.
- Built-in presets are pinned by digest in `presets/`; Dependabot cannot update them (*gap*:
  updated by hand, reviewed by the security team through CODEOWNERS).
- A preset given by path is trusted as the user's choice: a path into the workspace runs what the
  agent may have written. Documented in ARCHITECTURE.md; a warning when a preset path is inside a
  running sandbox's workspace is a possible next step.
