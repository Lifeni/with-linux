# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.5.2] - 2026-10-22

### Added

- 新增不显示提示的键盘操作：`Esc` 关闭弹窗/退出、`s/d/c` 打开设置/Docker/Caddy、`r` 刷新，方向键/PgUp/PgDn/Home/End 滚动或选择。

### Changed

- `q/Q` 和 `Ctrl+C` 保持退出应用；弹窗内 `Esc` 关闭整个弹窗，放大态下先返回布局。
- 设置输入框和日志过滤框中的普通字符仍按文本输入，只有 `Esc` 与 `Ctrl+C` 保留全局语义。
- 弹窗按钮从全行平均分散改为常用分组布局：操作按钮靠左、返回/关闭按钮靠右。
- 所有弹窗按钮行上方增加一条内宽分隔线；设置弹窗最大宽度收窄到 72 格。
- Caddy 状态摘要改为标签左、值右；配置文件路径整行可点击并直接打开配置视图。
- Caddy 状态卡的域名/路由/上游/请求四项压缩到同一行。
- CPU 多核两列重新分配整宽余数，百分比贴住核心子框边界；温度/负载值与对应列的百分比右边界一致。

### Fixed

- Docker/Caddy 短列表不再被整体改成灰色：可见行少于 4 行或对应方向没有隐藏内容时保持原始颜色。
- 服务卡需要渐隐时只压暗原 SGR 前景色并保留绿/红/琥珀等色相，不再删除 ANSI 样式后套统一灰阶。
- 系统面板的明细标签与数值统一两端对齐：CPU 温度/负载及用户/系统/IO、内存、磁盘、网络、Docker/Caddy 摘要按等宽列右对齐，宽度不足时自动降列。
- AI 用量每行的重置倒计时改为右对齐；窄时会优先压缩计量条而不是直接丢掉时间列。
- 宽屏内存明细不足两列宽度时，从右侧 AI/Docker/Caddy 服务栏借少量宽度；内存与磁盘保持等宽。
- Swap 明细去掉百分比，只显示已用/总量，为内存两列留出稳定空间。

## [0.5.1] - 2026-10-22

### Added

- Caddy 弹窗新增 `[配置文件]` 只读视图：显示带行号的 Caddyfile 原文，支持滚轮浏览、刷新和返回；读取上限 2 MiB，内容只驻留内存。

### Fixed

- Docker `[重启]` 不再被 5 秒普通查询超时截断：启停/重启使用独立的 60 秒超时。真实一次性容器验证中重启耗时 10.68 秒，旧实现会误报失败。
- Docker 状态卡按真实剩余行数显示容器；修复汇总行/列头被重复计入容器行数、导致底部留空并少显示一个容器的问题。
- Docker 弹窗的选中容器详情补上 CPU、内存使用/上限及占用百分比。
- Caddy 弹窗 `[刷新]` 同时刷新管理 API 和只读 systemd 摘要。
- CPU/网络/磁盘主图改为固定“1 采样 = 1 个 braille 点列”，旧历史随新采样整体左移，不再被插值重新量化。
- 网络/磁盘在采样时冻结历史点高度；后续出现更高峰值时只调整未来点的归一化量程，不再回头缩放已有柱状图。

### Changed

- 章程明确 Docker 日志解析允许只读调用 `GET /containers/{id}/json`，且只读取 `Config.Tty`。
- 宽屏右侧服务栏顺序改为 AI 用量、Docker、Caddy；AI 固定 5 行并位于 Docker 上方。
- Docker/Caddy 卡内正文支持鼠标滚轮独立滚动；可滚正文按离上下边界的距离做对称非线性灰阶渐隐，边界最暗、向中间提亮，固定汇总行与列头不受影响。
- 内存主面板精简为使用率历史图＋单个「已用」进度条，其他分类收成 1–2 行明细。
- 磁盘主面板只保留根目录进度条，其他挂载点收成文字摘要；释放出的高度用于放大读写历史图。
- CPU 放大态每核历史图由 1 行扩展为多行；网络放大态补充接口、IPv4、累计与错误/丢包明细。
- 镜像填充图上传/下载基线之间保留 1 个点行间距；下载基线固定使用下载序列蓝色。

