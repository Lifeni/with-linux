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
- **满格例外（2026-10-06，用户反馈的 bug）**：`pct >= 100` 时该列 `ramp` 置 0，整列实心填满。旧实现（`usage-tui.js`）在 100% 时仍保留顶部渐变，最上面几行稀疏，视觉上永远像没到 100%（矮终端 ramp 最小 4 行尤其明显）；此处为有意偏离旧实现的修正，其余 `<100%` 的渐变行为不变。
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
- **补充（2026-10-12，M8 第 6 步实测）**：上面那句「支持 CUP/EL/ED 即可」不够——Bubble Tea v2 的标准渲染器还会发 **`ESC[<n>X`（ECH，擦除 n 个字符）**（本机一次 4 秒的运行里 ECH 出现 43 次、CUP 39 次、EL 30 次、CHA 28 次、ED 1 次），重建器不实现 ECH 就会留下上一帧的残影（曾误判成「同一行提示渲染了两次」）。相对移动 `ESC[<n>A/B/C/D`、绝对列 `ESC[<n>G`、绝对行 `ESC[<n>d` 也要处理。
- **切视图后不要指望抓屏能给出干净整屏**：Bubble Tea 只重画变化的格子，旧视图的字符会留在网格里（除非程序自己发 ED）。抖动窗口尺寸能逼它重绘，但两种宽度的帧混在一张网格里同样会花。**版式正确性用单测断言（每行可见宽度恰好等于终端宽度、内容包含关系），抓屏只用来确认「程序真的跑起来了、字真的画出来了」。**
- **2026-10-12 结论：抓屏别自己写重建器，直接上 `tmux`。** 同一个字节流，手写重建器（即使补了 ECH）和 `pyte` 都还原出「上一帧的边框残留」，而 `tmux new-session` ＋ `tmux capture-pane -p` 完全正确——问题在它们对增量光标移动/滚动优化的解释，不在程序。pty 冒烟的可靠做法：
  ```bash
  tmux new-session -d -s wl -x 100 -y 28 "XDG_CONFIG_HOME=/tmp/xxx ./wl"
  sleep 5 && tmux capture-pane -p -t wl
  tmux send-keys -t wl s        # 按键；Escape / q 同理
  tmux kill-session -t wl
  ```
- **`tmux new-session` 不继承客户端的 `XDG_CONFIG_HOME`**（只转发 DISPLAY/SSH_* 等少数变量）：必须把环境变量写进 `-c` 命令里，否则冒烟会读到真实配置、用到真实 key。
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

## 7. 系统监控数据源（2026-10-11 调研，属范围扩张 A8/A9）

> 目标机实测环境：RK3588（arm64，内核 6.1.43-rockchip），8 核、7.5 GiB 内存、单块 NVMe、1 个物理网卡 ＋ 11 个 docker 虚拟接口、7 个 thermal zone。以下「第 N 列」均指按空白分隔后的列号，从 1 起。**零新依赖**：全部只读 `/proc` 与 `/sys`（CI 是 `CGO_ENABLED=0` 双架构构建，不引入 gopsutil 之类）。

### 7.1 CPU / 负载 / 运行时长

- `/proc/stat`：首行 `cpu` 是全部核合计，其后 `cpu0`…`cpuN` 是每核。本机首行共 11 列（`cpu` 标签 ＋ 10 个数值：user nice system idle iowait irq softirq steal guest guest_nice）。
  - busy = 数值之和 − idle − iowait；total = 数值之和（guest/guest_nice 内核已计入 user/nice，不重复加）。
  - **必须两次采样求差**：`% = Δbusy / Δtotal × 100`。单次读数是自开机以来的累计值，直接用没有意义（本机首行累计已达 1.5×10⁹ jiffies）。
  - 核数以 `cpuN` 行数为准（本机 8），**不用 `runtime.NumCPU()`**（受 CPU 亲和性/cgroup 配额影响，容器里会少报）。
  - 首次采样没有前值 → 该次 CPU 显示 `[--]`，不要显示 0% 假数据。
  - 两次采样间隔用实测时间差而非假定 1 秒（tick 会漂移），否则速率类指标系统性偏高/偏低。
  - **采样间隔小于一个 jiffy 时会没有数据**：`USER_HZ=100` → 10 ms  granularity，连读两次 `/proc/stat` 很可能拿到完全相同的计数（Δtotal=0），此时按「无数据」处理而不是显示 0%。1 秒采样碰不到，但单测里连着调两次采集就会（`internal/monitor` 的用例为此 sleep 40 ms）。
