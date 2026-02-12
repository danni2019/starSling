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
	ui.liveMarket.SetSelectionChangedFunc(func(row int, _ int) {
		if row <= 0 {
			return
		}
		cell := ui.liveMarket.GetCell(row, 0)
		if cell == nil {
			return
		}
		symbol := strings.TrimSpace(cell.Text)
		if symbol == "" {
			return
		}
		ui.focusSymbol = symbol
		ui.focusSyncPending = false
		if ui.rpcClient != nil {
			ui.pushFocusSymbol(symbol)
		}
		ui.renderFlowAggregation()
	})

	if runtimeDebugUIEnabled() {
		ui.liveLog = tview.NewTextView()
		ui.liveLog.SetDynamicColors(false)
		ui.liveLog.SetTextColor(colorLogText)
		ui.liveLog.SetBackgroundColor(colorBackground)
		ui.liveLog.SetBorder(true).SetTitle("Runtime log (internal)")
		ui.liveLog.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	} else {
		ui.liveLog = nil
	}

	ui.liveFlow = tview.NewTable()
	ui.liveFlow.SetSelectable(false, false)
	ui.liveFlow.SetFixed(1, 0)
	ui.liveFlow.SetBorder(true).SetTitle("Flow Aggregation")
	ui.liveFlow.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.liveCurve = tview.NewTextView()
	ui.liveCurve.SetTextColor(colorMuted)
	ui.liveCurve.SetBackgroundColor(colorBackground)
	ui.liveCurve.SetBorder(true).SetTitle("VIX + forward curve")
	ui.liveCurve.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.liveCurve.SetText("Waiting for curve snapshot...")

	ui.liveOpts = tview.NewTextView()
	ui.liveOpts.SetTextColor(colorMuted)
	ui.liveOpts.SetBackgroundColor(colorBackground)
	ui.liveOpts.SetScrollable(true)
	ui.liveOpts.SetWrap(false)
	ui.liveOpts.SetBorder(true).SetTitle("Options T-quote")
	ui.liveOpts.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.liveOpts.SetText("Waiting for options snapshot...")

	ui.liveTrades = tview.NewTable()
	ui.liveTrades.SetSelectable(false, false)
	ui.liveTrades.SetFixed(1, 0)
	ui.liveTrades.SetBorder(true).SetTitle("Unusual option volume (newest at top)")
	ui.liveTrades.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.liveMarket, 0, 7, true).
		AddItem(ui.liveFlow, 0, 3, false)

	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.liveCurve, 0, 1, false).
		AddItem(ui.liveOpts, 0, 1, false).
		AddItem(ui.liveTrades, 0, 1, false)
	if ui.liveLog != nil {
		right.AddItem(ui.liveLog, 0, 1, false)
	}

	root := tview.NewFlex().
		AddItem(left, 0, 6, true).
		AddItem(right, 0, 4, false)

	root.SetBackgroundColor(colorBackground)

	ui.focusables = []tview.Primitive{
		ui.liveMarket,
		ui.liveCurve,
		ui.liveOpts,
		ui.liveTrades,
		ui.liveFlow,
	}
	ui.focusIndex = 0
	fillMarketTable(ui.liveMarket, nil)
	fillTradesTable(ui.liveTrades, nil)
	fillFlowTable(ui.liveFlow, nil)
	if ui.liveLog != nil {
		ui.liveLog.SetText("Waiting for runtime logs...")
	}

	return root
}

func (ui *UI) updateLiveData() {
	if ui.liveMarket == nil {
		return
	}
	fillMarketTable(ui.liveMarket, ui.data.MarketRows)
	fillTradesTable(ui.liveTrades, ui.data.Trades)
	fillFlowTable(ui.liveFlow, nil)
	if ui.liveLog != nil {
		fillLog(ui.liveLog, ui.data.Logs)
	}
}

func fillMarketTable(table *tview.Table, rows []MarketRow) {
	table.Clear()
	headers := []string{"CONTRACT", "EXCH", "LAST", "CHG", "CHG%", "BIDV", "BID", "ASK", "ASKV", "VOL", "TURNOVER", "OI", "OI_CHG%", "TS"}
	for col, label := range headers {
		cell := tview.NewTableCell(padTableCell(label)).
			SetTextColor(colorTableHeader).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		table.SetCell(0, col, cell)
	}

	for i, row := range rows {
		values := []string{
			row.Symbol,
			row.Exchange,
			row.Last,
			row.Chg,
			row.ChgPct,
			row.BidVol,
			row.Bid,
			row.Ask,
			row.AskVol,
			row.Vol,
			row.Turnover,
			row.OI,
			row.OIChgPct,
			row.TS,
		}
		for col, value := range values {
			cell := tview.NewTableCell(padTableCell(value)).
				SetTextColor(colorTableRow).
				SetAlign(tview.AlignLeft)
			table.SetCell(i+1, col, cell)
		}
	}
}

type FlowRow struct {
	Underlying   string
	Direction    string
	Vol          string
	Gamma        string
	Theta        string
	Position     string
	Confidence   string
	PatternHint  string
	TopContracts string
	TimeWindow   string
}

func fillFlowTable(table *tview.Table, rows []FlowRow, emptyMessage ...string) {
	table.Clear()
	headers := []string{
		"UNDERLYING",
		"DIRECTION",
		"VOL",
		"GAMMA",
		"THETA",
		"POSITION",
		"CONFIDENCE",
		"PATTERN_HINT",
		"TOP_CONTRACTS",
		"TIME_WINDOW",
	}
	for col, label := range headers {
		cell := tview.NewTableCell(padTableCell(label)).
			SetTextColor(colorTableHeader).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		table.SetCell(0, col, cell)
	}
	if len(rows) == 0 {
		message := "Waiting for unusual events..."
		if len(emptyMessage) > 0 && strings.TrimSpace(emptyMessage[0]) != "" {
			message = emptyMessage[0]
		}
		cell := tview.NewTableCell(message).
			SetTextColor(colorMuted).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		table.SetCell(1, 0, cell)
		return
	}
	for i, row := range rows {
		values := []string{
			row.Underlying,
			row.Direction,
			row.Vol,
			row.Gamma,
			row.Theta,
			row.Position,
			row.Confidence,
			row.PatternHint,
			row.TopContracts,
			row.TimeWindow,
		}
		for col, value := range values {
			cell := tview.NewTableCell(padTableCell(value)).
				SetTextColor(colorTableRow).
				SetAlign(tview.AlignLeft).
				SetSelectable(false)
			table.SetCell(i+1, col, cell)
		}
	}
}

func fillTradesTable(table *tview.Table, trades []TradeRow) {
	table.Clear()
	headers := []string{"TIME", "CONTRACT", "CP", "K", "TTE", "PX", "CHG", "RATIO", "TAG"}
	for col, label := range headers {
		cell := tview.NewTableCell(padTableCell(label)).
			SetTextColor(colorTableHeader).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		table.SetCell(0, col, cell)
	}

	for i, trade := range trades {
		values := []string{trade.Time, trade.Sym, trade.CP, trade.Strike, trade.TTE, trade.Price, trade.Size, trade.IV, trade.Tag}
		for col, value := range values {
			cell := tview.NewTableCell(padTableCell(value)).
				SetTextColor(colorTableRow).
				SetAlign(tview.AlignLeft)
			table.SetCell(i+1, col, cell)
		}
	}
}

func padTableCell(value string) string {
	text := strings.TrimSpace(value)
	if len(text) < unifiedColumnWidth {
		return fmt.Sprintf("%-*s", unifiedColumnWidth, text)
	}
	return text
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
