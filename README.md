# with-linux

[![Release](https://img.shields.io/github/v/release/Lifeni/with-linux)](https://github.com/Lifeni/with-linux/releases)
[![License](https://img.shields.io/github/license/Lifeni/with-linux)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Lifeni/with-linux)](go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/Lifeni/with-linux.svg)](https://pkg.go.dev/github.com/Lifeni/with-linux)

个人自用的 Linux 终端工具箱（TUI）：面向 arm64/amd64，SSH 使用、按需启动。命令名 **`wl`**。

## 功能

- OpenCode Go 用量查询：三列点阵进度与重置倒计时
- 设置：填写 OpenCode Go 的 API Key（写回配置文件）＋「关于」里的版本/构建信息

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

配置缺失时面板会提示期望的路径，不会崩溃。JSON 损坏或文件无法读取时会显示对应错误；在设置页重新保存 Key 修复损坏文件时，原文件会自动备份为 `.bak`。

日常开发检查：

```bash
gofmt -l .
go mod tidy
go vet ./...
go test ./...
```

设计与开发文档见 [AGENTS.md](AGENTS.md)、[PLAN.md](PLAN.md)、[CHANGELOG.md](CHANGELOG.md)。

## License

[MIT](LICENSE) © 2026 Lifeni