- `/proc/loadavg`：实测 `0.37 0.29 0.33 1/677 1484162` → 前三个浮点是 1/5/15 分钟负载。第 4 列是「运行中/总进程数」，**本次不用**（章程：系统状态页不采集进程）。
- `/proc/uptime`：实测 `1908131.90 14714140.36` → 第 1 列是开机秒数（浮点，本机约 22 天）；第 2 列是各核空闲秒数之和，不用。格式化沿用现有倒计时风格：`22d3h` / `5h11m`。

### 7.2 内存与 Swap

- `/proc/meminfo` 单位 kB。本机字段名已逐个核对存在：`MemTotal MemFree MemAvailable Buffers Cached SwapCached … SwapTotal SwapFree … KReclaimable Slab SReclaimable …`。
  - **used = MemTotal − MemAvailable**。本机实测 `MemTotal 7867792` / `MemFree 303320` / `MemAvailable 3222652` → 用 MemFree 会算出 **96% 占用（假象）**，用 MemAvailable 才是 ~59%。这是本次最容易踩的坑。
  - 缓存 = `Buffers + Cached + SReclaimable`（本机 408988 + 1836788 + SReclaimable），作为暗色次要数值显示，**不计入 used**。
  - Swap = `SwapTotal − SwapFree`；`SwapTotal == 0` 时该项显示 `[--]`（本机有 zram swap，非 0）。
  - `MemAvailable` 缺失（<3.14 内核）→ 回退 `MemFree + Buffers + Cached`；仍缺则 `[--]`。

### 7.3 网络

- `/proc/net/dev`：前两行是表头，之后每行 `iface: rx_bytes rx_packets rx_errs rx_drop rx_fifo rx_frame rx_compressed rx_multicast tx_bytes tx_packets …`（冒号后第 1 列 = rx bytes，第 9 列 = tx bytes）。接口名与冒号之间可能有空格（本机 `lo:` 前有 4 个空格），按冒号切分后要 `TrimSpace`。
- 本机 12 个接口里只有 1 个物理网卡（`enp1s0`，rx 35.1 GB / tx 23.3 GB），其余是 `lo`、`docker0`、5 个 `br-*`、4 个 `veth*`。
- **过滤规则**：排除 `lo`、`veth*`、`docker*`、`br-*`、`virbr*`；再排除 `/sys/class/net/<if>/operstate` 为 `down` 的（本机 `docker0` 与一个 `br-*` 是 down）。**不能只靠 operstate**：`lo` 的 operstate 是 `unknown`，`veth*`/`br-*` 多为 `up`。
- **主网卡选择**：读 `/proc/net/route`，取 Destination 列 == `00000000`（默认路由）且 Metric 最小的行的 Iface。本机唯一命中 `enp1s0`（Flags `0003` = RTF_UP|RTF_GATEWAY，Metric 100）；表里其余行是 docker 网段的直连路由（Destination 非 0）。没有默认路由时退化为「过滤后 rx+tx 累计最大的接口」。
  - **进制陷阱**：Destination/Gateway/Mask 是 `%08X` 十六进制小端（本机 gateway `010200C0` = 192.0.2.1），而 **Metric 是十进制**——实测 Metric 列 `100` 与 `ip route show default` 的 `metric 100` 吻合；按十六进制解析会把 100 当成 256，多条默认路由时会选错网卡。
