---
name: ssdlc
description: Secure development lifecycle for burrow. Use when planning a feature, changing a security property or default, adding or updating a dependency or tool, changing CI or the release process, or handling a vulnerability report.
---

# Secure development lifecycle (SSDLC) for burrow

Each step leaves a trace in the repository, so an auditor can follow why the code is the way it is.

## 1. Requirements

- Link the work to a requirement (R*, SB*, W*, E* in docs/ROADMAP.md) and, where it applies, to the
  organisation's controls (SCF IDs such as AAT-39.11, NET-04, TDA-15; credit SCF as in
  secure-standard-platforms' NOTICE.md).
- Write the security property as a sentence that can be tested: "a preset with NETWORK=none
  cannot open a TCP connection".

## 2. Design and threat model

- Update the threat model table in docs/ARCHITECTURE.md: asset, threat, control, gap. Think about
  the agent as the attacker (it runs code inside the sandbox), a compromised tool in a preset, a
  malicious definition file, and a compromised dependency of burrow itself.
- A new choice of standard, tool, format or design gets a decision record (docs/decisions/) with
  the options discarded and why. Changes to defaults that weaken security need an exception with
  owner, reason and expiry.
- Prefer integrating an existing, maintained project over building (O4); prefer what Podman can
  already do (decision 0002).

## 3. Dependencies and tools (supply chain)

- Pin everything to an immutable identifier: Go modules (go.sum), images (digest, tag kept for
  readers), Actions (commit SHA), tools (mise.lock with checksums), mise itself (SHA-256 in
  scripts/install-mise.sh).
- Only versions at least **7 days** old (mise `minimum_release_age`, Dependabot `cooldown`).
  Security fixes may skip the cooldown, recorded in the commit.
- Check the licence (no AGPL, BSL, NC, ND surprises), the maintainers (single-maintainer risk) and
  the release date from primary sources. Mark anything you could not verify.
- No third-party GitHub Actions beyond `actions/checkout`; scanners run as pinned containers.

## 4. Implementation

Follow the `secure-go-dev` skill.

## 5. Verification (local first)

- `mise run check`: go vet, golangci-lint (gosec, staticcheck, errcheck, noctx...), unit tests,
  govulncheck, build for every platform.
- `mise run scan`: TruffleHog (secrets in the tree and the whole git history, no network),
  Semgrep (SAST), Trivy (dependencies, misconfigurations, secrets).
- `mise run e2e`: the security properties on real rootless Podman, including attempts to break
  them. A new property gets a new check.
- CI runs exactly these tasks; a red CI is fixed, never bypassed.

## 6. Review and merge

- Pull requests describe the property changed, the tests that prove it, and the gaps left.
- Generated files (mise.lock, go.sum) are reviewed like code: a changed checksum without a
  version change is a red flag.

## 7. Release (when releases start)

Reproducible builds (`-trimpath`, `-buildid=`, no cgo), `SHA256SUMS`, an SBOM, and signatures once
the organisation's signing decision (D7) exists. Until then, each release records how it could be
verified.

## 8. Vulnerabilities

Reports come through SECURITY.md (private advisories). Acknowledge, reproduce with a test, fix,
release, credit. Known gaps stay documented in SECURITY.md and docs/ARCHITECTURE.md until closed.
