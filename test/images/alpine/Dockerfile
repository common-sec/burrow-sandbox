# Test image for the presets in test/presets (built by `make test-images` as localhost/burrow-test-NAME:dev).
# Only a pinned base and a non-root user, so Dependabot (docker ecosystem) keeps it current.
FROM docker.io/library/alpine:3.22@sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f
# Non-root by default, also without burrow (burrow sets --user from the preset anyway).
USER 65532:65532
