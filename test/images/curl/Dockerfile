# Test image for the presets in test/presets (built by `make test-images` as localhost/burrow-test-NAME:dev).
# Only a pinned base and a non-root user, so Dependabot (docker ecosystem) keeps it current.
FROM docker.io/curlimages/curl:8.23.0@sha256:3fb7e13e5d179a845c97d78204f3f1064d1907037f26a3ebf91710571151fd93
# Non-root by default, also without burrow (burrow sets --user from the preset anyway).
USER 65532:65532
