# Scanner images for `make scan` (scripts/scan.sh), pinned by tag (for readers and Dependabot) and
# multi-platform digest (what runs). Not built: scripts/scan.sh reads each image from its FROM line
# (by stage name) and runs it with its own hardening: non-root, no capabilities, read-only root
# filesystem, repository mounted read-only. Dependabot (docker ecosystem) keeps them current.
# To be replaced by burrow itself once released (scanners as presets).
FROM docker.io/trufflesecurity/trufflehog:3.99.9@sha256:2ec535840f860aa27f11fee20222fb790326ecd66325c96bd49d02ec6af46eca AS trufflehog
FROM docker.io/semgrep/semgrep:1.185.0@sha256:67518c174185a2043fdbed16c70631b79209499b3671178e7194379b48295a0d AS semgrep
FROM docker.io/aquasec/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb AS trivy
USER 65532:65532
