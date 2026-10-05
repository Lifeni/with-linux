![With Linux](assets/banner-dark.svg)

# <img src="assets/icon.svg" width="32" alt="With Linux"> With Linux

![版本](https://img.shields.io/github/v/release/Lifeni/with-linux?label=%E7%89%88%E6%9C%AC)
![协议](https://img.shields.io/github/license/Lifeni/with-linux?label=%E5%8D%8F%E8%AE%AE)

> 本项目由 AI 协作完成：代码、文档与迭代均经 AI 生成和优化。

个人自用的 Linux 终端工具箱（TUI）：面向 arm64 / amd64，SSH 使用、按需启动。命令名 **`wl`**。

## 功能

**OpenCode Go 用量查询**：轮询用量接口，三列点阵进度（5 小时 / 每周 / 每月）＋ 重置倒计时。

**Command Code 用量查询**：轮询 Command Code 用量接口，三列点阵进度（5 小时 / 每周 / 月度额度）＋ 重置倒计时。

**设置**：填写各提供商的 API Key（OpenCode Go 与 Command Code）并写回配置文件；「关于」里显示版本、构建日期、commit、Go 版本与平台。

界面是「顶部工具 tab 栏 ＋ 中间内容区 ＋ 底部状态提示」，键盘优先、鼠标可用，窄屏逐级降级。

## 文档

安装、配置与开发约定的细节都在下面几份里：

- [AGENTS.md](AGENTS.md) —— 项目章程：目标 / 非目标 / 验收标准 / 界面布局 / 工作流指令
- [CHANGELOG.md](CHANGELOG.md) —— 版本变更
- [PLAN.md](PLAN.md) · [FINDINGS.md](FINDINGS.md) · [PROGRESS.md](PROGRESS.md) —— 计划、调研结论、进度与验证记录
- [web/](web/README.md) —— 介绍页（本地预览用，暂不对外发布）

## 开发

```bash
go build -o wl ./cmd/wl     # 交叉编译：GOOS=linux GOARCH=arm64 go build -o wl ./cmd/wl

gofmt -l .
go mod tidy
go vet ./...
go test ./...
```
