#!/bin/sh
# Builds the images burrow starts:  sh scripts/images.sh [proxy] [forward] [test] [sandboxes [NAME...]]
#   proxy, forward  localhost/burrow-proxy:dev, localhost/burrow-forward:dev (packaging/<name>)
#   test            the small test images the e2e needs (alpine, curl, trivy)
#   sandboxes       the sandbox images of the test presets (all, or the NAMEs given): ubuntu,
#                   kali and the agents (claude, codex, opencode, pi, hermes); 1 to 2 GB each
# Each is localhost/burrow-test-<name>:dev from test/images/<name>. Uses podman (or $ENGINE); on
# Windows and macOS this builds inside the Podman machine, from the build context (no shared
# folder). Published, prebuilt images replace local builds for users later (roadmap 0.4).
set -eu
cd "$(dirname "$0")/.."
engine=${ENGINE:-podman}
build_test() { "$engine" build --pull=missing -t "localhost/burrow-test-$1:dev" "test/images/$1"; }
[ $# -gt 0 ] || set -- proxy forward
while [ $# -gt 0 ]; do
  case $1 in
    proxy|forward)
      "$engine" build --pull=missing -f "packaging/$1/Dockerfile" -t "localhost/burrow-$1:dev" . ;;
    test)
      for n in alpine curl trivy; do build_test "$n"; done ;;
    sandboxes)
      shift
      [ $# -gt 0 ] || set -- ubuntu kali claude codex opencode pi hermes
      for n in "$@"; do build_test "$n"; done
      exit 0 ;;
    *) echo "usage: $0 [proxy] [forward] [test] [sandboxes [NAME...]]" >&2; exit 2 ;;
  esac
  shift
done
