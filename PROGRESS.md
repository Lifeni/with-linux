# PROGRESS.md — with-linux 进度与验证记录

> 计划见 `PLAN.md`，调研结论见 `FINDINGS.md`。宣称「完成」前必须有本文件里的真实验证输出。

## 2026-09-25 · M0 只读调研（完成）

- 旧面板源码 `usage-tui.js` 由用户拷入，移植信息逐条记录于 `FINDINGS.md` §1。
- Charm 三件套 v2 模块确认，规范路径为 `charm.land/*`（github.com/charmbracelet/*/v2 的 go.mod 声明为 charm.land，直接引用会报 module path 不匹配——实测踩过一次后改用 charm.land）。
- v2 API 与 v1 差异（读源码确认）：`Init() Cmd`；`View() View`；alt-screen / 鼠标模式在 `View` 字段上声明（`v.AltScreen`、`v.MouseMode = tea.MouseModeCellMotion`），无 `WithAltScreen`/`WithMouse*` 选项；鼠标消息 `MouseClickMsg`/`MouseReleaseMsg`/`MouseWheelMsg`/`MouseMotionMsg`（`Mouse{X, Y, Button, Mod}`）；`tea.Quit` 可直接作 Cmd 返回。
- 目标机即本机：aarch64 / go1.27.1；交叉编译走内建 `GOOS/GOARCH`（M3 实测）。

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

### 验证 2：pty 真实运行 smoke（`/tmp/opencode/pty_smoke.py`）— PASS

100x30 终端，真实进程渲染三帧 ＋ 退出码：

- 帧1 初始：` with-linux    [OpenCode Go 用量] ` / 内容区 28 行占位 / 状态栏 `OpenCode Go 用量 · q 退出 · ←→ 切换工具 · ↑↓/滚轮 滚动   待刷新`
- 帧2 模拟滚轮向下一次：内容首行 `第 01 行` → `第 02 行`，滚动生效
- 帧3 模拟点击 tab：内容回到 `第 01 行`（点击切 tab 时 scroll 归 0），程序不崩
- 按 `q` 后进程退出码 **0**，alt-screen 正常还原

已知瑕疵（诚实记录）：帧3 行首有一个残留 `M` 字节，来自 pty 对鼠标序列的回显/交错（程序在 raw 模式下运行，用户真实终端不会出现）；不影响行为，待实机复核。

### 待办（M1 收尾）

- [ ] 用户在 SSH 终端实机验证：鼠标点击 tab、滚轮滚动、`q` 退出（v1 仅一个 tab，点击无可见切换，加第二个工具后肉眼可见）
- 2026-09-25 用户说「继续」推进 M2；实机鼠标验证留给用户随时做。Windows 桌面弹窗路径已确认不可行（Windows 22 端口未开），已交给用户直连命令 `ssh -t you@192.168.31.3 /home/you/codes/with-linux/with-linux`。临时 tmux 会话 `with-linux` 已清理。

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
      /tmp/opencode/empty-xdg/with-linux/config.json
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
- arm64 交付产物本机真实跑 A2 smoke：PASS 9/9（点阵/百分比/倒计时/轮询状态/退出码）

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
