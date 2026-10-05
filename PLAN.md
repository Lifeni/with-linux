# PLAN.md — with-linux 实施计划

> 章程见 `AGENTS.md`。计划经用户确认（2026-09-25「按这个开工」）。进度与验证记录见 `PROGRESS.md`，调研结论见 `FINDINGS.md`。

**状态（2026-10-05）：M0–M7 完成并发布 v0.3.0**，A1–A6 验收核对见 `PROGRESS.md`。遗留：403 真实路径未实测（无「有 key 无订阅」的账号，靠 httptest 覆盖）。窄屏方案 C/D（窄宽两段式、极小终端提示）：**用户 2026-09-28 决定不做，不列入范围**。

## M0 · 只读调研（动手前）

1. 找到旧零依赖用量面板源码，逐条读出：取数逻辑、401/403/网络失败的错误文案、重试策略、轮询间隔、三列点阵进度与倒计时的文案格式 → 记入 `FINDINGS.md`
2. Bubble Tea v2 ＋ lipgloss ＋ bubbles 兼容性验证：确认可用版本组合，有坑此刻暴露（**不换栈，只记录**）
3. 确认 Go 工具链与交叉编译方式（直接构建 or `GOOS/GOARCH` 交叉编译出两个产物）

## M1 · TUI 骨架（第一个里程碑，验收 A1）

- 顶部 tab 栏 ＋ 中间内容区 ＋ 底部状态提示三段式布局
- 鼠标点击 tab 切换内容区、键盘 `←`/`→` 切换、`1`–`9` 直达、滚轮滚动、`q`/`Ctrl+C` 退出、resize 自适应
- 即使 v1 只有一个 tab，切换逻辑照做（"余地要留"）
- **完成定义：在目标机真实跑起来，鼠标点击切换面板的真实输出展示给用户看**

## M2 · 用量工具（验收 A2/A3/A4）

- XDG JSON 配置加载 ＋ `apiKey`；缺失时底部提示配置路径（A4）
- HTTP 取数 ＋ 轮询 ＋ 错误处理（401/403/网络失败文案与重试策略对照 `FINDINGS.md` 移植）＋ 倒计时本地递减（A2/A3）
- 三列点阵进度渲染，格式对照旧实现

## M3 · 交付与文档

- 交叉编译 `linux/amd64` ＋ `linux/arm64`，单一可执行文件
- `AGENTS.md`（已落盘，随决策更新）/ `FINDINGS.md` / `PROGRESS.md` 齐备

## M4 · 验收核对

- A1–A4 逐条在目标机实测，展示真实输出后才说「完成」

## M5 · 设置页（2026-09-28，范围扩张已确认，验收 A5）

- 章程先改（`AGENTS.md`：`A5` ＋「界面布局 · 设置页」＋ 版本注入约束），再动代码
- 抽出 `internal/config`（`Path` / `Load` / `Save`）：`usage` 改为共用；`Save` 保留未知字段、目录 0700、文件 0600、写临时文件再 rename
- 新增 `internal/settings`（第三个包，与 `usage` 平级）：`API Key` 可编辑行 ＋ `配置文件` 路径 ＋「关于」区块（版本/构建日期/commit/Go/平台/仓库）
- 交互：表单式（`↑↓` 选行、`Enter` 编辑/保存、`Esc` 取消；编辑态下全局键让位给输入，`Ctrl+C` 仍退出；鼠标点可编辑行即进编辑）
- 保存成功 → `settings.SavedMsg` → 框架转成 `usage.RefreshMsg`（重读配置并立即取数）
- 构建信息：`internal/meta` 统一注入/回退逻辑；`cmd/wl` 注入 `version`/`commit`/`date`，`--version` 与「关于」共用
- 打包：`scripts/mkdeb.sh`、`.goreleaser.yaml` 补 `-X main.commit` / `-X main.date`

### M5 完成定义

- `go test ./...` 全绿；pty 冒烟脚本重建画面显示：tab 栏两个标签、设置页掩码与「关于」、Esc 取消不改文件、Enter 保存回写 0600、切回用量页轮询仍在、`q` 退出码 0

## M6 · 审查缺陷修复（2026-09-28，用户确认）

1. 用量刷新状态机：唯一 tick 循环；手动刷新不额外启 tick；请求代次 ＋ context 取消；旧结果不得覆盖新状态。
2. 配置诊断：区分文件缺失、JSON 损坏、读取失败；损坏文件修复保存前自动备份 `.bak`，不静默覆盖。
3. CI/发布：增加 push/PR 检查；发布前检查格式、模块文件、vet、tests；GoReleaser 不再在发布过程中修改 `go.mod`。
4. 细节：倒计时改为向上取整；鼠标只响应左键；补 `cmd/wl` 参数处理测试。
5. 密钥轮换和 Git 历史清理属于用户执行的安全事件处置，不在本次代码改动范围。

## M7 · Command Code 用量页（2026-10-05，范围扩张已确认，验收 A6）

- 章程先改（`AGENTS.md`：`A6` ＋「数据获取（Command Code）」＋ tab 栏加 `Command Code` ＋ 设置页加 `Command Code Key` 行），再动代码
- `internal/config`：加 `commandCodeApiKey` 字段与 `SaveCommandCodeAPIKey`（抽出通用 `saveField`，只改指定字段、保留未知字段）
- `internal/gauge`（新）：把三列点阵渲染从 `internal/usage/view.go` 抽成共享包（`Window`/`Panel`/`Render`/`ClampLines`）；`internal/usage` 改为映射 ＋ 调用 gauge，行为不变
- `internal/cmdusage`（新）：轮询 Command Code alpha 端点，解析成 5h/weekly/月度三窗；模型与错误文案、重试策略对齐 `internal/usage`
- `internal/settings`：加 `Command Code Key` 可编辑行（复用单个输入框，按选中行读写 `apiKey` 或 `commandCodeApiKey`）；`SavedMsg` 带 `CommandCode` 标志
- `internal/app`：标签增至三个（`OpenCode Go 用量` / `Command Code` / `设置`）；`SavedMsg` 按标志刷新对应页面；两个用量页同时收 tick

### M7 完成定义

- `go test ./...` 全绿；双架构交叉编译通过；pty 冒烟对已安装 `/usr/bin/wl` 全绿；真实 key 实测 Command Code 页渲染出三列真实百分比与倒计时

## 纪律

一次只改一件事；改坏即回滚上个可用版本；同一问题修 3 次不好就停下质疑设计；范围扩张先标注再问用户；只读调研先行，破坏性操作先问。
