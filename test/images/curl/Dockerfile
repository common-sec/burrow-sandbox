# Test image for the presets in test/presets (built by `make test-images` as localhost/burrow-test-NAME:dev).
# Only a pinned base and a non-root user, so Dependabot (docker ecosystem) keeps it current.
FROM docker.io/curlimages/curl:8.16.0@sha256:463eaf6072688fe96ac64fa623fe73e1dbe25d8ad6c34404a669ad3ce1f104b6
# Non-root by default, also without burrow (burrow sets --user from the preset anyway).
USER 65532:65532
