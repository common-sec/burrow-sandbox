#!/bin/sh
# Security scans, local first: the same containers run here and in CI.
#   sh scripts/scan.sh all|secrets|sast|deps|images|sandboxes [IMAGE...]
# Every scanner runs from an image pinned by digest, as a non-root user, with no capabilities, a
# read-only root filesystem and the repository mounted read-only. The secrets scan has no network.
# ENGINE=podman (default) or docker.
# To be replaced by burrow itself once released: each scanner becomes a preset (`burrow exec`).
set -eu
cd "$(dirname "$0")/.."
engine=${ENGINE:-podman}
command -v "$engine" >/dev/null 2>&1 || engine=docker
repo=$(pwd)
mkdir -p .cache/trivy
# Images come from packaging/scanners/Dockerfile (pinned by tag and digest; Dependabot keeps them
# current): the FROM line of the stage with that name.
img() { awk -v s="$1" '$1=="FROM" && $3=="AS" && $4==s {print $2; exit}' packaging/scanners/Dockerfile; }
TRUFFLEHOG=$(img trufflehog); SEMGREP=$(img semgrep); TRIVY=$(img trivy)
case "$TRUFFLEHOG$SEMGREP$TRIVY" in *@sha256:*@sha256:*@sha256:*) ;; *) echo "scan: unpinned image in packaging/scanners/Dockerfile" >&2; exit 2 ;; esac

harden="--rm --read-only --cap-drop ALL --security-opt no-new-privileges --tmpfs /tmp:rw,size=512m -e HOME=/tmp"
# Podman: keep the host user's id, so the read-only mount is readable without relabelling.
userflag="--user 1000:1000"
[ "$engine" = podman ] && [ "$(podman info --format '{{.Host.Security.Rootless}}' 2>/dev/null)" = true ] && userflag="--userns keep-id"

secrets() {
  echo "== secrets (trufflehog: git history and working tree, no network)"
  # shellcheck disable=SC2086
  "$engine" run $harden $userflag --network none -v "$repo:/work:ro" \
    -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' \
    --entrypoint sh "$TRUFFLEHOG" -c \
    'trufflehog git file:///work --no-update --no-verification --fail &&
     trufflehog filesystem /work --no-update --no-verification --fail --exclude-paths /work/.trufflehog-exclude'
}
sast() {
  echo "== SAST (semgrep; downloads its rule packs)"
  # shellcheck disable=SC2086
  "$engine" run $harden $userflag -w /work -v "$repo:/work:ro" -e SEMGREP_SEND_METRICS=off "$SEMGREP" \
    semgrep scan --metrics=off --error --config p/golang --config p/gosec --config p/secrets \
    --config p/dockerfile --config p/github-actions /work
}
deps() {
  echo "== dependencies, misconfigurations, secrets (trivy; downloads its database)"
  # shellcheck disable=SC2086
  "$engine" run $harden $userflag -v "$repo:/work:ro" -v "$repo/.cache/trivy:/cache" "$TRIVY" \
    fs --cache-dir /cache --no-progress --skip-version-check --disable-telemetry --exit-code 1 \
    --severity HIGH,CRITICAL --scanners vuln,misconfig,secret \
    --skip-dirs /work/.cache --skip-dirs /work/dist --skip-dirs /work/crates/burrow-forward/target /work
}
# scan_image IMAGE VULN_EXIT: secrets always fail the scan; vulnerabilities fail it when VULN_EXIT=1.
scan_image() {
  img=$1
  tar=.cache/images/$(echo "$img" | tr '/:' '__').tar
  rm -f "$tar"
  "$engine" save -o "$tar" "$img"
  # Trivy copies large files (agent binaries of 200 MB and more) to its temp folder: use the
  # on-disk cache, not the small in-memory /tmp.
  rm -rf .cache/trivy/tmp && mkdir -p .cache/trivy/tmp
  # shellcheck disable=SC2086
  "$engine" run $harden $userflag -v "$repo:/work:ro" -v "$repo/.cache/trivy:/cache" -e TMPDIR=/cache/tmp "$TRIVY" \
    image --cache-dir /cache --no-progress --skip-version-check --disable-telemetry --exit-code 1 \
    --scanners secret --input "/work/$tar"
  # shellcheck disable=SC2086
  "$engine" run $harden $userflag -v "$repo:/work:ro" -v "$repo/.cache/trivy:/cache" -e TMPDIR=/cache/tmp "$TRIVY" \
    image --cache-dir /cache --no-progress --skip-version-check --disable-telemetry --exit-code "$2" \
    --severity HIGH,CRITICAL --scanners vuln --ignore-unfixed --input "/work/$tar"
  rm -rf "$tar" .cache/trivy/tmp
}
# The sandbox images of the test presets (scripts/images.sh sandboxes). Secrets fail the scan;
# vulnerabilities are reported but do not fail it: full Debian/Ubuntu/Kali bases always carry
# some, and they are fixed by updating the pinned base (Dependabot), not here (decision 0009).
sandboxes() {
  echo "== sandbox images (trivy: secrets fail; vulnerabilities reported)"
  mkdir -p .cache/images
  [ $# -gt 0 ] || set -- alpine curl trivy ubuntu kali claude codex opencode pi hermes
  for n in "$@"; do scan_image "localhost/burrow-test-$n:dev" 0; done
}
images() {
  echo "== images (trivy: vulnerabilities and secrets in every image we build)"
  mkdir -p .cache/images
  for img in localhost/burrow-proxy:dev localhost/burrow-iron-proxy:dev localhost/burrow-forward:dev; do scan_image "$img" 1; done
}
what=${1:-all}; [ $# -gt 0 ] && shift
case $what in
  all) secrets; sast; deps; images ;;
  secrets|sast|deps|images) "$what" ;;
  sandboxes) sandboxes "$@" ;;
  *) echo "usage: $0 all|secrets|sast|deps|images|sandboxes [NAME...]" >&2; exit 2 ;;
esac
