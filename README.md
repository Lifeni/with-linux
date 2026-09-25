# with-linux

个人自用的 Linux 终端工具箱（TUI）：arm64 开发板、SSH 使用、按需启动。命令名 **`wl`**。
当前版本 v0.1.0，首个工具是「OpenCode Go 用量查询」。

## 功能

- **OpenCode Go 用量查询**：三列点阵进度（rolling / weekly / monthly）＋ 各窗口重置倒计时
- 60s 自动轮询、失败重试；401 / 403 / 网络失败有明确错误提示
- 键盘优先、鼠标可用（点击 tab、滚轮滚动）

## 安装

从 [Releases](https://github.com/Lifeni/with-linux/releases) 下载二进制（单一可执行、全静态）放进 `PATH`，或源码构建：

```bash
go build -o wl ./cmd/wl
```

## 配置

`~/.config/with-linux/config.json`（遵循 XDG）：

```json
{ "apiKey": "oc_sk_..." }
```

配置缺失时面板会提示期望的路径，不会崩溃。

## 操作

| 操作 | 键盘 | 鼠标 |
|------|------|------|
| 退出 | `Q` / `Esc` / `Ctrl+C` | — |
| 切换工具 | `←` `→` / `Tab` / `1`–`9` | 点击顶部 tab |
| 滚动 | `↑` `↓` / `PgUp` `PgDn` | 滚轮 |
| 刷新 | `R` | — |

设计与开发文档见 [AGENTS.md](AGENTS.md)、[PLAN.md](PLAN.md)、[CHANGELOG.md](CHANGELOG.md)。

## License

[MIT](LICENSE) © 2026 Lifeni