- 速率 = 字节差 / 实测时间差。**计数器回绕或接口重建会得到负差** → 该次按 0 处理并重置基线，不画负值尖峰。

### 7.4 磁盘与温度

- `/proc/diskstats` 列：1 major、2 minor、3 设备名、4 读完成数、5 读合并、**6 读扇区数**、7 读耗时 ms、8 写完成数、9 写合并、**10 写扇区数**、11 写耗时 ms、12 进行中 IO、**13 IO 耗时 ms（算 util% 用）**、14 加权 IO 耗时，其后是 discard/flush（内核 4.18+；本机每行 18 列）。
  - 扇区一律按 **512 字节**换算（内核文档口径，与设备物理扇区大小无关）。
  - 本机 `nvme0n1` 实测：读扇区 145960574（≈69.7 GiB）、写扇区 401413261（≈191.4 GiB）、IO 耗时 4302197 ms。
  - **util% 口径已实测核对**：字段 13（IO 耗时 ms）的每秒增量 ÷ 10 就是 iostat 的 `%util`。本机同一时刻采集器算出 `1.0%`、`iostat -dx 1 2 nvme0n1` 末列 `1.00`，完全一致。多队列设备能并发，util 可能 >100% → 显示时钳到 100。
  - **过滤**：只留整盘设备 `sd[a-z]+`／`nvme\d+n\d+`／`mmcblk\d+`／`vd[a-z]+`；排除 `loop*`、`ram*`、`zram*`、`mtdblock*`、`dm-*`（本机噪声：一堆 loop/ram ＋ `mtdblock0`）。分区（`nvme0n1p1`/`p2`）不单独统计——其 IO 已计入父设备，重复计会翻倍。
- `/proc/self/mounts`：每行 `设备 挂载点 fstype 选项 dump pass`。本机真实 fs 只有 `/`（ext4，`/dev/nvme0n1p2`）与 `/boot`（vfat）；实测伪 fs 类型多达 21 种（autofs bpf cgroup2 configfs debugfs devpts devtmpfs functionfs fusectl hugetlbfs mqueue nsfs overlay proc pstore ramfs rpc_pipefs securityfs sysfs tmpfs tracefs）。
  - **用白名单不用黑名单**：只留 ext2/3/4、xfs、btrfs、vfat、exfat、f2fs、ntfs（伪 fs 种类太多，黑名单必漏）。
  - **按 `st_dev` 去重**：本机 `/var/log.hdd` 是 `/dev/nvme0n1p2[/var/log]` 的 bind mount，与 `/` 同设备 → 去重掉。**`st_dev` 不能从 statfs 拿**：statfs(2) 不返回设备号（内核 `f_fsid` 各文件系统实现不一，ext4 之外常为 0），设备号取自 `stat(2)` 的 `st_dev`。bind mount 的设备字段带 `[子路径]` 后缀，也不能拿它当唯一键。本机 `/var/log` 在 `zram1` 上但 fstype 是 ext4 → **白名单只看 fstype，所以它保留**（独立设备、独立空间，显示出来才对；实测它就是 188M 的 zram）。
  - 容量用 `syscall.Statfs` ＋ `syscall.Stat`：total = `Blocks × Bsize`，avail = `Bavail × Bsize`，used = `(Blocks − Bfree) × Bsize`（**与 df 的 Used 同口径**，含 root 保留块）。`Statfs_t`/`Stat_t` 的字段名在 amd64/arm64 一致，`CGO_ENABLED=0` 双架构交叉编译已实测通过。
  - **百分比用 df 的口径**：`used ÷ (used + avail)`，分母不含 root 保留块。用 `used ÷ total` 会系统性偏低（ext4 默认保留 5%），磁盘快满时差得最多，而「还剩多少能写」正是用户拿 df 对数的那个数。实测本机 `/`：used/total = 9.15%、used/(used+avail) = 9.23%，而 `df` 显示 **10%** —— 因为 **df 对百分比向上取整**；渲染时磁盘百分比也向上取整以对齐 df（其余指标仍四舍五入）。
  - 挂载点路径里的八进制转义（`\040` 空格、`\011` 制表符等）需解码；显示时按可用宽截断。
  - 排序：`/` 永远第一，其余按已用降序；面板高度不足时从末尾丢行（A9）。
