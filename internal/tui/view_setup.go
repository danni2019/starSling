package tui

import "github.com/rivo/tview"

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
	ui.setupActions.SetCurrentItem(0)
	ui.app.SetFocus(ui.setupActions)
	if !ui.setupAutoStart {
		return
	}
	ui.setupAutoStart = false
	go func() {
		ui.app.QueueUpdateDraw(func() {
			if ui.currentScreen() != screenSetup {
				return
			}
			ui.startBootstrap()
		})
	}()
}

func (ui *UI) startBootstrap() {
	if ui.setupRunning {
		return
	}
	ui.setupRunning = true
	ui.setupStatus.SetText("Running bootstrap...")
	ui.setSetupOutputText("")

	go func() {
		output, err := runBootstrapStreamFn(func(chunk string) {
			ui.app.QueueUpdateDraw(func() {
				ui.appendSetupOutputChunk(chunk)
			})
		})
		ui.app.QueueUpdateDraw(func() {
			ui.finishBootstrap(output, err)
		})
	}()
}

func (ui *UI) finishBootstrap(output string, err error) {
	ui.setupRunning = false
	if err != nil {
		ui.setupStatus.SetText("Error: " + err.Error())
	} else {
		ui.setupStatus.SetText("Completed.")
	}
	ui.setSetupOutputText(output)
	if err != nil || !ui.setupResumeLive {
		return
	}
	ui.setupResumeLive = false
	ui.openLiveScreenFromMain()
}

func (ui *UI) appendSetupOutputChunk(chunk string) {
	if ui == nil || chunk == "" {
		return
	}
	ui.setupOutputMu.Lock()
	ui.setupOutputText += normalizeProgressOutputChunk(chunk, &ui.setupOutputCR)
	text := ui.setupOutputText
	ui.setupOutputMu.Unlock()
	ui.renderSetupOutput(text)
}

func (ui *UI) setSetupOutputText(text string) {
	if ui == nil {
		return
	}
	ui.setupOutputMu.Lock()
	ui.setupOutputCR = false
	ui.setupOutputText = normalizeProgressOutputText(text)
	current := ui.setupOutputText
	ui.setupOutputMu.Unlock()
	ui.renderSetupOutput(current)
}

func (ui *UI) renderSetupOutput(text string) {
	if ui.setupOutput == nil {
		return
	}
	ui.setupOutput.SetText(text)
	ui.setupOutput.ScrollToEnd()
}

func normalizeProgressOutputText(text string) string {
	pendingCR := false
	return normalizeProgressOutputChunk(text, &pendingCR)
}

func normalizeProgressOutputChunk(chunk string, pendingCR *bool) string {
	if chunk == "" {
		return ""
	}
	var out []rune
	for _, r := range chunk {
		if pendingCR != nil && *pendingCR {
			if r == '\n' {
				out = append(out, '\n')
				*pendingCR = false
				continue
			}
			out = append(out, '\n')
			*pendingCR = false
		}

		if r == '\r' {
			if pendingCR != nil {
				*pendingCR = true
			} else {
				out = append(out, '\n')
			}
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
