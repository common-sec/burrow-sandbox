# 0011: Secure by default, tunable by the user, with warnings; root via --user

**Status:** proposed (2026-11-22). **Owner:** J.
**Why:** burrow must be safe without any configuration, but it runs in many environments nobody
can foresee (other distributions, corporate networks, tools that need more). A tool that cannot
be adjusted gets replaced by one that is not secure at all (O7). So the defaults are strict, and
the person running burrow can loosen them for their case, knowingly.

Serves O3 (secure defaults; deviations are exceptions), O6 (gaps documented), O7 (usable), and the
organisation's controls CFG-02 (secure baseline configurations), CFG-04 (software use) and
IAC-30 (least privilege). SCF is credited as in secure-standard-platforms' NOTICE.md.

## The principle

1. **Defaults are secure.** No capabilities, no new privileges, a non-root user, no network but
   the proxy, nothing allowed, pinned images. A user who changes nothing gets these.
2. **Every setting that weakens a default is visible.** burrow prints a warning when it runs with
   one (`burrow: warning: ...`), and `--dry-run` shows the exact podman flags.
3. **The user can tune and make exceptions.** Settings that weaken a default are accepted when
   they come from the person running burrow: a command-line flag, their presets folder, a sandbox
   file they wrote. They are never taken from a place the agent can write without the user
   seeing it (the workspace is mounted, the sandbox file read-only inside; decision 0005).
4. **Some things stay impossible**, because no scenario needs them and they would end the
   sandbox: `--privileged`, `--network host`, the Podman or Docker socket, private addresses
   through the proxy.
5. **For burrow's own code** (AGENTS.md) nothing changes: a change to a *default* needs an
   exception with owner, reason and expiry in a decision record.

## First case: root with `--user`

| Option | For | Against |
|---|---|---|
| A. Never root | simplest; nothing in the sandbox can change its image's files | installing a package or debugging in an existing sandbox needs a new image |
| B. Root only in `shell`, behind an extra flag | smallest exposure | two flags for one intent; does not fit `run`/`exec` cases (tools that must run as root) |
| **C. `--user 0:0` on `run`, `up`, `exec`, `shell`, with a warning** | one way, everywhere; fits the principle above | root inside can change everything in the container, and files it writes in the workspace belong to the host user |

**Decision: C.** Root is accepted only from the `--user` flag, never from `RUN_AS` in a preset or
sandbox file (a file can be shared or come from a preset index later; a flag is typed by the
person who runs it). A stored sandbox remembers that its user came from the flag (`USER_FLAG`),
and a stored file that says root without it is refused. Root keeps `--cap-drop ALL` and
`no-new-privileges`, the same network and mounts; with rootless Podman, root inside is the host
user, not host root.

## Exception (O3)

| What | Owner | Reason | Expiry |
|---|---|---|---|
| `--user` may run as root (uid 0) inside the sandbox | J | users need to install packages and debug in existing sandboxes; refusing pushes them to `podman exec --user root`, which bypasses burrow entirely | review when the preset index exists (packages then come from built images), or 2027-05-22 |

## Gaps

- The warning is printed once per command; nothing records it yet (audit log: roadmap).
- Only `--user` follows the principle so far. Other settings (wider `CAP_ADD`, `NETWORK`
  overrides) will be decided one by one, each with its warning.
- Whether `RUN_AS=0:0` in the user's own presets folder should be allowed (with a warning) is
  open: the folder is the user's, but presets will also come from an index.

## Why the others were discarded

- **A:** the user went around burrow (`podman exec --user root`), which is worse than a warned,
  capability-free root inside burrow.
- **B:** a second flag for the same intent, and it leaves `run` and `exec` without the option.
