#!/usr/bin/env bash
# 本地构建 .deb（无 goreleaser 依赖）。用法：bash scripts/mkdeb.sh [goarch]
set -euo pipefail
ARCH="${1:-$(dpkg --print-architecture)}"
# 版本号：显式 VERSION 优先；否则取最近 tag ＋ ~dev。
# `~` 让本地包排序**低于**正式版（dpkg: 0.2.0~dev < 0.2.0），
# 免得本地包装过之后改回正式版还要 --allow-downgrades。
if [ -z "${VERSION:-}" ]; then
  if tag=$(git describe --tags --abbrev=0 2>/dev/null) && [ -n "$tag" ]; then
    VERSION="${tag#v}~dev"
  else
    VERSION=dev
  fi
fi
# 注入构建信息（「关于」区块展示）；拿不到就留空，由程序回退 Go 构建信息。
COMMIT="${COMMIT:-$(git rev-parse --short=7 HEAD 2>/dev/null || echo "")}"
if [ -n "$COMMIT" ] && ! git diff --quiet 2>/dev/null; then
  COMMIT="$COMMIT-dirty"
fi
DATE="${DATE:-$(date +%F)}"
# 包名用 with-linux，不用 wl：发行版源里已有同名包 wl（Emacs Wanderlust），
# 版本号更低会被 unattended-upgrade 当成旧版覆盖，导致 wl 命令消失。
# 命令名仍由下面 install 到 /usr/bin/wl 决定。
PKG="with-linux"
BIN="wl-linux-$ARCH"
DEST="dist/pkg/$BIN"

CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE" -o "$DEST" ./cmd/wl

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
