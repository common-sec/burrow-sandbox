#!/bin/sh
# Builds the images burrow starts:  sh scripts/images.sh [proxy] [forward] [test] [sandboxes [NAME...]]
#   proxy, forward  localhost/burrow-proxy:dev, localhost/burrow-forward:dev (packaging/<name>)
#   test            the small test images the e2e needs (alpine, curl, trivy)
#   sandboxes       moved: the agent and distribution presets are built and published by
#                   burrow-repo (https://github.com/common-sec/burrow-repo)
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
      echo "images: the agent and distribution presets moved to https://github.com/common-sec/burrow-repo" >&2
      echo "images: build one there (make image NAME=claude) or download a published, pinned preset" >&2
      exit 2
      exit 0 ;;
    *) echo "usage: $0 [proxy] [forward] [test] [sandboxes [NAME...]]" >&2; exit 2 ;;
  esac
  shift
done
