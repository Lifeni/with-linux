package main

import (
	"fmt"
	"io"
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

const helpText = `用法: wl [选项]

选项:
  -h, --help     显示帮助
  -v, --version  显示版本
`

type runTUIFunc func(meta.Info) error

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, runTUI))
}

func run(args []string, stdout, stderr io.Writer, runTUI runTUIFunc) int {
	info := meta.Current(version, commit, date)

	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v":
			fmt.Fprintln(stdout, "wl "+info.Version)
			return 0
		case "--help", "-h":
			fmt.Fprint(stdout, helpText)
			return 0
		default:
			fmt.Fprintf(stderr, "with-linux: 未知参数 %q\n", args[0])
			fmt.Fprint(stderr, helpText)
			return 2
		}
	}

	if err := runTUI(info); err != nil {
		fmt.Fprintln(stderr, "with-linux:", err)
		return 1
	}
	return 0
}

func runTUI(info meta.Info) error {
	p := tea.NewProgram(app.New(info))
	_, err := p.Run()
	return err
}
