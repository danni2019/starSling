package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/settingsstore"
)

func (ui *UI) buildSettingsScreen() tview.Primitive {
	ui.settingsStatus = tview.NewTextView()
	ui.settingsStatus.SetTextColor(colorMuted)
	ui.settingsStatus.SetBackgroundColor(colorBackground)
	ui.settingsStatus.SetBorder(true).SetTitle("Status")
	ui.settingsStatus.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.settingsForm = tview.NewForm()
	ui.settingsForm.SetBorder(true).SetTitle("Settings")
	ui.settingsForm.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.settingsForm.SetBackgroundColor(colorBackground)
	ui.settingsForm.SetFieldBackgroundColor(colorBackground)
	ui.settingsForm.SetFieldTextColor(colorTableRow)
	ui.settingsForm.SetButtonBackgroundColor(colorHighlight)
	ui.settingsForm.SetButtonTextColor(colorMenuSelected)

	ui.settingsInputRiskFree = tview.NewInputField().SetLabel("Risk-free rate: ")
	ui.settingsInputDaysInYear = tview.NewInputField().SetLabel("Days in year (1-370): ")
	ui.settingsInputDaysInYear.SetAcceptanceFunc(func(text string, ch rune) bool {
		if ch == 0 {
			return true
		}
		return (ch >= '0' && ch <= '9') || ch == '-' || ch == '+'
	})
	ui.settingsInputGammaFront = tview.NewInputField().SetLabel("Gamma front max days: ")
	ui.settingsInputGammaFront.SetAcceptanceFunc(func(text string, ch rune) bool {
		if ch == 0 {
			return true
		}
		return (ch >= '0' && ch <= '9') || ch == '-' || ch == '+'
	})
	ui.settingsInputGammaMid = tview.NewInputField().SetLabel("Gamma mid max days: ")
	ui.settingsInputGammaMid.SetAcceptanceFunc(func(text string, ch rune) bool {
		if ch == 0 {
			return true
		}
		return (ch >= '0' && ch <= '9') || ch == '-' || ch == '+'
	})

	ui.settingsForm.AddFormItem(ui.settingsInputRiskFree)
	ui.settingsForm.AddFormItem(ui.settingsInputDaysInYear)
	ui.settingsForm.AddFormItem(ui.settingsInputGammaFront)
	ui.settingsForm.AddFormItem(ui.settingsInputGammaMid)

	ui.settingsForm.AddButton("Save", func() {
		ui.saveSettingsForm()
	})
	ui.settingsForm.AddButton("Reset to defaults", func() {
		ui.populateSettingsForm(settingsstore.Default())
		ui.setSettingsStatus("reset form to defaults (not saved)")
	})
	ui.settingsForm.AddButton("Back", func() {
		ui.setScreen(screenMain)
	})

	help := tview.NewTextView()
	help.SetTextAlign(tview.AlignCenter)
	help.SetTextColor(colorMuted)
	help.SetBackgroundColor(colorBackground)
	help.SetText("Keys: Enter select  Esc back  q quit")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.settingsForm, 0, 1, true).
		AddItem(ui.settingsStatus, 3, 0, false).
		AddItem(help, 1, 0, false)
	layout.SetBackgroundColor(colorBackground)
	return layout
}

func (ui *UI) showSettingsScreen() {
	cfg, err := settingsstore.Load()
	ui.populateSettingsForm(cfg)
	if err != nil {
		ui.setSettingsStatus("load warning: " + err.Error())
	} else {
		ui.setSettingsStatus(" ")
	}
	if ui.settingsForm != nil {
		ui.app.SetFocus(ui.settingsForm)
	}
}

func (ui *UI) populateSettingsForm(cfg settingsstore.Settings) {
	if ui.settingsInputRiskFree != nil {
		ui.settingsInputRiskFree.SetText(strconv.FormatFloat(cfg.RiskFreeRate, 'f', 6, 64))
	}
	if ui.settingsInputDaysInYear != nil {
		ui.settingsInputDaysInYear.SetText(strconv.Itoa(cfg.DaysInYear))
	}
	if ui.settingsInputGammaFront != nil {
		ui.settingsInputGammaFront.SetText(strconv.Itoa(cfg.GammaBucketFrontDays))
	}
	if ui.settingsInputGammaMid != nil {
		ui.settingsInputGammaMid.SetText(strconv.Itoa(cfg.GammaBucketMidDays))
	}
}

func (ui *UI) setSettingsStatus(message string) {
	if ui.settingsStatus == nil {
		return
	}
	if strings.TrimSpace(message) == "" {
		message = " "
	}
	ui.settingsStatus.SetText(message)
}

func (ui *UI) saveSettingsForm() {
	current, loadErr := settingsstore.Load()
	if loadErr != nil {
		ui.setSettingsStatus("load warning: " + loadErr.Error())
	}
	riskFree, err := strconv.ParseFloat(strings.TrimSpace(ui.settingsInputRiskFree.GetText()), 64)
	if err != nil || math.IsNaN(riskFree) || math.IsInf(riskFree, 0) {
		ui.setSettingsStatus("invalid risk_free_rate")
		return
	}
	daysInYear, ok := parseIntInRange(ui.settingsInputDaysInYear.GetText(), 1, 370)
	if !ok {
		ui.setSettingsStatus("invalid days_in_year: must be integer in (0, 370]")
		return
	}
	frontDays, ok := parseIntInRange(ui.settingsInputGammaFront.GetText(), 1, 370)
	if !ok {
		ui.setSettingsStatus("invalid gamma front days: must be integer in (0, 370]")
		return
	}
	midDays, ok := parseIntInRange(ui.settingsInputGammaMid.GetText(), 1, 370)
	if !ok || midDays <= frontDays {
		ui.setSettingsStatus("invalid gamma mid days: must be integer and > front")
		return
	}

	restartOptionsWorker := current.RiskFreeRate != riskFree || current.DaysInYear != daysInYear
	current.RiskFreeRate = riskFree
	current.DaysInYear = daysInYear
	current.GammaBucketFrontDays = frontDays
	current.GammaBucketMidDays = midDays
	if err := settingsstore.Save(current); err != nil {
		ui.setSettingsStatus(err.Error())
		return
	}
	ui.setSettingsStatus("settings saved")
	ui.appendLiveLogLine(fmt.Sprintf("settings saved: r=%.6f days=%d gamma_buckets=%d/%d", riskFree, daysInYear, frontDays, midDays))
	ui.applyGlobalSettingsRuntime(frontDays, midDays, restartOptionsWorker)
}
