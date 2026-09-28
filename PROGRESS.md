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

- [ ] 用户在 SSH 终端实机验证：鼠标点击 tab、滚轮滚动、`q` 退出（v1 仅一个 tab，点击无可见切换，加第二个工具后肉眼可见）
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

- [ ] 用户实机鼠标验证（A1 的鼠标点击只有单测＋pty 模拟证据；v1 单 tab 点击无可见切换）
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
