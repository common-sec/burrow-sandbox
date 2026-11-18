#!/bin/sh
# End-to-end test against real rootless Podman (`make e2e`; builds the images first).
# The sandbox image is curl's (it speaks CONNECT through HTTPS_PROXY; busybox wget does not).
# A blocked check passes only if the command actually ran and failed (exit 1 to 124), never when
# it could not start (R12).
set -u
cd "$(dirname "$0")/.."
export BURROW_STATE_DIR=${BURROW_STATE_DIR:-$(mktemp -d)}
mkdir -p .cache
if [ -n "${BURROW_BIN:-}" ]; then b=$BURROW_BIN; else go build -o .cache/burrow ./cmd/burrow || exit 1; b=.cache/burrow; fi
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
curlimg=mirror.gcr.io/curlimages/curl:8.16.0@sha256:463eaf6072688fe96ac64fa623fe73e1dbe25d8ad6c34404a669ad3ce1f104b6
alpine=docker.io/library/alpine:3.22@sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f
# The sandbox file lives in the workspace, like a project's own burrow.sandbox (`burrow up`).
cat > "$ws/burrow.sandbox" <<EOT
NAME=e2e
PRESET=alpine
IMAGE=$curlimg
WORKSPACE=.
ALLOW=pypi.org
EOT
trap '$b down e2e --purge >/dev/null 2>&1; $b down e2e-run --purge >/dev/null 2>&1' EXIT
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
check P1-tool-reads-workspace ok $b exec -s e2e .cache/e2e-read.preset
check P2-tool-cannot-write blocked $b exec -s e2e .cache/e2e-sh.preset -- 'echo x > /work/x'
check P3-tool-no-network blocked $b exec -s e2e .cache/e2e-sh.preset -- 'wget -q -T 5 -O /dev/null https://1.1.1.1/'
check P4-tool-from-sandbox-folder ok sh -c "cd '$ws' && '$b' exec '$PWD/.cache/e2e-read.preset'"
check I1-inherit-workspace-rw ok $b exec -s e2e .cache/e2e-inherit.preset -- sh -c 'echo x > /work/inherit.txt'
check I2-inherit-allowlist ok $b exec -s e2e .cache/e2e-inherit.preset -- curl -fsS $nocert -m 20 -o /dev/null https://pypi.org/simple/
check I3-inherit-blocks-others blocked $b exec -s e2e .cache/e2e-inherit.preset -- curl -fsS $nocert -m 10 -o /dev/null https://example.com/
check I4-inherit-cannot-reach-sandbox blocked $b exec -s e2e .cache/e2e-inherit.preset -- curl -fsS --noproxy '*' -m 5 -o /dev/null "http://$(podman inspect --format '{{(index .NetworkSettings.Networks "burrow-e2e").IPAddress}}' burrow-e2e):8080/"
# Without a sandbox: inherit means read-only and no network.
ws2=$(mktemp -d); echo 'alone' > "$ws2/README.txt"
check S1-standalone-reads ok $b exec --workspace "$ws2" .cache/e2e-read.preset
check S2-standalone-inherit-ro blocked $b exec --workspace "$ws2" .cache/e2e-inherit.preset -- sh -c 'echo x > /work/x'
check S3-standalone-inherit-no-network blocked $b exec --workspace "$ws2" .cache/e2e-inherit.preset -- curl -fsS $nocert -m 10 -o /dev/null https://pypi.org/simple/
# An ad-hoc persistent sandbox from a built-in preset.
check R1-run-builtin-preset ok $b run alpine --name e2e-run --workspace "$ws2"
check R2-shell-in-run ok $b shell e2e-run -- cat /work/README.txt
check R3-run-no-direct-route blocked $b shell e2e-run -- wget -Y off -q -T 5 -O /dev/null https://1.1.1.1/
echo "# e2e: $fails failure(s)"
exit "$fails"