## [0.5.0] - 2026-10-21

### Added

- **btop 式系统状态页**（A8/A9）：CPU（总占用 / 最忙核镜像图、两列多核、用户/系统/IO、温度/负载、型号与平均频率）、内存、磁盘（挂载点、inode、读写历史曲线）、网络（宽面板左图＋独立速率子框、IP、错误/丢包）与 Docker/Caddy 状态卡；主图保留 2 分钟历史。
- **Docker 服务**（A10，属范围扩张）：通过 Unix socket 读取容器列表/统计，卡片列出容器与 `UP`/`EXIT` 状态，弹窗支持启动、停止、重启、刷新及最近 200 行日志（滚动、过滤）；只调用受控端点，不做 exec/删除/镜像/Compose 管理。
- **Caddy 服务**（A10，属范围扩张）：只读管理 API/systemd 状态、路由数、上游健康、域名与目的端口；弹窗支持刷新和热重载 Caddyfile，不做启停或配置编辑。
- **AI 用量面板**：OpenCode Go 与 Command Code 的 5 小时 / 每周 / 月度额度，使用方块计量条；设置弹窗中的勾选框控制展示，出错时可点击面板重试。
- **设置 / Docker / Caddy 弹窗**：Canvas 整屏灰化、弹窗居中覆盖，点击外侧关闭；设置支持保存/取消/关闭、Key 掩码与「关于」信息。
- **只读细节**：CPU 平均频率与 usr/sys/io 分解、网卡 IPv4 与错误/丢包、磁盘 `%util` 与 inode、Docker 列头；面板标题行可放大到整屏（bottom expand 式）。

### Changed

- **单页 btop 布局**：宽屏 CPU 一整行，下半区左侧内存/磁盘与整行网络，右侧 Docker/Caddy/可见 AI 用量；窄屏回退两栏＋服务摘要。面板线框按 btop 分色、盒子零间隙，分类标题用细线连接标签和数值。
- **纯鼠标交互**：键盘只保留 `q` 退出；设置按钮在标题栏右上角，所有操作改为点击按钮/行，滚动用鼠标滚轮；弹窗内的 `Esc`、`D`/`C` 等快捷键提示全部移除。
- **AI 面板固定 5 行**（3 行窗口内容＋线框），页面变高时富余高度给 Docker/Caddy；Docker 的 CPU/内存总量放在内容区首行。
- CPU 多核小图改为**一个 braille 点列 1 秒**，只取最近约 20 秒；历史柱体固定蓝色，百分比文字按阈值着色。主图仍保留 2 分钟。
- 弹窗、Docker/Caddy 状态卡与设置文案全部改为鼠标按钮语义；Docker/Caddy 卡片去掉底部管理提示和填充分隔线。

### Removed

- 顶部工具 tab 栏、底部状态栏/快捷键栏，以及 `s`/`d`/`c`/`Esc`/`R`/方向键/`PgUp`/`PgDn` 等快捷键（`q` 仍退出）。
- 旧的 `internal/gauge` 三列点阵画法；旧设置独立视图与 `[ 返回 ]` 按钮。
- Docker 操作二级菜单/二次确认（改为弹窗内直接按钮）。

### Fixed

- 设置弹窗按钮、长状态文案不再挤出线框；Docker/Caddy 内容不足时不再补分隔线占行。
- 去掉重复的 Swap 展示、过时的快捷键提示，以及 Docker/Caddy 卡片的 `D/C 管理` 文字。
- 修正 CPU 标题频率位置、多核历史小图不左移/颜色随当前值变化的问题。

## [0.3.0] - 2026-10-05

### Added

