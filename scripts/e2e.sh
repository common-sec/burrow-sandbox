#!/bin/sh
# End-to-end test against real rootless Podman (`mise run e2e`; builds the images first).
# The sandbox image is curl's (it speaks CONNECT through HTTPS_PROXY; busybox wget does not).
# A blocked check passes only if the command actually ran and failed (exit 1 to 124), never when
# it could not start (R12).
set -u
cd "$(dirname "$0")/.."
export BURROW_STATE_DIR=${BURROW_STATE_DIR:-$(mktemp -d)}
mkdir -p .cache
if [ -n "${BURROW_BIN:-}" ]; then b=$BURROW_BIN; else go build -o .cache/burrow ./cmd/burrow || exit 1; b=.cache/burrow; fi
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
cat > .cache/e2e.env <<EOT
NAME=e2e
IMAGE=mirror.gcr.io/curlimages/curl:8.16.0@sha256:463eaf6072688fe96ac64fa623fe73e1dbe25d8ad6c34404a669ad3ce1f104b6
WORKSPACE=$ws
ALLOW=pypi.org
EOT
trap '$b down e2e --purge >/dev/null 2>&1' EXIT
check E1-up ok $b up .cache/e2e.env
check E2-workspace ok $b exec e2e -- cat /work/README.txt
check E3-allowed ok $b exec e2e -- curl -fsS $nocert -m 20 -o /dev/null https://pypi.org/simple/
check E4-other-host blocked $b exec e2e -- curl -fsS $nocert -m 10 -o /dev/null https://example.com/
check E5-direct-ip blocked $b exec e2e -- curl -fsS --noproxy '*' -k -m 5 -o /dev/null https://1.1.1.1/
check E6-no-root blocked $b exec e2e -- touch /etc/burrow-test
check E7-no-engine-socket blocked $b exec e2e -- ls /run/podman/podman.sock
# A port opened after start (R9): a tiny server in the sandbox, published through the forwarder.
podman exec -d burrow-e2e sh -c 'while true; do printf "HTTP/1.0 200 OK\r\n\r\nburrow-ok" | nc -l -p 8080; done' >/dev/null
check F1-port-publish ok $b port e2e 8080 18080
sleep 1
check F2-port-reachable ok sh -c 'curl -fsS -m 5 http://127.0.0.1:18080/ | grep -q burrow-ok'
check F3-forwarder-no-route-for-sandbox blocked $b exec e2e -- curl -fsS --noproxy '*' -k -m 5 -o /dev/null https://1.1.1.1/
cat > .cache/e2e-scan.preset <<EOT
NAME=nonet
IMAGE=docker.io/library/alpine:3.22@sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f
NETWORK=none
ARGS=cat /work/README.txt
EOT
check P1-preset-reads-workspace ok $b run e2e .cache/e2e-scan.preset
sed -e 's/^NAME=nonet/NAME=shell/' -e 's/^ARGS=.*/ARGS=sh/' .cache/e2e-scan.preset > .cache/e2e-shell.preset
check P2-preset-cannot-write blocked $b run e2e .cache/e2e-shell.preset -- -c 'echo x > /work/x'
check P3-preset-no-network blocked $b run e2e .cache/e2e-shell.preset -- -c 'wget -q -T 5 -O /dev/null https://1.1.1.1/'
echo "# e2e: $fails failure(s)"
exit "$fails"
