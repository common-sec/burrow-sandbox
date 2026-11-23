# 0012: Workspace path, extra mounts, and flags that override them

**Status:** proposed (2026-11-23), implemented; the host-folder question was settled the same day (option 1).
**Owner:** J.
**Why:** a tool often needs more than "the workspace at `/work`". For example, it reads the code,
writes its build output to `/dist`, and builds in a large `/tmp`. Some images also expect their
code somewhere other than `/work`. Today burrow mounts only the workspace (always at `/work`), the
caches and the sandbox's home, so such tools either do not fit or get the whole workspace
read-write. Following [0011](0011-secure-defaults-user-overrides.md), the defaults stay as they
are, the definitions can say what a tool needs, and the person running burrow can override both
with flags.

Serves W2 (roadmap: extra volumes and mounts; inheritance), SB7 (least privilege for tools), O7
(usable), and the organisation's controls CFG-02 (secure baseline configurations), IAC-30 (least
privilege) and NET-04 (data flow enforcement). SCF is credited as in secure-standard-platforms'
NOTICE.md.

## What changes

1. **`WORKDIR`** (preset and sandbox file; `--workdir PATH` on `run`, `up`, `exec`). This is
   where the workspace is mounted inside the container and the working directory (`podman -w`).
   The default is `/work`, as today.
2. **`WORKSPACE_MODE`** stays as it is (`none`, `ro`, `rw`, and `inherit[:FALLBACK]` for tools). It
   gains a flag, `--workspace-mode MODE` on `run`, `up` and `exec`, so a user can force a tool or
   sandbox to `ro` without editing its definition.
3. **`MOUNT`** (preset and sandbox file; `--mount ENTRY` on `run`, `up` and `exec`, repeatable). It
   is a comma-separated list of typed entries. burrow builds each podman flag from these entries,
   so no podman option string is ever passed through as written:

   | Entry | Example | podman flag | Notes |
   |---|---|---|---|
   | `tmpfs:TARGET[:SIZE]` | `tmpfs:/tmp:2g` | `--tmpfs TARGET:rw,exec,nosuid,nodev,size=SIZE` | memory-backed; gone when the container stops; `exec` because builds run what they compile there |
   | `volume:NAME:TARGET:MODE` | `volume:dist:/dist:rw` | `-v burrow-vol-<sandbox>_NAME:TARGET:MODE` | a named volume, scoped to the sandbox like caches; `--purge` removes it |
   | `work:SUBDIR:TARGET:MODE` | `work:dist:/dist:rw` | `-v <workspace>/SUBDIR:TARGET:MODE` | a folder inside the workspace, relative, no `..` and no symlink out of it; this lets a read-only workspace have one writable output folder |
   | `host:PATH:TARGET:MODE` | `host:C:\out:/out:rw` | `-v PATH:TARGET:MODE` | a host folder outside the workspace, from the sandbox file or `--mount` only (below); always printed as a warning |

   `MODE` is `ro` or `rw` (and `O` for volumes, as for caches). The precedence is the one burrow
   already uses: preset, then sandbox file, then flags. Entries are merged by `TARGET`, and the
   later source wins.
4. **Validation**, the same way as every other value (`internal/config`):
   - `TARGET` must be absolute and clean, with no commas, colons, quotes or newlines.
   - `TARGET` cannot be `/`, `/proc`, `/sys`, `/dev`, `/run` or `/etc` (or a path under them),
     the home volume, or a target already used.
   - `NAME` follows the cache name rules.
   - `SUBDIR` must resolve inside the workspace. `PATH` is absolute, follows `CheckPath`, must
     exist, and must be a directory, never a socket or a device.
   - `PATH` cannot be `/`, the user's home folder itself, `/run`, `/var/run` or
     `$XDG_RUNTIME_DIR`. It also cannot be a parent of any of them, because those hold the Podman
     socket, which 0011 keeps impossible.
   - Strict mode caps a tool's mounts by its sandbox. A `work:` mount can be no wider than the
     sandbox's workspace mode, and a tool can have no `host:` mount the sandbox does not have.
   - burrow never reads definitions from the workspace implicitly, and the sandbox file inside
     it stays read-only inside the sandbox.

