package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (ui *UI) buildLiveScreen() tview.Primitive {
	ui.liveMarket = tview.NewTable()
	ui.liveMarket.SetSelectable(true, false)
	ui.liveMarket.SetFixed(1, 0)
	ui.liveMarket.SetSelectedStyle(tcell.StyleDefault.Foreground(colorMenuSelected).Background(colorHighlight))
	ui.liveMarket.SetBorder(true).SetTitle("Market (select a contract)")
	ui.liveMarket.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.liveLog = tview.NewTextView()
	ui.liveLog.SetDynamicColors(false)
	ui.liveLog.SetTextColor(colorLogText)
	ui.liveLog.SetBackgroundColor(colorBackground)
	ui.liveLog.SetBorder(true).SetTitle("Runtime log")
	ui.liveLog.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.liveCurve = tview.NewTextView()
	ui.liveCurve.SetTextColor(colorMuted)
	ui.liveCurve.SetBackgroundColor(colorBackground)
	ui.liveCurve.SetBorder(true).SetTitle("Forward curve + VIX (2nd axis)")
	ui.liveCurve.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.liveCurve.SetText(renderChartPlaceholder())

	ui.liveOpts = tview.NewTextView()
	ui.liveOpts.SetTextColor(colorMuted)
	ui.liveOpts.SetBackgroundColor(colorBackground)
	ui.liveOpts.SetBorder(true).SetTitle("Options chain (IV curve / OI / Greeks)")
	ui.liveOpts.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.liveOpts.SetText(renderChartPlaceholder())

	ui.liveTrades = tview.NewTable()
	ui.liveTrades.SetSelectable(false, false)
	ui.liveTrades.SetFixed(1, 0)
	ui.liveTrades.SetBorder(true).SetTitle("Unusual option trades (newest at top)")
	ui.liveTrades.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.liveMarket, 0, 7, true).
		AddItem(ui.liveLog, 0, 3, false)

	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.liveCurve, 0, 1, false).
		AddItem(ui.liveOpts, 0, 1, false).
		AddItem(ui.liveTrades, 0, 1, false)

	root := tview.NewFlex().
		AddItem(left, 0, 3, true).
		AddItem(right, 0, 2, false)

	root.SetBackgroundColor(colorBackground)

	ui.focusables = []tview.Primitive{
		ui.liveMarket,
		ui.liveCurve,
		ui.liveOpts,
		ui.liveTrades,
		ui.liveLog,
	}
	ui.focusIndex = 0
	ui.updateLiveData()

	return root
}

func (ui *UI) updateLiveData() {
	if ui.liveMarket == nil {
		return
	}
	fillMarketTable(ui.liveMarket, ui.data.MarketRows)
	fillTradesTable(ui.liveTrades, ui.data.Trades)
	fillLog(ui.liveLog, ui.data.Logs)
}

func fillMarketTable(table *tview.Table, rows []MarketRow) {
	table.Clear()
	headers := []string{"SYMBOL", "LAST", "CHG", "BID", "ASK", "VOL", "OI", "TS"}
	for col, label := range headers {
		cell := tview.NewTableCell(label).
			SetTextColor(colorTableHeader).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		table.SetCell(0, col, cell)
	}

	for i, row := range rows {
		values := []string{row.Symbol, row.Last, row.Chg, row.Bid, row.Ask, row.Vol, row.OI, row.TS}
		for col, value := range values {
			cell := tview.NewTableCell(value).
				SetTextColor(colorTableRow).
				SetAlign(tview.AlignLeft)
			table.SetCell(i+1, col, cell)
		}
	}
}

func fillTradesTable(table *tview.Table, trades []TradeRow) {
	table.Clear()
	headers := []string{"TIME", "SYM", "CP", "K", "PX", "SZ", "IV", "TAG"}
	for col, label := range headers {
		cell := tview.NewTableCell(label).
			SetTextColor(colorTableHeader).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		table.SetCell(0, col, cell)
	}

	for i, trade := range trades {
		values := []string{trade.Time, trade.Sym, trade.CP, trade.Strike, trade.Price, trade.Size, trade.IV, trade.Tag}
		for col, value := range values {
			cell := tview.NewTableCell(value).
				SetTextColor(colorTableRow).
				SetAlign(tview.AlignLeft)
			table.SetCell(i+1, col, cell)
		}
	}
}

func fillLog(view *tview.TextView, logs []LogLine) {
	lines := make([]string, 0, len(logs))
	for _, line := range logs {
		lines = append(lines, fmt.Sprintf("[%s] %s", line.Time, line.Message))
	}
	view.SetText(strings.Join(lines, "\n"))
}

func renderChartPlaceholder() string {
	return strings.Join([]string{
		"  . . . . . . . . . .",
		" .               .  .",
		".                  . ",
		" .                .  ",
		"  . . . . . . . . .  ",
		"",
		"placeholder chart - filled by python later",
	}, "\n")
}
