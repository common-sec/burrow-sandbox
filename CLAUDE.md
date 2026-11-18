# Working in burrow

burrow is a security tool: it decides what AI agents and their tools can reach. Treat every
change as security-relevant.

- Before writing code, use the `secure-go-dev` skill; for anything that changes a security
  property, a dependency or the release process, also the `ssdlc` skill.
- Run `mise run check` before saying a change is done, and `mise run e2e` when sandbox behaviour
  changes (needs rootless Podman). Report failures as they are.
- Name the requirement the change serves (R1 to R12, SB1 to SB7, W1 to W7, E1: see
  docs/ROADMAP.md) in the commit message.
- New dependencies, formats or design choices need a record in `docs/decisions/` with the options
  discarded and why. Verify licences, versions and release dates from primary sources and say
  when something is unverified.
- Never weaken a default (capabilities, network, mounts, user, pinning, cooldown) to make a test
  pass. Never skip or disable a test.
