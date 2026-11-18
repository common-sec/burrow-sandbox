# Security policy

## Reporting a vulnerability

Please **don't open a public issue** for security problems. Report them privately through
GitHub's "Report a vulnerability" (Security → Advisories) on this repository. Include steps to
reproduce and the impact you expect. You'll get an acknowledgement within a few days, and credit
in the release notes if you'd like.

## Scope

burrow is a **prototype**. In scope already:
- a way for code in a sandbox or preset to reach a host or address its allowlist does not allow;
- a way for a sandbox to start containers, reach the Podman socket, gain capabilities or become
  root on the host;
- a preset getting more access than its definition (wider mounts, the sandbox's network);
- injection through definition files or names into podman arguments.

Known gaps, out of scope until the roadmap closes them: the proxy stand-in does not inspect TLS
(an allowed host is allowed for any path); container escapes through kernel bugs with the crun
engine (use krun, experimental); on Windows, the WSL backend shares a kernel and a network with
other WSL distros (exception E1, see [docs/WINDOWS.md](docs/WINDOWS.md)).

## For auditors

Start with [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) (trust boundaries and threat model), then
`internal/sandbox/sandbox.go` (every podman flag burrow sets) and `internal/proxy/proxy.go`.
`scripts/e2e.sh` shows the intended properties as executable checks.
