package tui

import "github.com/rivo/tview"

func buildPlaceholderScreen(title string) tview.Primitive {
	view := tview.NewTextView()
	view.SetTextAlign(tview.AlignCenter)
	view.SetTextColor(colorMuted)
	view.SetBackgroundColor(colorBackground)
	view.SetText(title + "\n\nPress Esc to return.")
	view.SetBorder(true).SetTitle(title)
	view.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	return view
}