- 「Command Code 用量」tab（`A6`，属范围扩张，2026-10-05 用户确认）：轮询 Command Code 的 alpha 用量端点（`alpha/whoami`、`alpha/billing/credits`、`alpha/billing/subscriptions`、`alpha/usage/summary`，`Authorization: Bearer`），以三列点阵显示 5 小时窗 / 每周窗 / 月度额度与重置倒计时，展示格式与 OpenCode 页一致；401/403 或网络失败复用同一套文案与重试策略
- 配置字段按提供商区分：`openCodeApiKey`（原通用的 `apiKey` 更名）与 `commandCodeApiKey`；设置页增加 `Command Code Key` 可编辑行（只写回 `commandCodeApiKey`，不影响 `openCodeApiKey`）
- 配置迁移：打开新版时自动把旧配置的 `apiKey` 一次性改写为 `openCodeApiKey` 并删除旧字段（`config.Migrate`，程序启动时调用）
- 抽出新包 `internal/gauge`：三列点阵渲染（点阵/标签/倒计时）由 OpenCode 页与 Command Code 页共用

### Changed

- 顶栏标签增至三个：`OpenCode Go 用量`、`Command Code 用量`、`设置`；窄屏短名取最后一个词，重名时退到两个词（两个用量页为 `Go 用量` / `Code 用量`）；命中区间按宽度钳制

### Fixed

- Command Code 页在「已配置 key 但首帧数据还没到」时渲染会空指针崩溃（pty 冒烟发现的回归，已修并补测试）

## [0.2.1] - 2026-09-28

### Changed

- `scripts/mkdeb.sh`：版本号默认取最近 tag ＋ `~dev`（如 `0.2.0~dev`，显式 `VERSION` 仍可覆盖）；`~` 让本地包排序低于正式版，避免本地包装过之后改回正式版需要 `--allow-downgrades`

### Fixed

- 用量刷新改为单一 tick 循环；手动刷新/保存会取消旧请求并立即发起新请求，迟到的旧响应不会覆盖新状态
- 配置文件缺失、JSON 损坏、读取失败分开提示；修复损坏 JSON 前自动保留 `.bak` 备份，不再静默覆盖，并处理 JSON 根节点为 `null` 的边界情况
- 刷新倒计时改用向上取整，与旧 JS 实现的 `Math.ceil` 一致
- 鼠标点击只响应左键，右键不再误切 tab 或进入设置编辑态

### Added

- 增加 push/PR CI（格式、模块文件、vet、单测、race、双架构交叉编译）；发布前执行同样的固定检查，GoReleaser 不再运行 `go mod tidy`
- `wl` 新增 `--help`/`-h`，未知参数给出错误与帮助并以退出码 2 结束

## [0.2.0] - 2026-09-28

### Added

- 「设置」tab（`A5`，属范围扩张，2026-09-28 用户确认）：
  - `API Key` 行可编辑并写回配置文件；保留文件里其他未知字段，目录 `0700`、文件 `0600`，写临时文件再 rename
  - 保存空串等于清除 key；保存成功后立即重读配置并刷新用量
  - 非编辑态掩码显示 key（前 6 位 ＋ `…` ＋ 后 4 位）
  - 「关于」区块：版本、构建日期、commit、Go 版本、平台/架构、仓库地址
- 构建信息注入：`version` / `commit` / `date` 由 ldflags 注入（`.goreleaser.yaml`、`scripts/mkdeb.sh`），缺失时回退 Go 构建信息（模块版本 / `vcs.revision` / `vcs.time`）
- 新依赖：`charm.land/bubbles/v2`（`textinput`）

### Changed