- **温度**：`/sys/class/thermal/thermal_zone*/{type,temp}`，`temp` 是**毫摄氏度**。本机 7 个 zone 实测：`soc-thermal` 39769、`bigcore0-thermal` 39769、`bigcore1-thermal` 39769、`littlecore-thermal` 39769、`center-thermal` 38846、`gpu-thermal` 38846、`npu-thermal` 38846（≈38.8–39.8 °C）。规则：优先 type 含 `soc`/`cpu` 的 zone（本机命中 `soc-thermal`），没有则取 zone0；一个 zone 都没有（无 thermal 的机器、容器）→ `[--]`。

### 7.5 字符宽度与 braille 画法（2026-10-11 实测）

- 用 `github.com/mattn/go-runewidth@v0.0.27` ＋ `charm.land/lipgloss/v2@v2.0.6` 实测：braille（`⡀⣿⠉⠛⠒`）、方块（`▁▂▃▄▅▆▇█`）、半块（`█▓▒░`）每字符 `runewidth.StringWidth` 与 `lipgloss.Width` **都是 1**；`runewidth.IsEastAsian()` 在本环境为 false（非 ambiguous）→ 现有窄屏截断逻辑（`clampStyled`/`ClampLines`）可直接用于 braille 界面，无需特殊处理。
- braille 位映射（每字符 2 列 × 4 行，基码 U+2800）：列 0 自上而下是 bit `0x01/0x02/0x04/0x40`，列 1 是 `0x08/0x10/0x20/0x80`。
  - 横向计量条：填充段是 U+28FF `⣿`（亮色），**空位是空格**（2026-10-12 用户明确「空的地方不要填充点阵」）；不再有半格/轨道字符。
  - 时间序列是**填充面积图**：每一列都从底填到该列的值（btop 风格），0% 时保留一条基线。
  - 竖向计量条每行 4 级，用底行渐升的 `⣀ ⣤ ⣶ ⣿`。
- **未验证**：用户终端字体是否真有 braille 字形（代码里检测不到）。对策见 `PLAN.md M8.4` 第 3 步——先出静态样张给用户过目；若缺字形，`internal/chart` 只换字形映射表退回方块字符，布局与算法不动。

### 7.6 采样开销

- 一次采样读 8 个文件（`/proc/stat`、`/proc/loadavg`、`/proc/uptime`、`/proc/meminfo`、`/proc/net/dev`、`/proc/net/route`、`/proc/diskstats`、`/proc/self/mounts`）＋ 每个白名单挂载点一次 `statfs` ＋ thermal zone 若干。全是内核内存里的伪文件，1 秒一次开销可忽略。
- tick 所有权：`monitor` 自持每秒 tick（与 `usage`/`cmdusage` 各自的**包内私有** `tickMsg` 并存，类型不同故互不串台，沿用现有做法）。
- **在设置页期间继续采样**（程序在跑就采，返回时曲线连续、无空洞）；程序退出即停止，不常驻、不自启。

## 8. Docker / Caddy / 弹窗（2026-10-20 只读调研）

> 以下为本机只读实测；容器名、域名、上游地址等示例一律用假值。真实 API Key 不出现。

### 8.1 Docker Engine API

- 版本：client/server `29.8.1`，API `1.56`，`linux/arm64`。socket `/var/run/docker.sock` 权限 `srw-rw---- root:docker`；当前用户在 `docker` 组，`docker ps` 与 `curl --unix-socket` 均无需 sudo。
- cgroup v2：`/sys/fs/cgroup/cgroup.controllers` 存在。内存口径要扣 `inactive_file`，与 `docker stats` 接近。
- 实测本机 6 个容器全部 `State=running`。`GET /containers/json?all=1` 返回 `Id`、`Names`、`Image`、`State`、`Status` 等；`Status` 含 `Up 2 days (healthy)` 这类健康信息。
- `/version` 返回 `ApiVersion`；后续请求可按该版本前缀，也可用 daemon 支持的非版本路径。客户端以 `ApiVersion` 为准，失败时回退。

