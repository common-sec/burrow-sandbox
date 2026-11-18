#!/bin/sh
# Builds the proxy and forwarder images that `burrow` starts (localhost/burrow-*:dev).
# Uses podman (or $ENGINE). On Windows and macOS this builds inside the Podman machine.
set -eu
cd "$(dirname "$0")/.."
engine=${ENGINE:-podman}
for p in proxy forward; do
  "$engine" build --pull=missing -f "packaging/$p/Dockerfile" -t "localhost/burrow-$p:dev" .
done
