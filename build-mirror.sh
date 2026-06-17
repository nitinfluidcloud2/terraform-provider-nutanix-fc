#!/usr/bin/env bash
#
# build-mirror.sh — build the forked Nutanix provider and lay out BOTH mirror
# formats (filesystem + network) under dist/. Single source of truth so every
# rebuild is reproducible and the network-mirror JSON is always correct.
#
# Usage:
#   ./build-mirror.sh                          # version 2.4.3-beta1-fc2, all 4 platforms
#   VERSION=2.4.3-beta1-fc3 ./build-mirror.sh  # override version
#   PLATFORMS="darwin_arm64" ./build-mirror.sh # subset of platforms (faster local dev)
#
set -euo pipefail

VERSION="${VERSION:-2.4.3-beta1-fc2}"
PLATFORMS="${PLATFORMS:-darwin_arm64 darwin_amd64 linux_amd64 linux_arm64}"
HOST="registry.terraform.io"          # mirror path host (NOT registry.opentofu.org)
NS="nutanix"                          # namespace
TYPE="nutanix"                        # provider type
NAME="terraform-provider-nutanix"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FS_BASE="$ROOT/dist/fs-mirror/$HOST/$NS/$TYPE"
NET_BASE="$ROOT/dist/net-mirror/$HOST/$NS/$TYPE"

echo ">> version=$VERSION  platforms=[$PLATFORMS]"
rm -rf "$ROOT/dist/fs-mirror/$HOST" "$ROOT/dist/net-mirror/$HOST"
mkdir -p "$FS_BASE/$VERSION" "$NET_BASE"

# sha256 in lowercase hex — portable across macOS (shasum) and Linux (sha256sum)
sha256hex() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# --- build per platform: unpacked binary (fs mirror) + zip (net mirror) -------
first=1
ARCHIVES=""
for plat in $PLATFORMS; do
  os="${plat%_*}"; arch="${plat#*_}"
  echo ">> building $plat"

  # 1) filesystem mirror: unpacked executable named with _v<VERSION>
  #    -trimpath + -buildvcs=false make the binary as reproducible as Go allows,
  #    so hashes only change when the CODE changes (not on every rebuild).
  fsdir="$FS_BASE/$VERSION/$plat"
  mkdir -p "$fsdir"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false -o "$fsdir/${NAME}_v${VERSION}" "$ROOT"
  chmod +x "$fsdir/${NAME}_v${VERSION}"

  # 2) network mirror: zip containing that same executable
  zip_name="${NAME}_${VERSION}_${plat}.zip"
  ( cd "$fsdir" && zip -qj "$NET_BASE/$zip_name" "${NAME}_v${VERSION}" )

  # 3) zh: hash = SHA256 of the .zip in lowercase hex (the mirror-protocol format)
  zh="zh:$(sha256hex "$NET_BASE/$zip_name")"

  [ $first -eq 0 ] && ARCHIVES="$ARCHIVES,"
  first=0
  ARCHIVES="$ARCHIVES
    \"$plat\": { \"url\": \"$zip_name\", \"hashes\": [\"$zh\"] }"
done

# --- network-mirror index.json + <version>.json ------------------------------
printf '{"versions":{"%s":{}}}\n' "$VERSION" > "$NET_BASE/index.json"
cat > "$NET_BASE/$VERSION.json" <<EOF
{
  "archives": {$ARCHIVES
  }
}
EOF

echo ">> done."
echo "   fs-mirror : $FS_BASE/$VERSION/<os>_<arch>/${NAME}_v${VERSION}"
echo "   net-mirror: $NET_BASE/{index.json,$VERSION.json,*.zip}"
echo
echo "   verify the JSON uses zh: (NOT sha256:) ->"
grep -o '"hashes": \["[a-z0-9:]*' "$NET_BASE/$VERSION.json" | head -1
