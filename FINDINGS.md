# FINDINGS.md — M0 只读调研结论

> 记录调研事实与出处。未经验证的写「待查」，不写猜测。

## 1. 旧零依赖用量面板（已拿到源码：`usage-tui.js`，用户 2026-09-25 拷入项目根目录）

> 以下逐条读自源码，是 A2/A3 的验收判据（"与旧实现一致"以此为准）。

### 1.1 取数逻辑

- `GET https://opencode.ai/zen/go/v1/usage`，请求头 `Authorization: Bearer <apiKey>`；**请求超时 10s**。
- 解析 `json.usage.{rolling,weekly,monthly}`，每窗取 `{ percent: number, resetsAt: ISO字符串, status: string }`；`percent` 非 number 的窗按"无数据"处理（显示 `[--]`，倒计时 `--`）。
- 列顺序固定 `rolling, weekly, monthly`；标签 `rolling→5h`、`weekly→Week`、`monthly→Month`。

### 1.2 错误码与文案（状态栏显示文本）

| 情况 | 内部错误码 | 显示文案 | 颜色 |
|------|-----------|----------|------|
| config.apiKey 缺失（不发请求） | NO_KEY | `未配置 Key` | amber |
| HTTP 401 | KEY_INVALID | `Key 无效（401）` | red |
| HTTP 403 | NO_SUB | `无权限（403）` | red |
| 其他非 2xx | HTTP_<status> | `请求失败：HTTP_<status>` | red |
| 响应缺 `usage` 字段 | BAD_RESPONSE | `请求失败：BAD_RESPONSE` | red |
| fetch 异常/超时 | NETWORK | `连接失败 · 10s 后重试` | red |
| 正常但正在取数 | — | `查询中…` | dim |
| 正常空闲 | — | `<n>s 后刷新`（首启未取过时 `待刷新`） | dim |

### 1.3 重试策略

- 一轮取数最多 **3 次尝试**（FETCH_ATTEMPTS=3），失败间隔 **2s**（FETCH_RETRY_DELAY）。
- **NO_KEY / KEY_INVALID / NO_SUB 不重试**，立即返回；NETWORK / HTTP_* / BAD_RESPONSE 会重试。
- 周期轮询：成功后 **60s**（REFRESH_SEC）再取；一轮失败后 **10s**（RETRY_SEC）再取。
- 定时器每 1s tick（顺带递减所有倒计时）；`R` 键手动刷新（并重读 config 判断有无 key）；`q`/`Q`/`ESC`/`Ctrl+C` 退出。

### 1.4 三列点阵进度（展示格式）

- 布局：标题行（左标题 `OpenCode Go 用量`，右上角状态，无页脚栏）→ 空行 → 生长区（占满剩余高度）→ 空行 → 标签行。
- 三列柱状，**底部实心、顶部渐变稀疏**：填充高度 `h = max(1, round(pct/100*H))`（pct>0 时）；顶部渐变段 `ramp = clamp(round(H*0.16), 4, 16)`，每往一行点亮格数递减 `n = round(q*W)`。
- **确定性哈希**（imul 常数 0x9e3779b1 / 0x85ebca6b / 0xc2b2ae35 / 0x2545f491 / 0x27d4eb2f）决定点亮哪几格：同格状态永不变化，画面静止不抖动；用量增长时点亮集合嵌套，只生长不熄灭。
- 默认点阵样式：点亮 `●`、暗 `·`（暗格 dim/灰色），每格 2 字符宽；列宽取偶数。
- 空位铺灰底：bgStyle 默认 `block`（`░` + 256 色 236 底色）。
- 列块布局：`inner = cols-2`；`gapW = inner>=60 ? 3 : 1`；`blockW = floor((inner-gapW*2)/3)`（dot 模式取偶，最小 4）；三列整体居中。
- 百分比颜色：`>=85` 红(174)、`>=60` 琥珀(179)、其余蓝(68)。
- 标签行：每列 `5h [42%]` + 右侧重置倒计时；对齐模式自适应（字段对齐/紧凑，条件见源码 331-350 行）。
- 倒计时格式 `fmtCountdown`：无数据 `--`；已到期 `now`；≥1 天 `3d11h`；≥1 小时 `3h11m`；否则 `15m`。
- window.status 非 'ok' 时，标签后追加琥珀色 status 文本。

### 1.5 旧实现配置项（`config.json` 与网页版共用）

- 必填：`apiKey`。可选：`fillStyle`(dot/block)、`dotLit`、`dotDim`、`rampRows`、`bgStyle`(block/shade/dot/none)、`bgChar`、`bgColor`(0-255)。
- **v1 移植决定（建议）**：仅移植 `apiKey` ＋ 上述默认样式；可选样式字段留到以后按需加（属范围问题，加时先问用户）。

### 1.6 移植到新布局的映射

- 旧版把状态放标题右上角；新版按章程放**底部状态提示右侧**（文案文本保持 1.2 表格一致）。
- 面板内容区保持旧版三段：标题行 → 点阵生长区 → 标签行；倒计时每秒递减（tea tick）。

## 2. Bubble Tea v2 兼容性（已查，风险解除）

查询源：`proxy.golang.org` 模块版本列表（2026-09-25）：

| 模块 | 最新版本 |
|------|----------|
| `github.com/charmbracelet/bubbletea/v2` | v2.0.10 |
| `github.com/charmbracelet/bubbles/v2` | v2.2.1 |
| `github.com/charmbracelet/lipgloss/v2` | v2.0.6 |

- **结论：bubbles 存在专门适配 Bubble Tea v2 的 `/v2` 模块，章程里"bubbles 可能卡在 v1"的风险不成立；直接使用三个 v2 模块。**

## 3. 目标机工具链（已查）

- 目标平台：Linux arm64 / amd64（依赖均为纯 Go 实现，无 cgo）。
- 交付交叉编译：纯 Go 依赖（bubbletea/lipgloss/bubbles 均无 cgo），直接 `GOOS=linux GOARCH=amd64 go build` ＋ `GOOS=linux GOARCH=arm64 go build` 即可，无需额外工具链。
- 实际交叉编译验证留到 M3（有产物后展示真实输出）。

## 4. 工作目录现状（已查）

- 项目根目录起始为空（仅本次落盘的 `AGENTS.md`、`TASK_PLAN.md`）；当时非 git 仓库。