#### 8.1.1 统计与 CPU%

- `GET /containers/{id}/stats?stream=false&one-shot=true` 实测约 **54ms** 返回，但 `precpu_stats` 为 0/null，**不能单独算 CPU%**。
- `stream=false`（不带 one-shot）约 **1.04s** 返回，含 `precpu_stats`；`stream=true` 每秒一帧，第一帧 `precpu_stats` 为 0，第二帧起有效。
- 选择：每 2 秒对运行中容器发 `one-shot=true`，在客户端保存上一次样本自行求差，省掉 1 秒阻塞和服务端双采样。字段：
  - `cpu_stats.cpu_usage.total_usage`（纳秒累计）、`cpu_stats.system_cpu_usage`（主机累计）、`cpu_stats.online_cpus`（本机 8）。
  - `memory_stats.usage`、`.limit`、`.stats.inactive_file`；`pids_stats.current`。
  - `networks`：每个接口 `rx_bytes` / `tx_bytes`，容器总计需求和。
  - `blkio_stats.io_service_bytes_recursive`：`{op, value}` 列表，取 `read`/`write`。
- CPU% 公式（与 Docker CLI 同口径，100% ＝ 1 核）：
  ```
  cpuDelta = cpu_total - prev_cpu_total
  sysDelta = system_cpu_usage - prev_system_cpu_usage
  cpuPct   = cpuDelta / sysDelta * online_cpus * 100
  ```
  第一帧或计数器回退（容器重启）时清基线并显示 `[--]`；多容器总 CPU% 可以直接相加，可超过 100%。
- 内存显示口径：`max(usage - inactive_file, 0)` / `limit`；百分比按该值。网络与块 I/O 取累计字节，速率本版不强制显示在状态卡，详情弹窗显示累计。

#### 8.1.2 动作

- `POST /containers/{id}/start`、`/stop`、`/restart`，成功返回 204；失败返回 JSON `message`。2026-10-20 用户要求弹窗内直接点按钮执行，**没有二级菜单与二次确认**；测试只打 fake socket。
- 2026-10-22 真实一次性 Alpine 容器验证：`restart` 耗时 **10.68s**，因为 Docker 会先按停止超时优雅终止进程再启动。普通查询的 5 秒 HTTP 超时不适用于动作请求；客户端已为 start/stop/restart 使用独立 60 秒超时。
- 只允许对列表中存在的容器 ID 操作；不实现 exec、删除、镜像/Compose。socket 等价 root，代码只调用这几个端点。

#### 8.1.3 日志

- `GET /containers/{id}/logs?stdout=1&stderr=1&tail=200&timestamps=0`，一次性读取。
- 非 TTY 容器的流是**多路复用帧**：8 字节头 = 1 字节流类型（0 stdin/1 stdout/2 stderr）＋ 3 字节 0 ＋ 4 字节大端 payload 长度；随后是 payload。TTY 容器是原始字节流。容器是否 TTY 从 `GET /containers/{id}/json` 的 `.Config.Tty` 判断；该只读 inspect 端点已明确纳入允许范围，且只读取这一个字段。
- 实现上限：200 行、256 KiB；超限截断并标记；显示前剥离 SGR（防止命令输出里的 ANSI 破坏弹窗布局）；日志只在内存，不写盘；不做 follow。
- 空日志显示 `（无日志）`；API 404/权限/超时在弹窗内显示短错误。

#### 8.1.4 错误与依赖

