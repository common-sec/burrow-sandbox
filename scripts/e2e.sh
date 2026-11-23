#!/bin/sh
# End-to-end test against real rootless Podman (`make e2e`; builds the images first).
# Uses the test presets (test/presets) and their images (test/images, built as
# localhost/burrow-test-NAME:dev), so every pinned base image is in a Dockerfile Dependabot updates.
# The sandbox uses the curl preset (curl speaks CONNECT through HTTPS_PROXY; busybox wget does not).
# What each check proves, as a user story: docs/FEATURES.md.
# A blocked check passes only if the command actually ran and failed (exit 1 to 124), never when
# it could not start (R12).
set -u
cd "$(dirname "$0")/.."
export BURROW_STATE_DIR=${BURROW_STATE_DIR:-$(mktemp -d)}
export BURROW_PRESETS_DIR=$PWD/test/presets
mkdir -p .cache
# The binary from `make build` (built in a container), for this machine's architecture.
case $(uname -m) in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) arch=$(uname -m) ;; esac
b=${BURROW_BIN:-dist/burrow-linux-$arch}
[ -x "$b" ] || { echo "e2e: $b not found: run 'make build' first" >&2; exit 1; }
case $b in /*) ;; *) b=$PWD/$b ;; esac
# Networks that intercept TLS (some corporate and cloud sandboxes) break certificate checks inside
# containers; E2E_TLS_INSECURE=1 skips them for the reachability checks only.
nocert=; [ "${E2E_TLS_INSECURE:-}" = 1 ] && nocert=--insecure
fails=0
check() { # id want(ok|blocked) cmd...
  id=$1; want=$2; shift 2; out=$("$@" 2>&1); rc=$?
  case "$want-$rc" in
    ok-0) echo "PASS $id" ;;
    blocked-*) if [ "$rc" -ge 1 ] && [ "$rc" -le 124 ] && ! echo "$out" | grep -qiE 'no such container|not found|executable file|improper state'; then echo "PASS $id"; else echo "FAIL $id rc=$rc: $out"; fails=$((fails+1)); fi ;;
    *) echo "FAIL $id rc=$rc: $out"; fails=$((fails+1)) ;;
  esac
}
ws=$(mktemp -d); echo 'hello' > "$ws/README.txt"
curlimg=localhost/burrow-test-curl:dev
alpine=localhost/burrow-test-alpine:dev
# The sandbox file lives in the workspace, like a project's own burrow.sandbox (`burrow up`).
cat > "$ws/burrow.sandbox" <<EOT
NAME=e2e
PRESET=curl
WORKSPACE=.
ALLOW=pypi.org
EOT
trap '$b down e2e --purge >/dev/null 2>&1; $b down e2e-run --purge >/dev/null 2>&1; podman volume rm -f burrow-cache-exec_trivy >/dev/null 2>&1' EXIT
check E1-up ok $b up -f "$ws/burrow.sandbox"
check E2-workspace ok $b shell e2e -- cat /work/README.txt
check E3-allowed ok $b shell e2e -- curl -fsS $nocert -m 20 -o /dev/null https://pypi.org/simple/
check E4-other-host blocked $b shell e2e -- curl -fsS $nocert -m 10 -o /dev/null https://example.com/
check E5-direct-ip blocked $b shell e2e -- curl -fsS --noproxy '*' -k -m 5 -o /dev/null https://1.1.1.1/
check E6-no-root blocked $b shell e2e -- touch /etc/burrow-test
check E7-no-engine-socket blocked $b shell e2e -- ls /run/podman/podman.sock
check E8-workspace-writable ok $b shell e2e -- touch /work/written-by-sandbox
check E9-sandbox-file-read-only blocked $b shell e2e -- sh -c 'echo ALLOW=evil.example >> /work/burrow.sandbox'
# A port opened after start (R9): a tiny server in the sandbox, published through the forwarder.
podman exec -d burrow-e2e sh -c 'while true; do printf "HTTP/1.0 200 OK\r\n\r\nburrow-ok" | nc -l -p 8080; done' >/dev/null
check F1-port-publish ok $b port e2e 8080 18080
sleep 1
check F2-port-reachable ok sh -c 'curl -fsS -m 5 http://127.0.0.1:18080/ | grep -q burrow-ok'
check F3-forwarder-no-route-for-sandbox blocked $b shell e2e -- curl -fsS --noproxy '*' -k -m 5 -o /dev/null https://1.1.1.1/
# Tools: presets run next to the sandbox with `exec` (defaults: workspace read-only, no network).
cat > .cache/e2e-read.preset <<EOT
NAME=read
IMAGE=$alpine
ARGS=cat /work/README.txt
EOT
cat > .cache/e2e-sh.preset <<EOT
NAME=sh
IMAGE=$alpine
ARGS=sh -c
EOT
cat > .cache/e2e-inherit.preset <<EOT
NAME=inherit
IMAGE=$curlimg
WORKSPACE_MODE=inherit
NETWORK=inherit
EOT
# Inherit with fallbacks: what the tool gets when there is no sandbox to inherit from.
cat > .cache/e2e-fallback.preset <<EOT
NAME=fallback
IMAGE=$curlimg
WORKSPACE_MODE=inherit:rw
NETWORK=inherit:proxy
ALLOW=pypi.org
EOT
# A tool that asks for more than the sandbox has: a host the sandbox does not allow. Runs by default (the preset is
# trusted to ask only for what it needs); refused with --strict.
cat > .cache/e2e-wider.preset <<EOT
NAME=wider
IMAGE=$curlimg
NETWORK=proxy
ALLOW=proxy.golang.org
EOT
check P1-tool-reads-workspace ok $b exec -s e2e .cache/e2e-read.preset
check P2-tool-cannot-write blocked $b exec -s e2e .cache/e2e-sh.preset -- 'echo x > /work/x'
check P3-tool-no-network blocked $b exec -s e2e .cache/e2e-sh.preset -- 'wget -q -T 5 -O /dev/null https://1.1.1.1/'
check P4-tool-from-sandbox-folder ok sh -c "cd '$ws' && '$b' exec '$PWD/.cache/e2e-read.preset'"
check I1-inherit-workspace-rw ok $b exec -s e2e .cache/e2e-inherit.preset -- sh -c 'echo x > /work/inherit.txt'
check I2-inherit-allowlist ok $b exec -s e2e .cache/e2e-inherit.preset -- curl -fsS $nocert -m 20 -o /dev/null https://pypi.org/simple/
check I3-inherit-blocks-others blocked $b exec -s e2e .cache/e2e-inherit.preset -- curl -fsS $nocert -m 10 -o /dev/null https://example.com/
check I4-inherit-cannot-reach-sandbox blocked $b exec -s e2e .cache/e2e-inherit.preset -- curl -fsS --noproxy '*' -m 5 -o /dev/null "http://$(podman inspect --format '{{(index .NetworkSettings.Networks "burrow-e2e").IPAddress}}' burrow-e2e):8080/"
check C1-default-tool-gets-its-preset ok $b exec -s e2e .cache/e2e-wider.preset -- curl -fsS $nocert -m 20 -o /dev/null https://proxy.golang.org/
check C2-strict-refuses-wider-tool blocked $b exec -s e2e --strict .cache/e2e-wider.preset -- true
check C3-strict-allows-covered-tool ok $b exec -s e2e --strict .cache/e2e-inherit.preset -- true
# Without a sandbox: inherit means read-only and no network, unless the preset names a fallback.
ws2=$(mktemp -d); echo 'alone' > "$ws2/README.txt"
check S1-standalone-reads ok $b exec --workspace "$ws2" .cache/e2e-read.preset
check S2-standalone-inherit-ro blocked $b exec --workspace "$ws2" .cache/e2e-inherit.preset -- sh -c 'echo x > /work/x'
check S3-standalone-inherit-no-network blocked $b exec --workspace "$ws2" .cache/e2e-inherit.preset -- curl -fsS $nocert -m 10 -o /dev/null https://pypi.org/simple/
check S4-fallback-rw ok $b exec --workspace "$ws2" .cache/e2e-fallback.preset -- sh -c 'echo x > /work/fallback.txt'
check S5-fallback-proxy-allowed ok $b exec --workspace "$ws2" .cache/e2e-fallback.preset -- curl -fsS $nocert -m 20 -o /dev/null https://pypi.org/simple/
check S6-fallback-proxy-blocks-others blocked $b exec --workspace "$ws2" .cache/e2e-fallback.preset -- curl -fsS $nocert -m 10 -o /dev/null https://example.com/
# The shimmable trivy preset: any arguments, no default command.
check T1-trivy-any-args ok $b exec --workspace "$ws2" trivy -- --version
# An ad-hoc persistent sandbox from a preset in the presets folder.
check R1-run-preset ok $b run alpine --name e2e-run --workspace "$ws2"
check R2-shell-in-run ok $b shell e2e-run -- cat /work/README.txt
check R3-run-no-direct-route blocked $b shell e2e-run -- wget -Y off -q -T 5 -O /dev/null https://1.1.1.1/
# Each proxy has its own egress network: a container on one sandbox's egress network cannot reach
# another sandbox's proxy (before, every proxy shared Podman's default network).
other=$(podman inspect --format '{{(index .NetworkSettings.Networks "burrow-e2e-run-egress").IPAddress}}' burrow-e2e-run-proxy 2>/dev/null)
check N0-other-proxy-found ok test -n "$other"
check N1-egress-networks-apart blocked podman run --rm --network burrow-e2e-egress --cap-drop ALL "$curlimg" -fsS --noproxy '*' -m 5 -o /dev/null "http://${other:-192.0.2.1}:3128/"
check N2-own-proxy-on-egress ok sh -c "podman inspect --format '{{range \$k, \$v := .NetworkSettings.Networks}}{{\$k}} {{end}}' burrow-e2e-proxy | grep -q 'burrow-e2e-egress'"
echo "# e2e: $fails failure(s)"
exit "$fails"
