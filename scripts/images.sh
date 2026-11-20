#!/bin/sh
# Builds the images burrow starts:  sh scripts/images.sh [proxy] [forward] [test]
#   proxy, forward  localhost/burrow-proxy:dev, localhost/burrow-forward:dev (packaging/<name>)
#   test            localhost/burrow-test-<name>:dev for each test/images/<name> (test presets)
# Uses podman (or $ENGINE). On Windows and macOS this builds inside the Podman machine.
# Published, prebuilt images replace local builds for users later (roadmap 0.4).
set -eu
cd "$(dirname "$0")/.."
engine=${ENGINE:-podman}
[ $# -gt 0 ] || set -- proxy forward
for what in "$@"; do
  case $what in
    proxy|forward)
      "$engine" build --pull=missing -f "packaging/$what/Dockerfile" -t "localhost/burrow-$what:dev" . ;;
    test)
      for d in test/images/*/; do
        n=$(basename "$d")
        "$engine" build --pull=missing -f "$d/Dockerfile" -t "localhost/burrow-test-$n:dev" "$d"
      done ;;
    *) echo "usage: $0 [proxy] [forward] [test]" >&2; exit 2 ;;
  esac
done