- socket 不存在、`EACCES`、daemon 未启动、API 版本不匹配都要转成短文案；状态卡与弹窗显示，不影响 CPU/内存等面板。
- **零新 Go 依赖**：`net/http` ＋ 自定义 `Transport.DialContext` 连接 Unix socket，`encoding/json` 解析。

### 8.2 Caddy 管理 API 与 systemd

- `caddy version` 实测 `v2.11.4`；服务 `caddy.service` 为 systemd system unit，`ActiveState=active`、`SubState=running`、`Restart=no`、`CanReload=yes`；`ExecReload=/usr/bin/caddy reload --config /etc/caddy/Caddyfile --force`。
- `/etc/caddy/Caddyfile` 当前用户可读（root:root 0644）；`caddy adapt --config ... --adapter caddyfile` 以当前用户执行成功（exit 0），可读/可解析。
- 管理 API `127.0.0.1:2019` 可达；`GET /config/` 返回 live JSON（实测 `apps.http.servers.srv0`；递归统计到 12 个带 `handle` 的对象），其 `match.host` / `hosts` / `tls.subjects` 可提取配置域名（去重保序）；`GET /reverse_proxy/upstreams` 返回 5 条，字段 `address`、`num_requests`、`fails`，测试时 5/5 `fails=0`。状态卡用两列展示域名与上游**目的端口**（只取端口，不显示完整地址）、上游健康、路由数与文件路径。**不展示完整配置内容与完整上游地址。** 热重载由弹窗内 `[重载配置]` 按钮触发，参数固定。
- 热重载：执行 `caddy reload --config /etc/caddy/Caddyfile --force`（与 `ExecReload` 相同）。需要 Caddyfile 可读、admin API 可达，不需要 root；未在真实服务上执行过（写操作，等实现后用临时实例验证）。失败时 Caddy 进程继续使用旧配置。
- 启停/重启：`systemctl start/stop/restart caddy` 需要 root；unit `Restart=no`，通过 admin API stop 后 TUI 无法再 start。**不做**。
- 只读状态补充：`systemctl show caddy -p ActiveState -p SubState -p MainPID -p NRestarts -p ExecMainStartTimestamp` 可无 root 读取；仅在弹窗打开/刷新时调用，不参与 5 秒轮询。
- 服务状态以管理 API 可达性为准；API 不可达但 systemd running（例如 admin 被关闭）无法区分，界面统一显示“管理 API 不可达”，并给 `systemctl status caddy` 提示。
- 安全：不显示配置 JSON（可能含口令/密钥）、不编辑 Caddyfile、不读 Caddy 日志；命令参数固定，无用户输入拼接。

### 8.3 弹窗遮罩与覆盖（lipgloss v2）

- 依赖版本 `charm.land/lipgloss/v2 v2.0.6` 自带 `Canvas`、`Layer`、`Compositor`：
  - `NewCanvas(w,h)` → `Compose(layer)` / `CellAt(x,y)` / `SetCell` / `Render`；
  - `Cell.Style.Fg/Bg` 可读写；`Layer.Draw(screen, image.Rect(...))` 可把内容画到指定矩形。
- 方案：先把底屏渲染为一个 Layer 画到 Canvas；遍历所有单元格，把 `Fg` 设为灰色（约 240/245）、`Bg` 设为 nil（去掉 [设置] 按钮底色），保留字符内容；再把弹窗 Layer 以居中矩形画到同一 Canvas，`Render()` 输出。终端没有 alpha，视觉上就是“背景浅灰、弹窗原色浮起”。
- Canvas 使用 grapheme 宽度；braille 与 CJK 混排要和现有 `lipgloss.Width` 一致。需用 80×24、120×30 与含 CJK/braille 的 fixture 测试，避免宽字符错位。
- 性能：120×30 ≈ 3600 格/帧，每秒重绘一次可忽略；弹窗内容变化时重建即可。
- 备选（不采用）：手工 strip ANSI 后按行叠加，宽字符/颜色边界容易错；Canvas 是现有依赖内的正路。
- 设置弹窗：现有 `settings.Lines` 会做整行居中/补空格，需要额外暴露“自然框”或计算未补白宽度后再覆盖；内容超长用弹窗内部滚动。

