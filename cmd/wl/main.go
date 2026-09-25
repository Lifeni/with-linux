package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Lifeni/with-linux/internal/app"
)

// version 由 goreleaser 通过 -ldflags "-X main.version=..." 注入；本地构建显示 dev。
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("wl " + version)
		return
	}
	p := tea.NewProgram(app.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "with-linux:", err)
		os.Exit(1)
	}
}
