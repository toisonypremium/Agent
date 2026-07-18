#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR=/home/admin/btc-agent/btc-agent
cd "$ROOT_DIR"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git -C .. rev-parse --short=12 HEAD 2>/dev/null || printf unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
OUTPUT="${OUTPUT:-bin/btc-agent}"
LDFLAGS="-X main.version=$VERSION -X main.commit=$COMMIT -X main.buildTime=$BUILD_TIME"
go test -count=1 ./...
go vet ./...
mkdir -p "$(dirname "$OUTPUT")"
tmp="${OUTPUT}.new"
rm -f "$tmp"
go build -trimpath -ldflags "$LDFLAGS" -o "$tmp" .
chmod 755 "$tmp"
mv -f "$tmp" "$OUTPUT"
sha256sum "$OUTPUT" | tee "${OUTPUT}.sha256"
printf "version=%s\ncommit=%s\nbuild_time=%s\nbinary=%s\n" "$VERSION" "$COMMIT" "$BUILD_TIME" "$OUTPUT" | tee "${OUTPUT}.manifest"
