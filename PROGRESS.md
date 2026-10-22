# PROGRESS.md — with-linux 进度与验证记录

> 计划见 `PLAN.md`，调研结论见 `FINDINGS.md`。宣称「完成」前必须有本文件里的真实验证输出。

## 2026-09-25 · M0 只读调研（完成）

- 旧面板源码 `usage-tui.js` 由用户拷入，移植信息逐条记录于 `FINDINGS.md` §1。
- Charm 三件套 v2 模块确认，规范路径为 `charm.land/*`（github.com/charmbracelet/*/v2 的 go.mod 声明为 charm.land，直接引用会报 module path 不匹配——实测踩过一次后改用 charm.land）。
- v2 API 与 v1 差异（读源码确认）：`Init() Cmd`；`View() View`；alt-screen / 鼠标模式在 `View` 字段上声明（`v.AltScreen`、`v.MouseMode = tea.MouseModeCellMotion`），无 `WithAltScreen`/`WithMouse*` 选项；鼠标消息 `MouseClickMsg`/`MouseReleaseMsg`/`MouseWheelMsg`/`MouseMotionMsg`（`Mouse{X, Y, Button, Mod}`）；`tea.Quit` 可直接作 Cmd 返回。
- 目标平台 arm64/amd64；交叉编译走内建 `GOOS/GOARCH`（M3 实测）。

## 2026-09-25 · M1 TUI 骨架（代码完成，待用户实机鼠标验证）

实现：`main.go` ＋ `internal/app/app.go`（三段式：顶部 tab 栏 / 中间内容区 / 底部状态提示）。
- 键盘：`q`/`esc`/`Ctrl+C` 退出，`←`/`→`/`Tab`/`Shift+Tab` 切换工具，`1`–`9` 直达，`↑`/`↓`/`PgUp`/`PgDn` 滚动。
- 鼠标：点击 tab 切换（命中区间按可见宽度计算，纯函数 `tabLayoutFor`），内容区滚轮滚动，触底 clamp。
- v1 占位内容 30 行（M2 替换为用量面板）；切换逻辑按工具列表写，不写死单面板。

### 验证 1：单元测试（`go test ./... -v`）— 6/6 PASS

```
=== RUN   TestMouseClickTabSwitchesPanel   --- PASS
=== RUN   TestMouseWheelScrollsContent     --- PASS
=== RUN   TestKeyLeftRightSwitchesPanel    --- PASS
=== RUN   TestKeyDigitJumpsToTool          --- PASS
=== RUN   TestKeyQQQuits                   --- PASS
=== RUN   TestResizeClampsScroll           --- PASS
ok  	with-linux/internal/app	0.093s
```

（含"鼠标点击 tab 切换面板""点击 tab 区间外不切换""滚轮触底 clamp"三个真实行为断言。）

### 验证 2：pty 真实运行 smoke（本地临时脚本 `pty_smoke.py`）— PASS

100x30 终端，真实进程渲染三帧 ＋ 退出码：

- 帧1 初始：` with-linux    [OpenCode Go 用量] ` / 内容区 28 行占位 / 状态栏 `OpenCode Go 用量 · q 退出 · ←→ 切换工具 · ↑↓/滚轮 滚动   待刷新`
- 帧2 模拟滚轮向下一次：内容首行 `第 01 行` → `第 02 行`，滚动生效
- 帧3 模拟点击 tab：内容回到 `第 01 行`（点击切 tab 时 scroll 归 0），程序不崩
- 按 `q` 后进程退出码 **0**，alt-screen 正常还原

已知瑕疵（诚实记录）：帧3 行首有一个残留 `M` 字节，来自 pty 对鼠标序列的回显/交错（程序在 raw 模式下运行，用户真实终端不会出现）；不影响行为，待实机复核。

### 待办（M1 收尾）

- [x] 用户 2026-09-28 实机验证：鼠标点击 tab、滚轮滚动、`q` 退出（无问题反馈；此时已有两个标签，点击切换肉眼可见）
- 2026-09-25 用户说「继续」推进 M2；实机鼠标验证留给用户随时做。桌面弹窗方案已确认不可行（相关端口未开），改为让用户直接在目标机终端运行程序。临时 tmux 会话已清理。

## 2026-09-25 · 文档重命名（用户指令）

- `findings.md`→`FINDINGS.md`、`task_plan.md`→`TASK_PLAN.md`、`progress.md`→`PROGRESS.md`（`AGENTS.md` 已是大写）；全部交叉引用（含 Go 注释里的 `FINDINGS.md §1.x`）已同步，grep 验证无小写残留。

## 2026-09-25 · M2 用量面板（代码完成，待真实 API 实测）

实现：`internal/usage/`（config.go 路径与读取 / fetch.go 取数与重试 / model.go 轮询与倒计时 / view.go 三列点阵）＋ app 框架集成（内容区渲染、状态栏文案与配色、消息转发、R 键转发）。
移植对照 `FINDINGS.md` §1 逐条实现；渲染层验证了两个关键性质：点阵「只生长不熄灭」「自下而上密度递减」。

### 验证 3：单元测试（`go test ./...`）— 29/29 PASS

- internal/app 7/7：鼠标点击切 tab（含点击空白不切换）、滚轮滚动与触底 clamp、←→ 切换、数字直达、q 退出、resize clamp、R 键转发工具
- internal/usage 22/22：取数 OK/非数字 percent 置窗 null/401/403/HTTP_500/HTTP_404/无 usage/usage 为 null/坏 JSON/网络失败/空 key 不发请求/重试 3 次恢复/重试 3 次放弃/401 403 不重试/config 读取/XDG 路径/倒计时格式/面板形态/无数据全暗/A4 配置提示/点阵生长性质×2/填充行数

### 验证 4：pty smoke（无配置场景，验证 A4）— PASS

```
帧1:  with-linux    [OpenCode Go 用量]
      未配置 Key
      请在配置文件里填入 apiKey：
      $XDG_CONFIG_HOME/with-linux/config.json
      {"apiKey": "你的 API Key"}
      OpenCode Go 用量 · q 退出 · ←→ 切换工具 · ↑↓/滚轮 滚动 · R 刷新   未配置 Key
帧2:  滚轮+R+点击后进程存活（无画面变化时差异渲染不输出，属正常）
退出: 码 0
检查: A4 提示未配置 Key ✓ / A4 提示配置路径 ✓ / 顶部 tab ✓ / 底部状态栏 ✓ / 不崩 ✓ / 退出码 0 ✓
```

### 验证 5：A3 真实错误路径实测（pty ＋ 确定性日志判据）— PASS

- 场景1 · 无效 key（真实 API `curl` 实测 401 `AuthError`）：
  `fetchDone err="KEY_INVALID"`（1.2s，认证错误不重试）→ `renderStatus "Key 无效（401）" sev=2`，帧里真实显示该文案
- 场景2 · 网络不可达（`HTTPS_PROXY` 指向死端口）：
  `fetchDone err="NETWORK"`（4.08s ＝ 3 次尝试 ＋ 2×2s 间隔，重试策略与 FINDINGS.md §1.3 一致）→ `renderStatus "连接失败 · 10s 后重试" sev=2`
- 验证方法教训（记录）：pty 抓帧判据脆弱（曾 3 次误判），按纪律质疑验证设计后改为**确定性日志判据**（临时 `WITHLINUX_DEBUG` 日志，验证后已删除）；帧只作展示。

### 验证 6：A2 真实数据实测（有效 key）— PASS 9/9

- 真实 API 200：`rolling 8% / weekly 45% / monthly 40%`（key 为用户提供测试 key 的补全版，原 key 少末位）
- 帧实测：三列点阵密度随百分比变化（8%/45%/40%）、标签行 `5h [8%] 1h11m / Week [45%] 2d10h / Month [40%] 12d22h`、状态栏 `60s 后刷新 → 59s 后刷新` 逐秒递减
- 倒计时人工核对：rolling 23:10 重置 1h11m ✓、weekly 周日 08:00 2d10h ✓、monthly 10-08 12d22h ✓

## M2 完成（2026-09-25），M3 开始

## 2026-09-25 · M3 交付（完成）

- `dist/with-linux-linux-amd64`（8.1M）/ `dist/with-linux-linux-arm64`（7.6M），均 `CGO_ENABLED=0` 全静态（首次构建 arm64 为动态链接，按"单一可执行文件"交付约束重建为静态），`file` 验证：`x86-64, statically linked` / `ARM aarch64, statically linked`
- arm64 交付产物在目标机真实跑 A2 smoke：PASS 9/9（点阵/百分比/倒计时/轮询状态/退出码）

## 2026-09-25 · M4 验收核对（A1–A4 对照 AGENTS.md）

| 验收 | 结论 | 证据 |
|------|------|------|
| A1 骨架与交互 | ✅（差用户实机鼠标） | 单测 7/7（点击切 tab/点击空白不切/滚轮 clamp/键盘切换/退出）、pty 模拟鼠标序列渲染正常、resize 自适应单测 |
| A2 用量数据 | ✅ 真实接口实测 | 200 响应 8%/45%/40% → 三列点阵密度、`[8%]`/`[45%]`/`[40%]`、`1h11m`/`2d10h`/`12d22h` 倒计时逐条核对、`60s 后刷新` 逐秒递减；格式对照 FINDINGS.md §1.4 移植 |
| A3 错误处理 | ✅ 真实实测 401/网络失败；403 仅单测 | `Key 无效（401）`（真实 401 不重试 1.2s）、`连接失败 · 10s 后重试`（真实重试链 4.08s）；403 无真实 key 可造，httptest 覆盖 |
| A4 配置缺失 | ✅ pty 实测 | `未配置 Key` ＋ 期望配置路径 ＋ 示例 JSON |

### 遗留（不影响交付）

- [x] 用户实机鼠标验证：2026-09-28 完成（此前 A1 的鼠标路径只有单测＋pty 模拟证据）
- 403 真实路径未实测（无无权限 key），文案与 401 同一机制
- 测试 key 已写入 `~/.config/with-linux/config.json` 并留在对话记录里，用户知情

## 2026-09-25 · 布局微调（用户指令）

- 内容区**去掉顶部「OpenCode Go 用量」标题行**；**最下方增加一个空行**。固定开销仍 4 行（顶部空行＋生长区上空行＋标签行＋底部空行），生长区高度不变。
- 验证：单测断言首行/末行为空行、标签行在倒数第 2 行（`go test ./...` 全绿）；A2 smoke PASS 7/7（真实数据 9%/46%/40%）。
- smoke 断言教训：不锁定真实数据快照值（5h 窗口滚动会漂移，8%→9% 曾致误判），改格式判据 `\[\d+%\]`×3。
- `dist/` 两个产物已重建。

## 2026-09-25 · v0.1.0 发版（GitHub）

- 命令名 `wl`（`cmd/wl/` 结构）；标题改「With Linux」、状态栏按键提示大写（`Q`/`R`，`Q` 退出行为同步支持）；`TASK_PLAN.md`→`PLAN.md`；README 精简。
- 验证：`go test ./...` 全绿（q/Q 双退出键）、A2 smoke PASS 7/7（帧实证新标题与提示）。
- 仓库 https://github.com/Lifeni/with-linux（公开，MIT，GitHub 已识别）；Release v0.1.0 附 `with-linux-linux-{amd64,arm64}` 两个全静态产物。
- 安装了 `gh` 2.23.0；并完成 GitHub 登录（设备码流程由用户完成）。
- `usage-tui.js`（旧实现参考）与 `config.json` 不入库。

## 2026-09-27 · 修复 `wl` 命令消失（deb 包名撞车）

- 现象：`wl` → command not found。
- 根因：本地打的 deb 包名是 `wl`（v0.1.2-dev），与 Debian 源里的 `wl`（Emacs Wanderlust，`2.15.9+0.20210131-2`）撞名；dpkg 认为 `2.15.9…` > `0.1.2-dev`，`unattended-upgrade` 于 **2026-09-27 06:54:34** 把它当旧版覆盖成 Wanderlust，而后者不含 `/usr/bin/wl`，命令随之消失。
  - 证据：`/var/log/apt/history.log` → `Commandline: /usr/bin/unattended-upgrade` / `Upgrade: wl:arm64 (0.1.2-dev, 2.15.9+0.20210131-2)`；`/var/log/dpkg.log` 同刻 `upgrade wl:arm64 0.1.2-dev 2.15.9+…`；`dpkg -L wl` 中 `usr/bin` 下 0 个文件。
- 修复：deb 包名 `wl` → `with-linux`（`scripts/mkdeb.sh` 的 `PKG`、`.goreleaser.yaml` 的 `nfpms[].package_name`），**命令名仍为 `wl`**（仍装到 `/usr/bin/wl`）。

### 验证 7：修复后实测 — PASS

- 构建：`VERSION=0.1.2-dev bash scripts/mkdeb.sh arm64` → `dist/with-linux_0.1.2-dev_arm64.deb`，`Package: with-linux`，内含 `./usr/bin/wl`。
- 安装：`sudo apt install ./dist/with-linux_0.1.2-dev_arm64.deb` → `Setting up with-linux (0.1.2-dev)`。
- `command -v wl` → `/usr/bin/wl`；`dpkg -S /usr/bin/wl` → `with-linux: /usr/bin/wl`；`wl --version` → `wl 0.1.2-dev`。
- `dpkg -l` 两个包并存互不干扰：`with-linux 0.1.2-dev arm64` ＋ `wl 2.15.9+… all`。
- `apt-get -s upgrade` / `apt list --upgradable` → 计划中无 `with-linux`（源里没有此包，不会再被自动升级覆盖）。
- pty 真实启动：渲染出 `With Linux` / `OpenCode Go` / `退出` / `刷新`，按 `q` 退出码 **0**。
- `go test ./...` 全绿（app、usage；仅改打包脚本与 release 配置，未动 Go 代码）。

### 收尾（2026-09-27，用户确认）

- [x] 删除旧包名产物 `dist/wl_0.1.1_arm64.deb`、`dist/wl_0.1.2-dev_arm64.deb`，避免手滑 `apt install` 重蹈覆辙。
- [x] `~/.bashrc` 增 `export PATH="$HOME/go/bin:$PATH"`（改前备份 `~/.backups/shell-20260927/.bashrc.bak-20260927`）；新 shell 验证 `command -v wl` → `/usr/bin/wl`、`wl --version` → `wl 0.1.2-dev`，`PATH` 含 `~/go/bin`。
- 注意：`~/go/bin` 在 PATH 中前置，若日后 `go install .../cmd/wl`，`~/go/bin/wl` 会遮蔽 deb 版。

## 2026-09-27 · 状态栏提示精简 ＋ README 改版（用户指令）

- 底部状态栏左侧提示去掉「←→ 切换工具 · ↑↓/滚轮 滚动」，只留有快捷键的「Q 退出 · R 刷新」（切换/滚动靠鼠标直操，不必占位提示）。`internal/app/app.go:211`。
- README 改版：加 4 个 badge（Release / License / Go Version / Go Reference）；开头简介只说定位、不再列功能与版本；功能拆成单行列表项、去掉解释；删掉「操作」整节。

### 验证 8：状态栏与 README — PASS

- `go test -count=1 ./...` 全绿（未动断言）。
- 重打包 `dist/with-linux_0.1.2-dev_arm64.deb` 并 `apt install --reinstall`（`Setting up with-linux (0.1.2-dev)`；`wl --version` → `wl 0.1.2-dev`）。
- pty 实测：`Q 退出` ✓ `R 刷新` ✓；`切换工具` ✗ `滚动` ✗ `←→` ✗ `↑↓` ✗ 均不再出现；退出码 0。

## 2026-09-28 · 确认安装状态并清理遮蔽的旧版（用户指令）

- 背景：用户问「项目中的版本装到本机了吗」。
- 结论：**已装**。`dpkg -l` → `with-linux 0.1.2-dev arm64 install ok installed`；`/usr/bin/wl`（`dpkg -S` → `with-linux`）sha256 `9dbf3dc1…`，与 `dist/with-linux_0.1.2-dev_arm64.deb` 内含文件一致（`dist/pkg/wl-linux-arm64` ≡ `dist/pkgroot/wl-linux-arm64/usr/bin/wl`）；`dpkg -V with-linux` 无差异；`apt-cache policy with-linux` 无源候选（不会再被发行版 `wl` 自动升级覆盖）。
- 「是不是当前源码」的判据（不是只看版本号）：二进制内置 VCS → `github.com/Lifeni/with-linux v0.1.2-0.20260927034601-beaa400c67ee+dirty`（11:52 脏树构建，1 分钟后被提交为 `6239c51`）；界面文案「Q 退出 · R 刷新」命中、「切换工具」不命中，与 `internal/app/app.go:211` 一致；`HEAD 4d690b1` 之后仅 README（docs）改动。注：用同一 `mkdeb.sh` 命令从当前源码重编的 sha256 不同（`76419fdd…`），因 Go 把 VCS revision/time/`+dirty` 写进二进制，非代码差异。
- 发现的坑：`~/.bashrc:160` 前置 `~/go/bin`，其中是 `go install ...@v0.1.1` 的旧版（内置 mod `v0.1.1`、`wl --version` → `wl dev`、含旧状态栏「切换工具/滚动」）。SSH 交互 shell 会读 `.bashrc`，故实际跑的是旧版而非 0.1.2-dev。
- 处置（用户确认）：`rm ~/go/bin/wl`；**保留** `~/.bashrc` 的 PATH 行（留给其他 Go 工具）。约定：本机正式渠道为 deb（`/usr/bin/wl`），开发迭代用 `go run ./cmd/wl` 或 `go build -o /tmp/wl ./cmd/wl`，**不再** `go install .../cmd/wl`；确需 dev 版则换名（如 `~/.local/bin/wl-dev`），避免再次遮蔽。

### 验证 9：清理后实测 — PASS

- `bash -ic 'command -v wl; wl --version'` → `/usr/bin/wl` / `wl 0.1.2-dev`（清理前为 `~/go/bin/wl` / `wl dev`）。
- `ls -la ~/go/bin/` → 空目录（无其他 Go 工具受影响）；`dpkg -V with-linux` 仍无差异。

## 2026-09-28 · 设置页（用户指令；范围扩张已确认，验收 A5）

- 指令原文：「增加设置页面，作为一个 tab，设置要对应配置文件，目前也就是可以填写 opencode go 的 key，然后设置页面增加关于模块，写一下当前的版本号、日期等」。
- **范围扩张**：章程原本写「v1 仅 OpenCode Go 用量」且只读展示；加设置 tab ＋ 写回配置文件超出已确认 v1 范围。已**先改章程再实现**（`AGENTS.md`：新增验收 `A5`、「界面布局 · 设置页」小节、约束里加「版本与构建信息」）。改章程前用选项征求确认（提问工具调用被中断，用户回「继续」），按推荐默认值执行：表单式交互、关于显示六项、key 掩码、先改章程。
- 实现（一次指令，一处一处改）：
  - `internal/config`（新）：`Path` / `Load` / `Save`。`Save` = 读原 JSON → 只改 `apiKey` → **保留未知字段** → 缩进写回；目录 `0700`、文件 `0600`、写临时文件再 rename。`internal/usage/config.go` 瘦成薄封装（`loadAPIKey()`），读取行为不变（缺失/坏 JSON → 空 key）。
  - `internal/settings`（新）：`API Key` 可编辑行 ＋ `配置文件` 路径 ＋「关于」（版本/构建日期/commit/Go/平台/仓库）；非编辑态掩码前 6 后 4；对外 `Editing()` / `Hints()` / `StatusText()` / `ClickLine()` / `Reload()`；保存成功产出 `settings.SavedMsg`（不让 settings 直接依赖 usage，翻译在框架层）。
  - `internal/meta`（新）：`Info` ＋ `Current(version, commit, date)`；ldflags 注入优先，缺失回退 `debug.ReadBuildInfo()`（模块版本 / `vcs.revision` / `vcs.time`，脏树加 `-dirty`）；commit 截 7 位、RFC3339 日期归档为 `YYYY-MM-DD`。
  - `internal/app`：标签列表 → `{OpenCode Go 用量, 设置}`；按键/鼠标按 `active` **显式分发**（未引入接口/注册机制，守章程「不约定任何接口」）；编辑态下全局键让位给输入框（`Ctrl+C` 仍退出整个程序）；`settings.SavedMsg` → `usage.RefreshMsg`；切标签时 `settings.Reload()`（重读配置并退出编辑态）；状态栏左侧提示与右侧状态随标签变；其余异步消息（每秒 tick / 取数完成 / 输入框 Blink）**同时**喂给两个页面 → 切到设置页时用量轮询不中断。
  - `cmd/wl`：`version` / `commit` / `date` 三个可注入变量；`--version` 与「关于」共用 `meta.Current`。
  - 打包：`scripts/mkdeb.sh` 从 git 取短哈希（脏树加 `-dirty`）与构建日期并注入；`.goreleaser.yaml` 补 `{{.Commit}}` / `{{.Date}}`。
  - 新依赖：`charm.land/bubbles/v2 v2.2.1`（`textinput`）＋传递依赖 `github.com/atotto/clipboard v0.1.4`；API 与 pty 踩坑记入 `FINDINGS.md §2.1 / §2.2`。

### 验证 10：单测 ＋ pty 冒烟（开发构建） — PASS

- `go test -count=1 ./...`：app / config / meta / settings / usage 全 `ok`；`gofmt -l` 无输出；`go vet ./...` 干净。
- pty 冒烟（`/tmp/wl_smoke.py`：`pty.fork` ＋ 自写 ANSI 屏幕重建；`XDG_CONFIG_HOME` 指向 `/tmp/wl-smoke`，**不碰真实配置**）：**27/27 PASS**。
- 画面实证（重建出的真实屏幕）：tab 栏 `With Linux  [OpenCode Go 用量]  [设置]`；设置页 `▸ API Key  oc_sk_…abcd` / `配置文件  /tmp/wl-smoke/with-linux/config.json` / `关于`（`版本 0.1.2-dev`、`构建日期 2026-09-28`、`提交 4d690b1-dirty`、`Go go1.27.1`、`平台 linux/arm64`、`仓库 https://github.com/Lifeni/with-linux`）；编辑态 `API Key  oc_sk_TEST_1234567890_abcdjunk` ＋ 状态栏 `未保存（Enter 保存 · Esc 取消）`；`Esc` 后回到掩码且配置文件未变；`Enter` 保存后状态栏 `已保存`、显示 `oc_sk_…zzzz`、文件内容与 `0600` 权限核对通过；切回用量页仍渲染；`q` 退出码 0。

### 验证 11：deb 重建 ＋ 安装 ＋ 装后实测 — PASS

- `VERSION=0.1.2-dev bash scripts/mkdeb.sh arm64` → `dist/with-linux_0.1.2-dev_arm64.deb`（`Package: with-linux`，内含 `./usr/bin/wl`）。
- `sudo apt install --reinstall ./dist/with-linux_0.1.2-dev_arm64.deb` → `Setting up with-linux (0.1.2-dev)`；`dpkg -s` → `install ok installed`；`command -v wl` → `/usr/bin/wl`；`wl --version` → `wl 0.1.2-dev`；`sha256sum /usr/bin/wl` == `dist/pkg/wl-linux-arm64`（`d7d9f13e…`）；`dpkg -V` 无差异。
- **对装好的 `/usr/bin/wl` 重跑同一 pty 冒烟：27/27 PASS**；「关于」显示 `0.1.2-dev / 2026-09-28 / 4d690b1-dirty`（`-dirty` 属实：本次改动尚未提交，`mkdeb.sh` 从 git 取哈希并标脏）。
- 遗留：提交后重打包即可把 `-dirty` 变纯哈希；实机鼠标点击（用户侧）仍未验。

### 验证 12：提交 ＋ 自动装到本机（新工作流） — PASS

- **提交前敏感信息扫描抓到两处泄露并已清理**：`internal/settings/settings_test.go` 的测试 key 直接抄了真实 key 的前 30 个字符、`AGENTS.md` 的掩码示例抄了真实 key 的后 4 个字符。已改成明显假值（`oc_sk_fake_key_for_tests_0123`、掩码示例 `oc_sk_aa…zz99`），并在章程「设置页 · Key 显示」补一句「文档/代码示例一律用明显假值」。
- 当时认为两组旧 key 搜索片段没有历史命中；2026-09-28 后续复核发现它们已被 `dcf866e` 提交进 `PROGRESS.md`，因此**历史仍含旧 key 片段**。当前工作树已删除这些具体值，但必须把对应 key 视为已泄露并轮换；是否重写远端历史由用户决定，不能只靠删工作树解决。另把 `PROGRESS.md` 里残留的家目录路径改成 `~/go/bin`（`/with-linux` 等构建产物本就在 `.gitignore`，`config.json`、`usage-tui.js` 也在）。
- **工作流新增两条**（用户 2026-09-28 要求，写进 `AGENTS.md §3`）：**8. 改完自动装到本机**（`mkdeb.sh` ＋ `apt install --reinstall`，并对**已安装的** `/usr/bin/wl` 复验）、**9. 提交前扫敏感信息**。
- 提交：`396fc1c feat: 设置页 —— 编辑 API Key 写回配置 ＋ 关于构建信息（A5）`，工作区干净。
- 自动安装：`VERSION=0.1.2-dev bash scripts/mkdeb.sh arm64` → `sudo apt install --reinstall` → `Setting up with-linux (0.1.2-dev)`；`command -v wl` → `/usr/bin/wl`；`wl --version` → `wl 0.1.2-dev`；`/usr/bin/wl` 与 `dist/pkg/wl-linux-arm64` sha256 一致（`6f765c45…`）；`dpkg -V` 无差异。
- 装后对 `/usr/bin/wl` 跑 pty 冒烟：**27/27 PASS**；「关于」显示 `版本 0.1.2-dev / 构建日期 2026-09-28 / 提交 396fc1c`（干净哈希，已无 `-dirty`）。
- 真实配置未被触碰：`~/.config/with-linux/config.json` 仍为 2026-09-25 21:58，md5 `fb8132d2…`。

## 2026-09-28 · 界面调整：点阵居中 ＋ 设置页线框居中（用户指令）

- 指令：「opencode用量页面中三个点阵没有在页面中水平居中，有点靠左，调整一下。设置页面用一个线框框起来，也在页面中水平和垂直居中」。
- **先量后改**：用 tmux `capture-pane` 在真实终端里量旧版点阵墨迹的左右留白（60–160 共 13 个宽度）——偶数宽 `左-右 = -1`（偏左半格）、奇数宽 `= -2`（偏左一整格）。根因：`lead = 1+padL` 且 `padL = (termCols-2-contentW)/2`，既少算 1 格，又把每块末尾那个空格算进了宽度。
- 实现：
  - 点阵按**墨迹宽度**居中：每格是「点+空」两字符，去掉每块末尾空格后 `inkW = (2*colW-1)*3 + 2*(gapW+1)`，`padL = (termCols-inkW)/2`。
  - 三块墨迹宽恒为奇数 ⇒ `inkW` 必为奇数，偶数宽终端里左右留白不可能同时精确相等；此时把多出的 1 格放进**第一段块间距**（而非右侧留白），保证左右留白相等（代价：两段块间距相差 1 格）。
  - 标签行同步：用同一 `labelBlockW = blockW-1` 与同一组 `gap1/gap2`，保证与点阵三列对齐。
  - 设置页：拆出 `innerLines()`（框内内容）→ `lipgloss.RoundedBorder` 线框 → `lipgloss.Place(w, h, Center, Center, box)`；行号映射改为 `boxTop(height)` / `apiKeyLine(height)`（整框垂直居中 ⇒ 行号依赖高度），`app` 鼠标命中随之传 `viewHeight()`。
  - 窄终端兜底：新增 `usage.clampLines`，任何行超过终端宽度就 `MaxWidth` 截断（修掉此前标签行 / 配置文件行折行）。
- 新增断言：`TestDotMatrixHorizontallyCentered`、`TestLinesNeverExceedWidth`、`TestBoxCenteredInContent`（线框水平＋垂直居中、行数、不超宽）。

### 验证 13：单测 ＋ tmux 实测 ＋ 装后实测 — PASS

- `go test -count=1 ./...`：app / config / meta / settings / usage 全 `ok`；`gofmt -l` 空、`go vet` 干净。
- tmux 实测（假 key，13 个宽度）：新版左右留白**全部 0 差**（60 / 80 / 90 / 100 / 101 / 110 / 120 / 121 / 130 / 140 / 141 / 150 / 160）；旧版同宽度为 -1（偶数）/ -2（奇数）。
- 装后实测 `/usr/bin/wl`（宽 100×24）：点阵每行 `左 2 右 2`；设置页线框 `上 4 下 4`、`左 24 右 24`，框内 `提交 43ee6c2`（干净哈希）。
- 提交 `43ee6c2 fix: 用量页点阵精确水平居中；设置页加线框并水平垂直居中`；自动安装：`Setting up with-linux (0.1.2-dev)`，`wl --version` → `wl 0.1.2-dev`，`/usr/bin/wl` 与 `dist/pkg/wl-linux-arm64` sha256 一致（`48d939cf…`），`dpkg -V` 无差异。
- 已知取舍：偶数宽终端里第一段块间距比第二段宽 1 格（换来左右留白相等）。若更在意两段间距一致，把 `view.go` 的 `gapA` 特殊分支去掉即回到「间距相等 + 右侧多 1 格留白」。
- 过程失误：一次 tmux 调试因会话已存在、环境变量没进会话，误用了**真实配置**（画面里出现过真实 key 明文与真实用量数字）。已把脚本改成 `env XDG_CONFIG_HOME=...` 显式传入，之后均用假 key；这也再次说明该 key 建议轮换。

## 2026-09-28 · 窄屏适配（A+B）＋ 线框内边距（用户指令）

- 指令：「设置页面线框左右边距再增大，剩下的做A+B」。
- **先量后改**（tmux）：窄屏现状——30×12 顶栏被截成 `[OpenCode Go 用`、状态栏右侧**整个消失**、设置页线框底部被切且**滚不动**；24×10 只显示到 `版本`；20×8 只到 `配置文件`。根因两条：(1) 顶/底栏按全宽硬画，超宽就被终端截掉；(2) 两个工具都返回「恰好视高」行 ⇒ `maxScroll()==0` ⇒ 章程里的「可滚动」是**死代码**。
- 实现：
  - **A 顶/底栏逐级降级**：顶栏新增 `tabPlanFor()`（全名 → 短名 → 去程序名 → 只留当前 tab ＋ `…`），`tabLayoutFor` 与 `renderTabBar` **共用同一份方案**，命中区间与渲染不会脱节；状态栏改候选表（完整提示 → `Q 退出` ＋ 短状态 → 短名 → 只留状态），**右侧状态始终保留**；两个工具加 `ShortStatusText()`（`401`/`403`/`网络`/`格式`/`未配置`/`查询中`/`Ns`；`已保存`/`未保存`/`编辑中`/`失败`）；最后 `clampStyled` 兜底截断。
  - **B 内容区「自然高度 ＋ 框架居中/滚动」**：设置页 `Lines(width)` 返回自然高度（16 行），**水平居中留在工具内、垂直居中与滚动交给框架**；框架新增 `contentOffset()`（不足一屏时 `(视高−行数)/2`）与 `contentStart()`（渲染与鼠标命中共用），`render` 按窗口取行；`PgUp`/`PgDn`/滚轮在两个页面都生效。用量页仍是图表，按可用高度铺满（不需要滚动）。
  - 顺带修两个真 bug：① `PgDown` **从未生效**（bubbletea v2 键名是 `pgdown`，原代码写的 `pgdn` 是死分支）；② 居中偏移被当成「跳过内容」（`start = offset + scroll`），内容不足一屏时反而把顶部顶掉。
  - 设置页线框：`Padding(1, 4)`（上下各 1 行、左右各 4 格），`boxChromeW` 4 → 10。
- 测试：新增 `TestBarsNeverExceedWidth` / `TestTabBarDegrades` / `TestStatusDegrades` / `TestSettingsContentCenteredWhenFits` / `TestSettingsContentScrollsWhenTaller` / `TestUsageContentFillsViewWithoutScroll` / `TestShortStatusText`；app 包加 `TestMain` 把 `XDG_CONFIG_HOME` 指到临时目录（**测试不再读真实配置**）；settings 测试改为自然高度口径。

### 验证 14：单测 ＋ tmux 实测 ＋ 装后实测 — PASS

- `go test -count=1 ./...` 全绿；`gofmt -l` 空、`go vet` 干净；pty 冒烟 **27/27 PASS**（宽 100 无回归）。
- tmux 实测（假 key）：
  - **30×12**：顶栏 `[用量] [设置]`（不再被截）；状态栏 `用量 · Q 退出` ＋ `待刷新`（右侧状态保留）；设置页线框完整，`PgDn` 滚到底可见 `平台`/`仓库` 与下边框。
  - **24×10**：顶栏 `[用量] [设置]`；状态栏 `用量 · Q 退出  401`；设置页可滚。线框**右侧仍略被截**（24 宽 < 线框自然宽）——属「方案 C 两段式」范围，用户本次未选。
- 装后实测 `/usr/bin/wl`（30×12）：同上；「关于」显示 `提交 47b8f61`（干净哈希）。
- 提交 `47b8f61 feat: 窄屏适配 A+B；设置页线框左右内边距加到 4 格`；自动安装：`Setting up with-linux (0.1.2-dev)`，`/usr/bin/wl` 与 `dist/pkg/wl-linux-arm64` sha256 一致（`a9925178…`），`dpkg -V` 无差异。
- 遗留（用户 2026-09-28 决定不做、不列入计划）：窄宽两段式排版、极小终端提示（`usage-tui.js` 式文本条降级）。

## 2026-09-28 · 发布 v0.2.0（用户指令）

- 版本决策：`[Unreleased]` 里是**新功能**（设置页 A5）＋ 一批 UI/窄屏修复 ⇒ 按语义化版本发 **0.2.0**（0.1.1 → 0.2.0）。
- 发布前收尾：CHANGELOG `[Unreleased]` → `## [0.2.0] - 2026-09-28` ＋ 底部 tag 链接；PLAN 状态行更新为「M0–M5 完成；遗留仅 403 真实路径」＋ 记录「A1 实机鼠标验证 2026-09-28 完成」「窄屏 C/D 不做」。
- 提交：`f805466 docs: 发布 v0.2.0（CHANGELOG 定版、PLAN 记录 A1 实机验证完成与窄屏 C/D 不做）`。

### 验证 15：发版全链路 — PASS

- 推送：`git push origin main`（`4d690b1..f805466`）；`git tag -a v0.2.0` ＋ `git push origin v0.2.0`。
- CI：`release` workflow（tag `v*` 触发）run **36411738152** → `completed / success`。备注：CI 有 Node.js 20 弃用告警（`checkout@v4`/`setup-go@v5`/`goreleaser-action@v6` 被强制跑在 Node 24），不影响构建产物。
- Release：https://github.com/Lifeni/with-linux/releases/tag/v0.2.0（`draft: false`、`prerelease: false`），产物齐全：`with-linux-linux-{amd64,arm64}.tar.gz`、`with-linux_0.2.0_{amd64,arm64}.deb`、`with-linux_0.2.0_{amd64,arm64}.rpm`、`checksums.txt`。
- 产物校验：下载 `with-linux_0.2.0_arm64.deb` → sha256 `41930652…`，与 Release 里 `checksums.txt` 同值。
- 装机（用**发布产物**，不是本地 `mkdeb.sh`）：`sudo apt install --reinstall ./with-linux_0.2.0_arm64.deb` → `Unpacking with-linux (0.2.0) over (0.1.2-dev)` / `Setting up with-linux (0.2.0)`；`command -v wl` → `/usr/bin/wl`；`wl --version` → `wl 0.2.0`；`dpkg -V` 无差异；`apt-cache policy` 显示 `Installed/Candidate: 0.2.0`。
- 装后实测 `/usr/bin/wl`：pty 冒烟 **27/27 PASS**（「关于」显示 `版本 0.2.0` / `构建日期 2026-09-28` / `提交 f805466`）；tmux 100×26 设置页线框与点阵居中正常；30×12 顶栏降级 `[用量] [设置]`、状态栏双侧保留、设置页 `PgDn` 可滚到底。
- 真实配置未被触碰：`~/.config/with-linux/config.json` 仍 2026-09-25 21:58，md5 `fb8132d2…`。
- 备注：本地 `0.1.2-dev` 的 deb 已被 0.2.0 覆盖；以后本地迭代仍用 `VERSION=... bash scripts/mkdeb.sh`，正式发布一律走 tag ＋ CI。

## 2026-09-28 · 收尾清理（用户指令）

- **删掉仓库根目录的旧二进制 `./with-linux`**（2026-09-25 19:04，5.4MB，`.gitignore` 里，从未入库）：`go version -m` 显示 `path with-linux / mod with-linux (devel)`——是**改模块路径之前**的 v0.1.0 期本地构建，与已装的 `/usr/bin/wl`（0.2.0）内容不同（sha256 `40a43c86…` vs `a3d7138f…`）。留着只会让人手滑跑到旧版，删掉；代码都在 git 里。
- **`scripts/mkdeb.sh` 版本号默认值改进**：`VERSION` 未显式给时，取 `git describe --tags --abbrev=0` 的最近 tag ＋ `~dev`（拿不到 tag 才回退 `dev`）。
  - 用 `~` 而不是 `-dev` 的原因：Debian 版本比较里 `~` 排序**低于**正式版 ⇒ 本地包装过之后，改回正式版**不需要** `--allow-downgrades`。
  - 实测：`bash scripts/mkdeb.sh arm64`（不给 VERSION）→ `Package: with-linux`、`Version: 0.2.0~dev`；`dpkg --compare-versions 0.2.0~dev lt 0.2.0` ✓、`gt 0.1.1` ✓；`VERSION=9.9.9-test` 仍能覆盖 ✓。
- **清 dist 里的旧产物**：删 `with-linux_0.1.2-dev_arm64.deb`（被 0.2.0 取代）与验证用的 `with-linux_9.9.9-test_arm64.deb`；保留当前树的 `with-linux_0.2.0~dev_arm64.deb`。
- **本次没有重装**：这轮只改了打包脚本和文档，**二进制与已装的发布版 0.2.0 完全一致**（当前 HEAD `e337b41` 相对 tag `f805466` 只多了 docs/scripts 提交），所以按「改完自动装」的精神说明：装了也没有可测的新东西，保留发布版 0.2.0；要装 `dist/with-linux_0.2.0~dev_arm64.deb` 随时可装（版本号显示 `wl 0.2.0~dev`）。
- 本机状态：`wl --version` → `wl 0.2.0`（发布产物装的），`dpkg -V` 无差异。

## 2026-09-28 · 审查缺陷修复（M6，用户确认）

- **用量刷新状态机**：`usage.New()` 直接进入初始取数状态；`Init()` 不再丢弃模型更新；只有 tick 消息维护唯一每秒循环。手动刷新和设置保存不再额外创建 tick 链。新增请求代次与 `context` 取消，旧请求的迟到结果不能覆盖新请求。
- **配置诊断与修复**：读取时区分文件缺失、JSON 损坏、文件不可读；JSON 根节点不是对象（包括 `null`）也按损坏处理。设置页修复损坏配置前生成 `.bak`，已有备份递增为 `.bak.1`，无法读取时不会覆盖原路径。
- **细节与 CLI**：倒计时改用 `math.Ceil`；鼠标只响应左键；`wl` 新增 `--help`/`-h`，未知参数退出码 2。
- **CI/发布**：新增 push/PR CI（格式、`go mod tidy` 无 diff、vet、单测、race、双架构交叉编译）；发布前执行固定检查；GoReleaser 不再执行 `go mod tidy`。

### 验证 16：静态检查、单测、双架构构建 — PASS（race 受主机限制）

- `gofmt -l .` 无输出；`git diff --check` 无输出；`go mod tidy` 后 `go.mod`/`go.sum` 无 diff；`go vet ./...` 退出码 0。
- `go test -count=1 ./...`：`cmd/wl`、`internal/app`、`internal/config`、`internal/meta`、`internal/settings`、`internal/usage` 全部 `ok`。
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath ./cmd/wl` 与同参数 `GOARCH=arm64` 均退出码 0。
- `go test -race -count=1 ./...` 在当前 arm64 主机失败：`FATAL: ThreadSanitizer: unsupported VMA range / Found 39 - Supported 48`。所有包启动 race 前即被 TSAN 拒绝，不是测试断言或竞态报告；CI 的 amd64 runner 会继续执行该检查。

### 验证 17：deb 安装 ＋ 已安装程序复验 — PASS

- `env GOCACHE=/tmp/with-linux-go-cache GOTMPDIR=/tmp VERSION=0.2.1~dev bash scripts/mkdeb.sh arm64` → `dist/with-linux_0.2.1~dev_arm64.deb`。
- `sudo apt install --reinstall ./dist/with-linux_0.2.1~dev_arm64.deb` → `Unpacking with-linux (0.2.1~dev) over (0.2.0)` / `Setting up with-linux (0.2.1~dev)`。
- `command -v wl` → `/usr/bin/wl`；`wl --version` → `wl 0.2.1~dev`；`wl --help` 正常；`dpkg -s` → `install ok installed`；`dpkg -V` 无输出。
- `sha256sum /usr/bin/wl dist/pkg/wl-linux-arm64`：两处均为 `50b34824ec36c3164e169e8b50e31dfb494f2053b7877193321b10e61c58a26a`。
- 对已安装的 `/usr/bin/wl` 重跑 `/tmp/wl_smoke.py`：**27/27 PASS**；「关于」显示 `0.2.1~dev / 2026-09-28 / 03e1421-dirty`。
- 真实配置未被触碰：`~/.config/with-linux/config.json` 仍为 2026-09-25 21:58，md5 `fb8132d26135194262078e243b311ce3`。

### 安全事件遗留

- 当前工作树已不再包含已知旧 key 搜索片段；`git grep` 对这两个具体片段无命中。
- `git log --all -S...` 证明旧片段仍存在于历史提交 `dcf866e` 的 `PROGRESS.md`。删除当前行不能清除远端历史。**必须轮换对应 API Key**；是否用 `git filter-repo` 重写历史并 force-push 需用户另行确认，本次未执行破坏性 Git 操作。

## 2026-09-28 · 发布 v0.2.1（用户指令）

- 版本决策：`0.2.0 → 0.2.1`。本次是可靠性修复、CLI 帮助与 CI 完善，没有新增功能，按语义化版本发补丁版。
- 提交：`8f16a91 fix: 修复用量刷新与配置错误处理`；`6e13b12 docs: 发布 v0.2.1`。`main` 已推送到 `6e13b12`。
- 用户已确认轮换密钥。旧 key 片段仍存在于历史提交 `dcf866e`；本次未重写 Git 历史，也未执行 force-push。

### 验证 18：v0.2.1 发版全链路 — PASS

- push CI run **36417248873** → `completed / success`。格式、`go mod tidy` 无 diff、`go vet`、单测、race、`linux/amd64` ＋ `linux/arm64` 构建全部通过。
- `git tag -a v0.2.1 -m "v0.2.1"` ＋ `git push origin v0.2.1`；Release workflow run **36417509467** → `completed / success`（47s）。
- Release：https://github.com/Lifeni/with-linux/releases/tag/v0.2.1（`draft: false`、`prerelease: false`）。产物齐全：`with-linux-linux-{amd64,arm64}.tar.gz`、`with-linux_0.2.1_{amd64,arm64}.deb`、`with-linux_0.2.1_{amd64,arm64}.rpm`、`checksums.txt`。
- 下载 arm64 deb 与 `checksums.txt`：deb sha256 `674f3e92677c6f6e5f00535f42444f849cbb6efedb5203eca165da4a451da8e6`，与 Release 校验文件一致；`dpkg-deb --info` 显示 `Package: with-linux`、`Version: 0.2.1`、`Architecture: arm64`。
- 用 Release 产物执行 `sudo apt install --reinstall`：`Unpacking with-linux (0.2.1) over (0.2.1~dev)` / `Setting up with-linux (0.2.1)`；`/usr/bin/wl --version` → `wl 0.2.1`；`--help` 正常；`dpkg -V with-linux` 无输出；`sha256sum /usr/bin/wl` → `360ca0f65f329655f55265c02e0b25459627afa0dfb6bb8cc0fb52c0c3d01815`。
- 对已安装的 `/usr/bin/wl` 运行 PTY 冒烟：**27/27 PASS**；「关于」显示 `版本 0.2.1`、`构建日期 2026-09-28`、`提交 6e13b12`、`Go go1.27.1`、`平台 linux/arm64`。冒烟使用 `/tmp/wl-smoke/with-linux/config.json`，未读取真实配置。
- Release workflow 仍有 Node.js 20 弃用告警（`checkout@v4` / `setup-go@v5` / `goreleaser-action@v6` 被强制运行在 Node 24），不影响本次产物与验证结果。

## 2026-10-05 · Command Code 用量页（用户指令；范围扩张已确认，验收 A6）

- 指令原文：「我想加两个功能，分别是 commandCode 的用量显示和 mimotokenplan 的用量显示，你先调研一下支不支持」→ 调研后用户：「先做 commandcode」。
- **调研结论**：Command Code 有可用端点（`Authorization: Bearer <key>`）；MiMo TokenPlan 只有小米账号 Cookie 登录态、无 API key，暂缓（见 `FINDINGS.md` §6 与对话记录）。
- 用户拍板：key 来源＝**只用 with-linux 配置文件**新增的 `commandCodeApiKey`；允许只读探测真实端点；用量页**复用三列点阵**。
- **范围扩张**：先改章程（`AGENTS.md`：`A6`、约束「数据获取（Command Code）」、界面布局 tab 栏与设置页），再实现。
- 实现：
  - `internal/config`：加 `CommandCodeAPIKey` 字段 ＋ `SaveCommandCodeAPIKey`；`saveTo` 抽成通用 `saveField(path, field, value)`。
  - `internal/gauge`（新）：三列点阵渲染从 `usage/view.go` 抽成共享包（`Window`/`Panel`/`Render`/`ClampLines`）。
  - `internal/usage`：`view.go` 瘦成「映射 + 调 gauge」，行为不变；纯渲染测试迁到 `gauge` 包。
  - `internal/cmdusage`（新）：`fetch.go`（四端点、容错、重试）、`model.go`（轮询/倒计时/文案，对齐 usage）、`view.go`、`config.go`。
  - `internal/settings`：加 `Command Code Key` 行（同一 textinput 复用，按选中行读写对应字段）；`SavedMsg{CommandCode bool}`。
  - `internal/app`：`tabCmdUsage` 插在中间；标签三个；`SavedMsg` 按标志刷新对应页面；两个用量页同时收 tick。

### 验证 19：单测 ＋ 静态检查 ＋ 双架构 — PASS

- `gofmt -l .` 无输出；`go vet ./...` 干净；`go mod tidy` 后 `go.mod`/`go.sum` 无 diff。
- `go test -count=1 ./...`：`cmd/wl`、`internal/{app,cmdusage,config,gauge,meta,settings,usage}` 全 `ok`。
- `CGO_ENABLED=0 GOOS=linux GOARCH={amd64,arm64} go build -trimpath ./cmd/wl`：两产物 `file` 均 `statically linked`（x86-64 / ARM aarch64）。

### 验证 20：deb 安装 ＋ 装后 pty 冒烟 ＋ 真实 key 实测 — PASS

- `VERSION=0.3.0~dev bash scripts/mkdeb.sh arm64` → `dist/with-linux_0.3.0~dev_arm64.deb`（`Package: with-linux`，内含 `./usr/bin/wl`）。
- `sudo apt install --reinstall ./dist/with-linux_0.3.0~dev_arm64.deb` → `Setting up with-linux (0.3.0~dev)`；`command -v wl` → `/usr/bin/wl`；`wl --version` → `wl 0.3.0~dev`。
- 对已安装的 `/usr/bin/wl` 跑 pty 冒烟（`XDG_CONFIG_HOME` 指向临时目录，不碰真实配置）：**36/36 PASS**（三标签、Command Code 未配置提示、设置页两行 key、Command Code Key 保存写回且不动 apiKey、q 退出码 0）。
- 真实 key 只读实测：Command Code 页渲染 `5h [16%] 1h14m / Week [8%] 6d20h / Month [5%] 30d20h`，状态 `60s 后刷新`；与 `used/cap`、`used/(used+remaining)` 吻合。
- 真实配置未被触碰：`~/.config/with-linux/config.json` 仍为 2026-09-28，md5 `9e895b8f…`。

### 冒烟发现的回归（已修）

- 首次对真实 key 冒烟时 `cmdusage.Model.panel()` 在「已配置 key 但首帧数据未到（`data==nil`）」时**空指针崩溃**。根因：`panel()` 直接解引用 `m.data`（`usage` 版走 `window()` 有空判）。修复：`panel()` 对 nil data 全按无数据；补回归测试 `TestLinesWithKeyButNoData`。修复后重打包重装，冒烟与真实实测复跑通过。

### 后续调整（同日，用户指令）

- 指令：「tab 页都加上用量两个字，然后配置文件的字段区分一下两个提供商」；随后补充「不用考虑向后兼容和旧配置，而是在打开新版时自动迁移配置」。
- tab 标签改为 `OpenCode Go 用量` / `Command Code 用量` / `设置`；`shortName` 改为「取最后一个空格分隔词，重名时退到两个词」→ 两个用量页短名为 `Go 用量` / `Code 用量`；`tabLayoutFor` 的命中区间按终端宽度钳制（极窄终端下单个标签可能比终端还宽）。
- 配置字段按提供商区分：`apiKey` → `openCodeApiKey`（与 `commandCodeApiKey` 并列）；**读路径不做兼容回退**。
- 新增 `config.Migrate()`：启动时把旧 `apiKey` 一次性改写为 `openCodeApiKey` 并删除旧字段（保留未知字段）；无该字段、文件缺失或 JSON 损坏时不动作。`cmd/wl` 的 `runTUI` 在 `app.New` 之前调用它（best-effort，失败不阻断启动）。
- `internal/config`：写入逻辑拆成 `readFields` / `writeFields` / `saveField`，供保存与迁移复用；`Save` 保存 OpenCode key 时顺带删除旧 `apiKey`。

### 验证 21：单测 ＋ 装后冒烟（含迁移端到端） — PASS

- `gofmt -l .` 无输出；`go vet ./...` 干净；`go test -count=1 ./...` 全 `ok`（新增 `TestMigrateLegacyAPIKey`、`TestMigrateNoopWhenMissingOrAlreadyNew`、`TestSaveDropsLegacyAPIKey`、`TestShortNamesAreDistinct`）。
- 重打包 `0.3.0~dev`、`sudo apt install --reinstall`；对已安装 `/usr/bin/wl` 跑 pty 冒烟：**37/37 PASS**（三标签含 `[Command Code 用量]`、**旧 `apiKey` 启动自动迁移为 `openCodeApiKey`**、设置页两行 key、Command Code Key 写回且不动 OpenCode key、`q` 退出码 0）。
- 真实配置未被触碰（冒烟用临时 `XDG_CONFIG_HOME`）。

### 发布 v0.3.0（2026-10-05，用户指令）

- 版本决策：本次是新增功能（Command Code 页、配置字段改名＋自动迁移）→ 按语义化版本 `0.2.1` → **`0.3.0`**（minor）。
- 发布前收尾：README 功能段加 Command Code；`scripts/deb-control.in` 描述改为两个用量页；CHANGELOG `[Unreleased]` → `## [0.3.0] - 2026-10-05` ＋ 底部 tag 链接；PLAN 状态更新为「M0–M7 完成并发布 v0.3.0」；`.gitignore` 加 `.commandcode/`（AI 助手本地状态）。
- 提交：`5cd4843 feat: 新增 Command Code 用量页，配置字段按提供商区分（A6）`（28 files changed, +2212 −612）；`git push origin main`（`a242151..5cd4843`）。
- 打 tag：`git tag -a v0.3.0 -m v0.3.0` ＋ `git push origin v0.3.0`。

### 验证 22：发版全链路 — PASS

- push CI run **37320894422** → `completed / success`（格式、模块文件、vet、单测、race、双架构交叉编译）。
- Release workflow run **37321182644**（`v0.3.0`）→ `completed / success`（1m5s）。
- Release：https://github.com/Lifeni/with-linux/releases/tag/v0.3.0（`draft=false`、`prerelease=false`）。产物齐全：`with-linux-linux-{amd64,arm64}.tar.gz`、`with-linux_0.3.0_{amd64,arm64}.deb`、`with-linux_0.3.0_{amd64,arm64}.rpm`、`checksums.txt`。
- 下载 `with-linux_0.3.0_arm64.deb`：sha256 `29ab56c0…`，与 `checksums.txt` 同值（MATCH）；`dpkg-deb --info` → `Package: with-linux` / `Version: 0.3.0` / `Architecture: arm64`。
- 用发布产物 `sudo apt install --reinstall`：`Unpacking with-linux (0.3.0) over (0.3.0~dev)` / `Setting up with-linux (0.3.0)`；`command -v wl` → `/usr/bin/wl`；`wl --version` → `wl 0.3.0`；`dpkg -V` 无输出。
- 对已安装的 `/usr/bin/wl` 跑 pty 冒烟：**37/37 PASS**；「关于」显示 `版本 0.3.0` / `构建日期 2026-10-05` / `提交 5cd4843`。
- 备注：`gh auth status` 报 token invalid，但 git push（经 `gh auth git-credential`）与 `gh run` / `gh release` 查询均正常，属误报，未影响本次发布。

### 后续调整（2026-10-06，用户指令）

- 指令：「在设置页面增加两个开关，是否展示这两个页面，就在每个 key 的前面增加选项即可，默认是都关闭的，如果用户填了对应 key，则自动打开一次，但如果后续用户关闭了，就不自动打开」；布局用户选「每行 key 前加勾选框（同行）」——`空格` 切换开关、`回车` 编辑 key。记为 `A7`（属范围扩张）。
- `internal/config`：新增 `ShowOpenCode` / `ShowCommandCode`（`*bool`，nil = 从未显式设置，区别于显式 false）；`saveField` 泛化为 `saveValue(path, field, value, drop...)`，bool 与 key 共用同一条读写路径（保留未知字段、`.bak` 备份语义不变）；新增 `SaveShowOpenCode` / `SaveShowCommandCode`。
- `internal/settings`：key 行首加勾选框（`[✓]`/`[ ]`，两态同宽 4，`checkWidth` 由 runewidth 算）；新增 `Visibility()`（显式开关优先，nil 时按 key 是否存在推导）与 `TogglesChangedMsg`；`ClickLine` → `Click(x, line, width)`：点勾选框切开关、点行内其余处编辑，线框水平居中故按 `boxLeft` 定位勾选框列。
- `internal/app`：顶栏按可见性增删 tab；`←`/`→`、`1`–`9`、短名去重只在**可见**标签间；当前页被关闭时退回「设置」；`SavedMsg` 后重算可见性（填 key 可能让用量页自动出现）；`TogglesChangedMsg` 直接驱动可见性。

### 验证 23：单测 ＋ 装后冒烟（A7） — PASS（race 受环境限制，见下）

- `gofmt -l cmd internal` 无输出；`go mod tidy` 无 diff；`go vet ./...` 干净；`go test ./...` 全 `ok`。新增用例：config（开关写回/缺省 nil/保留其他字段）、settings（默认关闭、填 key 自动打开、显式关闭不重开、空格切换写回、勾选框点击命中、两态同宽）、app（无 key 默认隐藏且落在设置、保存 key 自动出现 tab、关闭当前页退回设置、隐藏页不可键盘切换）。
- 双架构交叉编译 `GOOS=linux GOARCH={amd64,arm64} go build ./...` 通过。
- `go test -race ./...` 在本机 TSAN 初始化即失败（`ThreadSanitizer: unsupported VMA range`），未改动的包（`meta`/`gauge`）同样失败 → 环境限制，非本次改动。
- 重打包 `0.3.1~dev`、`sudo apt install --reinstall`；对已安装 `/usr/bin/wl` 跑 pty 冒烟（一律用临时 `XDG_CONFIG_HOME`，真实配置未触碰）：
  - 空配置 → 顶栏只有「设置」，两个 key 行均 `[ ]`（默认关闭）。
  - 有 `openCodeApiKey` → 顶栏出现 `Go 用量`、无 `Code 用量`；进设置页显示 `[✓] OpenCode Key`（填 key 自动打开）。
  - `showOpenCode=false` ＋ 有 key → 无 OpenCode tab、勾选框 `[ ]`（显式关闭后不再自动打开）。

### 修复：100% 用量点阵未填满（2026-10-06，用户反馈 bug）

- 现象：用量到 100% 时点阵整列看着没满，顶部几行稀疏。排查：`litRow` 的顶部 ramp（渐变段 `clamp(round(H*0.16),4,16)`）固定在填充区顶部，100% 时被削成稀疏——与旧 `usage-tui.js` 行为一致，非移植偏差，属设计导致的观感问题（矮终端 ramp 最小 4 行，100% 看着只有六七成满）。
- 改法：`internal/gauge` 中当 `math.Round(w.Percent) >= 100`（与标签取整一致，`≥99.5%` 即显示 `[100%]`）时把该列 `ramp` 置 0，整列实心；`<100%` 的渐变行为不变。有意偏离旧实现，已记 `FINDINGS.md §1.4`、`CHANGELOG.md [Unreleased].Fixed`。
- 验证 24 — PASS：
  - `gofmt -l cmd internal` 无输出；`go mod tidy` 无 diff；`go vet ./...` 干净；`go test ./...` 全 `ok`。
  - 新增用例：`TestFullPercentColumnHasNoDimDots`（多宽度/高度下 100% 全列无暗点）、`TestRampCancelsAtRoundedFull`（99.5%→无暗点、99.4%/98%/50%→仍有暗点）。渲染实测（height 12/20/30）：100% 列整列 `●`，50% 列保留渐变。
  - 双架构交叉编译 `GOOS=linux GOARCH={amd64,arm64} go build ./...` 通过。
  - `VERSION=0.3.1~dev bash scripts/mkdeb.sh arm64` → `sudo apt install --reinstall` → `Setting up with-linux (0.3.1~dev)`；`/usr/bin/wl` 与 `dist/pkg/wl-linux-arm64` sha256 一致（`bf71a4e4…`）；`dpkg -V` 无输出；`wl --version` → `wl 0.3.1~dev`。
  - 对已安装 `/usr/bin/wl` 跑 pty 冒烟（临时 `XDG_CONFIG_HOME`，真实配置未触碰）：顶栏三标签、「关于」显示 `0.3.1~dev`、key 掩码与配置路径正常、假 key 得 401 不崩溃；5/5 PASS。

### 修复：设置页「关于」用例的脆弱断言（2026-10-11）

- 现象：本机 `go test ./...` 里 `internal/settings.TestAboutShowsBuildInfo` 失败。它断言「完整配置路径出现在 width 120 的渲染中」，而配置文件行按可用宽截断：`avail = 120 − boxChromeW(10) − gutterW(2) − labelW(20) − 2 = 86`。远程工作区下 `t.TempDir()` 落在 `/run/user/1000/qoder-remote-worker-*/…`，路径 101 格被 `runewidth.Truncate(..., "…")` 截掉 → `Contains` 必然失败。判据：`TMPDIR=/tmp/tp`（短路径）跑同一用例即 `ok`；与验证 24 的 gauge 改动无关（HEAD 上同样红），CI 的 amd64 runner `/tmp` 短，所以一直是绿的。
- 改法：只动用例——渲染宽度按完整路径反推 `max(120, runewidth.StringWidth(m.configPath)+40)`（40 = 线框/选择标记/标签列/间距 34 ＋ 余量）。产品行为不变：窄屏截断是章程要求的（「窄终端先截宽，避免折行」），错的是「路径一定容得下 120」这个断言前提。
- 验证 25 — PASS：
  - 本机长 TMPDIR（原失败环境）与 `TMPDIR=/tmp/tp` 短路径下，`go test ./internal/settings/ -count=1` 均 `ok`；`go test ./... -count=1` 八个包全 `ok`。
  - `gofmt -l cmd internal` 无输出；`go vet ./...` 干净；`go mod tidy` 无 diff。
  - 双架构交叉编译 `GOOS=linux GOARCH={amd64,arm64} go build ./...` 通过。
  - `go test -race ./...` 本机仍是 `FATAL: ThreadSanitizer: unsupported VMA range`（本次未改动的 `meta`/`gauge` 同样失败）→ 环境限制，同验证 16/23；CI runner 继续执行该检查。
  - 本次仅改测试文件，二进制无变化 → 不重装 deb（`/usr/bin/wl` 仍为 `0.3.1~dev`）；`CHANGELOG.md` 不加条目（非用户可见变更）。

## 2026-10-11 · M8 第 1 步：章程与调研文档（系统状态页 ＋ 界面结构重构）

- 用户两轮指令：① 「类似 btop 的系统监视功能，不需要选择应用操作，只要有监控图表和数据；需要 CPU、内存、网络、磁盘」；② 「去掉上方 tab 栏；把两个提供商的用量合并进系统状态页、展示小一点；整个界面的点阵统一成 btop 那种；设置页改成快捷键或右上角按钮打开，进去后可选两个提供商是否展示；不展示时空位由其他监控信息自动填充、尺寸自适应」。**属范围扩张＋章程重写，已逐条确认**。
- 拍板的决定（AskUserQuestion）：braille 曲线／附加指标＝温度＋负载＋运行时长＋每核占用＋Swap／**行式网格**布局／用量用**横向 braille 计量条**／设置入口＝右上角按钮 ＋ 快捷键 `s`／设置页只有 `Esc` 返回、`q` 仍退出程序／1 秒采样、保留 2 分钟历史。**第一轮定的 `showMonitor` 开关被第二轮作废**（监控页成为唯一主页面，无从隐藏）。
- 只读调研（写进 `FINDINGS.md §7`）：目标机 RK3588／8 核／7.5 GiB／NVMe；`/proc/{stat,loadavg,uptime,meminfo,net/dev,net/route,diskstats,self/mounts}` 与 `/sys/class/thermal`（7 个 zone）全部可用；实测确认四个坑——内存必须用 `MemAvailable`（本机 `MemFree` 只有 303 MB／7.5 GB，用错会显示 96%）、12 个网卡里只有 1 个物理网卡（主网卡按 `/proc/net/route` 的默认路由选）、`diskstats` 要滤掉 loop/ram/mtdblock 且分区不重复计、挂载点要按 `st_dev` 去重（本机 `/var/log.hdd` 与 `/` 同设备）；`diskstats` 的 util 字段是**第 13 列**（先前 PLAN 里误记为第 10 列，已改正）。
- 字符宽度实测：braille／方块／半块字符在 `runewidth@v0.0.27` 与 `lipgloss/v2@v2.0.6` 下每字符宽度均为 1、非 ambiguous → 现有窄屏截断逻辑可直接复用（`FINDINGS.md §7.5`）。
- `PLAN.md`：新增 M8 段（M8.0 调研表／M8.1 新界面结构样张／M8.2 包结构（新增 `sysstat`、`chart`、`grid`、`monitor`，删除 `gauge`，`usage`/`cmdusage` 去渲染化，`app` 缩成两视图切换器）／M8.3 行式网格与自动填充规则／M8.4 九步实施顺序／M8.5 完成定义／M8.6 章程 14 条改动清单／M8.7 九项风险），状态行标注 M8 为提案。
- `AGENTS.md`：按 M8.6 的 14 条改完——§1 一句话目标（标题栏＋网格＋状态栏，tab 结构作废）／定位（改为「以系统状态仪表盘为主页面的工具箱」）／扩展性（新工具默认作为系统状态页的面板）／配置（`showOpenCode`/`showCommandCode` 语义改为控制面板，采样间隔不进配置）；非目标删两条（系统状态、一屏总览页）；**A1 重写**、**A2/A6 重写**（「三列点阵、与旧实现展示格式一致」作废）、A3/A4/A5/A7 改写、**新增 A8（系统状态）与 A9（布局自适应）**；约束新增「数据获取（系统监控）」「系统状态页只读、不采集进程」「采样固定 1 秒」；界面布局整段重写（标题栏含温度/运行时长/负载与可点击设置按钮、三行网格、统一 braille 画法与旧点阵作废、自动量程、全局键、无动画条款重写）；设置页改入口与返回键、勾选框语义改为「控制该提供商面板是否在系统状态页展示」。
- `FINDINGS.md`：新增 §7（7.1 CPU/负载/运行时长、7.2 内存与 Swap、7.3 网络、7.4 磁盘与温度、7.5 字符宽度与 braille 画法、7.6 采样开销），逐字段映射 ＋ 本机实测数值 ＋ 过滤规则。
- 本步**只改文档、不动代码**，故不重打包、不装 deb（`/usr/bin/wl` 仍为 `0.3.1~dev`，行为未变）；`CHANGELOG.md` 待代码落地时一并加条目。

### 验证 26：文档改动范围与代码树未受影响 — PASS

- `git diff --stat` → 只有 `AGENTS.md`（+79/−40 行区间内改动）、`FINDINGS.md`（+60）、`PLAN.md`（+137）三个文件，共 `3 files changed, 236 insertions(+), 40 deletions(-)`；`git status --short` 无未跟踪文件。
- `gofmt -l cmd internal scripts` 无输出；`go build ./...` OK；`go vet ./...` OK；`go test ./...` 八个包全 `ok`（`cmd/wl`、`app`、`cmdusage`、`config`、`gauge`、`meta`、`settings`、`usage`）→ 证实本步没碰代码。
- 敏感信息扫描（工作流第 9 条）：`AGENTS.md`/`FINDINGS.md`/`PLAN.md` 中家目录、主机名、私网地址均无命中；`\bpi\b` 唯一命中是既有的第三方包名 `pi-commandcode-provider`（非主机名）；`you` 的命中全部来自标识符 `tabLayoutFor` 里的 `la-you-t`（假阳性）。M8.1 样张里的主机名占位写成 `<主机名>`/`host`，未落真实值。
- 残留 `tab`／`顶栏`／`三列点阵` 字样已逐条核对：`AGENTS.md` 里剩下的 8 处全是**历史标注或作废说明**（如「原『顶部工具 tab 栏』结构作废」「取代原顶部 tab 栏」），无与新结构矛盾的活条款。
- 未做：`go test -race`（本机 TSAN 仍受 VMA 限制，同验证 16/23/25）；本次无代码变更，交由 CI 在后续代码步骤覆盖。

## 2026-10-12 · M8 第 2 步：`internal/sysstat` 采集包（系统状态数据源）

- 新增 `internal/sysstat`（7 个源文件 838 行 ＋ 7 个测试文件 1101 行、47 个用例）：`Collector{Root}` 复用并保存上次原始计数，`Read()` 一次采样。CPU（`/proc/stat` 求差、guest/guest_nice 不重复计、核数按 `cpuN` 行数、离线核按 0% 记以保住核号对齐）、负载、运行时长、内存与 Swap（**used = MemTotal − MemAvailable**；`MemAvailable` 缺失退回 `MemFree+Buffers+Cached`，连 `MemFree` 都没有就不猜）、网络（名字过滤 ＋ `operstate` 二次过滤 ＋ 默认路由选主网卡 ＋ 计数器回绕按 0）、磁盘（只留整盘设备、扇区 ×512、util 用第 13 列并钳到 100）、挂载点（fstype 白名单 ＋ `st_dev` 去重 ＋ 八进制转义解码）、温度（优先 type 含 soc/cpu 的 zone，符号链接 zone 也能读）。**单项失败 → `HasData=false`，不是错误**；只有 `Root/proc` 不存在才返回 error。
- 测试留了两个缝：`Collector.statfs`（fixture 里的挂载点在真实文件系统上不存在）与 `Collector.now`（固定两次采样间隔，速率断言不必用容差）。fixture 形状取自目标机实测：12 个网卡（含 docker/veth/br 噪声）、9 行 diskstats（含 loop/ram/zram/mtdblock/dm-/分区）、7 行 mounts（含同设备 bind mount 与带 `\040` 的路径）、7 个 thermal zone。
- 实现中改正了两条调研结论（已回写 `FINDINGS.md §7.3/§7.4`）：① `/proc/net/route` 的 **Metric 是十进制**（Destination/Gateway/Mask 才是 `%08X` 十六进制），按十六进制解析会在多条默认路由时选错网卡；② `/var/log`（`zram1`，fstype `ext4`）**会**被白名单保留——先前 FINDINGS 写「不在白名单，自然滤掉」是错的（白名单只看 fstype）。
- 磁盘百分比口径改成 df 的 `used/(used+avail)`：`used/total` 会系统性偏低（ext4 默认给 root 保留 5%），磁盘快满时差得最多，而用户就是拿 df 对数的。渲染时对磁盘百分比向上取整以对齐 df（其余指标四舍五入），已记进 FINDINGS §7.4。

### 验证 27：单测 ＋ 真实对数 ＋ 静态检查 — PASS（race 受环境限制）

- `gofmt -l cmd internal scripts` 无输出；`go mod tidy` 后 `git diff --exit-code -- go.mod go.sum` 无 diff（**零新依赖**）；`go vet ./...` OK；`go test -count=1 ./...` 九个包全 `ok`（`internal/sysstat` 47 个用例）。
- 双架构交叉编译 `CGO_ENABLED=0 GOOS=linux GOARCH={amd64,arm64} go build -trimpath ./cmd/wl` 均 OK（`syscall.Stat_t.Dev` 与 `Statfs_t.{Bsize,Blocks,Bfree,Bavail}` 两边字段名一致）。
- `go test -race ./internal/sysstat/` 本机仍 `FATAL: Found 39 - Supported 48`（TSAN/VMA 环境限制，同验证 16/23/25），交 CI 跑。
- **真实对数**：临时探针 `WL_REAL=1 go test -run TestRealMachineProbe -v`（对真实 `/proc` 采两次样、间隔 2 秒），**跑完已删除该文件、未入库**。

  | 指标 | 采集器输出 | 系统命令 | 结论 |
  | --- | --- | --- | --- |
  | 核数 | 8 | `nproc` → 8 | 一致 |
  | CPU | 总 5.1%、每核 `[9 2 4 6 13 4 2 2]` | `/proc/loadavg` → 0.55 0.51 0.57 | 量级相符（低负载） |
  | 运行时长 | 530h41m33s | `uptime -p` → up 3 weeks, 1 day, 2 hours, 41 minutes | 一致（22 天 2h41m） |
  | 内存 | total 7.50G / used 4.36G / avail 3.14G / cache 2.91G → 58.2% | `free -h` → 7.5Gi / 4.4Gi / 3.2Gi / 2.9Gi | 一致（`MemFree` 只有 389Mi，验证了必须用 `MemAvailable`） |
  | Swap | 3.75G / 1.59G → 42.4% | `free -h` → 3.8Gi / 1.6Gi | 一致 |
  | 网络 | 12 个接口过滤后只剩 `enp1s0`（primary=true），累计 ↓32.84G ↑21.98G | `ip -s link show enp1s0` → RX 35262736858 / TX 23599851846 字节 | **完全一致** |
  | 磁盘 I/O | 只剩 `nvme0n1`（分区/loop/ram/zram/mtdblock 全滤掉），util 1.0% | `iostat -dx 1 2 nvme0n1` 末列 `%util` → 1.00 | **完全一致**（含 util 口径） |
  | 挂载点 | `/` 21.17G/231.37G 9.2%、`/boot` 0.18G/1.00G 17.7%、`/var/log` 0.06G/0.18G 34.0%（bind mount `/var/log.hdd` 已去重） | `df -h` → 232G/22G/10%、1022M/181M/18%、188M/59M/35% | 绝对值一致；百分比差 1 是 df 向上取整（见上） |
  | 温度 | 39.769 °C（soc-thermal） | `cat /sys/class/thermal/thermal_zone0/{type,temp}` → `soc-thermal` / `39769` | 一致 |

- 本步只加新包、尚未接线（`internal/app` 仍是旧 tab 结构，`internal/gauge` 仍在），二进制行为不变 → 不重打包、不装 deb。

## 2026-10-12 · M8 第 3 步：`internal/chart` 渲染原语 ＋ 样张过目

- 新增 `internal/chart`（5 个源文件 540 行 ＋ 5 个测试文件 653 行、34 个用例）：
  - `Canvas`：braille 画布（每格 2 列 × 4 行点，y 轴自下而上）。`Plot(values, slots, min, max, series)` 在相邻样本间线性插值，画出来是连续曲线而不是一串孤点；**最新样本贴右边缘、样本间距按 slots（历史环容量）算**，所以刚启动时曲线从右侧生长，不会被拉伸成「看起来已经跑满 2 分钟」。每格记住自己属于哪条序列，双序列（网络 ↓↑、磁盘读写）各染各的色、互不覆盖。
  - `MeterH`（横向 3 态：空 / `⡇` 半 / `⣿` 满，每格 2 级）与 `MeterV`（竖向每行 4 级 `⣀⣰⣼⣿`，每核一条）；**非 0 值至少亮半格/一级**，否则 0.4% 和 0% 长得一样。
  - `Frame`：圆角线框、标题嵌在上边框左侧、当前值靠右，输出恰好 height 行 × width 格；宽度不够时先丢值、再截标题（带 `…`）、极窄只留横线。
  - `FitFields`（超宽时**从右边成对丢字段**，不是截断）、`ClampLines`/`clampTo`/`padTo`、`PctStyle`（≥85 红 / ≥60 琥珀 / 其余蓝，与原三列点阵同一套阈值）。
  - 格式化：`Bytes`（<1024 带 `B` 单位）、`Rate`、`Pct`（四舍五入）、`PctCeil`（只有磁盘用，对齐 df）、`Dur`、`Temp`、`Load`、`NiceCeil`/`AutoMax`（自动量程圆整到 1/2/5×10ⁿ）。
- **样张**：临时探针 `WL_SAMPLE=1 go test -run TestSamplePage`，用**真实** `/proc` 读数 ＋ 模拟历史曲线渲了 4 种尺寸（100×28 全开、100×28 两个用量面板都关、60×24、100×14）；跑完已删、未入库。结论：**braille 字形在用户终端正常显示**（曲线、计量条、线框都对），线框不折行；开 4 个 `yes` 制造负载后 CPU 总占用 51–52%、每核显示 4 条 `⣿` ＋ 4 条 `⣀`、温度从 39 °C 升到 50 °C，与负载相符；关掉两个用量面板后上面四块自动变高、不留空白（用户第 5 点）。
- 样张暴露的 4 个问题已回写 `PLAN.md M8.3` 第 9–12 条：① 自动量程要有下限（否则 ↓249 B/s 的空闲链路被放大成满屏曲线，看着像出事）；② 行高要按面板实际需要收敛（用量行只需 5 行，按权重分到 6 行就在底部留空白）；③ 宽度不足时丢字段而不是截数字（样张里出现过 `21G/231G 10`，百分号被截掉比整段不显示更糟）；④ `Bytes` 对 <1024 要带单位（否则「量程 500/s」看不出是字节）。
- 另修一处线框缺陷：标题后少一个空格（`╭─ CPU───` 应为 `╭─ CPU ───`）。

### 验证 28：单测 ＋ 静态检查 ＋ 双架构 ＋ 样张 — PASS（race 受环境限制）

- `gofmt -l cmd internal scripts` 无输出；`go mod tidy` 后 `go.mod`/`go.sum` 无 diff（**零新依赖**）；`go vet ./...` OK；`go test -count=1 ./...` 十个包全 `ok`（`chart` 34 例、`sysstat` 47 例）。
- 双架构交叉编译 `CGO_ENABLED=0 GOOS=linux GOARCH={amd64,arm64} go build -trimpath ./cmd/wl` 均 OK。
- `go test -race` 本机仍 `FATAL: Found 39 - Supported 48`（TSAN/VMA 环境限制，同验证 16/23/25/27），交 CI 跑。
- 样张已贴给用户过目；**字形是否可接受由用户拍板**，若终端字体缺 braille 字形，按 `PLAN.md M8.7-1` 只换字形映射表退回方块字符。
- 本步仍只加新包、未接线，二进制行为不变 → 不重打包、不装 deb。

## 2026-10-12 · M8 第 4 步：`internal/grid` 行式网格布局

- 新增 `internal/grid`（280 行 ＋ 309 行测试、11 个用例）：**纯几何、不依赖 lipgloss**，所以「不留空白」这件事能用位图证死。`Layout(width, height, rows) []Placement`，每块面板给一个矩形；`Panel` 有三个高度：`MinH`（完整形态）、`FloorH`（退化成「标题＋1 行数值」的下限）、`WantH`（内容够用就行，0 = 不设上限），加一个 `Weight`。
- 规则实现（章程 A9 / PLAN M8.3）：空行删掉；行内均分宽度、余数从左到右 +1（右侧不留空洞）；`width < 60` 拆成单列堆叠；行高先按 `FloorH` 保底，再按权重用**最大余数法**长到 `MinH`，然后按 `WantH` 收敛（用量行只要 5 行，多分的还回去），最后仍有剩余就按权重摊掉，保证 `sum(heights) == height`；连 `FloorH` 之和都超过 `height` 才返回 `nil`（调用方进紧凑模式）。最大余数法的余数比较用整数、同余数按序号小的优先，所以结果确定、不依赖 map 迭代顺序。
- 高度不够时行高自然落在 `FloorH` 与 `MinH` 之间（面板自己按高度降级），不需要「先按 MinH 试一遍再按 FloorH 试一遍」两趟。

### 验证 29：单测 ＋ 静态检查 ＋ 双架构 — PASS（race 受环境限制）

- `gofmt -l cmd internal scripts` 无输出；`go mod tidy` 无 diff（零新依赖）；`go vet ./...` OK；`go test -count=1 ./...` 十一个包全 `ok`；双架构 `CGO_ENABLED=0` 交叉编译 OK。
- `go test -race ./internal/grid/` 本机仍 `FATAL: ThreadSanitizer: unsupported VMA range` / `Found 39 - Supported 48`（同验证 16/23/25/27/28，环境限制），交 CI 跑。
- 关键用例：
  - `TestLayoutFillsViewportExactly`：7 种尺寸（120×40、100×28、80×24、60×20、60×14、100×12、61×30）× 3 种可见性组合，用**占用位图**逐格断言「不越界、不重叠、空洞数恰好等于面板之间的 `ColGap` 间隙」——21 组全过。这就是「关掉面板后空位被吃掉、不留空白」的硬证据。
  - `TestHiddenProviderReflowsWithoutHoles`：关一个提供商 → 那块占满整行宽（100 格）、行高不变（行还在）；关两个 → 整行删除，CPU 行变高，且 CPU 底边与网络顶边严格相接（中间不会留空行）。
  - `TestWantHReclaimsUnusedHeight`：20 行高里用量行拿 5 行、CPU 行吃掉其余 15 行（样张 A 底部那 3 行空白的修法）。
  - `TestSingleColumnBelowThreshold`：宽 59 → 6 块各自独占一行且从上到下排列；宽 60 → 仍是两块一行。
  - `TestDegradesBetweenFloorAndMin`：高 12 喂不饱 `MinH`（7+6+5=18）但够 `FloorH`（9）→ 行高落在两者之间且和为 12。
  - `TestNilWhenEvenFloorDoesNotFit`：59×12（单列 6 块 ×3=18）与 100×6 都返回 `nil` → 调用方进紧凑模式。
  - `TestLayoutIsDeterministic`：同一输入连跑 20 次结果逐字节相同。
- 本步仍只加新包、未接线，二进制行为不变 → 不重打包、不装 deb。

## 2026-10-12 · M8 第 5 步：`internal/monitor` 系统状态页

- 新增 `internal/monitor`（`monitor.go` 269 行 ＋ `view.go` 509 行 ＋ 测试 538 行、21 个用例）：
  - **状态与生命周期**：`Collector` 每秒采样（包内私有 `tickMsg`，与 usage/cmdusage 的 tick 类型不同，三条链互不串台）；`history` 六条环形缓冲（cpu/mem/rx/tx/read/write，各 120 点）；**没有数据的采样不入队**，曲线从右侧生长而不是画出假的 0；采样**同步**做（一次就是几个小文件、微秒级），不开 goroutine 是为了采集器的「上次计数」没有竞争（CI 跑 `-race`）；整体采样失败时**保留上一屏数据**、状态栏报错，不整屏变空。
  - **子模型**：持有 `usage.Model` 与 `cmdusage.Model`，非 tick 消息一律转发（轮询、倒计时、`R` 刷新都在各自包里，一行没改）；`Refresh(commandCode bool)` 给框架在设置页保存后调用；`SetVisibility(oc, cc)` 给框架在勾选框切换后调用。
  - **渲染**：`Lines(w,h)` → `grid.Layout` → 逐面板渲染 → `compose` 按矩形拼屏（同行横向拼接、行间纵向相接）。六个面板各自按高度降级：CPU（曲线 → 每核竖条 → 核数行；核数太多时按可用宽度截断并标「+N 核未显示」）、内存（曲线 → 缓存/可用 → Swap 计量条）、网络（双序列曲线 ＋ 量程 ＋ 累计）、磁盘（曲线先砍、挂载点后砍、`/` 永远保留）、两个用量面板（三窗计量条 → 单行摘要取百分比最高的一窗）。`grid` 返回 nil 时进**紧凑模式**：每面板 1 行数值、无线框、返回自然高度交框架滚动。
  - **状态栏**：可见提供商有问题（缺配置/401/403/网络）时**优先**显示它，配置类文案带完整配置路径（改写后的 A4）；否则 `CPU 12% · 内存 58%`；采集整体失败显示「系统状态不可用」（严重度 2）。
- `internal/chart` 补 `Countdown`（从即将删除的 `gauge` 移过来，格式不变：`--`/`now`/`3d11h`/`3h11m`/`15m`），并把 `clampTo`/`padTo` 导出为 `ClampTo`/`PadTo`（拼屏要用）。
- `internal/usage` 与 `internal/cmdusage` 各加一个 `snapshot.go`：导出 `Labels` 与 `Snapshot()`（三窗指针 ＋ 面板内短提示 `Notice` ＋ 状态栏文案/短文案/严重度）。**纯新增**，两个包的 `view.go` 与轮询状态机一行没动，所以 `internal/app` 仍能编译（删除留到第 6 步）。
- 自动量程下限按第 3 步的结论落地：网络 1 KiB/s、磁盘 64 KiB/s；实测空闲机器上磁盘显示 `读0B/s 写0B/s` ＋ 一条贴底直线，而不是被放大成满屏曲线。

### 验证 30：单测 ＋ 真实预览 ＋ 静态检查 — PASS（race 受环境限制）

- `gofmt -l cmd internal scripts` 无输出；`go mod tidy` 无 diff（零新依赖）；`go vet ./...` OK；`go test -count=1 ./...` **十二个包全 `ok`**（monitor 21 例、grid 11 例、chart 35 例、sysstat 47 例）；双架构 `CGO_ENABLED=0` 交叉编译 OK；`-race` 本机仍 `FATAL: ThreadSanitizer: unsupported VMA range`（同验证 16/23/25/27/28/29），交 CI。
- 关键用例：
  - `TestLinesNeverWrap`：10 种宽（20–200）× 9 种高（1–40）× 4 种可见性 = **360 组**，断言每行可见宽度不超过终端宽度；图表模式下还断言**恰好等于**终端宽度（短了右侧会留空洞）。
  - `TestChartModeFillsHeight`：100×28／80×24／120×40／60×20 都恰好铺满高度。
  - `TestCompactModeWhenNothingFits`：59×12 → 6 行纯数值、无线框、CPU/内存/网络/磁盘/两个提供商都在。
  - `TestHiddenProviderIsAbsent`：关掉的面板不渲染；整行关掉后仍铺满 28 行、底部没有空行堆着。
  - `TestPageShowsSampledValues`：页面里能读到 `42%`、`4.0G/8.0G`、`网络 enp1s0`、`↓50K/s`、`累计 ↓35G ↑23G`、`磁盘 nvme0n1`、`21G/231G 10%`（df 口径向上取整）、`每核`、`最高 100%`、`8 核`。
  - `TestNoDataPage`：什么都没采到时不 panic、显示 `[--]`、不出现假数值。
  - `TestUsagePanelWindowsAndDegradation`：完整形态三窗齐全；高度不够退成 `5h 62% · 3h11m` 单行摘要；宽 22 时仍保住 `62%`（不把数字截一半）；无数据的窗显示 `[--]`；出错时面板内显示 `Key 无效（401）`。
  - `TestDiskPanelAlwaysKeepsRoot`：高度只剩 1 行时留的是 `/`，`/boot` 被砍。
  - `TestStatusTextPrefersProviderProblems`：缺 key 时状态栏是「未配置 Key：…/config.json」（含路径）、短状态 `未配置`；关掉两个提供商后回到 `CPU 42% · 内存 50%`。
- **真实预览**（临时探针 `WL_PREVIEW=1`，真实采集 ＋ 回填历史；跑完已删、未入库）：100×28（两个提供商都开／都关）、80×24、59×20（单列堆叠 6 块）、59×12（紧凑模式 6 行）五种形态都正常，线框对齐、无折行；空闲时网络 `量程 1.0K/s`、磁盘 `量程 64K/s` 下限生效。
- 测试自己抓到一个判别错误：原本用「行数 == height」判断图表模式，但紧凑模式也可能恰好出 h 行（20×4 就是），已改成直接问 `grid.Layout(...) != nil`。
- 本步仍未接线（`internal/app` 还是旧 tab 结构、`internal/gauge` 还在），二进制行为不变 → 不重打包、不装 deb。

## 2026-10-12 · M8 第 6 步：界面切换（app 改两视图、用量包去渲染化、删 `internal/gauge`）

- `internal/app` 重写为**两视图切换器**：标题栏（第 0 行）＋ 内容区 ＋ 状态栏。删掉 tab 那一整套（`tabSpan`/`tabPlan`/`tabPlanFor`/`tabLayoutFor`/`stepTab`/`visibleTabs`/`shortName`/`lastWords`/`renderTabBar`/`tabLabel`、`1`–`9` 直达、`←`/`→`/`Tab` 切换、`toolNames`/`active`/`showOpenCode`/`showCommandCode`）。新增：`s`/`S` 与右上角按钮进设置页、非编辑态 `Esc` 返回；标题栏左侧 `with-linux · 主机名 · 温度 · 已运行 X · 负载`（后三项来自 `monitor.Title()`，主机名 `os.Hostname`，取不到时退 `localhost`），逐级降级顺序＝负载 → 运行时长 → 主机名 → 程序名、**温度保留到最后**；按钮降级 `[ 设置 s ]` → `[ 设置 ]` → `[s]`，命中区间与渲染共用同一份计算（按钮右对齐贴最右列）。`SavedMsg` 改为转成 `monitor.Refresh(commandCode)`，`TogglesChangedMsg` 改为 `monitor.SetVisibility`。
- 标题栏降级抽成**纯函数** `titleFields(base, avail)`：不依赖本机有没有 thermal zone，CI runner 上也能测（否则这类断言只能在目标机上跑）。
- `internal/usage` / `internal/cmdusage` **去渲染化**：删掉各自 `view.go`（三列点阵映射、`noKeyLines`、`Lines`），只留取数、轮询状态机、错误文案与 `Snapshot()`/`Labels`。
- **删除 `internal/gauge` 整包**（`gauge.go` 431 行 ＋ `gauge_test.go` 196 行）：旧三列点阵画法作废，不留兼容壳。
- `internal/app/app_test.go` 重写：旧 tab 用例大部分作废，新写 24 个（视图切换与 Esc/q、按钮命中与右对齐、标题栏降级、状态栏降级、三段式整屏、内容铺满不滚动、滚轮/resize、R 转发、旧 tab 键不再切视图、编辑态让位、保存 key 后面板自动出现、开关关掉后面板消失并重排、进出设置页不动用量状态、设置页居中/滚动/鼠标命中）。
- 受影响单测改为断言 `Snapshot()`：`usage.TestBrokenConfigShowsSpecificStatusAndPanel`、`cmdusage.TestMissingAndBrokenConfig`、`cmdusage.TestLinesWithKeyButNoData` → `TestSnapshotWithKeyButNoData`。**面板内不再显示 `.bak` 备份提示与 JSON 字段名**（5 行的小面板放不下）：备份行为不变，只是文案位置变了（新 A4：面板给短提示＋`S 设置`，状态栏给完整路径）。
- `internal/settings`：`Hints()` 非编辑态改为 `Q 退出 · Esc 返回 · Enter 编辑 · 空格 开关`。

### 验证 31：单测 ＋ 静态检查 ＋ 双架构 ＋ pty 真实运行 — PASS（race 受环境限制）

- `gofmt -l cmd internal scripts` 无输出；`go mod tidy` 无 diff（零新依赖）；`go vet ./...` OK；`go test -count=1 ./...` **11 个包全 ok、223 个用例**；双架构 `CGO_ENABLED=0` 交叉编译 OK；`-race` 本机仍 `FATAL: ThreadSanitizer: unsupported VMA range`（同验证 16/23/25/27–30）。
- `git diff --stat -- internal cmd` → `11 files changed, 714 insertions(+), 1686 deletions(-)`（净删 972 行：删掉 `gauge` 整包与两个 `view.go`，换上新 `app`）。
- **pty 真实运行**（`/tmp/wlpty.py`：`pty.fork` ＋ `TIOCSWINSZ` 给尺寸，再重建字符网格；配置走临时 `XDG_CONFIG_HOME` ＋ 明显假 key，真实配置未碰）：
  - **100×28**：标题栏 `With Linux · <主机名> · 38°C · 已运行 29d0h · 0.25 0.57 0.63` ＋ 右上 `[ 设置 s ]`；三行网格，行高实测 **11/10/5**——与 grid 的最大余数法 ＋ WantH 收敛算出来的一致（用量面板只要 5 行，多出来的给了上面两行，底部不再留空白）；假 key 得 **401**，两个面板内显示 `Key 无效（401）`、状态栏右侧也优先显示它（A3）；CPU/内存/网络/磁盘都是本机真实读数。
  - **设置页**（按 `s`）：线框表单、`[ ] OpenCode Key`／`[ ] Command Code Key`（未设置）、配置文件路径、关于区块（本地构建没注入 ldflags，版本回退到 Go 构建信息 `v0.3.1-0.2026…+dirty`）、状态栏 `设置 · Q 退出 · Esc 返回 · Enter 编辑 · 空格 开关`。
  - `Esc` 返回系统状态页 ✓；`q` 退出码 **0** ✓；80×24 同样正常（三行网格、用量面板在最后一行）。
- 抓屏工具踩到一个坑，记下来备用：**Bubble Tea 用 `ESC[X`（ECH，擦除 n 个字符）做增量重绘**，重建器不处理它就会留下上一帧的残影（一度误判成「401 提示重复了两行」，实际是残影；不抖窗口尺寸重抓就正常，单测也证明提示只渲染一次）。切视图后只有变化的格子被重画，所以**版式正确性要靠单测断言，不能靠抓屏截图**。
- 新写的两个用例先失败后修正，都不是产品缺陷：① 断言设置页内容时把「内容行号」和「终端行号」混用、重复加了居中偏移（`contentStart()` 在内容不足一屏时是负数）；② 用 `TogglesChangedMsg` 伪造可见性，而进设置页会 `Reload()` 重读配置——真实流程里开关是写回配置文件的，改成写配置文件后两边一致。

## 2026-10-12 · M8 第 7 步：README 与 CHANGELOG

- `README.md` 功能段重写：系统状态（四块面板 ＋ 标题栏三项）、AI 用量（底部两个小面板、可关）、设置（`s`／右上角按钮、`Esc` 返回、勾选框控展示）；界面描述改为「标题栏 ＋ 系统状态网格 ＋ 底部状态提示」并补降级规则。
- `CHANGELOG.md` 的 `[Unreleased]` 重写为「v0.3.0 → 现在」的净变化：**Added**（A8 系统状态页、A9 布局自适应、标题栏、A7 改写后的展示开关）、**Changed**（结构重构、设置页入口/返回、缺 key 提示位置、标签改名、用量包去渲染化）、**Removed**（顶部 tab 栏、`internal/gauge` 整包）。**删掉一条已失效的 Fixed**（「100% 点阵整列填满」）——那是同一未发布周期内对旧画法的修正，旧画法本身已经删除，写进变更日志会让人以为现在还有点阵。
- `AGENTS.md` 的状态栏提示文案与实现对齐：设置页非编辑态 `Q 退出 · Esc 返回 · Enter 编辑 · 空格 开关`、编辑态 `Q 退出 · Enter 保存 · Esc 取消`。

### 验证 32：文档一致性与全量测试 — PASS

- `gofmt -l cmd internal scripts` 无输出；`go vet ./...` OK；`go test -count=1 ./...` 11 个包全 ok（223 例）。
- `grep` 核对 `README.md`／`CHANGELOG.md` 里的 `tab`／`三列点阵` 字样：README 已无；CHANGELOG 剩下的命中都在**历史版本条目**与新写的 Removed/Changed 里（即「去掉了什么」），没有把旧结构当成现状。
- `git diff --stat`：17 个已跟踪文件、`1112 insertions(+), 1736 deletions(-)`；另有 4 个未跟踪新包（`chart`/`grid`/`monitor`/`sysstat`）。

## 2026-10-12 · M8 第 8 步：本机真实验证（与系统命令对数）

- 用 **tmux**（成熟终端模拟器）跑真实二进制并 `capture-pane` 抓屏：100×28 下三种形态都正确——① 空配置 → 两行网格（13＋13 行），边框齐全；② 只开 OpenCode → 第三行整行 100 格宽，状态栏 `未配置 Key：/tmp/wl-none/with-linux/config.json`（新 A4）；③ 两个都开 → 三行 11/10/5，与 grid 的 WantH 收敛一致。
- **负载对数**（4 个 `yes`）：标题 CPU 52%、每核 **4 条 `⣿` ＋ 4 条 `⣀`**；同时刻 `mpstat -P ALL 1 1` 显示 4/5/6/7 号核 idle = 0%、0–3 号核 87–92% —— **每核竖条与 mpstat 逐个吻合**；总占用 52% vs mpstat 的 55%（采样时点差）；温度 39 °C → 53 °C（标题值在上升过程中，读数 53615 毫度在抓屏后 1–2 秒）。
- 其它对数：内存 4.9/7.5G 66% ↔ `free -h` 4.8Gi/4.9Gi；Swap 224M/3.8G ↔ `free` 224Mi/3.8Gi；`/` 24G/231G 11%、`/boot` 181M/1022M 18%、`/var/log` 108M/187M 63% ↔ `df -h` 同值；网络 `enp1s0` 累计 ↓53G ↑35G ↔ `ip -s link` 同量级（速率在 1 K/s–80 K/s 之间波动，与 1 秒实测速率同量级）；温度 ↔ `thermal_zone0`。
- **40×12 紧凑模式**：5 行纯数值、无线框（CPU/内存/网络/磁盘/OpenCode），状态栏降级为 `系统状态 · Q 退出` ＋ `未配置`。
- **设置页**：`tmux send-keys s` → 线框表单、`[✓] OpenCode Key`（因为配置里 `showOpenCode: true`）、配置文件路径、关于区块；`Escape` 返回系统状态页 ✓；`q` 后 tmux 会话消失（程序退出）✓。

### 修复：尺寸未知时的首帧导致旧布局残留（真实缺陷，第 8 步发现）

- 现象：抓屏里 CPU 面板的下边框位置出现了**上一帧网络面板的标题栏**。分帧检查证明 app 自己的每一帧都是对的（首帧 28 行、13/13 布局、边框齐全），问题出在 Bubble Tea 的增量重绘：`New()` 里 `width/height` 初值是 `80×24`，在真实 `WindowSizeMsg` 到达前先按 80×24 画了一帧；尺寸变成 100×28 后它只补差异格子，旧布局里「网络标题栏」那一格被判定为未变化，就留在了新布局的「CPU 下边框」位置。
- 改法：`New()` 的尺寸初值改为 **0**（尺寸未知就不画内容），并在 `WindowSizeMsg` 时返回 `tea.ClearScreen` 强制整屏重绘（窗口尺寸变化后旧字符不会自己消失）。
- 验证：`gofmt`／`go vet`／`go test -count=1 ./...` 全过（app 24 例）；tmux 抓屏在 100×28／40×12、三种可见性组合下都不再有残留。

### 抓屏工具的教训（已写进 FINDINGS §2.2）

- 手写 ANSI 重建器不可靠：补了 ECH（`ESC[X`）之后仍然会被 Bubble Tea 的增量移动/滚动优化绕进去；**pyte 也会误判**（同一个字节流，pyte 重建出残影、tmux 完全正确）。**结论：pty 冒烟一律用 tmux `capture-pane` 抓屏。**
- `tmux new-session` 不会继承客户端的 `XDG_CONFIG_HOME`（只转发 DISPLAY/SSH_* 等少数变量），必须把环境变量写进 `-c` 命令里，否则会读到真实配置（本次误用真实 key 发了两条只读 GET，未做任何写操作；之后的冒烟都改成命令内显式赋值）。

## 2026-10-12 · M8 第 9 步：打包安装 ＋ 已安装程序冒烟

- `VERSION=0.4.0~dev bash scripts/mkdeb.sh arm64` → `dist/with-linux_0.4.0~dev_arm64.deb`（2.35 MB）。
  版本取 `0.4.0~dev` 是因为本次是**破坏性结构变更**（去 tab 栏、删 `internal/gauge`），按语义化版本应当进 0.4.0；正式版本号仍由用户发版时决定。
- `sudo apt install --reinstall` → `Unpacking with-linux (0.4.0~dev) over (0.3.1~dev)` / `Setting up with-linux (0.4.0~dev)`；`wl --version` → `wl 0.4.0~dev`；`dpkg -V with-linux` 无输出；`/usr/bin/wl` 与 `dist/pkg/wl-linux-arm64` sha256 同为 `490c3d97…`。
- **已安装程序冒烟**（`/tmp/wl_smoke.sh`，tmux 起会话 ＋ `capture-pane`，一律临时 `XDG_CONFIG_HOME`）：**29 PASS / 0 FAIL**
  - S1 空配置 100×28：标题栏（程序名/温度/`[ 设置 s ]`）、CPU/内存/网络/磁盘四块面板、状态栏提示；**没有顶部 tab 栏**、两个提供商面板都不出现。
  - S2 只开 OpenCode：面板出现、提示 `未配置 Key · S 设置`、状态栏给完整配置路径、Command Code 仍关闭。
  - S3 两个都开（假 key）：两块都在，得到 `Key 无效（401）`／`无权限（403）` 且不崩溃。
  - S4 `s` 进设置页：`设置` 标题、两行 key、`关于`、版本 `0.4.0~dev`、状态栏 `Esc 返回`；`Esc` 回到系统状态页且 CPU 面板还在。
  - S5 40×12：紧凑模式（无框、每面板 1 行数值），磁盘行仍在。
  - S6 `q`：会话退出。

### 验证 34：全量静态检查与测试（装后复跑）— PASS

- `gofmt -l cmd internal scripts` 无输出；`go mod tidy` 无 diff；`go vet ./...` OK；`go test -count=1 ./...` 11 个包全 ok、**223 个用例**；双架构 `CGO_ENABLED=0` 交叉编译 OK。
- `go test -race` 本机仍 `FATAL: ThreadSanitizer: unsupported VMA range`（同验证 16/23/25/27–31），交 CI。
- 工作区：`AGENTS.md`/`FINDINGS.md`/`PLAN.md`/`PROGRESS.md`/`README.md`/`CHANGELOG.md` ＋ 代码改动（17 个已跟踪文件、4 个新包）；**尚未提交**，等用户指令。

## M8 收尾状态

- A1/A2/A3/A4/A5/A6/A7 的改写、新增 A8/A9 全部实现并有对应单测；`AGENTS.md` 章程已按 M8.6 的 14 条改完。
- 新增包：`sysstat`（采集）、`chart`（braille 原语）、`grid`（布局）、`monitor`（系统状态页）；删除 `gauge`；`usage`/`cmdusage` 去渲染化。
- 已知遗留：① `go test -race` 在本机跑不了（TSAN/VMA，CI 覆盖）；② 本机没有第二个终端尺寸的真实 SSH 客户端，宽高组合靠 tmux 多尺寸 ＋ 单测矩阵验证；③ 发版（tag / GoReleaser）未做，按项目惯例由用户指令触发。

## 2026-10-12 · 界面按 btop 再调（用户四项指令）

- 指令：① 设置按钮里不要有 `s`、挪到右下角，右下角原来的 CPU/内存信息不要了；② 温度/运行时长/负载从左下角（原文如此，实为标题栏左侧）移到右上角；③ 计量条用实心的、参考 btop；④ CPU 面板标题加型号、每核图表参考 btop 调整、网络面板的「量程」有无必要、整体布局按 btop（CPU 占一行、左下内存/磁盘、右上网络、右下 AI 用量）。
- **标题栏**：左＝`with-linux · 主机名`，右＝`温度 · 已运行 X · 负载`；降级顺序不变（负载 → 运行时长 → 主机名 → 程序名，温度最后）。降级函数改成返回 (left, right) 两段，仍是纯函数。
- **右下角按钮**：`[ 设置 ]`（无 `s`），设置页里变 `[ 返回 ]`；窄屏 `[设置]` → `[设]`；点击命中区间与渲染共用同一份计算。**状态栏正常时留空**（CPU/内存只在面板里出现），错误/保存状态文案出现在按钮左边；状态优先级改成「完整状态 > 快捷键提示」（冒烟发现配置路径曾被挤掉，A4 要求完整路径，已修并加单测）。
- **实心计量条**（参考 btop 实测：它的磁盘条整条都是 `■`、CPU 核条整条 `⣀`，填充与轨道同形，只靠颜色区分）：横向计量条改成整条 `⣿`，填充亮色、轨道 dim(238)，按整格取整；删掉不再使用的竖向 `MeterV`（每核改横条后无调用）。
- **CPU 面板**：标题在空间够时带型号（读 `/proc/cpuinfo` 的 `model name`/`Model`/`Hardware`/`Processor`，ARM SBC 上退到 `/proc/device-tree/model`；本机 `CPU Orange Pi 5`）；每核改成 btop 风格**横向条**：一行并排 2–4 个核（按内宽自适应），`C0 <bar> 15%  C1 …`，高度不够先砍核行数、必要时标「＋N 核未显示」；不再有「每核 / 最高 / 0% / 100%」那行。
- **网络面板**：去掉「量程 xxx」——它只是自动量程的上界（曲线纵轴最大值），曲线本来就是相对高度，当前速率就在旁边；自动量程与下限逻辑保留。
- **布局树**：`internal/grid` 从「行列表」升级为布局树（`Leaf`/`Row`/`Col`，列内按权重分配高度、行内均分宽度），页面结构＝`Col{ CPU, Row{ Col{内存,磁盘}, Col{网络, Row{OpenCode,CommandCode}} } }`；关掉提供商时该行不进树、网络吃掉右栏下部；宽度 <60 仍摊平成单列。`grid` 的旧测试重写为树版本（含 `TestBtopShape` 钉住结构、`TestWantHConvergence`、`TestHiddenProvidersCollapse`）。
- **拼屏 bug（新布局暴露）**：`compose` 原来按「同一 Y 就是同一行」分组，嵌套布局里内存（Y=10,H=9）与网络（Y=10,H=13）同 Y 不同高，后画的整行覆盖先画的 → 页面上只剩 CPU 和网络。改成逐行扫描所有面板、按 X 排序拼接，并加注释说明为什么不能按 Y 分组。

### 验证 35：全量测试 ＋ 装后 tmux 冒烟 — PASS

- `gofmt` 无输出；`go vet ./...` OK；`go test -count=1 ./...` 11 个包全 ok（223 例）；双架构 `CGO_ENABLED=0` 交叉编译 OK。
- 重新 `VERSION=0.4.0~dev bash scripts/mkdeb.sh` ＋ `sudo apt install --reinstall`；`wl --version` → `wl 0.4.0~dev`。
- 对已安装 `/usr/bin/wl` 的 tmux 冒烟 **33 PASS / 0 FAIL**；100×28 实拍：CPU 整行（标题 `CPU Orange Pi 5`，每核 `C0 … C7` 两行横条）、左下内存/磁盘、右上网络、右下两个 AI 面板并排、状态栏最右 `[ 设置 ]`；带颜色抓包确认每核横条是「1 格亮 + 12 格 dim」（2%）而不是整条全亮。80×24 同样成立（每核 8 格条）。

## 2026-10-12 · 按反馈修正画法与 CPU 信息（五项）

- **CPU 型号**：之前取了 device-tree 的板名（Orange Pi 5）。改为优先 `/proc/cpuinfo` 的 model name/Model/Hardware，其次 `/proc/device-tree/compatible` 里**逗号后最短的一条**（SoC 名比板名短）→ 本机得到 `RK3588`；再退到 CPU part 映射（A53/A55/A76…）。实测 `CPUModel()` = `RK3588`。
- **时间序列改成填充面积图**（用户：「我之前指的是上面折线图改成从上到下有填充的样子，类似柱状图」）：新增 `Canvas.PlotFill` / `chart.Fill`——每个点列从底填到插值高度，0% 保留一条基线；CPU、内存、网络、磁盘全部改用；双序列（网络 ↓↑、磁盘读/写）画成**上下两张**填充图（高度 <4 时只画第一张）。
- **每核也改成随时间变化**（参考 btop）：`history` 增加每核一条环形缓冲（120 点），每核渲染成 1 行高的填充小图：`C0 <图> 15%  C1 …`，一行并排 2–4 个核（按宽度自适应）。
- **进度条空位留白**（用户：「空的地方不要填充点阵」）：`MeterH` 去掉轨道字符，只在填充段画实心 `⣿`，其余补空格；`MeterV` 整段删除（已无调用）。
- **计量条等宽**：内存/磁盘/用量面板都先算「标签列、数值列、百分比列」的最宽值，再统一计量条宽度——`总计/已用/可用/缓存/Swap`、每条挂载点、三个用量窗的条现在严格同宽（宽度不够时先丢次要列：内存丢总计/缓存/可用，磁盘丢「可用」，用量丢倒计时）。
- **内存/磁盘信息更接近 btop**：内存面板改为 `总计 / 已用 / 可用 / 缓存 / Swap` 五行（各自带百分比与等宽计量条，按高度优先保留已用与 Swap）；磁盘面板最多列 5 个挂载点，每行给「路径、已用/总量、可用、等宽计量条、百分比」，上方是读/写两张填充图。
- 顺带修：挂载点百分比曾被格式化成 `11%%`（`PctCeil` 已带 `%`，又拼了一个），已改。

### 验证 36：全量测试 ＋ 装后 tmux 冒烟 — PASS

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 11 包全 ok；双架构交叉编译通过。
- 重新打包安装（`0.4.0~dev`），对 `/usr/bin/wl` 冒烟 **33 PASS / 0 FAIL**。
- 实拍 100×28：标题 `CPU RK3588`；每核两行 `C0 … C7` 小图；内存五行等宽条（总计/已用/可用/缓存/Swap）；磁盘三行挂载点（路径、已用/总量、可用、条、百分比）＋ 读写填充图；负载下 CPU 填充图从底往上实心、C4–C7 满格。

## 2026-10-12 · 宽屏分栏与中线镜像图（用户两项指令）

- **面板内部左图右明细**：`splitColumns(innerW, detailNeed)` 按各面板实际需要给明细列宽度（CPU 46、内存 36、网络 26、磁盘 44），图区至少留 24 格，不够宽就退回上下排。100×28 时只有 CPU（整行、内宽 96）会分栏；160×40 时内存/磁盘/网络也分栏（磁盘分栏后仍保留「可用」）。左右两段用 `zipColumns` 拼成同一高度。
- **中线镜像面积图**：新增 `Canvas.PlotFillMirror` / `chart.FillMirror`——以垂直中线为基线，`up` 往上长、`down` 往下长；并抽出 `eachDotColumn` 复用插值逻辑。按用户拍板：CPU 上＝总占用、下＝最忙的那个核（`history` 新增 `coreMax` 环形缓冲）；网络＝上传 ↑ / 下载 ↓；磁盘＝读 ↑ / 写 ↓。窄屏和宽屏都用同一套镜像图。
- **每核明细**：宽屏右栏按宽度每行放 1–4 个核（`C0 <图> 15%  C1 …`），高度不够时先减行数、再标「＋N 核未显示」；修了截断计数少算一个的 bug 和重复定义的 `coreDetailRows`。
- 文档同步：A8 / 界面布局 / 统一画法 / README / CHANGELOG 都改成「镜像填充图 ＋ 宽屏左图右明细」。

### 验证 37：全量测试 ＋ 装后 tmux 冒烟 — PASS

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 11 包全 ok；双架构交叉编译通过；装后冒烟 **33 PASS / 0 FAIL**。
- 实拍 100×28（跑 25 秒让历史攒起来）：CPU 左图上下镜像（上半 ~54% 的总占用、下半最忙核 100% 填满），右栏 C0–C7 三行；内存/磁盘/网络窄屏仍是上下排（100 列时内宽 46，不满足分栏阈值）。
- 实拍 160×40：内存左图右五行数值、磁盘左图右三行挂载点（含「可用」）、网络左图右速率与累计，均正确。

## 2026-10-12 · 按 btop 截图再校（三项）

- **内存面板**：宽屏分栏的明细列从 36 收到 28（图区保底从 24 降到 14，100×28 时内存也能左图右明细了）；明细补上 btop 的「空闲（Free）」行（`sysstat.Memory` 新增 `Free` 字段，读 MemFree）。
- **镜像基线**：`PlotFillMirror` 的中线两侧基线各让 1 个点行（上基线 = mid+1、下基线 = mid-1），**值为 0 时只画自己那一行基线**，所以「读 0 / 写 0」「上传 0 / 下载 0」看起来是两行分开的点，中间空一行点（用户 2026-10-12 要求）。新增 `TestFillMirrorDirectionAndGap` 钉住方向与空隙。
- **明细铺满高度**：新增 `spreadRows`——明细行数少于面板高度时在行间均匀插空行；CPU 每核明细、内存数值行、磁盘挂载点、网络统计都改用它；CPU 右栏优先一行一个核（放不下再每行 2–4 个）。
- 网络宽屏统计改为多行（`↓ 下载` / `↑ 上传` / `累计 ↓` / `↑`），也纵向摊开。

### 验证 38：全量测试 ＋ 装后冒烟 — PASS

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 11 包全 ok；双架构交叉编译通过；重打包安装 `0.4.0~dev` 后冒烟 **33 PASS / 0 FAIL**。
- 实拍 100×28：内存左图右五行（总计/已用/可用/缓存/空闲/Swap 中按高度保留）、网络左图右四行统计、CPU 右栏 C0–C7 均匀铺开；空闲时磁盘镜像图是「上＝读基线、下＝写基线」两行点、中间一行空。

## 2026-10-12 · 补：历史短时图表出现竖列空隙

- 现象：网络/磁盘镜像图在历史没攒满（样本间距 >1 个点列）时，填充里会出现竖列空隙（`⡇  ⢸`）。
- 原因：`eachDotColumn` 只在「样本落点」和「相邻落点之间的整列」画点，当两个样本的 x 相距不足 2 个点列时，中间的列被跳过。
- 改法：改成**逐点列插值**——遍历最旧到最新样本之间的每个点列，找到它所在的区间做线性插值再回调（保证连续）；范围之外（历史还没到）保持空白。
- 验证：`chart`/`monitor` 单测全绿；镜像样张（临时探针）确认 0/0 是两行分开的点＋中间一行空、50/0 上半填满＋下行基线、25/75 按比例；重装后冒烟 **33 PASS / 0 FAIL**。

## 2026-10-12 · 再按 btop 细化（用户四项指令）

- **内存/网络回到上下布局**（用户要求）：去掉这两个面板的内部左右分栏，图在上、明细在下；磁盘与 CPU 仍在宽屏时分栏。
- **CPU 面板**：宽屏右栏改成 btop 结构——`用量 <meter> 53%` ＋ **两列多核**（左列 C0–C3、右列 C4–C7，每核 1 行历史图）＋ `温度 41°C · 负载 0.28 0.28 0.34`；窄屏则是图在上、两列多核与温度/负载在下。温度与负载从标题栏移到 CPU 面板（用户要求）。
- **标题栏**：左侧改为 `with-linux · 主机名 · 已运行 X`，右侧改成**当前时间** `HH:MM:SS`（每 tick 更新）；温度/负载不再在这里。
- **颜色**：CPU 用量条与百分比按阈值着色；每核百分比按自身占用着色；内存/磁盘/用量百分比文字也上色（原来只有计量条上色）；温度 <60 蓝 / <75 琥珀 / 其余红；负载 ≥核数 琥珀、≥2×核数 红；时钟亮色、其余标题信息暗色。
- 内存面板的历史图保底 2 行（原来可能只剩 1 行），明细按优先级保留已用/Swap 等。

### 验证 39：全量测试 ＋ 装后冒烟 — PASS

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 11 包全 ok；双架构交叉编译通过；重打包安装后冒烟 **33 PASS / 0 FAIL**。
- 实拍 100×28：标题栏 `With Linux · host · 已运行 30d15h … 12:59:17`；CPU 右栏用量条＋C0–C3/C4–C7＋温度/负载；内存与网络上下布局；磁盘挂载点带「可用」与彩色百分比。

## 2026-10-20 · M9 范围确认与章程/PLAN/FINDINGS

- 用户确认：Docker 面板增加**日志查询**；Caddy 按建议**只做状态＋热重载**，不做启停；设置、Docker、Caddy 全部改为灰化背景上的居中弹窗；宽屏布局按「内存/磁盘/Docker/Caddy 一行，网络/AI 一行」重排。
- `AGENTS.md`：重写 A1/A5/A8/A9，新增 A10；更新只读约束（唯三写操作）、数据获取（Docker/Caddy）、弹窗渲染与安全边界、设置/Docker/Caddy 弹窗规格、键位与窄屏回退。原独立设置视图和 `[ 返回 ]` 作废。
- `PLAN.md`：M9 范围、布局、弹窗系统、数据模型、实施顺序、完成定义、章程清单与风险落盘。
- `FINDINGS.md`：新增 §8（Docker Engine API 统计/日志/动作、Caddy admin API/systemd/reload、lipgloss Canvas 遮罩、120 列断点）。
- **尚未改代码**；下一步 M9 第 2 步：弹窗遮罩原语 + 设置弹窗化。

## 2026-10-20 · M9 完成：Docker/Caddy ＋ btop 布局 ＋ 弹窗化

### 实现

- **弹窗基建**：新增 `internal/overlay`，用 lipgloss v2 Canvas/Layer 把底层逐格灰化（前景 240、清按钮底色），再把弹窗居中覆盖；单测覆盖居中、宽字符/braille、背景保留与颜色保留。
- **设置弹窗**：`internal/app` 从 `viewMonitor/viewSettings` 两视图改为「主页面 + modal 状态」；设置表单/保存/勾选框/「关于」移入弹窗，Esc 层级关闭，鼠标行与勾选框仍可用。
- **新布局**：宽 ≥120 时 CPU 整行，下排上半内存/磁盘/Docker/Caddy、下半网络/AI；<120 回退旧两栏 + 两行无框服务摘要；<60 单列/紧凑滚动；AI 全关时网络吃满下排。
- **Docker**：新增 `internal/docker`（Unix socket Engine API、版本协商、列表/one-shot stats、相邻样本 CPU%、内存扣 inactive_file、网络/块 I/O、启停/重启、TTY/非 TTY 日志拆帧与 200 行/256 KiB 上限、SGR/控制符清理）；`internal/monitor` 每 2 秒异步轮询并存储动作/日志状态。
- **Caddy**：新增 `internal/caddy`（admin config/upstreams 摘要、只读 systemd 状态、`caddy reload` 假运行器测试）；`monitor` 每 5 秒轮询；不做启停/重启。
- **Docker/Caddy 弹窗**：容器表 + 详情 + 启动/停止/重启（停止/重启二次确认）+ 日志视图（滚动、`/` 过滤、`r` 刷新、Esc 层级返回）；Caddy 状态 + 只读 systemd + 热重载；`d`/`c` 或点击服务卡/摘要条打开，鼠标可选容器、选操作、点 Caddy 重载。
- **文档**：AGENTS A1/A4/A5/A7/A8/A9 重写、新增 A10 与弹窗规格；PLAN M9、FINDINGS §8；README/CHANGELOG；deb 描述更新。

### 验证

- `gofmt -l cmd internal` 无输出；`go vet ./...` 干净；`go mod tidy` 无 diff；`go test -count=1 ./...` **14 个测试包全绿**；`CGO_ENABLED=0` 交叉编译 amd64/arm64 通过。`-race` 仍受本机 TSAN/VMA 限制，按既有结论交 CI。
- **真实只读验证**：120×30 实拍服务卡、四栏布局与 Docker 统计（与 `docker stats --no-stream` 的量级/内存一致）、真实容器日志视图与过滤；80×24 实拍两栏 + 两行服务摘要与弹窗。背景在 `capture-pane -e` 中已变为 240 灰，弹窗保留彩色。
- **Caddy**：系统服务仅只读（admin API/路由/上游/systemd）；在临时 Caddy 实例上执行 `caddy reload --config … --force --address 127.0.0.1:2999`，退出码 0 且响应正常。未对系统 Caddy 执行热重载。
- **未执行**：真实容器的启动/停止/重启（避免中断用户服务）；Docker fake socket 单测覆盖三动作、二次确认与错误文案。
- **安装冒烟**：重打包安装 `with-linux_0.4.0~dev_arm64.deb`，对已安装 `/usr/bin/wl` 在 120×30 与 80×24 跑 tmux 冒烟 **28 PASS / 0 FAIL**（标题/四栏/服务卡/灰化/设置/Docker 列表与日志/Caddy 管理/窄屏摘要/`q` 退出）。
- **敏感信息扫描**：真实主机名、私网地址、真实接口名、真实容器名、apiKey 片段均未进入仓库；文档/测试示例使用 `host`、`enp1s0`、`192.0.2.1`、假容器名与临时目录。

## 2026-10-20 · M10：btop 式重排、服务栏放大、点外关闭

### 实现

- **布局**：`grid.Node` 增加 Row 宽度权重；宽屏下半区改为左 3／右 2——左栏上排内存/磁盘、下排网络/AI，右栏 Docker/Caddy 纵向服务栏。窄屏仍回退两栏 + 2 行服务摘要；AI 全关时网络吃满左侧下排。
- **面板**：内存、磁盘、网络、AI 用量改为「标题＋数值 / 整宽实心进度条」两行分类；磁盘空间不足时补「可用」明细；网络保留镜像图 + 下载/上传两行；CPU 右栏高度够时包「核心」子线框，否则竖线分隔；历史不足补 0 基线，内容不足补明细/分隔线，不留空白块。
- **服务卡**：Docker 直接列运行中容器（名称/CPU/内存），底部汇总 + `D 管理`；Caddy 从配置 host/subjects 提取域名并列出，兼显示上游健康与路由数。
- **交互**：点击弹窗外侧关闭；点击 AI 用量面板立即重试对应提供商，错误态显示「点击面板重试」；超长域名/容器名/日志行边缘渐隐（`chart.FadeText`）。
- **文档/测试**：AGENTS A1/A7/A8/A9/A10、布局、统一画法、弹窗规则全部同步；PLAN M10、README、CHANGELOG、FINDINGS；新增 grid 权重、Caddy 域名、服务卡列表、provider 重试、点外关闭等测试。

### 验证

- `gofmt -l cmd internal` 无输出；`go vet ./...` 干净；`go test -count=1 ./...` **14 个测试包全绿**；`CGO_ENABLED=0` 双架构构建通过。
- 120×30 实拍：CPU 核心子线框 + 补基线；内存/磁盘两行统计；右侧 Docker 6 个运行容器与 Caddy 5 个域名；80×24 实拍两栏 + 服务摘要 + 两行统计。真实 Caddy 只读状态正常；AI 面板（无 key 错误态）显示「点击面板重试」。
- 重打包安装 `0.4.0~dev` 后对 `/usr/bin/wl` 跑既有 tmux 冒烟 **28 PASS / 0 FAIL**；未对真实容器执行启停、未对系统 Caddy 执行重载（临时实例 reload 已在 M9 验证）。

## 2026-10-20 · M11/M12/M13：方块条、纯鼠标、btop 收尾

### 实现

- **M11**：`chart.MeterH` 改用 `■`；内存/磁盘/网络/AI 全部两行分类；Docker 卡最右侧列 `UP`/`EXIT`/`RESTART`/`PAUSE`/`DEAD` 短状态；Caddy 卡两列展示域名与上游端口；AI 面板按 btop 磁盘区域两行统计。
- **M12**：删除底部快捷键栏；标题栏最右是 `[ 设置 ]`；键盘只留 `q`，设置/日志输入框内 `q` 是文本；设置弹窗按钮 `[保存]`/`[取消]`/`[关闭]`，Docker 直接按钮 `[启动]`/`[停止]`/`[重启]`/`[日志]`/`[刷新]`/`[关闭]`，日志 `[刷新]`/`[过滤]`/`[返回]`/`[关闭]`，Caddy `[重载配置]`/`[关闭]`；全部点击执行、无二级菜单、无二次确认。
- **M13**：内存/磁盘/AI 分类之间按高度加分隔线与均匀留白，标签改亮色；网络图与速率之间加分隔线、标题显示上下行；Docker 名称列吃掉剩余宽度、状态右对齐；Docker/Caddy 加载态字段化占位；去掉重复 Swap、残留 `S 设置`/`R 刷新`/`Enter/Esc`/`/ 输入` 文案；设置弹窗状态与按钮移入线框底部，编辑态输入不再挤出右边框。

### 测试与验证

- `gofmt -l cmd internal` 无输出；`go vet ./...` 干净；`go mod tidy` 无 diff；`go test -count=1 ./...` **14 个测试包全绿**（app/settings/monitor/chart/docker/caddy/grid/overlay/usage/cmdusage/sysstat/config/meta/cmd）。
- `CGO_ENABLED=0` 交叉编译 `linux/arm64`、`linux/amd64` 通过，两个产物均 static。
- 新增/更新测试：只 `q` 退出、快捷键不弹窗、标题栏按钮命中、设置行/勾选框/保存/取消/关闭、设置弹窗按钮在线框内、Docker 列表与日志按钮、Caddy 重载按钮、Docker 状态右对齐、Swap 不重复、AI 错误态不铺满横线。
- **安装冒烟**：重打包安装 `with-linux_0.4.0~dev_arm64.deb`，对 `/usr/bin/wl` 在 120×30 与 80×24 用 tmux + SGR 鼠标事件验证：
  - 120×30：标题栏最右时钟＋`[ 设置 ]`、无底部状态栏；点击右上角打开设置，点行进入编辑，输入 `oc_sk_fake_key_for_tests_0123`，点 `[保存]` 后配置写回、显示「已保存」、点外侧关闭、OpenCode 面板出现；Docker 卡状态右对齐；点击 Docker 卡进入弹窗，点 `[日志]` 进入日志、`[返回]` 回列表、`[关闭]` 关闭；点击 Caddy 卡打开弹窗，`[重载配置]` 可见（未执行）；关闭后 `q` 退出。
  - 80×24：两行服务摘要、无底部栏、右上角按钮仍可点开设置、`q` 退出。
  - `s`/`d`/`c`/`Esc`/`R`/方向键在主页面均无反应；设置输入框内 `q` 作为文本。
- **未执行**：真实容器启动/停止/重启（避免中断用户服务，fake socket 单测覆盖）；系统 Caddy 热重载（只读与临时实例已在 M9 验证）。
- 安装冒烟中发现并修复：设置弹窗按钮原先落在线框外；编辑态输入把右边框挤出屏幕。修复后重打包、重装并重跑以上冒烟。

## 2026-10-21 · M14：对照 btop 1.2.13 收尾

### 实现

- 用本机 `/usr/bin/btop`（1.2.13）在 120×30 实拍对照；主要差距是 CPU 占比过高、右侧 Docker/Caddy 分界与左侧网络起点错一行、分类标题没有 btop 的分隔线。
- 高度：CPU/下半区权重改为 1:7，并给 CPU/内存/磁盘/网络/Docker/Caddy 设置 `WantH: 10`；120×30 下 CPU 10 行、左右两栏各 10/10；160×40 下 CPU 13 行、左右两栏各 14/14，分界对齐。
- 分类标题：`statTitleLine` 改为 `标签 ───── 数值`，对应 btop 的 `├─Used: … ┤`；分类之间在高度富余时再插整行分隔线。
- 颜色：分类标签、核心编号改主题青色；Docker CPU% 按阈值着色；Caddy 上游健康按 6/6、部分、0/6 分绿/琥珀/红。
- 新增测试：`TestWidePanelsShareHorizontalBoundaries` 钉住 120 列下左右栏的上/下分界行；`TestPageShowsSampledValues` 增加「已用 ─」分类线断言。

### 验证

- `gofmt -l cmd internal` 无输出；`go vet ./...` 干净；`go test -count=1 ./...` **14 个测试包全绿**。
- `CGO_ENABLED=0` 双架构交叉编译通过，产物均 static。
- 重打包安装 `with-linux_0.4.0~dev_arm64.deb`；对 `/usr/bin/wl` 在 120×30 与 160×40 实拍：标题栏设置按钮、无底部栏、`已用 ─` 分类线、Docker `UP` 状态、网络与 Caddy 同行起始、内存/磁盘/Docker 同行起始、160×40 下 Swap 只出现一次、CPU C0–C7 完整，`q` 退出；全部通过。
- 未执行真实容器启停与系统 Caddy 热重载。

## 2026-10-21 · M15：分色线框与零间隙

### 实现

- 用 `tmux capture-pane -e` 读取 btop 1.2.13 的真实配色：CPU 框 `#556d59`、mem 框 `#6c6c4b`、net 框 `#5c588d`，标题接近白色，而不是统一的暗灰框＋青色标题。
- 新增 `chart.FrameColored`，保留 `chart.Frame` 默认主题；主页面 `frameTheme` 按标题选 256 色近似：CPU 65、内存 101、磁盘 137、网络 61、Docker 67、Caddy 66、OpenCode 73、Command Code 68，标题 252；「核心」子框跟随 CPU。
- `grid.ColGap` 改为 0：面板盒子紧挨，线框本身作为分界；grid/monitor 的相关断言改为按 `ColGap` 计算。
- 分类标签恢复亮灰、核心编号亮灰；数值、状态点、百分比继续按阈值着色。

### 验证

- `gofmt -l cmd internal` 无输出；`go vet ./...` 干净；`go test -count=1 ./...` **14 个测试包全绿**（含新增 `TestFrameColoredUsesPanelTheme`、`TestWidePanelsShareHorizontalBoundaries`）。
- amd64/arm64 静态交叉编译通过。
- 120×30 实拍：CPU 绿框、内存橄榄框、磁盘棕框、网络紫框、Docker 蓝框、Caddy 青框，标题近白；盒子零间隙；分类线、状态右对齐、左右分界对齐保持不变。80×24 与 160×40 窄/宽屏检查通过。
- 未执行真实容器启停与系统 Caddy 热重载。

## 2026-10-21 · M16：网络「速率」子框

### 实现

- 宽网络面板按 btop：`splitColumns(innerW, 30, 24)` 拆左曲线＋右子框；子框标题「速率」，内容用两行分类（↓下载/↑上传）、方块条与累计，框色沿用网络紫。
- 面板不够宽时保持原来的「上图＋下方两行下载/上传」，不强行塞子框。
- 新增 `TestWideNetworkUsesRateSubBox`（72 宽出子框，40 宽回退）。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿。
- 120×30 实拍：网络面板左侧镜像图、右侧速率子框；160×40 子框更高、累计与分隔线正常；80×24 回退上下布局且标题只显示下载（宽度不足）。
- 重打包安装 `with-linux_0.4.0~dev_arm64.deb`；对 `/usr/bin/wl` 实拍 120×30 与 160×40 有「速率」子框与累计，80×24 回退上下布局；标题按钮、无底部栏、零间隙、分类线、Docker `UP`、五类框色与 `q` 退出均通过。未执行真实容器启停与系统 Caddy 热重载。

## 2026-10-21 · M17：磁盘读写历史曲线

### 实现

- 复用 `history.read` / `history.write`（已有 120 点），磁盘面板 `innerH >= 8` 时在顶部画读 ↑/写 ↓ 镜像曲线：120×30 用 2 行曲线、不加分隔线，保留 3 个挂载点；`innerH >= 12` 用 3 行＋分隔线；`innerH >= 16` 用 4 行。
- 自动量程下限 `ioMinScale=1 KiB/s`；80×24 的磁盘内高不足 8，仍只显示挂载点。
- 新增 `TestTallDiskPanelShowsIOHistory`：46×14 与 36×10 两个面板都要出现 braille 曲线，行数分别保持 14/10。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿。
- 120×30 实拍：磁盘顶部出现 2 行读写曲线，同时保留 `/`、`/boot`、`/var/log` 三个挂载点；160×40 曲线加高并带分隔线、挂载点与可用明细正常。
- 重打包安装 `with-linux_0.4.0~dev_arm64.deb`；对 `/usr/bin/wl` 实拍 120×30 与 160×40，磁盘读写曲线、挂载点、零间隙、分色框、网络子框与 `q` 退出均通过。未执行真实容器启停与系统 Caddy 热重载。

## 2026-10-21 · M18：只读统计补充与 Caddy 刷新

### 实现

- **CPU 频率**：读 `/sys/devices/system/cpu/cpuN/cpufreq/{scaling_cur_freq,cpuinfo_cur_freq}`，各核取平均；标题显示 `CPU <型号> <频率>`，频率读不到时省略。`readCPUFreq` 有 fixture 单测。
- **网络 IP/错误/丢包**：`Net` 增加 `IP`、`RxErrors/TxErrors/RxDropped/TxDropped`；`/proc/net/dev` 解析错误/丢包四列；生产 Root 下用 `net.InterfaceByName` 取首个 IPv4，fixture 测试跳过真实网卡。错误/丢包非零时在速率子框显示一行，正常隐藏。
- **磁盘 IO**：把已有的 `Disk.Util` 接到磁盘标题 `IO n%`。
- **Caddy 刷新**：运行态弹窗增加 `[刷新]`，复用 `RefreshCaddyNow`，只读重取管理 API 和 systemd 摘要。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿（新增 CPU 频率、网络错误/丢包、Caddy 刷新按钮测试）。
- amd64/arm64 静态交叉编译通过。
- 重打包安装；对 `/usr/bin/wl` 在 160×40 实拍：CPU 标题含平均频率、网络标题含主网卡 IPv4、磁盘标题含 IO 占用率；点 Caddy 卡打开弹窗可见 `[刷新] [重载配置] [关闭]`，`q` 退出正常。
- 未执行真实容器启停与系统 Caddy 热重载。

## 2026-10-21 · M19：同类工具布局调研与面板放大

### 调研

- 搜索并对照 btop、bottom（btm）、glances、gtop、atop 的布局：btop 是固定网格，bottom 是 widget 化布局并带 **expand 模式**，glances 自适应尺寸并带进程列表；结论写入 `FINDINGS.md §9`。
- 决策：保留现有 btop 式单页网格，采纳 bottom 的 expand 模式；不加入进程列表、可编辑布局文件或标签页（超出非目标）。

### 实现

- `monitor.PanelLines(id,w,h)` 暴露单面板渲染；`app.focusPanel` 保存放大目标。
- 点击 CPU/内存/磁盘/网络面板铺满整个内容区；点击内容区或标题栏按钮返回网格；放大态标题栏按钮变为 `[ 返回布局 ]`（窄屏 `[返回]`/`[返]`）。Docker/Caddy 仍开弹窗，AI 仍重试。
- 新增 `TestPanelExpandsAndReturns`，覆盖 CPU 放大、内容行数、返回按钮、内存放大和点内容返回。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿。
- amd64/arm64 静态交叉编译通过。
- 重打包安装后对 `/usr/bin/wl` 在 160×40 与 120×30 实拍：CPU 面板放大后铺满内容区、标题栏显示返回按钮，点击内容或返回按钮恢复网格，`q` 退出正常。未执行真实容器启停与系统 Caddy 热重载。

## 2026-10-21 · M20：补元素不改网格

### 实现

- **CPU 分解**：`/proc/stat` 增量计算 user/system/iowait 百分比；CPU 核框 detail 高 ≥7 时在用量条下加 `用户/系统/IO` 一行。120×30 也能放下，C0–C7 不丢。
- **Docker 列头**：卡片内高 ≥10 时先画 `容器 / CPU / 内存 / 状态` 列头；120×30 内高 8 不加。
- **磁盘 inode**：`statfs` 取 Files/Ffree，挂载点「可用」行追加 `· inode n%`；无 inode 数据的文件系统自动省略。
- 不改任何面板位置；新增 CPU 分解、inode、Docker 列头测试。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿。
- amd64/arm64 静态交叉编译通过。
- 重打包安装；160×40 实拍 CPU 分解、Docker 列头、磁盘 inode 均出现；120×30 实拍 CPU 分解出现、Docker 列头按高度省略；`q` 退出正常。未执行真实容器启停与系统 Caddy 热重载。

## 2026-10-21 · M21：宽屏试用布局与去按键提示

### 实现

- 宽屏布局改为：左栏内存/磁盘 + 网络整行；右栏 Docker/Caddy + 可见 AI 用量纵向排列。AI 全关时 Docker/Caddy 均分右栏；只开一个时另一个占满底部；窄屏保持原两栏。
- 网络因此稳定拿到整行宽度，速率子框保留；AI 面板在右栏底部并排。
- 界面文案检查：不显示 `Q 退出`/`R 刷新`/`S 设置`/`Enter`/`Esc`/`PgUp`/`PgDn` 等按键提示，只剩鼠标按钮/点击提示；`q` 仍只作为退出键。
- 文档同步 A7/A9、布局图、README、CHANGELOG、PLAN。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿。
- amd64/arm64 静态交叉编译通过。
- 重打包安装；120×30/160×40 实拍 AI 在右侧、网络整行带速率子框、无按键提示，`q` 退出正常。未执行真实容器启停与系统 Caddy 热重载。

## 2026-10-21 · M22：面板内部细节与标题行放大

### 实现

- **CPU 高度**：宽屏使用 `panelCPUWide`（MinH 10），120×30 也画出「核心」子线框并显示 `用户/系统/IO`；核区内部有完整边框和呼吸空间。
- **核区分隔**：`zipColumnsBar` 分隔条左右各留 1 格（左侧曲线让出 1 格），不改变总宽。
- **放大命中**：新增 `HitPanelTitle`，只有面板标题线框行触发放大；点图表/空白区不再误触。
- **历史稳定**：CPU 主图与每核小图固定蓝/青色，历史柱体不会因当前值跨阈值整体变色；百分比文字仍按阈值着色。
- **去键位提示**：Docker/Caddy 卡片和摘要条的 `D 管理`/`C 管理` 全部改成「点击管理」。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿。
- amd64/arm64 静态交叉编译通过。
- 重打包安装；120×30 实拍 CPU 子线框/分解、点击内容不放大、点击标题放大、「点击管理」无 D/C 提示，`q` 退出正常。未执行真实容器启停与系统 Caddy 热重载。
## 2026-10-21 · M23：CPU 标题频率与多核时间轴

### 实现

- CPU 标题左侧只写 `CPU <型号>`；右上角值改为平均频率 `freqText`，占用率只在核心框的用量条上。
- 每核小图改为 `slots=max(graphW*2,4)` + `tailHist`：一个 braille 点列对应 1 秒（半格），窗口约 20 秒；刷新时柱体逐秒左移一个点列，不再把 120 个样本压进十几格。
- 历史柱体固定蓝色，百分比文字仍按当前值着色。
- 实测连续多秒 C0 图形逐点列左移；最终采用一个点列/秒，避免一次跳两格。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿。
- amd64/arm64 静态交叉编译通过。
- 120×30 实拍 CPU 左侧 `CPU RK3588`、右上角频率，核心框与 `用户/系统/IO` 正常；`q` 退出正常。
## 2026-10-21 · M24：AI 固定行数 ＋ Docker/Caddy 卡细节

### 实现

- AI 用量面板设置 `WantH:5`，删除两行统计分支，始终是 3 行窗口内容＋线框；页面变高时富余高度分给 Docker/Caddy。
- Docker/Caddy 卡片和窄屏摘要全部去掉「点击管理」；点击卡片仍打开弹窗。
- Docker 的 `CPU % · 内存` 汇总从底部移到内容区第一行；列头和容器列表从第二行开始。
- Caddy 去掉底部提示后，把空间让给域名/上游、API 与配置文件行。

### 验证

- `gofmt`/`go vet` 干净；`go test -count=1 ./...` 14 个测试包全绿（新增 AI 面板固定 5 行、Docker 汇总首行、无管理提示测试）。
- amd64/arm64 静态交叉编译通过。
- 重打包安装；160×40 实拍 AI 面板 5 行、Docker 汇总在首行、Docker/Caddy 无管理提示，`q` 退出正常。

## 2026-10-21 · M25：去掉服务卡底部填充线

### 实现

- `finishPanel` 删除「内容不足补分隔线」逻辑，改为补空行；Docker/Caddy 内容不足时不再出现一条占行的 `─`。

### 验证

- `go test -count=1 ./...` 14 个测试包全绿；amd64/arm64 静态交叉编译通过。
- 重装后 160×40 实拍 Docker 底部只剩空白行、无填充线，`q` 退出正常。

## 2026-10-22 · M26：v0.5.0 发布后修复

### 实现

- **Docker 重启超时**：普通列表/统计/日志请求继续用 5 秒客户端超时；start/stop/restart 改用独立 60 秒 HTTP 客户端，monitor 的动作 context 同步延长到 60 秒。
- **Docker 卡末行**：容器行预算改为 `内高 − 汇总行 − 可选列头`，不再把汇总行重复计入容器行。新增 8 容器回归测试，最后一行必须显示第 8 个容器。
- **Docker 弹窗详情**：选中容器补 `CPU`、`内存 usage / limit (pct)`；网络、块 I/O、PIDs、状态和短 ID 保留。
- **Caddy 刷新**：`[刷新]` 现在并行重取管理 API 与只读 `systemctl show caddy`，不是只刷新管理 API。
- **端点口径**：章程明确日志解析可用只读 `GET /containers/{id}/json`，但只读取 `Config.Tty`。
- **文档清理**：去掉代码注释里残留的“顶栏/tab/底部状态提示”旧结构描述；PLAN 状态更新到 `v0.5.0` 已发布并登记 M26。

### 验证

- Docker 定向测试通过：新增用例证明普通查询客户端 10ms 超时时，动作客户端仍能完成 50ms 的重启请求；构造器断言动作超时为 60 秒。
- **真实 Docker 重启**：启动一次性 `alpine sleep 600`，用项目 `docker.NewClient().Act` 执行 restart，**10.68 秒后成功返回**；旧 5 秒超时会在该场景误报。验证后容器已停止并随 `--rm` 自动删除。
- `go test -count=1 ./...`：14 个测试包全绿；`gofmt -l` 无输出；`go mod tidy -diff` 无差异；`go vet ./...` 干净。
- `CGO_ENABLED=0` 的 `linux/amd64`、`linux/arm64` 均构建成功，产物为静态 ELF。
- 120×30 用临时空配置实拍：Docker 卡 `8/8`，汇总行下连续显示当前能容纳的容器，底边框前不再留空白容器行。
- 打包并安装 `with-linux 0.5.1~dev`；`wl --version` → `wl 0.5.1~dev`，`dpkg -V with-linux` 无差异，`/usr/bin/wl` 与 `dist/pkg/wl-linux-arm64` sha256 同为 `bcc32b43…`。
- 对已安装 `/usr/bin/wl` 在 120×30 复跑：标题栏、系统网格、Docker `8/8` 与容器行正常；发送 `q` 后 `wl-installed-smoke` 会话消失，退出正常。

## 2026-10-22 · M27：AI 置顶＋服务卡滚动与逐行渐隐

### 实现

- 宽屏右侧栏顺序改为 `AI 用量（固定 5 行）→ Docker → Caddy`；AI 隐藏时该行消失，Docker/Caddy 使用释放出的高度。
- `monitor.Model` 新增 Docker/Caddy 独立滚动偏移；鼠标位于对应卡片内时，滚轮由卡片消费，不再滚动整页。
- Docker 正文包含全部容器与“最忙”明细；Caddy 正文包含域名/上游配对行、未解析提示、API 与配置文件行。汇总/列头/表头固定，正文可滚。
- 新增 `chart.FadeRows`：正文末尾仍有隐藏行时，可见区域最后三行按 245→242→238 渐暗；到列表末端后不再渐暗。
- 滚轮偏移在窗口 resize、数据缩短后按当前最大偏移重新 clamp；切弹窗或页面时仍保留，不影响其它面板。

### 验证

- 新增布局顺序测试：右栏必须满足 `AI.Y < Docker.Y < Caddy.Y`，AI 行保持 5 行，AI 与左栏上排同起点。
- 新增 Docker/Caddy 滚动、末端、clamp 和卡片内滚轮优先测试；渐隐行颜色逐级为 245/242/238。
- 120×30 双 Provider 假 Key 实拍：右侧顺序为 `OpenCode / Command Code → Docker → Caddy`；Docker/Caddy 隐藏内容下方各有正常/245/242/238 四档。
- 已安装 `0.5.1~dev` 后复跑实拍与 `q` 退出；最终源码继续叠加 M28 后再统一打包装机。

## 2026-10-22 · M28：历史图表不可回写

### 实现

- 根因一：网络/磁盘每帧都用当前历史最大值重新计算自动量程，新峰值会把全部旧柱整体缩放。
- 根因二：主图把 120 个样本按任意图宽线性插值；样本横向步长非整数点列时，旧轨迹会在字符栅格上重新量化，视觉高度发生变化。
- 修复一：历史新增 `rxPlot/txPlot/readPlot/writePlot` 归一化 ring。每个点在采样时按当时量程冻结为百分比，后续峰值只改变新点量程，不回写旧点。
- 修复二：CPU/网络/磁盘主图统一使用 `slots = 图宽 × 2` 与 `tailHist`，一个采样固定占一个 braille 点列。宽度不足时显示最近 N 秒，宽度足够时保留完整 120 秒。
- CPU 历史仍固定 0–100%，只追加不覆盖。

### 验证

- `TestHistoricalPlotHeightsAreImmutable`：新采样产生更高 CPU/网络/磁盘值后，旧点数值逐项保持不变。
- `TestFillHistoryShiftsWithoutHeightRecomputation`：新点并入后，旧点阵必须原样左移一个点列，逐点纵坐标完全相等。
- `go test -count=1 ./...`：14 个测试包全绿；`gofmt -l` 无输出；`go mod tidy -diff` 无差异；`go vet ./...` 干净。
- amd64/arm64 静态交叉编译通过。
- 打包安装 `0.5.1~dev`；`wl --version` → `wl 0.5.1~dev`，`dpkg -V with-linux` 无差异，`/usr/bin/wl` 与包内二进制 sha256 同为 `6127267b…`。
- 对已安装 `/usr/bin/wl` 在 120×30 用假 Key 连续运行：AI 位于 Docker 上方、服务卡滚动与渐隐正常，`q` 退出后 tmux 会话消失。

## 2026-10-22 · M29：精简内存/磁盘＋放大态充实＋镜像基线修正

### 实现

- **内存主面板**：改为“使用率历史图＋唯一已用主进度条＋紧凑分类”。可用/缓存放在第一摘要行，空闲/Swap 放在第二摘要行；不再为每个分类各画一条进度条。
- **磁盘主面板**：只保留根目录 `/` 的进度条；其他挂载点收成 `路径 + 占用率` 摘要。历史图按剩余内高分配，120×30 主页面从旧 2 行提升到 4 行左右。
- **CPU 放大态**：新增专用渲染，左图占可用高度，右栏每核历史图由主页面 1 行扩展为多行；8 核按两列、四组纵向铺开，用户/系统/IO/温度/负载保留。
- **网络放大态**：右栏扩大并补充接口、IPv4、累计、错误/丢包；下载/上传各保留实时速率和进度条。
- **网络镜像基线**：按用户反馈恢复上传/下载基线之间 1 个点行间距；显式回归检查下载基线 owner 为下载序列，颜色保持蓝色。
- **服务卡渐隐**：原先只处理最后 3 行且灰阶过近；改为整组可见列表按 255→237 连续灰阶，实际可见到逐行变淡。

### 验证

- 新增测试确认：
  - 内存主面板只有一个 `■` 进度条且包含历史；
  - 磁盘主面板只有一个目录进度条，`/boot` 等保留为摘要，内高 8 时历史图至少 4 行；
  - CPU 放大态 8 核每核 5 行历史时输出 20 行；
  - 网络放大态包含接口、IPv4、累计和错误明细；
  - 镜像图上下基线之间空 1 个点行，下载基线 owner=下载序列。
- `go test -count=1 ./...`：14 个测试包全绿；`gofmt`/`go mod tidy -diff`/`go vet` 干净；amd64/arm64 静态构建通过。
- 120×30 临时构建实拍：内存/磁盘各只有一个主进度条，磁盘图明显变高；网络镜像中线不再空一行。
- 打包安装 `0.5.1~dev`；`wl --version` → `wl 0.5.1~dev`，`dpkg -V with-linux` 无差异，`/usr/bin/wl` 与包内二进制 sha256 同为 `7296f394…`。
- 对已安装 `/usr/bin/wl` 实拍主页面和 CPU 放大态；CPU 右栏已是多行每核历史图，`q` 正常退出。

## 2026-10-22 · M30：Caddy 配置文件只读查看

### 实现

- `internal/caddy` 新增 `ConfigFile` 与 `LoadConfigFile`：直接只读 Caddyfile，最大 2 MiB；超限截断标记，CRLF/CR 归一化，非法 UTF-8 替换，NUL 移除。
- `internal/monitor` 管理配置读取的异步状态与请求序号，避免旧结果覆盖新刷新。
- Caddy 状态弹窗新增 `[配置文件]`；配置子视图显示路径、行数/截断状态、带行号原文，支持滚轮，`[刷新]`/`[返回]`/`[关闭]`。
- 配置子视图仍使用同一灰化背景与居中弹窗；点击外侧关闭整个 Caddy 弹窗。状态视图仍不展示配置原文。
- 章程与安全边界同步：查看仅由用户显式点击触发；内容只在内存，不能编辑或写回。

### 验证

- Caddy 单测覆盖正常文件读取、CRLF 归一化、缺失文件报错、2 MiB 截断。
- app 测试覆盖 `[配置文件]` 入口、内容/行号显示、滚轮、`[返回]` 回状态视图。
- `go test -count=1 ./...`：14 个测试包全绿；`gofmt`/`go mod tidy -diff`/`go vet` 干净；amd64/arm64 静态构建通过。
- 打包安装后复核版本、包校验与启动/退出；真实 Caddyfile 内容不在日志中展开。

## 2026-10-22 · M31：服务卡双端渐隐并定版 v0.5.1

### 实现

- `chart.FadeRows` 改为按离最近上下视口边界的行数取非线性灰阶：边界 237，向内依次 246/252/254/255；单行退回可读的中间灰阶。
- Docker/Caddy 可滚正文不再依赖“下方还有隐藏行”才渐隐，始终做上下双端过渡；固定汇总行、Docker 列头和 Caddy 表头保持原色。
- 发布口径从尚未打 tag 的 `0.6.0` 修正为 `0.5.1`，同步章程、README、PLAN、FINDINGS 与 CHANGELOG。

### 验证

- `internal/chart`：双端对称、矮列表非线性档位、单行回退、ANSI 清理均通过。
- `internal/monitor`：Docker/Caddy 在列表未溢出、顶部没有隐藏内容时，仍分别验证了上下边界与中间行的灰阶。
- `go test -count=1 ./...`：沙箱外全量 13 个测试包通过（本地测试需要监听 TCP/Unix socket）。
- `gofmt -l .`、`go mod tidy -diff`、`go vet ./...`、`git diff --check` 均无输出。
- `CGO_ENABLED=0` 的 `linux/amd64`、`linux/arm64` 构建通过，`file` 均确认静态链接。
- 本机为 RK3588 39 位用户地址空间；`go test -race` 被 ThreadSanitizer 以 `unsupported VMA range` 拒绝，属工具链环境限制。race 仍由 GitHub Actions 的 amd64 runner 覆盖。
- 生成 `dist/with-linux_0.5.1_arm64.deb`（SHA-256 `cfb4b45c…`），包内 `wl --version` → `wl 0.5.1`；包内二进制 SHA-256 `2c684125…`。
- 用该包替换安装：`with-linux 0.5.1 arm64 install ok installed`；`/usr/bin/wl --version` → `wl 0.5.1`，与 `dist/pkg/wl-linux-arm64` 哈希一致，`dpkg -V with-linux` 无差异。
- 120×30 临时空配置启动已安装版本：标题栏、CPU/内存/磁盘/网络与服务卡正常；发送 `q` 后 tmux 会话退出。

## 2026-10-22 · M32：服务卡渐隐保留颜色（v0.5.1 发布后修复）

### 实现

- 根因：M31 无条件把整行 ANSI 清空后套统一灰阶，Docker/Caddy 只有 3–4 行时全部变灰。
- `FadeRows` 增加 `topHidden` / `bottomHidden`：没有隐藏内容或可见行少于 4 行时完全不处理；只有实际隐藏方向渐隐。
- 渐隐改为缩放 SGR 前景色对应的 xterm RGB 亮度（边界 58%→72%→86%→100%），保留绿/红/琥珀色相；真彩与基础前景色同样支持。
- Docker/Caddy 固定汇总行、列头和表头仍不参与渐隐。

### 验证

- chart 测试覆盖短列表、无隐藏内容、单边隐藏、绿色/红色/琥珀色相保留。
- Docker/Caddy 测试确认短列表保持原色；滚动到中间后边缘行仍是压暗后的彩色，而非灰色。

## 2026-10-22 · M33：面板明细两端对齐

### 实现

- 新增 `metricCell` / `metricRows`：标签贴左、值贴右；同组自动排成 1–3 个等宽单元格，每个数值贴各列右边界。
- CPU：温度/负载、用户/系统/IO 改为对齐列；空间不足自动降列。
- 内存：可用/缓存、空闲/Swap 改为对齐列，容量和百分比不再随标签长度漂移。
- 磁盘：可用/inode、其他挂载点改为对齐列；长路径可截断路径本身，标签与右边界保留。
- 网络：接口/IPv4、上下行累计、错误/丢包使用同一规则。
- Docker：总 CPU/内存、最忙容器；Caddy：API/文件路径也统一为两端对齐。

### 验证

- 新增等宽列测试验证第一列与行尾百分比边界；CPU 温度/负载测试验证两端对齐分隔线。
- `go test -count=1 ./...`：全量 13 个测试包通过。
- `gofmt -l .`、`go mod tidy -diff`、`go vet ./...`、`git diff --check` 干净。
- `CGO_ENABLED=0` 的 linux/amd64 与 linux/arm64 静态构建通过。
- 120×30 临时构建实拍：CPU 温度/负载、内存容量/百分比、磁盘可用/inode、Docker 总量均已对齐；内存较长 Swap 值自动降为整行，不再截断百分比；`q` 正常退出。
- 构建并安装 `with-linux 0.5.2~dev` ARM64 包供用户实测：`/usr/bin/wl --version` → `wl 0.5.2~dev`，安装二进制与 `dist/pkg/wl-linux-arm64` SHA-256 同为 `b4b31b9e…`，`dpkg -V with-linux` 无差异。

## 2026-10-22 · M34：弹窗按钮、AI 时间与内存借宽

### 实现

- `buttonLine` 接收弹窗内宽：多按钮行首按钮贴左、末按钮贴右，中间空间等距分配；设置、Docker、日志、Caddy 全部接入。
- AI 用量倒计时从 `%-*s` 左对齐改为 `%*s` 右对齐；保留时间列的最低计量条宽度从 4 格降到 1 格，空间紧张时优先显示时间。
- 宽屏默认左右约 3:2；内存不足两列宽度时左栏从右侧 AI/Docker/Caddy 服务栏借少量宽度，但内存与磁盘始终保持等宽。Swap 去掉百分比，只显示已用/总量。120 宽下内存和磁盘各 38 格、右栏 44 格。

### 验证

- app 测试：双按钮 40 宽和五按钮 78 宽均首尾贴边、总宽准确。
- monitor 测试：三种不同长度倒计时的右边界一致；内存与磁盘等宽；Swap 不显示百分比；AI 全关时网络仍占满左侧下栏。
- `go test -count=1 ./...`：全量 13 个测试包通过；`gofmt`、`go mod tidy -diff`、`go vet`、`git diff --check` 干净；amd64/arm64 静态构建通过。
- 120×30 实拍：内存可用/缓存与空闲/Swap 稳定两列，Swap 值不截断；左侧网络与右侧服务栏、Docker/Caddy 排版正常，`q` 退出。
- 内存与磁盘恢复等宽，120 宽实测各 38 格；Swap 已去掉百分比，仅保留已用/总量。
- 重新构建并安装 `with-linux 0.5.2~dev`：`/usr/bin/wl --version` → `wl 0.5.2~dev`，安装二进制与 `dist/pkg/wl-linux-arm64` SHA-256 同为 `c8091703…`，`dpkg -V` 无差异。

## 2026-10-22 · M35：静默键盘操作

### 实现

- 全局：`q/Q/Ctrl+C` 退出；`Esc` 在有弹窗时关闭弹窗、有放大面板时返回布局、否则退出。
- 主页面：`s/d/c` 打开设置/Docker/Caddy，`r` 刷新全部 AI/Docker/Caddy 数据；方向键、PgUp/PgDn、Home/End 滚动。
- Docker 列表：上下键选择容器，PgUp/PgDn 每次移动 5 行，Home/End 到首/末；日志和 Caddy 配置视图键盘滚动。
- 设置输入框和 Docker 日志过滤框中的普通字符仍按文本处理；Esc/Ctrl+C 在输入态仍保留全局语义。
- 所有快捷键均不显示屏幕提示。

### 验证

- app 测试覆盖四种退出键、弹窗 Esc、放大态 Esc、`s/d/c` 打开、`r` 刷新、Docker 键盘选择、弹窗与主页面键盘滚动。
- 输入态测试确认 Enter/q 不保存/退出，Esc 关闭整个设置弹窗，Ctrl+C 退出。

## 2026-10-22 · M36：按钮分组、Caddy 路径与 CPU 列边界

### 实现

- 弹窗按钮改为“左侧操作组 + 右侧刷新/返回/关闭组”，不再整行平均分散。
- Caddy 状态卡的域名/路由/上游/请求与弹窗摘要统一标签左、值右。
- Caddy 弹窗底部配置文件路径整行可点击，直接加载只读配置视图。
- CPU 多核按实际标签/百分比宽度计算图宽并分配整宽余数；温度/负载与对应核心列的百分比右边界一致。

### 验证

- app 测试：按钮分组边界、Caddy 路径点击进入配置视图。
- monitor 测试：Caddy 摘要对齐、CPU 多核总宽、温度/负载与左右列百分比边界。
- 120×30 实拍：CPU 核心最右百分比贴住子框右下边界；Caddy 摘要 `域名/路由`、`上游/请求` 两端对齐；`s` 打开设置、Esc 关闭且应用继续运行、q 退出。
- 全量测试、gofmt、go mod tidy -diff、vet、双架构静态构建通过。
