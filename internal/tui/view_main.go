package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/configstore"
)

func (ui *UI) buildMainScreen() tview.Primitive {
	ui.curveView = tview.NewTextView()
	ui.curveView.SetTextAlign(tview.AlignLeft)
	ui.curveView.SetTextColor(colorCurve)
	ui.curveView.SetBackgroundColor(colorBackground)

	ui.titleView = tview.NewTextView()
	ui.titleView.SetTextAlign(tview.AlignLeft)
	ui.titleView.SetTextColor(colorTitle)
	ui.titleView.SetBackgroundColor(colorBackground)

	author := tview.NewTextView()
	author.SetTextAlign(tview.AlignCenter)
	author.SetTextColor(colorMuted)
	author.SetBackgroundColor(colorBackground)
	author.SetText("by Yang | github: danni2019/starSling | email: muzexlxl@foxmail.com")

	ui.divider = tview.NewTextView()
	ui.divider.SetTextAlign(tview.AlignLeft)
	ui.divider.SetTextColor(colorMuted)
	ui.divider.SetBackgroundColor(colorBackground)

	ui.menu = tview.NewList()
	ui.menu.ShowSecondaryText(false)
	ui.menu.SetMainTextColor(colorMenu)
	ui.menu.SetSelectedTextColor(colorMenuSelected)
	ui.menu.SetSelectedBackgroundColor(colorHighlight)
	ui.menu.SetBackgroundColor(colorBackground)

	ui.menu.AddItem("Live market data", "", 0, func() {
		ui.openLiveScreenFromMain()
	})
	ui.menu.AddItem("Setup Python runtime", "", 0, func() {
		ui.openSetupScreen(false, false)
	})
	ui.menu.AddItem("Config", "", 0, func() {
		ui.setScreen(screenConfig)
	})
	ui.menu.AddItem("Settings", "", 0, func() {
		ui.setScreen(screenSettings)
	})
	ui.menu.AddItem("Quit", "", 0, func() {
		ui.app.Stop()
	})

	help := tview.NewTextView()
	help.SetTextAlign(tview.AlignCenter)
	help.SetTextColor(colorMuted)
	help.SetBackgroundColor(colorBackground)
	help.SetText("Keys: ↑/↓ move  Enter select  q quit")

	menuWrap := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(ui.menu, 32, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.curveView, curveHeight, 0, false).
		AddItem(ui.titleView, titleHeight, 0, false).
		AddItem(author, 1, 0, false).
		AddItem(ui.divider, 1, 0, false).
		AddItem(menuWrap, 7, 0, true).
		AddItem(help, 1, 0, false)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox().SetBackgroundColor(colorBackground), 0, 1, false).
		AddItem(content, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(colorBackground), 0, 1, false)

	root.SetBackgroundColor(colorBackground)
	return root
}

func (ui *UI) openLiveScreenFromMain() {
	if runtimeBootstrapNeeded() {
		ui.promptRuntimeBootstrapRequired(
			"Live Market Data requires a local Python runtime first.\n\nOpen Setup to run the bundled bootstrap flow before continuing.",
			true,
		)
		return
	}
	configName, cfg, err := configstore.LoadDefault()
	if err != nil {
		ui.promptLiveConfigRequired(fmt.Sprintf("Load config failed.\n\nConfigure Host and Port in Config before entering Live Market Data.\n\nDetails: %s", err.Error()))
		return
	}
	if err := cfg.ValidateLiveMD(); err != nil {
		ui.promptLiveConfigRequired(fmt.Sprintf("Config %q is not ready for Live Market Data.\n\nConfigure Host and Port in Config before continuing.\n\nDetails: %s", configName, err.Error()))
		return
	}
	ui.setScreen(screenLive)
}

func (ui *UI) promptRuntimeBootstrapRequired(message string, resumeLive bool) {
	ui.showModal("runtime-bootstrap-required", message, []string{"Open Setup", "Later"}, func(index int, _ string) {
		if index == 0 {
			ui.openSetupScreen(true, resumeLive)
			return
		}
		ui.app.SetFocus(ui.menu)
	})
}

func (ui *UI) promptLiveConfigRequired(message string) {
	ui.showModal("live-config-required", message, []string{"Open Config", "Cancel"}, func(index int, _ string) {
		if index == 0 {
			if ui.configStatus != nil {
				ui.setConfigStatus("Configure Host and Port before starting Live Market Data.")
			}
			ui.setScreen(screenConfig)
			return
		}
		ui.app.SetFocus(ui.menu)
	})
}

func (ui *UI) updateLogo(width int) {
	_, _, curveWidth, _ := ui.curveView.GetInnerRect()
	_, _, titleViewWidth, _ := ui.titleView.GetInnerRect()
	logoWidth := min(min(curveWidth, titleViewWidth), maxLogoWidth)
	logo := RenderLogo(logoWidth, ui.logoFrame)
	if len(logo) < logoHeight {
		return
	}
	titleLines := logo[curveHeight:]
	titleLeft, titleRight := blockBounds(titleLines)
	titleBlockWidth := 0
	if titleLeft >= 0 && titleRight >= titleLeft {
		titleBlockWidth = titleRight - titleLeft + 1
	}
	ui.logoTitleWidth = titleBlockWidth
	titleShift := 0
	if titleBlockWidth > 0 {
		titleShift = (logoWidth-titleBlockWidth)/2 - titleLeft
	}
	curvePad := max(0, (curveWidth-logoWidth)/2)
	titlePad := max(0, (titleViewWidth-logoWidth)/2)
	curvePadding := strings.Repeat(" ", curvePad)
	titlePadding := strings.Repeat(" ", titlePad)
	curve := make([]string, 0, curveHeight)
	title := make([]string, 0, titleHeight)
	for _, line := range logo[:curveHeight] {
		curve = append(curve, curvePadding+line)
	}
	for _, line := range titleLines {
		shifted := shiftLine(line, titleShift)
		title = append(title, titlePadding+padRight(shifted, logoWidth))
	}
	ui.curveView.SetText(strings.Join(curve, "\n"))
	ui.titleView.SetText(strings.Join(title, "\n"))
}

func (ui *UI) updateDivider(width int) {
	_, _, dividerWidthRaw, _ := ui.divider.GetInnerRect()
	dividerWidth := dividerWidthRaw
	if ui.logoTitleWidth > 0 {
		dividerWidth = ui.logoTitleWidth
	}
	dividerWidth = min(dividerWidth, dividerWidthRaw)
	dividerWidth = min(dividerWidth, maxLogoWidth)
	if dividerWidth < 0 {
		dividerWidth = 0
	}
	leftPad := max(0, (dividerWidthRaw-dividerWidth)/2)
	ui.divider.SetText(strings.Repeat(" ", leftPad) + strings.Repeat("-", dividerWidth))
}

func blockBounds(lines []string) (int, int) {
	left := -1
	right := -1
	for _, line := range lines {
		col := 0
		for _, r := range line {
			if r == ' ' {
				col++
				continue
			}
			if left == -1 || col < left {
				left = col
			}
			if col > right {
				right = col
			}
			col++
		}
	}
	return left, right
}

func shiftLine(line string, shift int) string {
	if shift == 0 {
		return line
	}
	if shift > 0 {
		return strings.Repeat(" ", shift) + line
	}
	remove := -shift
	runes := []rune(line)
	i := 0
	for i < len(runes) && remove > 0 {
		if runes[i] != ' ' {
			break
		}
		i++
		remove--
	}
	return string(runes[i:])
}
