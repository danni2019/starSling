package tui

import (
	"strings"

	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/runtime"
)

func (ui *UI) buildSetupScreen() tview.Primitive {
	ui.setupStatus = tview.NewTextView()
	ui.setupStatus.SetTextColor(colorMuted)
	ui.setupStatus.SetBackgroundColor(colorBackground)
	ui.setupStatus.SetBorder(true).SetTitle("Status")
	ui.setupStatus.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.setupStatus.SetText("Ready.")

	ui.setupOutput = tview.NewTextView()
	ui.setupOutput.SetTextColor(colorLogText)
	ui.setupOutput.SetBackgroundColor(colorBackground)
	ui.setupOutput.SetBorder(true).SetTitle("Output")
	ui.setupOutput.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.setupActions = tview.NewList()
	ui.setupActions.ShowSecondaryText(false)
	ui.setupActions.SetMainTextColor(colorMenu)
	ui.setupActions.SetSelectedTextColor(colorMenuSelected)
	ui.setupActions.SetSelectedBackgroundColor(colorHighlight)
	ui.setupActions.SetBackgroundColor(colorBackground)
	ui.setupActions.SetBorder(true).SetTitle("Actions")
	ui.setupActions.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.setupActions.AddItem("Run Python bootstrap", "", 0, func() {
		ui.startBootstrap()
	})
	ui.setupActions.AddItem("Back", "", 0, func() {
		ui.setScreen(screenMain)
	})

	help := tview.NewTextView()
	help.SetTextAlign(tview.AlignCenter)
	help.SetTextColor(colorMuted)
	help.SetBackgroundColor(colorBackground)
	help.SetText("Keys: Enter run  Esc back  q quit")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.setupStatus, 3, 0, false).
		AddItem(ui.setupOutput, 0, 1, false).
		AddItem(ui.setupActions, 6, 0, true).
		AddItem(help, 1, 0, false)

	layout.SetBackgroundColor(colorBackground)
	return layout
}

func (ui *UI) showSetupMenu() {
	ui.app.SetFocus(ui.setupActions)
}

func (ui *UI) startBootstrap() {
	if ui.setupRunning {
		return
	}
	ui.setupRunning = true
	ui.setupStatus.SetText("Running bootstrap...")
	ui.setupOutput.SetText("")

	go func() {
		output, err := runtime.RunBootstrap()
		ui.app.QueueUpdateDraw(func() {
			ui.setupRunning = false
			if err != nil {
				ui.setupStatus.SetText("Error: " + err.Error())
			} else {
				ui.setupStatus.SetText("Completed.")
			}
			ui.setupOutput.SetText(strings.TrimSpace(output))
		})
	}()
}
