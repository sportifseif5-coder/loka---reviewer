#!/usr/bin/env bash
# Cross-compile the loka CLI for Linux, macOS, and Windows.
# Produces checksummed archives under dist/. Code signing (Apple/Microsoft)
# is a release-host step and is not performed here.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="${LOKA_DIST:-$ROOT/dist}"
mkdir -p "$DIST"

targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"

cd "$ROOT"
for t in $targets; do
    os="${t%/*}"
    arch="${t#*/}"
    ext=""
    if [ "$os" = "windows" ]; then
        ext=".exe"
    fi
    out="$DIST/loka-${os}-${arch}${ext}"
    echo "building $out"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$out" ./cmd/loka
done

(
    cd "$DIST"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum loka-* > SHA256SUMS
    else
        shasum -a 256 loka-* > SHA256SUMS
    fi
)
echo "packaged into $DIST"
cat "$DIST/SHA256SUMS"
