package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Lifeni/with-linux/internal/app"
	"github.com/Lifeni/with-linux/internal/meta"
)

// 构建信息由 goreleaser / scripts/mkdeb.sh 通过 -ldflags "-X main.version=... -X main.commit=... -X main.date=..." 注入；
// 本地构建三者都是空/默认值，由 meta.Current 回退到 Go 构建信息。见 AGENTS.md「约束 · 版本与构建信息」。
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	info := meta.Current(version, commit, date)

	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("wl " + info.Version)
		return
	}

	p := tea.NewProgram(app.New(info))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "with-linux:", err)
		os.Exit(1)
	}
}
