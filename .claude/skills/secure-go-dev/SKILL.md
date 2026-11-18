---
name: secure-go-dev
description: Rules for writing or reviewing Go code in burrow. Use before changing any .go file, the podman arguments burrow builds, the proxy, the forwarder, or the definition parser.
---

# Secure Go development in burrow

burrow starts containers and decides what they can reach. A bug here is a sandbox escape or a
data leak, so these rules are not style preferences.

## Before you write

1. Find the requirement the change serves (docs/ROADMAP.md) and the threat it touches
   (docs/ARCHITECTURE.md, threat model). If the change touches a security property, also follow
   the `ssdlc` skill.
2. Prefer the standard library. A new module needs a decision record (docs/decisions/): licence,
   maintainers, release age (at least 7 days), size of its dependency tree.

## Rules

**Running podman and other programs**
- Only through `internal/podman.Runner`, with an argument list. Never `sh -c`, never a command
  built by string concatenation, never user input in the binary name.
- Every value that ends up in a podman argument is validated first (config package). Reject what
  could change the argument's meaning: commas, colons and quotes in paths for `-v`, leading `-`
  in names, newlines anywhere.
- Secure defaults on every container: `--cap-drop ALL`, `--security-opt no-new-privileges`,
  non-root `--user`, `--pids-limit`; `--read-only` unless the container must write its root
  filesystem. Never `--privileged`, never `--network host`, never a Podman or Docker socket mount.
  A change that relaxes one of these needs an exception (owner, reason, expiry) in a decision.

**Network code (proxy, forwarder)**
- Deny by default. Allowlists match whole labels (`*.example.com` must not match `badexample.com`).
- Resolve names yourself and check every address before dialling (no private, loopback,
  link-local, CGNAT or IP literals), then dial the checked address, never the name again.
- Timeouts on every dial, read header and shutdown; contexts on every network call (`noctx`).
- Log decisions as structured JSON. Never log secrets, tokens or request bodies.

**Files and input**
- Definitions are data: no expansion, no includes, unknown keys are errors.
- Create state files with 0600 and folders with 0700. Use `filepath`, not string joins, and
  remember Windows paths (`C:\...`): burrow runs on Windows too.
- Errors: wrap with context (`fmt.Errorf("...: %w", err)`); never ignore one silently. A `_ =`
  needs a reason (closing on an error path is fine).

**Concurrency**
- Every goroutine has a way to end (context or closed connection). Close both sides of a relay.

## Tests

- Table-driven unit tests for every validation rule, including the attack: the input that tries
  to get around it.
- Sandbox behaviour: assert the exact podman arguments with `podman.Recorder`, and add an
  end-to-end check in `scripts/e2e.sh` for anything a container could exploit.
- A "blocked" check passes only when the command ran and failed (exit 1 to 124), never when it
  could not start (R12).
- Never skip, disable or loosen a test to get green.

## Before you finish

Run `mise run fmt` and `mise run check` (lint with gosec, tests, govulncheck, build). Run
`mise run e2e` if sandbox behaviour changed and Podman is available; say so if it is not. Report
results as they are.
