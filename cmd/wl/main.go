package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"with-linux/internal/app"
)

func main() {
	p := tea.NewProgram(app.New())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "with-linux:", err)
		os.Exit(1)
	}
}
