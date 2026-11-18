#!/bin/sh
# Security scans, local first: the same containers run here and in CI.
#   sh scripts/scan.sh all|secrets|sast|deps
# Every scanner runs from an image pinned by digest, as a non-root user, with no capabilities, a
# read-only root filesystem and the repository mounted read-only. The secrets scan has no network.
# ENGINE=podman (default) or docker.
set -eu
cd "$(dirname "$0")/.."
engine=${ENGINE:-podman}
command -v "$engine" >/dev/null 2>&1 || engine=docker
repo=$(pwd)
mkdir -p .cache/trivy
# Images come from compose.yaml (pinned by tag and digest; Dependabot keeps them current).
img() { awk -v svc="  $1:" '$0==svc{f=1;next} f&&/image:/{print $2; exit}' compose.yaml; }
TRUFFLEHOG=$(img trufflehog); SEMGREP=$(img semgrep); TRIVY=$(img trivy)
case "$TRUFFLEHOG$SEMGREP$TRIVY" in *@sha256:*@sha256:*@sha256:*) ;; *) echo "scan: unpinned image in compose.yaml" >&2; exit 2 ;; esac

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
    --skip-dirs /work/.cache --skip-dirs /work/dist /work
}
case "${1:-all}" in
  all) secrets; sast; deps ;;
  secrets|sast|deps) "$1" ;;
  *) echo "usage: $0 all|secrets|sast|deps" >&2; exit 2 ;;
esac
