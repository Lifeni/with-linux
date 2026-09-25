#!/usr/bin/env bash
# 本地构建 .deb（无 goreleaser 依赖）。用法：bash scripts/mkdeb.sh [goarch]
set -euo pipefail
ARCH="${1:-$(dpkg --print-architecture)}"
VERSION="${VERSION:-dev}"
PKG="wl"
BIN="wl-linux-$ARCH"
DEST="dist/pkg/$BIN"

CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$DEST" ./cmd/wl

ROOT="dist/pkgroot/$BIN"
rm -rf "$ROOT"
install -Dm755 "$DEST" "$ROOT/usr/bin/wl"
install -Dm644 README.md "$ROOT/usr/share/doc/$PKG/README.md"
install -Dm644 LICENSE "$ROOT/usr/share/doc/$PKG/copyright"

mkdir -p "$ROOT/DEBIAN"
sed -e "s/@VERSION@/$VERSION/g" -e "s/@ARCH@/$ARCH/g" -e "s/@PKG@/$PKG/g" scripts/deb-control.in > "$ROOT/DEBIAN/control"

OUT="dist/${PKG}_${VERSION}_${ARCH}.deb"
dpkg-deb --build --root-owner-group "$ROOT" "$OUT"
dpkg-deb --info "$OUT"
dpkg-deb --contents "$OUT"
