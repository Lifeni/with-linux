package main

import (
	"fmt"
	"os"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/Lifeni/with-linux/internal/app"
)

// version 由 goreleaser 通过 -ldflags "-X main.version=..." 注入；本地构建显示 dev。
var version = "dev"

// versionString 优先取 ldflags 注入的版本；go install 从源码构建时
// 回退读 Go 注入的模块版本（如 v0.1.2），都拿不到才显示 dev。
func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("wl " + versionString())
		return
	}
	p := tea.NewProgram(app.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "with-linux:", err)
		os.Exit(1)
	}
}
