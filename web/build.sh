#!/usr/bin/env bash
# 把版本号写进 index.html 里那一行 <p class="version">。
# Vercel 的 Build Command 指向本脚本；本地直接跑也一样（页面本身不依赖它）。
#
# 版本号来源优先级：$WL_VERSION → version.txt → 最近的 git tag → 保持原样。
set -euo pipefail
cd "$(dirname "$0")"

ver="${WL_VERSION:-}"

if [ -z "$ver" ] && [ -f version.txt ]; then
  ver="$(head -n1 version.txt | tr -d '[:space:]')"
fi

if [ -z "$ver" ] && command -v git >/dev/null 2>&1; then
  ver="$(git describe --tags --abbrev=0 2>/dev/null || true)"
fi

if [ -z "$ver" ]; then
  echo "build: 没找到版本号，index.html 保持原样"
  exit 0
fi

case "$ver" in
  v*) ;;
  *) ver="v$ver" ;;
esac

sed -E "s#(<p class=\"version\">)[^<]*(</p>)#\1${ver}\2#" index.html > index.html.tmp
mv index.html.tmp index.html
echo "build: 版本号写入 $ver"
