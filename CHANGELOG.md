# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Changed

- `scripts/mkdeb.sh`：版本号默认取最近 tag ＋ `~dev`（如 `0.2.0~dev`，显式 `VERSION` 仍可覆盖）；`~` 让本地包排序低于正式版，避免本地包装过之后改回正式版需要 `--allow-downgrades`

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

[0.2.0]: https://github.com/Lifeni/with-linux/releases/tag/v0.2.0
[0.1.1]: https://github.com/Lifeni/with-linux/releases/tag/v0.1.1
[0.1.0]: https://github.com/Lifeni/with-linux/releases/tag/v0.1.0
