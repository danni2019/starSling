package tui

import (
	"strings"
	"sync"

	"github.com/rivo/tview"
)

func (ui *UI) buildMetadataScreen() tview.Primitive {
	ui.metadataStatus = tview.NewTextView()
	ui.metadataStatus.SetTextColor(colorMuted)
	ui.metadataStatus.SetBackgroundColor(colorBackground)
	ui.metadataStatus.SetBorder(true).SetTitle("Status")
	ui.metadataStatus.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.metadataStatus.SetText("Ready.")

	ui.metadataOutput = tview.NewTextView()
	ui.metadataOutput.SetTextColor(colorLogText)
	ui.metadataOutput.SetBackgroundColor(colorBackground)
	ui.metadataOutput.SetBorder(true).SetTitle("Output")
	ui.metadataOutput.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.metadataActions = tview.NewList()
	ui.metadataActions.ShowSecondaryText(false)
	ui.metadataActions.SetMainTextColor(colorMenu)
	ui.metadataActions.SetSelectedTextColor(colorMenuSelected)
	ui.metadataActions.SetSelectedBackgroundColor(colorHighlight)
	ui.metadataActions.SetBackgroundColor(colorBackground)
	ui.metadataActions.SetBorder(true).SetTitle("Actions")
	ui.metadataActions.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.metadataActions.AddItem("Refresh market metadata", "", 0, func() {
		ui.startMetadataRefresh()
	})
	ui.metadataActions.AddItem("Back", "", 0, func() {
		ui.setScreen(screenMain)
	})

	help := tview.NewTextView()
	help.SetTextAlign(tview.AlignCenter)
	help.SetTextColor(colorMuted)
	help.SetBackgroundColor(colorBackground)
	help.SetText("Keys: Enter refresh  Esc back  q quit")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.metadataStatus, 3, 0, false).
		AddItem(ui.metadataOutput, 0, 1, false).
		AddItem(ui.metadataActions, 6, 0, true).
		AddItem(help, 1, 0, false)

	layout.SetBackgroundColor(colorBackground)
	return layout
}

func (ui *UI) showMetadataMenu() {
	ui.metadataActions.SetCurrentItem(0)
	ui.app.SetFocus(ui.metadataActions)
	if !ui.metadataAutoStart {
		return
	}
	ui.metadataAutoStart = false
	go func() {
		ui.app.QueueUpdateDraw(func() {
			if ui.currentScreen() != screenMetadata {
				return
			}
			ui.startMetadataRefresh()
		})
	}()
}

func (ui *UI) startMetadataRefresh() {
	if ui.metadataRunning {
		return
	}
	ui.metadataRunning = true
	ui.metadataStatus.SetText("Refreshing metadata...")
	ui.setMetadataOutputText("")

	go func() {
		var outputMu sync.Mutex
		var rawOutput strings.Builder
		err := refreshLiveMetadataFn(ui, func(chunk string) {
			outputMu.Lock()
			rawOutput.WriteString(chunk)
			outputMu.Unlock()
			ui.app.QueueUpdateDraw(func() {
				ui.appendMetadataOutputChunk(chunk)
			})
		})
		outputMu.Lock()
		output := normalizeProgressOutputText(rawOutput.String())
		outputMu.Unlock()
		ui.app.QueueUpdateDraw(func() {
			ui.finishMetadataRefresh(output, err)
		})
	}()
}

func (ui *UI) finishMetadataRefresh(output string, err error) {
	ui.metadataRunning = false
	if err != nil {
		ui.metadataStatus.SetText("Error: " + err.Error())
	} else {
		ui.metadataStatus.SetText("Completed.")
	}
	ui.setMetadataOutputText(output)
	if err != nil || !ui.metadataResumeLive {
		return
	}
	ui.metadataResumeLive = false
	ui.openLiveScreenFromMain()
}

func (ui *UI) appendMetadataOutputChunk(chunk string) {
	if ui == nil || chunk == "" {
		return
	}
	ui.metadataOutputMu.Lock()
	ui.metadataOutputText += normalizeProgressOutputChunk(chunk, &ui.metadataOutputCR)
	text := ui.metadataOutputText
	ui.metadataOutputMu.Unlock()
	ui.renderMetadataOutput(text)
}

func (ui *UI) setMetadataOutputText(text string) {
	if ui == nil {
		return
	}
	ui.metadataOutputMu.Lock()
	ui.metadataOutputCR = false
	ui.metadataOutputText = normalizeProgressOutputText(text)
	current := ui.metadataOutputText
	ui.metadataOutputMu.Unlock()
	ui.renderMetadataOutput(current)
}

func (ui *UI) renderMetadataOutput(text string) {
	if ui.metadataOutput == nil {
		return
	}
	ui.metadataOutput.SetText(text)
	ui.metadataOutput.ScrollToEnd()
}
