package tui

import (
	"fmt"
	"os"
)

func Run() int {
	ui := newUI()
	if err := ui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