## Options

| Option | For | Against |
|---|---|---|
| **A. `WORKDIR` and one `MOUNT` key with typed entries; flags override** | one list, like `CACHE`; every part is validated; `work:` covers "read the code, write `/dist`" without any host path | a small mount syntax of our own to document |
| B. Pass `--mount` / `-v` strings to podman as written | nothing to design | podman options (propagation, relabel, `bind-nonrecursive`, `U`, device or socket paths) would pass through unchecked, against AGENTS.md's "validate every value" |
| C. One key per kind (`TMPFS`, `VOLUMES`, `BINDS`, `WORK_MOUNTS`) | each key is simpler | four keys and four merge rules for one idea; precedence by target is harder to see |
| D. Move definitions to YAML or compose files | nesting, familiar to compose users | reopens [0003](0003-definition-format.md) (KEY=VALUE until 0.4) and adds a dependency |

**Decision: A** (proposed).

## Who may mount a host folder outside the workspace

| Option | For | Against |
|---|---|---|
| **1. Sandbox file and flags only (recommended)** | the user chooses host folders; a preset, which may be shared or downloaded later, can never choose a folder on the user's machine; `tmpfs:`, `volume:` and `work:` cover the tool cases seen so far | a tool that must write outside the workspace needs a line in the sandbox file or a flag |
| 2. Anywhere, including presets, with a warning (as root in 0011) | most flexible | a shared preset could mount `~/.ssh` or a cloud credentials folder, and only a warning shows it; much worse than root, which stays inside the container |
| 3. Flags only | the least exposure | a sandbox file cannot record a mount the project always needs |

**Decision: option 1.** Host folders are the user's to choose. Presets get `tmpfs:`, `volume:`
and `work:`, which cover a tool that reads the code, writes its output and builds in `/tmp`.

- **2** was discarded: a shared preset could mount a credentials folder, which a warning does not
  undo.
- **3** was discarded: a project that always needs a mount could not record it in its sandbox
  file.

## Consequences and gaps

- Every `host:` mount prints `burrow: warning: host folder PATH mounted at TARGET (MODE) from
  SOURCE`. `--dry-run` shows the flags.
- **Deny-list limits.** The checks on `PATH` stop the known disasters (the socket, the root folder,
  the whole home folder). They do not know which other folders hold secrets (`~/.ssh`,
  `~/.aws`). The warning and option 1 cover that, not a list.
- **Windows and macOS:** a `host:` path must be inside a folder shared with the Podman machine
  (WINDOWS.md); burrow cannot check that from the client. `work:` mounts have the same lock
  problem on machine shares as the workspace.
- **SELinux:** no `:z`/`:Z` relabelling yet (roadmap), the same as the workspace.
- **TOCTOU:** host paths are resolved (symlinks included) just before podman runs, and the
  resolved path is mounted. Someone who can write the parent folder could still swap it in
  between.
- **Tests:**
  - Table tests for every entry kind, with a good value, a bad value and the clever value
    (`work:../..:/x:rw`, `host:/run/user/1000:/x:ro`, `tmpfs:/tmp,exec:1g`,
    `volume:a:/proc:rw`).
  - Recorder tests that compare the exact podman flags, including precedence and strict mode.
  - An e2e check: a tool with a read-only workspace writes only to `work:dist`, and a write
    elsewhere in the workspace **runs and fails**.

## Why the others were discarded

- **B:** fails the rule that every podman argument is validated; a passed-through string can carry
  options that weaken the sandbox.
- **C:** works, but splits one concept into four keys with their own precedence; A gives the same
  power with one list.
- **D:** a format change belongs to the 0.4 review of 0003, not to this feature.
