#!/bin/sh
# Builds the burrow binaries inside a container and copies them to dist/: a machine needs only
# Podman (or $ENGINE), not Go or mise. Works with a Podman machine (Windows, macOS): the source is
# sent as the build context, so it never goes through the shared folder (no file locking there).
# The same three podman commands work from PowerShell: see docs/DEVELOPMENT.md.
set -eu
cd "$(dirname "$0")/.."
engine=${ENGINE:-podman}
version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
img=localhost/burrow-dist:dev
"$engine" build --pull=missing --build-arg "VERSION=$version" -f packaging/burrow/Dockerfile -t "$img" .
ctr=$("$engine" create "$img" /none)
trap '"$engine" rm -f "$ctr" >/dev/null' EXIT
rm -rf dist
"$engine" cp "$ctr:/dist" ./dist
cat dist/SHA256SUMS
