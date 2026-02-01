package tui

import (
	"strings"

	"github.com/rivo/tview"
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
	ui.divider.SetTextAlign(tview.AlignCenter)
	ui.divider.SetTextColor(colorMuted)
	ui.divider.SetBackgroundColor(colorBackground)

	ui.menu = tview.NewList()
	ui.menu.ShowSecondaryText(false)
	ui.menu.SetMainTextColor(colorMenu)
	ui.menu.SetSelectedTextColor(colorMenuSelected)
	ui.menu.SetSelectedBackgroundColor(colorHighlight)
	ui.menu.SetBackgroundColor(colorBackground)

	ui.menu.AddItem("Live market data", "", 0, func() {
		ui.setScreen(screenLive)
	})
	ui.menu.AddItem("Setup Python runtime", "", 0, func() {
		ui.setScreen(screenSetup)
	})
	ui.menu.AddItem("Config", "", 0, func() {
		ui.setScreen(screenConfig)
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
		AddItem(menuWrap, 6, 0, true).
		AddItem(help, 1, 0, false)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox().SetBackgroundColor(colorBackground), 0, 1, false).
		AddItem(content, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(colorBackground), 0, 1, false)

	root.SetBackgroundColor(colorBackground)
	return root
}

func (ui *UI) updateLogo(width int) {
	logoWidth := min(width, maxLogoWidth)
	logo := RenderLogo(logoWidth)
	if len(logo) < logoHeight {
		return
	}
	leftPad := max(0, (width-logoWidth)/2)
	padding := strings.Repeat(" ", leftPad)
	curve := make([]string, 0, curveHeight)
	title := make([]string, 0, titleHeight)
	for _, line := range logo[:curveHeight] {
		curve = append(curve, padding+line)
	}
	for _, line := range logo[curveHeight:] {
		title = append(title, padding+line)
	}
	ui.curveView.SetText(strings.Join(curve, "\n"))
	ui.titleView.SetText(strings.Join(title, "\n"))
}

func (ui *UI) updateDivider(width int) {
	dividerWidth := min(width-4, maxLogoWidth)
	if dividerWidth < 0 {
		dividerWidth = 0
	}
	leftPad := max(0, (width-dividerWidth)/2)
	ui.divider.SetText(strings.Repeat(" ", leftPad) + strings.Repeat("-", dividerWidth))
}