### 8.4 布局断点

- 宽 ≥120 时 CPU 一整行，下半区按 3:2 分左右两栏（左：内存/磁盘、网络/AI；右：Docker/Caddy 纵向服务栏）。低于 120 时回退旧两栏 + 底部 2 行服务摘要；在 119/120 两侧各跑一次布局测试。
- 80×24 的旧两栏已在本机验证可用；加 2 行摘要条后网格少 2 行，仍需保证不折行并可进入紧凑滚动。
- 宽屏左栏上下两排等高、右栏两块等高；窄屏摘要条不画边框，滚轮滚动随内容移动。

### 8.5 纯鼠标交互（2026-10-20）

- Bubble Tea v2 的鼠标点击消息给出 0 基坐标；命中判定必须与渲染使用**同一份矩形/行索引**。弹窗按钮在带线框的 `modalWindow()` 行里，行 inner 内容与 frame 行之间差 1 行边框 + 1 行 padding，命中要取 frame 行而不是 body 行。
- 点击弹窗外侧关闭：先判断点是否落在 `modalRect` 内，外侧直接 `closeModal`；不依赖 `Esc`。
- 文本输入隔离：只有输入态（设置编辑、日志过滤）消费 `tea.KeyPressMsg`；其他按键一律忽略，避免误触退出。`q` 始终退出主页面，但在输入框里作为文本。
- 滚轮：主页面与弹窗内容都支持 `MouseWheelUp/Down`；没有 `PgUp`/`PgDn` 键位。设置输入态不拦截滚轮，弹窗仍可滚动。
- 顶部标题栏的 `[ 设置 ]` 命中区间从右端向前量按钮宽度，与渲染共用 `settingsButtonSpan()`；窄屏依次使用 `[ 设置 ]`/`[设置]`/`[设]`。

## 9. 同类监控工具布局调研（2026-10-21）

