#!/bin/sh
# Updates the pinned mise release in scripts/install-mise.sh (version and SHA-256 per platform),
# which Dependabot cannot see. Only a release that is at least COOLDOWN_DAYS old (default 7, the
# same cooldown as everything else) is taken; COOLDOWN_DAYS=0 for a security fix (say so in the
# commit). Prints the new version, or nothing when there is nothing to do.
# Used by .github/workflows/update-pins.yml; runs locally too (needs curl, jq, GNU date).
set -eu
cd "$(dirname "$0")/.."
f=scripts/install-mise.sh
days=${COOLDOWN_DAYS:-7}
api=https://api.github.com/repos/jdx/mise/releases
cur=$(sed -n 's/^MISE_VERSION=//p' "$f")
# Newest release that is not a draft or pre-release and is old enough.
cutoff=$(date -u -d "-$days days" +%Y-%m-%dT%H:%M:%SZ)
ver=$(curl -fsSL --proto '=https' --tlsv1.2 "$api?per_page=30" |
  jq -r --arg c "$cutoff" '[.[] | select(.draft|not) | select(.prerelease|not) | select(.published_at <= $c)][0].tag_name // empty' |
  sed 's/^v//')
[ -n "$ver" ] || { echo "update-mise: no release older than $days days" >&2; exit 1; }
[ "$ver" != "$cur" ] || exit 0
sums=$(curl -fsSL --proto '=https' --tlsv1.2 "https://github.com/jdx/mise/releases/download/v$ver/SHASUMS256.txt")
for asset in linux-x64 linux-arm64 macos-arm64; do
  sum=$(printf '%s\n' "$sums" | awk -v n="./mise-v$ver-$asset.tar.gz" -v m="mise-v$ver-$asset.tar.gz" '$2==n || $2==m {print $1}')
  echo "$sum" | grep -Eq '^[a-f0-9]{64}$' || { echo "update-mise: no checksum for $asset" >&2; exit 1; }
  sed -i "s/\(asset=$asset; *sum=\)[a-f0-9]\{64\}/\1$sum/" "$f"
done
sed -i "s/^MISE_VERSION=.*/MISE_VERSION=$ver/" "$f"
echo "$ver"
