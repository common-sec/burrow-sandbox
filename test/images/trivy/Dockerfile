# Test image for the presets in test/presets (built by `make test-images` as localhost/burrow-test-NAME:dev).
# Only a pinned base and a non-root user, so Dependabot (docker ecosystem) keeps it current.
FROM docker.io/aquasec/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb
# Non-root by default, also without burrow (burrow sets --user from the preset anyway).
USER 65532:65532
