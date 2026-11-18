#!/bin/sh
# Installs a pinned mise release after checking its SHA-256 (used by CI; no third-party action).
# Update: new version (at least 7 days old) and its checksums from the release's SHASUMS256.txt.
set -eu
MISE_VERSION=2026.11.5
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)  asset=linux-x64;   sum=d7dee029352bf4b8986e7c4696b8ce54746c3eb648ffca3cc69bdeecc0ff06e9 ;;
  Linux-aarch64) asset=linux-arm64; sum=146b114c2714f6ed76a629778f162d578e14821e47b87389dbdd64068bd5cf06 ;;
  Darwin-arm64)  asset=macos-arm64; sum=f69c8a4f7c265d5f3ffdd06d44104aa15a7ffd9b6545a87b7768f893b70f0fd8 ;;
  *) echo "install-mise: unsupported platform $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac
dest=${1:-$HOME/.local/bin}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/mise.tgz" \
  "https://github.com/jdx/mise/releases/download/v$MISE_VERSION/mise-v$MISE_VERSION-$asset.tar.gz"
echo "$sum  $tmp/mise.tgz" | sha256sum -c -
tar -xzf "$tmp/mise.tgz" -C "$tmp"
mkdir -p "$dest" && install -m 0755 "$tmp/mise/bin/mise" "$dest/mise"
"$dest/mise" --version
