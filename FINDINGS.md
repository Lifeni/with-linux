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

## 2.1 bubbles v2 textinput（2026-09-28 为设置页实查）

读自本地 module cache `charm.land/bubbles/v2@v2.2.1/textinput/textinput.go`：

- 关键 API：`New()` / `Focus() tea.Cmd` / `Blur()` / `Value()` / `SetValue()` / `SetWidth()` / `CursorEnd()` / `Update(msg) (Model, tea.Cmd)` / `View()`。
- **默认用虚拟光标**（`useVirtualCursor: true`），`Cursor()` 在虚拟光标模式下返回 nil —— 所以**不需要**设 `tea.View.Cursor`，输入框自己画光标。
- `Update` 只处理 `tea.KeyPressMsg` 与 `tea.PasteMsg`（再往下是内部 pasteMsg）。
- 引入 `textinput` 会带进传递依赖 `github.com/atotto/clipboard v0.1.4`（粘贴键用，纯 Go、调用外部剪贴板命令，不用就碰不到），需 `go mod tidy` 补 go.sum。

## 2.2 pty 冒烟测试要点（2026-09-28）

- `pty.fork()` 建出的伪终端**默认窗口是 0×0**，Bubble Tea 认为没尺寸就什么都不画（画面全空）。必须先 `ioctl(TIOCSWINSZ, pack(HHHH, rows, cols, 0, 0))` 给个尺寸。
- 宽度对齐：抓到的 ANSI 流是**增量重绘**（只画变化的格子），直接删转义符会丢行结构；要还原"用户看到的画面"得自己维护一块字符网格（支持 CUP/EL/ED/CR/LF/BS/SGR 忽略即可），并且**东亚宽字符要占两格、填充格在拼接时丢掉**，否则中文之间会出现假空格。
- 结论：冒烟脚本 `/tmp/wl_smoke.py`（非项目文件）用上述两点重建画面，可稳定读出 tab 栏、设置页、状态栏文本。

## 3. 目标机工具链（已查）

- 目标平台：Linux arm64 / amd64（依赖均为纯 Go 实现，无 cgo）。
- 交付交叉编译：纯 Go 依赖（bubbletea/lipgloss/bubbles 均无 cgo），直接 `GOOS=linux GOARCH=amd64 go build` ＋ `GOOS=linux GOARCH=arm64 go build` 即可，无需额外工具链。
- 实际交叉编译验证留到 M3（有产物后展示真实输出）。

## 4. 工作目录现状（已查）

- 项目根目录起始为空（仅本次落盘的 `AGENTS.md`、`TASK_PLAN.md`）；当时非 git 仓库。

## 5. 后续审查与修复结论（2026-09-28）

- 用量 `Init` 曾丢弃 `doFetch` 返回的模型状态；`RefreshMsg`/`R` 又各自启动 tick，导致多条 tick 链和重叠取数。修复方向为单 tick 所有者、请求代次、旧请求 context 取消。
- 配置读取曾把缺失、读取失败、JSON 损坏都折叠为空 key，UI 无法区分。修复方向为分类 `LoadError`；损坏 JSON 修复保存前按 `.bak`、`.bak.1` 递增备份。
- 倒计时旧实现使用 `Math.ceil`，Go 实现曾用整数截断，剩余不足 1 秒会显示 `0s`。修复为 `math.Ceil`。
- 鼠标点击分支曾不检查按键类型，右键也会触发 tab/设置行点击。修复为只响应左键。
- 发版工作流曾只在 tag 上运行，并在 GoReleaser 钩子里执行 `go mod tidy`。修复为 push/PR CI 与发布前检查，发布钩子不再修改模块文件。

## 6. Command Code 用量端点（2026-10-05 调研，属范围扩张 A6）

来源：第三方 `pi-commandcode-provider` 的 `/commandcode-quota`（读的就是 `cmd` CLI `/usage` 用的那套端点）＋ 本机真实 key 只读探测核对。这些是**未公开的 alpha 端点**，可能变动，故一律按「缺字段 = 无数据」处理，不猜测。

- 端点（`https://api.commandcode.ai`，请求头 `Authorization: Bearer <commandCodeApiKey>`）：
  - `GET /alpha/whoami` → `{success, user:{id,name,email,userName}, org}`；个人账号 `org=null`，团队账号有 `org.id`（作为后续请求的 `?orgId=` 参数）。
  - `GET /alpha/billing/credits` → `credits{monthlyCredits,purchasedCredits,freeCredits,...}` ＋ `windowLimits{fiveHour{used,cap,exceeded,resetAt}, weekly{...}, limited, exceeded}`。
  - `GET /alpha/billing/subscriptions` → `data{planId,status,currentPeriodStart,currentPeriodEnd,...}`（`currentPeriodEnd` 为 ISO 字符串）。
  - `GET /alpha/usage/summary` → `{totalCount,totalCost,totalCredits,totalMonthlyCredits,totalFreeCredits,totalPurchasedCredits,...,periodBasis:"billing-period"}`。
- 真实探测样例（2026-10-05，个人账号）：`credits.monthlyCredits=9.9118`（月度剩余）、`fiveHour{used:0.0882,cap:3,resetAt:1791186972217}`、`weekly{used:0.0882,cap:6,resetAt:1791773772217}`（`resetAt` 是**毫秒**时间戳）、`summary.totalMonthlyCredits=0.0865`（本计费周期月度已用）、`subscriptions.data.planId`（如 `goat`）/`currentPeriodEnd`。
- **三列映射**：5h = `fiveHour.used/cap`；每周 = `weekly.used/cap`；月度 = `totalMonthlyCredits ÷ (totalMonthlyCredits + credits.monthlyCredits)`（已用 ÷（已用＋剩余））。缺任一侧字段 → 该列 `[--]`。
- 容错：whoami 的 401/403/非 2xx 直接判 `KEY_INVALID`/`NO_SUB`/`HTTP_<n>`；credits/subscriptions/summary 的 401/403 同样致命，其余失败该部分按无数据；三者全失败 → `BAD_RESPONSE`。重试策略与 OpenCode 页一致（3 次尝试、2s 间隔；认证类不重试）。
- 渲染实测（2026-10-05，真实 key）：`5h [16%] 1h14m / Week [8%] 6d20h / Month [5%] 30d20h`，与 `used/cap`、`used/(used+remaining)` 吻合。
