# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

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

[0.1.0]: https://github.com/Lifeni/with-linux/releases/tag/v0.1.0
