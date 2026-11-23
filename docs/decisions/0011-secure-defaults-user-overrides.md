# 0011: Secure by default, tunable by the user, with warnings; root on request

**Status:** proposed (2026-11-22; root from presets and sandbox files added 2026-11-23). **Owner:** J.
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

## First case: root on request

| Option | For | Against |
|---|---|---|
| A. Never root | simplest; nothing in the sandbox can change its image's files | installing a package or debugging in an existing sandbox needs a new image |
| B. Root only in `shell`, behind an extra flag | smallest exposure | two flags for one intent; does not fit `run`/`exec` cases (tools that must run as root) |
| C. Root only from `--user 0:0`, never from `RUN_AS` | the person running burrow types it; a file cannot ask for it | a program that always needs root (some build and system tools) needs the flag on every run, and its preset cannot say so |
| **D. Root from `--user`, a preset or a sandbox file, with a warning every time** | fits the principle above: the defaults stay non-root, a preset can say what its program needs, and the warning names where root came from | a shared or downloaded preset can ask for root; only the warning shows it until a verification step exists |

**Decision: D** (it replaces C, chosen first on 2026-11-22). Some programs and workflows need root,
and a preset is where a program's needs are written; refusing it there pushes the flag into every
command, or people out of burrow. `RUN_AS=0:0` is accepted from a preset, a sandbox file and
`--user`; every `run`, `up`, `exec` and `shell` that runs as root prints
`burrow: warning: <source> 0:0 runs as root inside ...`, with the source named (`--user`, the
preset and its path, or the sandbox file). Root keeps `--cap-drop ALL` and `no-new-privileges`,
the same network and mounts; with rootless Podman, root inside is the host user, not host root.
The rules on where definitions come from do not change: presets are never read from the
workspace, and a sandbox file inside the workspace is mounted read-only inside the sandbox, so the
agent cannot give its next sandbox root (U4, U17).

C's `USER_FLAG` key (where the user came from) is no longer needed; stored sandboxes that have it
still load, and it is ignored.

## Exception (O3)

| What | Owner | Reason | Expiry |
|---|---|---|---|
| A sandbox, tool or shell may run as root (uid 0) inside, from `--user`, a preset or a sandbox file | J | some programs and workflows need root, and users need to install packages and debug in existing sandboxes; refusing pushes them to `podman exec --user root`, which bypasses burrow entirely | review when the preset index exists (packages then come from built images) or a verification step for root presets is decided, or 2027-05-22 |

## Gaps

- The warning is printed once per command; nothing records it yet (audit log: roadmap).
- `burrow shell` into a sandbox that already runs as root warns only when `--user` is given; the
  warning was printed at `up` or `run`.
- A preset from someone else can ask for root and only the warning shows it. A verification step
  for such presets on a developer's machine, and default settings for CI where nobody reads the
  warning, are being evaluated separately.
- Only the user setting follows the principle so far. Other settings (wider `CAP_ADD`, `NETWORK`
  overrides) will be decided one by one, each with its warning.

## Why the others were discarded

- **A:** the user went around burrow (`podman exec --user root`), which is worse than a warned,
  capability-free root inside burrow.
- **B:** a second flag for the same intent, and it leaves `run` and `exec` without the option.
- **C:** a program that needs root could not say so in its preset, so every run needed the flag;
  it protected against a preset asking for root, which the warning now makes visible instead.
