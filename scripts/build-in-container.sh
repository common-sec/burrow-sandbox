#!/bin/sh
# Builds the burrow binaries inside a container into dist/: a machine needs only Podman (or
# $ENGINE), not Go. By default it works the way burrow runs a tool: scripts/build.sh in the pinned
# Go image, with the current folder mounted at /src, no capabilities, no new privileges, a
# read-only root and the caches in /tmp. Network is on (Go modules, checked against go.sum).
#   sh scripts/build-in-container.sh          # mount the current folder (like burrow exec)
#   BUILD_MODE=context sh scripts/build-in-container.sh
#       send the source as the build context instead: for a Podman machine's shared folder
#       that has no file locking (Go locks go.mod), see docs/WINDOWS.md "Known issues".
set -eu
cd "$(dirname "$0")/.."
engine=${ENGINE:-podman}
version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
case "$version" in *[!A-Za-z0-9._-]*) echo "build: unexpected version $version" >&2; exit 2 ;; esac
go_image=$(sed -n 's/^ARG GO_IMAGE=//p' packaging/burrow/Dockerfile)
case $go_image in *@sha256:*) ;; *) echo "build: unpinned Go image in packaging/burrow/Dockerfile" >&2; exit 2 ;; esac

if [ "${BUILD_MODE:-mount}" = context ]; then
  img=localhost/burrow-dist:dev
  "$engine" build --pull=missing --build-arg "VERSION=$version" -f packaging/burrow/Dockerfile -t "$img" .
  ctr=$("$engine" create "$img" /none)
  trap '"$engine" rm -f "$ctr" >/dev/null' EXIT
  rm -rf dist
  "$engine" cp "$ctr:/dist" ./dist
  cat dist/SHA256SUMS
  exit 0
fi

# Own the files it writes: the host user (rootless: keep-id), or the caller's uid:gid.
userflag="--user $(id -u):$(id -g)"
[ "$engine" = podman ] && [ "$(podman info --format '{{.Host.Security.Rootless}}' 2>/dev/null)" = true ] && userflag="--userns keep-id"
# shellcheck disable=SC2086
"$engine" run --rm --pull=missing --read-only --cap-drop ALL --security-opt no-new-privileges \
  --http-proxy=false --pids-limit 512 --tmpfs /tmp:rw,exec,size=2g $userflag \
  -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/gomod -e GOPATH=/tmp/go \
  -e CGO_ENABLED=0 -e GOFLAGS="-trimpath -mod=readonly" -e GOTOOLCHAIN=local \
  -e BURROW_VERSION="$version" ${BUILD_RUN_FLAGS:-} \
  -v "$PWD:/src" -w /src "$go_image" sh scripts/build.sh