- **btop**：固定网格——顶部 CPU（核心/频率/温度/负载），中部内存与磁盘，底部网络与进程；继续作为主布局参考（我们已采用 CPU 整行、左图右核、内存/磁盘/网络分栏、分色线框）。来源：[btop 布局说明](http://www.landui.com/help/show-13206.html)。
- **bottom（btm）**：widget 化布局，`[[row]]`/`[[row.child]]` 可自由组合 CPU/内存/网络/温度/电池；特色是 **expand 模式**，可把单个 widget 铺满全屏看细节。我们采纳 expand 模式（点击 CPU/内存/磁盘/网络放大，点击内容或标题栏返回），但不做可编辑配置文件。来源：[bottom 包说明](https://packages.debian.org/zh-cn/sid/arm64/btm)、[bottom 使用与 expand 介绍](https://m.blog.csdn.net/gitblog_00894/article/details/154969473)。
- **glances**：根据终端尺寸自动调整布局，包含逐核 CPU、内存/Swap、磁盘 I/O、网络、传感器与进程列表；确认我们按宽高分级的做法正确，但其进程列表/远程模式超出本项目非目标，不引入。来源：[Glances 介绍](https://cloud.tencent.com/developer/article/2700732)、[htop/top alternatives](https://linuxblog.io/alternatives-top-htop/)。
- **gtop / atop**：gtop 是更简单的总览卡片，atop 是高密度表格；都缺少本项目需要的 Docker/Caddy/AI 面板，不采用。
- **结论**：保留现有 btop 式单页网格（更符合「一屏控制面板」），补 bottom 的 expand 模式作为小屏查看细节的出口；不加入进程列表、可编辑布局文件或标签页，避免范围扩张。

## 10. OpenDesign 接入调研（2026-10-21）

- OpenDesign（open-design.ai，nexu-io/open-design）是本地优先的 AI 设计工作台；Cloud 套餐才有 Design Plan 额度与钱包，BYOK 的费用由模型提供商收取，OpenDesign 侧没有用量窗口。
- 开源客户端里能找到的 Cloud 接口：
  - `GET https://amr-api.open-design.ai/api/v1/wallet/balance`，`Authorization: Bearer <controlKey>`；响应 `{ balanceUsd: string, updatedAt?: string }`（`apps/daemon/src/integrations/vela-wallet.ts`）。实测无 key 返回 401 `{"error":"unauthenticated"}`。
  - Coding Plan 的 5h/7d 用量来自 `WorkspaceBillingPreflight.codingPlan.windows[]`，字段 `usedCredits`/`limitCredits`/`durationSeconds`/`resetsAt`（`packages/contracts/src/api/collab.ts`）；但客户端是通过 **`vela billing preflight --format json` CLI** 读取，未找到公开的 HTTP preflight 路径与 API Key 认证说明。
- 结论：不能像 OpenCode/Command Code 那样凭猜测加第三个用量面板。需要用户提供 OpenDesign API 文档/可用的 preflight HTTP 端点与 key 类型，或确认只做钱包余额（wallet balance）展示；`vela` 当前未安装。
- 2026-10-21 用用户提供的 OpenDesign Key 只读测试：`GET /api/v1/wallet/balance`、`/api/v1/me`、`/api/v1/models` 均返回 401；`/api/v1/usage`、`/api/v1/credits` 返回 404。该 Key 不是 OpenDesign 桌面 control key，且未能找到其对应的 usage HTTP 端点；继续接入需要用户提供 API 设置页的 Base URL/文档或一条可用的用量响应。

## 11. 历史图不可变（2026-10-22）

- 用户反馈 CPU/网络/磁盘图表中，已经出现过的历史柱高度会随刷新变化。
- 代码路径确认两个独立原因：
  1. 网络与磁盘原先每帧用当前 120 点中的最大速率重新 `AutoMax`；新峰值一出现，全部旧柱会按更大量程重新渲染并变矮。
  2. 主图把 120 个样本按任意图宽插值到 `2×图宽` 个点列。图宽与 119 个采样间隔不整除时，新样本会让旧样本的横向坐标移动非整数点列，字符栅格重新量化后产生“高度变了”的视觉变化。CPU 纵轴虽固定 0–100%，同样受这个横向重采样影响。
- 修复原则：采样时冻结新点高度、显示时固定一个样本占一个 braille 点列。
- 网络/磁盘保存两套历史：原始速率 ring 用于当前信息，归一化百分比 ring 用于绘图；新量程只影响之后插入的点。
- CPU/网络/磁盘主图使用点列容量作为 `slots`；历史不足时补 0，历史超出点列宽度时显示最近 N 秒。采样 ring 仍保留 120 点（2 分钟）。
- 回归判据：加入新样本后，取旧点列与上一帧对应点列比较，所有 braille 位必须相同；同一逻辑值在新量程出现前后不得变化。

## 12. 面板视觉密度与镜像基线（2026-10-22）

- 120×30 实拍显示：内存原有 4 条分类进度条、磁盘原有 3 个挂载点各一条进度条，压缩了历史图高度；CPU 放大态右侧 4 行一核 sparkline 被均匀摊到 28 行内，产生大片空白。
- 调整结论：主页面内存只保留“已用”一条主进度条；磁盘只保留 `/` 一条主进度条。其他分类改成文字摘要，把内高优先给历史图。
- CPU 放大态每核历史图改为多行（测试高度下 5 行/核），右栏信息密度随整屏高度增长；网络放大态右栏补接口、IPv4、累计与错误/丢包。
- 镜像图保留原画法：上基线 `mid+1`、下基线 `mid-1`，中间空 1 个点行。下载基线 owner 固定为第二条序列（蓝色）。
- 早期列表渐隐只改最后 3 行，且 245/242/238 在常见终端色盘里过于接近；现改为对整个可见列表按 255→237 连续取灰阶。
