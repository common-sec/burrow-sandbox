# Test image for the presets in test/presets (built by `make test-images` as localhost/burrow-test-NAME:dev).
# Only a pinned base and a non-root user, so Dependabot (docker ecosystem) keeps it current.
FROM docker.io/library/alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
# Non-root by default, also without burrow (burrow sets --user from the preset anyway).
USER 65532:65532
