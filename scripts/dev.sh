#!/bin/sh
# The developer checks in pinned containers (packaging/dev/Dockerfile): a machine needs only
# Podman (or $ENGINE), no Go, golangci-lint or mise (decision 0010).
#   sh scripts/dev.sh vet|test|vuln|lint|fmt
# vet, test, vuln and lint build one stage of packaging/dev/Dockerfile: the source is the build
# context, so they also work from a Podman machine's shared folder (no file locking there).
# fmt must write the files back, so it is the one step that mounts the repository (read-write,
# no network, no capabilities, as your own user).
# DEV_BUILD_FLAGS adds flags to `podman build` (for example a CA for a network that intercepts TLS).
set -eu
cd "$(dirname "$0")/.."
engine=${ENGINE:-podman}
what=${1:?usage: $0 vet|test|vuln|lint|fmt}
case $what in
  vet|test|lint)
    # shellcheck disable=SC2086
    "$engine" build --pull=missing ${DEV_BUILD_FLAGS:-} --target "$what" -f packaging/dev/Dockerfile . ;;
  vuln)
    # shellcheck disable=SC2086
    "$engine" build --pull=missing ${DEV_BUILD_FLAGS:-} --build-arg "VULN_DATE=$(date -u +%Y-%m-%d)" \
      --target vuln -f packaging/dev/Dockerfile . ;;
  fmt)
    lint=$(awk '/^FROM .*golangci-lint/{print $2; exit}' packaging/dev/Dockerfile)
    case $lint in *@sha256:*) ;; *) echo "dev: unpinned golangci-lint image" >&2; exit 2 ;; esac
    userflag="--user $(id -u):$(id -g)"
    [ "$engine" = podman ] && [ "$(podman info --format '{{.Host.Security.Rootless}}' 2>/dev/null)" = true ] && userflag="--userns keep-id"
    # shellcheck disable=SC2086
    "$engine" run --rm --pull=missing --network none --read-only --cap-drop ALL \
      --security-opt no-new-privileges --tmpfs /tmp:rw,size=512m -e HOME=/tmp \
      -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/gomod -e GOLANGCI_LINT_CACHE=/tmp/golangci \
      -e GOTOOLCHAIN=local -e GOPROXY=off -e GOFLAGS=-mod=readonly $userflag \
      -v "$PWD:/src" -w /src "$lint" golangci-lint fmt ;;
  *) echo "usage: $0 vet|test|vuln|lint|fmt" >&2; exit 2 ;;
esac