- 框架支持多标签：切到设置页时用量轮询继续（每秒 tick 仍喂给用量模型）；切标签时设置页重读配置并退出编辑态
- 设置页 `↑`/`↓` 用于选行，不再当作内容区滚动；编辑态下 `q`/`R`/数字等全局键让位给输入框（`Ctrl+C` 仍退出）
- 用量页三列点阵改为**精确水平居中**（2026-09-28 用户反馈「有点靠左」）：此前偶数宽终端偏左半格、奇数宽偏左一整格
- 设置页内容加圆角线框，整框在内容区**水平 ＋ 垂直居中**（2026-09-28 用户要求）
- 修复窄终端下标签行 / 配置文件行超宽折行：所有渲染行截到终端宽度（`usage.clampLines`）
- **窄屏适配 A：顶栏 / 状态栏逐级降级**（2026-09-28）——顶栏 全名 → 短名 → 去程序名 → 只留当前标签 ＋ `…`；状态栏 完整提示 → `Q 退出` ＋ 短状态（`401` / `未配置` / `查询中` / `58s` / `已保存`）→ 只留短名 → 只留状态；任何宽度下都不折行，顶部命中区间与渲染共用同一份计算
- **窄屏适配 B：内容区改为「自然高度 ＋ 框架居中 / 滚动」**（2026-09-28）——内容不足一屏垂直居中，超出一屏可滚动（`PgUp`/`PgDn`/滚轮；`↑`/`↓` 在用量页滚动、在设置页选行）；设置页矮屏下不再被切掉
- 修复 `PgDown` 从未生效：bubbletea v2 的键名是 `pgdown`，原代码写的 `pgdn` 是死分支
- 设置页线框内边距改为 上下各 1 行、左右各 4 格（用户 2026-09-28 要求）

## [0.1.1] - 2026-09-26

### Added

- `--version` / `-v` 打印版本号（构建时由 ldflags 注入）
- 发版 CI：推送 `v*` tag 才构建并发布到 Release，产出 `.deb` / `.rpm` / `tar.gz` / `checksums.txt`
- 可用 `go install github.com/Lifeni/with-linux/cmd/wl@latest` 安装
- `scripts/mkdeb.sh`：无外部依赖的本地 `.deb` 构建脚本

### Changed

- 左上角标题改为 `With Linux`；状态栏按键提示改大写（`Q` 退出、`R` 刷新）
- `TASK_PLAN.md` 重命名为 `PLAN.md`；README 精简
- Go 模块路径改为 `github.com/Lifeni/with-linux`

## [0.1.0] - 2026-09-25

### Added

- TUI 框架：顶部工具 tab 栏 ＋ 中间内容区 ＋ 底部状态提示三段式布局
- 首个工具「OpenCode Go 用量」：
  - 三列点阵进度（rolling / weekly / monthly），确定性哈希图案，静止不抖动、用量增长只生长不熄灭
  - 各窗口用量百分比 `[N%]` 与重置倒计时（`3d11h` / `3h11m` / `15m` / `now`）
  - 60 秒轮询、失败 10 秒重试；一轮取数最多 3 次尝试、间隔 2 秒；认证错误（401/403）不重试
  - 错误文案：`未配置 Key` / `Key 无效（401）` / `无权限（403）` / `连接失败 · 10s 后重试`
- 交互：键盘优先（`q` 退出、`←→` 切换工具、`1`–`9` 直达、`↑↓`/`PgUp`/`PgDn` 滚动、`R` 手动刷新）＋ 鼠标（点击 tab、滚轮滚动）
- 配置：`$XDG_CONFIG_HOME/with-linux/config.json`（默认 `~/.config/with-linux/config.json`），`apiKey` 字段
- 交付：单一可执行文件，全静态交叉编译 `linux/amd64` ＋ `linux/arm64`

[0.5.2]: https://github.com/Lifeni/with-linux/releases/tag/v0.5.2
[0.5.1]: https://github.com/Lifeni/with-linux/releases/tag/v0.5.1
[0.5.0]: https://github.com/Lifeni/with-linux/releases/tag/v0.5.0
[0.3.0]: https://github.com/Lifeni/with-linux/releases/tag/v0.3.0
[0.2.1]: https://github.com/Lifeni/with-linux/releases/tag/v0.2.1
[0.2.0]: https://github.com/Lifeni/with-linux/releases/tag/v0.2.0
[0.1.1]: https://github.com/Lifeni/with-linux/releases/tag/v0.1.1
[0.1.0]: https://github.com/Lifeni/with-linux/releases/tag/v0.1.0
