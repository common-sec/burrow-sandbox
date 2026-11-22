#!/bin/sh
# Static, reproducible binaries for every product and platform into dist/ (run by `make build`, inside packaging/burrow/Dockerfile).
# CGO_ENABLED=0 and -trimpath come from the build image (packaging/burrow/Dockerfile); -buildid= and the commit date make the output
# depend only on the source.
set -eu
cd "$(dirname "$0")/.."
version=${BURROW_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
rm -rf dist && mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64 windows/arm64; do
  os=${target%/*}; arch=${target#*/}; ext=; [ "$os" = windows ] && ext=.exe
  for cmd in burrow burrow-proxy; do
    # The proxy runs inside Linux containers only (the forwarder is Rust: packaging/forward).
    [ "$cmd" != burrow ] && [ "$os" != linux ] && continue
    GOOS=$os GOARCH=$arch go build -ldflags "-s -w -buildid= -X main.version=$version" \
      -o "dist/$cmd-$os-$arch$ext" "./cmd/$cmd"
  done
done
(cd dist && sha256sum -- * > SHA256SUMS)
cat dist/SHA256SUMS
