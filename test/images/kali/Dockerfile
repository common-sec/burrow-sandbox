# Sandbox image for Kali Linux (test preset test/presets/kali.preset), built by `make test-images`
# as localhost/burrow-test-kali:dev. Only a pinned base: tools are added in your own Dockerfile
# FROM this one (the sandbox user cannot install packages: no root, no capabilities).
# Cooldown exception (decision 0009): kali-rolling has no dated tags and is rebuilt every week,
# so no digest older than 7 days can be named. Pinned by digest on 2026-11-22; updated by hand.
FROM docker.io/kalilinux/kali-rolling:latest@sha256:69a9a33d98a584a537bd0f7144d20c3b6d23f3bafa9ffcf0cadf4c5f186d02ff
WORKDIR /work
USER 1000:1000
