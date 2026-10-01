# web — With Linux 介绍页

> 目前**不对外发布**，只在本地看效果。代码留着，将来想上线再说（部署步骤见文末）。

纯静态介绍页，**没有 JS**：`index.html` ＋ `style.css` ＋ `favicon.svg` ＋ `dots.svg`
＋ `fonts/`。整站等宽字体，亮暗色跟随系统。README 顶部那张海报由
`scripts/gen-banner.py` 用同一份点阵和图标生成（`assets/banner-{light,dark}.svg`）。

```
web/
  index.html          页面（底部一条 info / 按钮栏）
  style.css           样式 + @font-face
  dots.svg            背景点阵（生成物，当 CSS mask 用）
  favicon.svg         站点图标（与 assets/icon.svg 是同一枚，网页要自包含所以各放一份）
  fonts/              DejaVu Sans Mono 的 woff2（ASCII 子集）
  build.sh            构建时写入版本号
  version.txt         版本号来源
```

## 本地预览

```bash
python3 -m http.server 8080 --directory web
# 浏览器打开 http://localhost:8080
```

直接双击 `index.html`（`file://`）也可以，没有脚本、没有模块加载。

## 背景点阵

`dots.svg` 由 `scripts/gen-web-dots` 生成，用的是**和 TUI 同一个确定性哈希**
（`internal/usage/view.go` 的 `hash01`）：自下而上每行点亮数递减，点亮哪几格由哈希
决定 —— 所以是一行一行地随机变少，而不是整块整块地淡出。改动参数（行数、格子边长、
点半径、浓淡）就在那个 Go 文件顶部的常量里，然后重新生成：

```bash
go run ./scripts/gen-web-dots                      # 只更新 web/dots.svg
go run ./scripts/gen-web-dots -preview /tmp/out    # 顺便出两张预览 PNG（亮/暗）
```

SVG 里只有白色点和各自的不透明度，网页里当 mask 用，颜色由 CSS 的 `--dot` 给——
一张图同时服务亮色和暗色。

## 字体

用 **DejaVu Sans Mono**（多数 Linux 发行版自带的代码字体）。CSS 里先 `local()`，
装了就直接用系统字体、不下载；没装的（macOS / Windows）才走仓库里的 woff2。

woff2 是 ASCII 子集（各约 11KB），由系统字体自己转换而来：

```bash
pyftsubset /usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf \
  --output-file=web/fonts/dejavu-sans-mono-400.woff2 \
  --flavor=woff2 --unicodes=U+0020-007E --name-IDs='*'
```

字体授权：DejaVu 系（Bitstream Vera + Arev 授权），允许自由再分发。
页面里的「下载」两个字没有等宽覆盖，回落到系统 CJK 等宽字体。

## 版本号

`index.html` 里 `<p class="version">` 那一行由 `build.sh` 在构建时改写；不跑构建也能
直接打开，只是版本号停在仓库里的值。来源优先级：`$WL_VERSION` → `version.txt` →
最近的 git tag。

## Vercel 部署

项目 Settings：

- **Root Directory**：`web`
- **Framework Preset**：`Other`
- **Build Command**：`bash build.sh`
- **Output Directory**：`.`（或留空）

### 发版后自动重建（二选一）

1. **靠 git 推送自动触发**：更新 `version.txt` 并推到生产分支，Vercel 自己会重建。
2. **发版流程显式触发**：Vercel 项目 Settings → Git → Deploy Hooks 建一个 hook，
   URL 存成仓库 secret，发版 workflow 末尾 `curl -fsS -X POST "$DEPLOY_HOOK"`。
   Deploy Hook 的作用就是「触发一次部署并重跑 Build Step」。

注意：Vercel 默认浅克隆，构建时 `git describe` 不一定拿得到 tag，所以以
`version.txt` 为准。
