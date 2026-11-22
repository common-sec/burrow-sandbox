# Scanner images for `make scan` (scripts/scan.sh), pinned by tag (for readers and Dependabot) and
# multi-platform digest (what runs). Not built: scripts/scan.sh reads each image from its FROM line
# (by stage name) and runs it with its own hardening: non-root, no capabilities, read-only root
# filesystem, repository mounted read-only. Dependabot (docker ecosystem) keeps them current.
# To be replaced by burrow itself once released (scanners as presets).
FROM docker.io/trufflesecurity/trufflehog:3.99.8@sha256:674b2f187620a19df77f44cf9a7d77efcc581a5662accef2f2739cbb46ba4a3a AS trufflehog
FROM docker.io/semgrep/semgrep:1.184.0@sha256:cc1b68266f267878b39739373ea9985b9a78e5d64497dd875c8de100b22695ed AS semgrep
FROM docker.io/aquasec/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb AS trivy
USER 65532:65532
