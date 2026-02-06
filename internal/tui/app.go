package tui

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/configstore"
	"github.com/danni2019/starSling/internal/ipc"
	"github.com/danni2019/starSling/internal/live"
	"github.com/danni2019/starSling/internal/logging"
	"github.com/danni2019/starSling/internal/router"
)

type screen string

const (
	screenMain      screen = "main"
	screenLive      screen = "live"
	screenSetup     screen = "setup"
	screenConfig    screen = "config"
	screenDrilldown screen = "drilldown"
)

const defaultOptionsDeltaAbsMin = 0.25
const defaultOptionsDeltaAbsMax = 0.5
const unifiedColumnWidth = 8

type UI struct {
	app   *tview.Application
	pages *tview.Pages

	logger     *slog.Logger
	routerAddr string
	rpcClient  *ipc.Client

	screenMu   sync.RWMutex
	screen     screen
	curveView  *tview.TextView
	titleView  *tview.TextView
	divider    *tview.TextView
	menu       *tview.List
	liveMarket *tview.Table
	liveLog    *tview.TextView
	liveCurve  *tview.TextView
	liveOpts   *tview.TextView
	liveTrades *tview.Table

	focusables []tview.Primitive
	focusIndex int

	setupStatus  *tview.TextView
	setupOutput  *tview.TextView
	setupActions *tview.List
	setupRunning bool

	configPages       *tview.Pages
	configMenu        *tview.List
	configSelect      *tview.List
	configDelete      *tview.List
	configForm        *tview.Form
	configNameForm    *tview.Form
	configStatus      *tview.TextView
	configEditingName string
	configEditingCfg  config.Config
	configInputAPI    *tview.InputField
	configInputProto  *tview.InputField
	configInputHost   *tview.InputField
	configInputPort   *tview.InputField
	configInputUser   *tview.InputField
	configInputPass   *tview.InputField
	configNameInput   *tview.InputField

	data   MockData
	ticker *time.Ticker

	liveProc      *live.Process
	liveCancel    context.CancelFunc
	optsProc      *live.Process
	optsCancel    context.CancelFunc
	unusualProc   *live.Process
	unusualCancel context.CancelFunc

	lastMarketSeq         int64
	lastMarketStale       bool
	marketSortBy          string
	marketSortAsc         bool
	marketRawRows         []map[string]any
	marketRows            []MarketRow
	filterExchange        string
	filterClass           string
	filterSymbol          string
	filterContract        string
	focusSymbol           string
	focusSyncPending      bool
	lastOptionsSeq        int64
	lastOptionsStale      bool
	lastOptionsKey        string
	optionsRawRows        []map[string]any
	optionsDeltaAbsMin    float64
	optionsDeltaAbsMax    float64
	optionsDeltaEnabled   bool
	lastCurveSeq          int64
	lastCurveStale        bool
	lastUnusualSeq        int64
	lastUnusualStale      bool
	lastLogsSeq           int64
	unusualChgThreshold   float64
	unusualRatioThreshold float64
	liveLogLines          []string
	logoTitleWidth        int
	logoFrame             int
	lastWidth             int
}

func newUI(routerAddr string, logger *slog.Logger) *UI {
	if logger == nil {
		logger = logging.New("INFO")
	}
	ui := &UI{
		app:                   tview.NewApplication(),
		pages:                 tview.NewPages(),
		data:                  mockData(),
		logger:                logger,
		routerAddr:            strings.TrimSpace(routerAddr),
		screen:                screenMain,
		marketSortBy:          "volume",
		marketSortAsc:         false,
		unusualChgThreshold:   100000.0,
		unusualRatioThreshold: 0.05,
		optionsDeltaAbsMin:    defaultOptionsDeltaAbsMin,
		optionsDeltaAbsMax:    defaultOptionsDeltaAbsMax,
	}
	if ui.routerAddr != "" {
		ui.rpcClient = ipc.NewClient(ui.routerAddr)
	}

	ui.buildScreens()
	ui.bindKeys()
	ui.startTicker()

	ui.app.SetRoot(ui.pages, true)
	ui.app.SetBeforeDrawFunc(ui.beforeDraw)
	ui.app.SetFocus(ui.menu)

	return ui
}

func (ui *UI) Run() error {
	defer ui.stopTicker()
	defer ui.stopLiveProcess()
	return ui.app.Run()
}

func (ui *UI) buildScreens() {
	main := ui.buildMainScreen()
	liveView := ui.buildLiveScreen()
	setup := ui.buildSetupScreen()
	configView := ui.buildConfigScreen()

	ui.pages.AddPage(string(screenMain), main, true, true)
	ui.pages.AddPage(string(screenLive), liveView, true, false)
	ui.pages.AddPage(string(screenSetup), setup, true, false)
	ui.pages.AddPage(string(screenConfig), configView, true, false)
}

func (ui *UI) bindKeys() {
	ui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'q' {
			ui.app.Stop()
			return nil
		}

		switch ui.currentScreen() {
		case screenLive:
			return ui.handleLiveKeys(event)
		case screenSetup, screenConfig:
			if event.Key() == tcell.KeyEsc {
				ui.setScreen(screenMain)
				return nil
			}
		case screenDrilldown:
			if event.Key() == tcell.KeyEsc {
				ui.closeDrilldown()
				return nil
			}
		}

		return event
	})
}

func (ui *UI) handleLiveKeys(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEsc:
		ui.setScreen(screenMain)
		return nil
	case tcell.KeyTab, tcell.KeyRight:
		ui.cycleFocus(1)
		return nil
	case tcell.KeyBacktab, tcell.KeyLeft:
		ui.cycleFocus(-1)
		return nil
	case tcell.KeyEnter:
		if ui.app.GetFocus() == ui.liveMarket {
			ui.openMarketFilter()
		} else if ui.app.GetFocus() == ui.liveOpts {
			ui.openOptionsFilter()
		} else if ui.app.GetFocus() == ui.liveTrades {
			ui.openUnusualThresholdSettings()
		}
		return nil
	case tcell.KeyRune:
		if event.Rune() == 's' || event.Rune() == 'S' {
			ui.marketSortAsc = !ui.marketSortAsc
			ui.renderMarketRows()
			ui.appendLiveLogLine(fmt.Sprintf("sort order switched: %s", ui.sortDirection()))
			return nil
		}
	}
	return event
}

func (ui *UI) setScreen(next screen) {
	ui.setCurrentScreen(next)
	ui.pages.SwitchToPage(string(next))
	switch next {
	case screenMain:
		ui.app.SetFocus(ui.menu)
	case screenLive:
		ui.focusIndex = 0
		ui.setFocus(ui.focusIndex)
		ui.startLiveProcessIfNeeded()
	case screenSetup, screenConfig:
		if next == screenSetup {
			ui.showSetupMenu()
		} else {
			ui.showConfigMenu()
		}
	}
}

func (ui *UI) cycleFocus(direction int) {
	if len(ui.focusables) == 0 {
		return
	}
	ui.focusIndex = (ui.focusIndex + direction + len(ui.focusables)) % len(ui.focusables)
	ui.setFocus(ui.focusIndex)
}

func (ui *UI) setFocus(index int) {
	for i, item := range ui.focusables {
		color := colorBorder
		if i == index {
			color = colorFocus
		}
		setBorderColor(item, color)
	}
	ui.app.SetFocus(ui.focusables[index])
}

func (ui *UI) openMarketFilter() {
	ui.setCurrentScreen(screenDrilldown)
	exchangeInput := tview.NewInputField().SetLabel("Exchange: ").SetText(ui.filterExchange)
	classInput := tview.NewInputField().SetLabel("Product Class: ").SetText(ui.filterClass)
	symbolInput := tview.NewInputField().SetLabel("Symbol: ").SetText(ui.filterSymbol)
	contractInput := tview.NewInputField().SetLabel("Contract: ").SetText(ui.filterContract)
	sortOptions := marketSortableColumns(ui.marketRawRows)
	if len(sortOptions) == 0 {
		sortOptions = []string{"ctp_contract"}
	}
	sortIdx := indexOfFold(sortOptions, ui.marketSortBy)
	if sortIdx < 0 {
		sortIdx = indexOfFold(sortOptions, "volume")
		if sortIdx < 0 {
			sortIdx = 0
		}
	}
	selectedSortBy := sortOptions[sortIdx]

	orderOptions := []string{"desc", "asc"}
	orderIdx := indexOfFold(orderOptions, ui.sortDirection())
	if orderIdx < 0 {
		orderIdx = 0
	}
	selectedOrder := orderOptions[orderIdx]

	sortDropDown := tview.NewDropDown().
		SetLabel("Sort By: ").
		SetOptions(sortOptions, func(text string, _ int) {
			if strings.TrimSpace(text) != "" {
				selectedSortBy = text
			}
		})
	sortDropDown.SetCurrentOption(sortIdx)

	orderDropDown := tview.NewDropDown().
		SetLabel("Order: ").
		SetOptions(orderOptions, func(text string, _ int) {
			if strings.TrimSpace(text) != "" {
				selectedOrder = text
			}
		})
	orderDropDown.SetCurrentOption(orderIdx)

	form := tview.NewForm().
		AddFormItem(exchangeInput).
		AddFormItem(classInput).
		AddFormItem(symbolInput).
		AddFormItem(contractInput).
		AddFormItem(sortDropDown).
		AddFormItem(orderDropDown)
	form.SetBorder(true).SetTitle("Market filter & sort")
	form.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	form.SetBackgroundColor(colorBackground)
	form.SetFieldBackgroundColor(colorBackground)
	form.SetFieldTextColor(colorTableRow)
	form.SetButtonBackgroundColor(colorHighlight)
	form.SetButtonTextColor(colorMenuSelected)

	form.AddButton("Apply", func() {
		ui.filterExchange = strings.TrimSpace(exchangeInput.GetText())
		ui.filterClass = strings.TrimSpace(classInput.GetText())
		ui.filterSymbol = strings.TrimSpace(symbolInput.GetText())
		ui.filterContract = strings.TrimSpace(contractInput.GetText())
		sortBy := strings.TrimSpace(strings.ToLower(selectedSortBy))
		if sortBy == "" {
			sortBy = "ctp_contract"
		}
		ui.marketSortBy = sortBy
		order := strings.TrimSpace(strings.ToLower(selectedOrder))
		ui.marketSortAsc = order == "asc"
		ui.renderMarketRows()
		ui.closeDrilldown()
	})
	form.AddButton("Reset", func() {
		ui.filterExchange = ""
		ui.filterClass = ""
		ui.filterSymbol = ""
		ui.filterContract = ""
		ui.marketSortBy = "volume"
		ui.marketSortAsc = false
		ui.renderMarketRows()
		ui.closeDrilldown()
	})
	form.AddButton("Cancel", func() {
		ui.closeDrilldown()
	})

	ui.pages.AddPage(string(screenDrilldown), centerModal(form, 68, 16), true, true)
	ui.app.SetFocus(form)
}

func (ui *UI) openUnusualThresholdSettings() {
	ui.setCurrentScreen(screenDrilldown)
	chgInput := tview.NewInputField().
		SetLabel("Turnover Chg >= ").
		SetText(formatFloat(ui.unusualChgThreshold))
	ratioInput := tview.NewInputField().
		SetLabel("Turnover Ratio >= ").
		SetText(strconv.FormatFloat(ui.unusualRatioThreshold, 'f', 4, 64))

	form := tview.NewForm().
		AddFormItem(chgInput).
		AddFormItem(ratioInput)
	form.SetBorder(true).SetTitle("Unusual thresholds")
	form.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	form.SetBackgroundColor(colorBackground)
	form.SetFieldBackgroundColor(colorBackground)
	form.SetFieldTextColor(colorTableRow)
	form.SetButtonBackgroundColor(colorHighlight)
	form.SetButtonTextColor(colorMenuSelected)
	form.AddButton("Apply", func() {
		chg, errChg := strconv.ParseFloat(strings.TrimSpace(chgInput.GetText()), 64)
		ratio, errRatio := strconv.ParseFloat(strings.TrimSpace(ratioInput.GetText()), 64)
		if errChg != nil || errRatio != nil || chg <= 0 || ratio <= 0 {
			ui.appendLiveLogLine("invalid unusual thresholds")
			return
		}
		if err := ui.setUnusualThresholds(chg, ratio); err != nil {
			ui.appendLiveLogLine("failed to update unusual thresholds: " + err.Error())
			return
		}
		ui.appendLiveLogLine(fmt.Sprintf("unusual thresholds updated: chg>=%.0f ratio>=%.2f%%", chg, ratio*100))
		ui.closeDrilldown()
	})
	form.AddButton("Cancel", func() {
		ui.closeDrilldown()
	})

	ui.pages.AddPage(string(screenDrilldown), centerModal(form, 62, 10), true, true)
	ui.app.SetFocus(form)
}

func (ui *UI) openOptionsFilter() {
	ui.setCurrentScreen(screenDrilldown)
	minValue := ui.optionsDeltaAbsMin
	maxValue := ui.optionsDeltaAbsMax
	if minValue <= 0 {
		minValue = defaultOptionsDeltaAbsMin
	}
	if maxValue <= 0 {
		maxValue = defaultOptionsDeltaAbsMax
	}
	minInput := tview.NewInputField().
		SetLabel("Delta |abs| min >= ").
		SetText(strconv.FormatFloat(minValue, 'f', 4, 64))
	maxInput := tview.NewInputField().
		SetLabel("Delta |abs| max <= ").
		SetText(strconv.FormatFloat(maxValue, 'f', 4, 64))

	form := tview.NewForm().
		AddFormItem(minInput).
		AddFormItem(maxInput)
	form.SetBorder(true).SetTitle("Options filter")
	form.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	form.SetBackgroundColor(colorBackground)
	form.SetFieldBackgroundColor(colorBackground)
	form.SetFieldTextColor(colorTableRow)
	form.SetButtonBackgroundColor(colorHighlight)
	form.SetButtonTextColor(colorMenuSelected)
	form.AddButton("Apply", func() {
		minThreshold, maxThreshold, valid := parsePositiveRange(
			minInput.GetText(),
			maxInput.GetText(),
			defaultOptionsDeltaAbsMin,
			defaultOptionsDeltaAbsMax,
		)
		ui.optionsDeltaAbsMin = minThreshold
		ui.optionsDeltaAbsMax = maxThreshold
		ui.optionsDeltaEnabled = true
		if !valid {
			ui.appendLiveLogLine("invalid delta range, reset to defaults [0.25, 0.5]")
		}
		ui.renderOptionsSnapshot()
		ui.closeDrilldown()
	})
	form.AddButton("Reset", func() {
		ui.optionsDeltaEnabled = false
		ui.optionsDeltaAbsMin = defaultOptionsDeltaAbsMin
		ui.optionsDeltaAbsMax = defaultOptionsDeltaAbsMax
		ui.renderOptionsSnapshot()
		ui.closeDrilldown()
	})
	form.AddButton("Cancel", func() {
		ui.closeDrilldown()
	})

	ui.pages.AddPage(string(screenDrilldown), centerModal(form, 64, 10), true, true)
	ui.app.SetFocus(form)
}

func (ui *UI) openDrilldown() {
	ui.setCurrentScreen(screenDrilldown)
	modal := tview.NewModal().
		SetText("Drilldown (placeholder)").
		AddButtons([]string{"Back"}).
		SetDoneFunc(func(_ int, _ string) {
			ui.closeDrilldown()
		})
	modal.SetBackgroundColor(colorBackground)
	modal.SetTextColor(colorAccent)
	modal.SetButtonBackgroundColor(colorHighlight)
	modal.SetButtonTextColor(colorMenuSelected)
	ui.pages.AddPage(string(screenDrilldown), modal, true, true)
	ui.app.SetFocus(modal)
}

func (ui *UI) closeDrilldown() {
	ui.pages.RemovePage(string(screenDrilldown))
	ui.setCurrentScreen(screenLive)
	ui.setFocus(ui.focusIndex)
}

func (ui *UI) startTicker() {
	ui.ticker = time.NewTicker(500 * time.Millisecond)
	go func() {
		for range ui.ticker.C {
			if ui.currentScreen() == screenLive && ui.rpcClient != nil {
				ui.pollLiveSnapshot()
				continue
			}
			ui.app.QueueUpdateDraw(func() {
				ui.logoFrame = (ui.logoFrame + 1) % 2
				ui.updateLogo(ui.lastWidth)
			})
		}
	}()
}

func (ui *UI) currentScreen() screen {
	ui.screenMu.RLock()
	defer ui.screenMu.RUnlock()
	return ui.screen
}

func (ui *UI) setCurrentScreen(next screen) {
	ui.screenMu.Lock()
	ui.screen = next
	ui.screenMu.Unlock()
}

func (ui *UI) pollLiveSnapshot() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var view router.ViewSnapshot
	err := ui.rpcClient.Call(ctx, "router.get_view_snapshot", router.GetViewSnapshotParams{
		FocusSymbol: ui.currentFocusSymbol(),
	}, &view)
	ui.app.QueueUpdateDraw(func() {
		ui.logoFrame = (ui.logoFrame + 1) % 2
		ui.updateLogo(ui.lastWidth)
		if err != nil {
			ui.appendLiveLogLine("router poll failed: " + err.Error())
			return
		}
		ui.applyMarketSnapshot(view.Market)
		ui.applyCurveSnapshot(view.Curve)
		ui.applyOptionsSnapshot(view.Options)
		ui.applyUnusualSnapshot(view.Unusual)
		ui.applyRouterLogs(view.Logs)
	})
}

func (ui *UI) applyMarketSnapshot(snapshot router.MarketSnapshot) {
	if snapshot.Seq == 0 {
		shouldClear := ui.lastMarketSeq != 0 || ui.lastMarketStale || len(ui.marketRows) > 0 ||
			(ui.liveMarket != nil && ui.liveMarket.GetRowCount() > 1)
		if !shouldClear {
			return
		}
		if ui.lastMarketSeq != 0 {
			ui.focusSyncPending = true
		}
		ui.marketRawRows = nil
		ui.marketRows = nil
		ui.lastMarketSeq = 0
		ui.lastMarketStale = false
		ui.renderMarketRows()
		return
	}
	if snapshot.Seq < ui.lastMarketSeq {
		ui.focusSyncPending = true
	}
	seqChanged := snapshot.Seq != ui.lastMarketSeq
	staleChanged := snapshot.Stale != ui.lastMarketStale
	if !seqChanged && !staleChanged {
		return
	}
	if seqChanged {
		ui.marketRawRows = snapshot.Rows
		if ui.marketRawRows == nil {
			ui.marketRawRows = []map[string]any{}
		}
		ui.renderMarketRows()
		ui.lastMarketSeq = snapshot.Seq
		ui.ensureFocusSymbol()
	}
	if staleChanged && snapshot.Stale {
		ui.appendLiveLogLine("market snapshot stale")
	}
	if staleChanged && !snapshot.Stale {
		ui.appendLiveLogLine("market snapshot resumed")
	}
	ui.lastMarketStale = snapshot.Stale
}

func (ui *UI) applyOptionsSnapshot(snapshot router.OptionsSnapshot) {
	if ui.liveOpts == nil {
		return
	}
	if snapshot.Seq == 0 {
		if ui.lastOptionsSeq != 0 {
			ui.liveOpts.SetText("No options snapshot.")
			ui.lastOptionsSeq = 0
			ui.lastOptionsStale = false
			ui.lastOptionsKey = ""
			ui.optionsRawRows = nil
		}
		return
	}
	snapshotRowsKey := optionsRowsKey(snapshot.Rows)
	cachedRowsKey := optionsRowsKey(ui.optionsRawRows)
	if snapshot.Seq != ui.lastOptionsSeq || snapshotRowsKey != cachedRowsKey {
		ui.optionsRawRows = snapshot.Rows
		if ui.optionsRawRows == nil {
			ui.optionsRawRows = []map[string]any{}
		}
	}
	optionsKey := optionsRowsKey(ui.optionsRawRows)
	renderKey := fmt.Sprintf(
		"%s|focus=%s|delta=%t:%0.6f:%0.6f",
		optionsKey,
		strings.ToLower(strings.TrimSpace(ui.currentFocusSymbol())),
		ui.optionsDeltaEnabled,
		ui.optionsDeltaAbsMin,
		ui.optionsDeltaAbsMax,
	)
	seqChanged := snapshot.Seq != ui.lastOptionsSeq
	staleChanged := snapshot.Stale != ui.lastOptionsStale
	keyChanged := renderKey != ui.lastOptionsKey
	if !seqChanged && !staleChanged && !keyChanged {
		return
	}
	if seqChanged || keyChanged {
		ui.renderOptionsSnapshot()
		ui.lastOptionsSeq = snapshot.Seq
		ui.lastOptionsKey = renderKey
	}
	if staleChanged && snapshot.Stale {
		ui.appendLiveLogLine("options snapshot stale")
	}
	ui.lastOptionsStale = snapshot.Stale
}

func (ui *UI) renderOptionsSnapshot() {
	if ui.liveOpts == nil {
		return
	}
	ui.liveOpts.SetText(renderOptionsPanel(
		ui.optionsRawRows,
		ui.currentFocusSymbol(),
		optionRenderFilter{
			Enabled:     ui.optionsDeltaEnabled,
			DeltaAbsMin: ui.optionsDeltaAbsMin,
			DeltaAbsMax: ui.optionsDeltaAbsMax,
			DefaultMin:  defaultOptionsDeltaAbsMin,
			DefaultMax:  defaultOptionsDeltaAbsMax,
		},
	))
}

func (ui *UI) applyCurveSnapshot(snapshot router.CurveSnapshot) {
	if ui.liveCurve == nil {
		return
	}
	if snapshot.Seq == 0 {
		if ui.lastCurveSeq != 0 {
			ui.liveCurve.SetText("No curve snapshot.")
			ui.lastCurveSeq = 0
			ui.lastCurveStale = false
		}
		return
	}
	seqChanged := snapshot.Seq != ui.lastCurveSeq
	staleChanged := snapshot.Stale != ui.lastCurveStale
	if !seqChanged && !staleChanged {
		return
	}
	if seqChanged {
		ui.liveCurve.SetText(renderCurvePanel(snapshot.Rows))
		ui.lastCurveSeq = snapshot.Seq
	}
	if staleChanged && snapshot.Stale {
		ui.appendLiveLogLine("curve snapshot stale")
	}
	ui.lastCurveStale = snapshot.Stale
}

func (ui *UI) applyUnusualSnapshot(snapshot router.UnusualSnapshot) {
	if ui.liveTrades == nil {
		return
	}
	if snapshot.Seq == 0 {
		if ui.lastUnusualSeq != 0 {
			fillTradesTable(ui.liveTrades, nil)
			ui.lastUnusualSeq = 0
			ui.lastUnusualStale = false
		}
		return
	}
	seqChanged := snapshot.Seq != ui.lastUnusualSeq
	staleChanged := snapshot.Stale != ui.lastUnusualStale
	if !seqChanged && !staleChanged {
		return
	}
	if seqChanged {
		fillTradesTable(ui.liveTrades, convertUnusualTrades(snapshot.Rows))
		ui.lastUnusualSeq = snapshot.Seq
	}
	if staleChanged && snapshot.Stale {
		ui.appendLiveLogLine("unusual snapshot stale")
	}
	ui.lastUnusualStale = snapshot.Stale
}

func (ui *UI) applyRouterLogs(snapshot router.LogSnapshot) {
	if snapshot.Seq == 0 {
		if ui.lastLogsSeq != 0 {
			ui.focusSyncPending = true
			ui.liveLogLines = nil
			if ui.liveLog != nil {
				ui.liveLog.SetText("Waiting for runtime logs...")
			}
		}
		ui.lastLogsSeq = 0
		return
	}
	if snapshot.Seq < ui.lastLogsSeq {
		ui.focusSyncPending = true
		ui.liveLogLines = nil
		if ui.liveLog != nil {
			ui.liveLog.SetText("Waiting for runtime logs...")
		}
		ui.lastLogsSeq = 0
	}
	if snapshot.Seq <= ui.lastLogsSeq {
		return
	}
	missing := int(snapshot.Seq - ui.lastLogsSeq)
	if missing < 0 {
		missing = 0
	}
	if missing > len(snapshot.Items) {
		missing = len(snapshot.Items)
	}
	ui.lastLogsSeq = snapshot.Seq
	for i := missing - 1; i >= 0; i-- {
		item := snapshot.Items[i]
		msg := strings.TrimSpace(item.Message)
		if msg == "" {
			continue
		}
		source := strings.TrimSpace(item.Source)
		level := strings.TrimSpace(item.Level)
		if source != "" || level != "" {
			msg = strings.TrimSpace(strings.Join([]string{source, level, msg}, " "))
		}
		ui.appendLiveLogLineAt(msg, item.TS)
	}
}

func (ui *UI) renderMarketRows() {
	if ui.liveMarket == nil {
		return
	}
	selectedSymbol := ui.selectedMarketSymbol()
	selectedRow, _ := ui.liveMarket.GetSelection()
	if ui.marketRawRows != nil {
		filtered := filterMarketRows(ui.marketRawRows, ui.filterExchange, ui.filterClass, ui.filterSymbol, ui.filterContract)
		sortMarketRawRows(filtered, ui.marketSortBy, ui.marketSortAsc)
		ui.marketRows = convertMarketRows(filtered)
	} else {
		sortMarketRows(ui.marketRows, ui.marketSortBy, ui.marketSortAsc)
	}
	fillMarketTable(ui.liveMarket, ui.marketRows)
	ui.restoreMarketSelection(selectedSymbol, selectedRow)
}

func (ui *UI) selectedMarketSymbol() string {
	if ui.liveMarket == nil {
		return ""
	}
	row, _ := ui.liveMarket.GetSelection()
	if row <= 0 {
		return ""
	}
	cell := ui.liveMarket.GetCell(row, 0)
	if cell == nil {
		return ""
	}
	return strings.TrimSpace(cell.Text)
}

func (ui *UI) restoreMarketSelection(selectedSymbol string, selectedRow int) {
	if ui.liveMarket == nil || len(ui.marketRows) == 0 {
		return
	}
	targetRow := marketRowForSymbol(ui.marketRows, selectedSymbol)
	if targetRow == 0 {
		if selectedRow > 0 && selectedRow <= len(ui.marketRows) {
			targetRow = selectedRow
		} else {
			targetRow = 1
		}
	}
	currentRow, currentCol := ui.liveMarket.GetSelection()
	if currentRow == targetRow && currentCol == 0 {
		return
	}
	ui.liveMarket.Select(targetRow, 0)
}

func marketRowForSymbol(rows []MarketRow, symbol string) int {
	trimmed := strings.TrimSpace(symbol)
	if trimmed == "" {
		return 0
	}
	for idx, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Symbol), trimmed) {
			return idx + 1
		}
	}
	return 0
}

func filterMarketRows(rows []map[string]any, exchange, productClass, symbol, contract string) []map[string]any {
	exchangeTokens := csvTokens(exchange)
	classTokens := csvTokens(productClass)
	symbolTokens := csvTokens(symbol)
	contractTokens := csvTokens(contract)
	if len(exchangeTokens) == 0 && len(classTokens) == 0 && len(symbolTokens) == 0 && len(contractTokens) == 0 {
		return rows
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		rowExchange := strings.TrimSpace(asString(row["exchange"]))
		rowClass := strings.TrimSpace(asString(row["product_class"]))
		rowSymbol := strings.TrimSpace(asString(row["symbol"]))
		rowContract := strings.TrimSpace(asString(row["ctp_contract"]))
		if !tokenMatch(exchangeTokens, rowExchange) {
			continue
		}
		if !tokenMatch(classTokens, rowClass) {
			continue
		}
		if !tokenMatch(symbolTokens, rowSymbol) {
			continue
		}
		if !tokenMatch(contractTokens, rowContract) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func csvTokens(value string) map[string]struct{} {
	tokens := make(map[string]struct{})
	for _, raw := range strings.Split(value, ",") {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			continue
		}
		tokens[token] = struct{}{}
	}
	return tokens
}

func tokenMatch(tokens map[string]struct{}, value string) bool {
	if len(tokens) == 0 {
		return true
	}
	_, ok := tokens[strings.ToLower(strings.TrimSpace(value))]
	return ok
}

func indexOfFold(items []string, target string) int {
	for idx, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return idx
		}
	}
	return -1
}

func marketSortableColumns(rows []map[string]any) []string {
	if len(rows) == 0 {
		return nil
	}
	columnSet := make(map[string]struct{})
	for _, row := range rows {
		for key := range row {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			columnSet[strings.ToLower(key)] = struct{}{}
		}
	}
	columns := make([]string, 0, len(columnSet))
	for key := range columnSet {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return columns
}

func marketNumericColumns(rows []map[string]any) []string {
	return marketSortableColumns(rows)
}

func sortMarketRawRows(rows []map[string]any, sortBy string, asc bool) {
	key := strings.TrimSpace(sortBy)
	if key == "" {
		key = "volume"
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		leftRaw := left[key]
		rightRaw := right[key]

		lf, lok := asOptionalFloat(leftRaw)
		rf, rok := asOptionalFloat(rightRaw)
		if lok != rok {
			return lok
		}
		if lok && rok {
			if lf == rf {
				return strings.ToLower(asString(left["ctp_contract"])) < strings.ToLower(asString(right["ctp_contract"]))
			}
			if asc {
				return lf < rf
			}
			return lf > rf
		}

		ls := strings.ToLower(strings.TrimSpace(asString(leftRaw)))
		rs := strings.ToLower(strings.TrimSpace(asString(rightRaw)))
		if ls == rs {
			return strings.ToLower(asString(left["ctp_contract"])) < strings.ToLower(asString(right["ctp_contract"]))
		}
		if asc {
			return ls < rs
		}
		return ls > rs
	})
}

type curvePoint struct {
	Contract  string
	Forward   float64
	Volume    float64
	OI        float64
	BidVol    float64
	Bid       float64
	Ask       float64
	AskVol    float64
	VIX       float64
	HasVolume bool
	HasOI     bool
	HasBid    bool
	HasAsk    bool
	HasBidVol bool
	HasAskVol bool
	HasVIX    bool
}

func renderCurvePanel(rows []map[string]any) string {
	if len(rows) == 0 {
		return "No curve data."
	}
	points := make([]curvePoint, 0, len(rows))
	for _, row := range rows {
		contract := strings.TrimSpace(asString(row["ctp_contract"]))
		forward, ok := asOptionalFloat(row["forward"])
		if contract == "" || !ok {
			continue
		}
		bidVol, hasBidVol := asOptionalFloat(row["bid_vol1"])
		bid, hasBid := asOptionalFloat(row["bid1"])
		ask, hasAsk := asOptionalFloat(row["ask1"])
		askVol, hasAskVol := asOptionalFloat(row["ask_vol1"])
		volume, hasVolume := asOptionalFloat(row["volume"])
		oi, hasOI := asOptionalFloat(row["open_interest"])
		vix, hasVIX := asOptionalFloat(row["vix"])
		points = append(points, curvePoint{
			Contract:  contract,
			Forward:   forward,
			Volume:    volume,
			OI:        oi,
			BidVol:    bidVol,
			Bid:       bid,
			Ask:       ask,
			AskVol:    askVol,
			VIX:       vix,
			HasVolume: hasVolume,
			HasOI:     hasOI,
			HasBidVol: hasBidVol,
			HasBid:    hasBid,
			HasAsk:    hasAsk,
			HasAskVol: hasAskVol,
			HasVIX:    hasVIX,
		})
	}
	if len(points) == 0 {
		return "No valid curve points."
	}
	sort.Slice(points, func(i, j int) bool {
		return strings.ToLower(points[i].Contract) < strings.ToLower(points[j].Contract)
	})
	lines := []string{
		fmt.Sprintf("Contracts: %d", len(points)),
		"",
		formatAlignedColumns([]string{"CNTRCT", "FWD", "VOL", "OI", "BIDV", "BID", "ASK", "ASKV", "VIX"}, unifiedColumnWidth),
		"",
	}
	limitRows := len(points)
	if limitRows > 16 {
		limitRows = 16
	}
	for idx := 0; idx < limitRows; idx++ {
		p := points[idx]
		bidVolText := "-"
		if p.HasBidVol {
			bidVolText = formatFloat(p.BidVol)
		}
		volumeText := "-"
		if p.HasVolume {
			volumeText = formatFloat(p.Volume)
		}
		oiText := "-"
		if p.HasOI {
			oiText = formatFloat(p.OI)
		}
		bidText := "-"
		if p.HasBid {
			bidText = formatFloat(p.Bid)
		}
		askText := "-"
		if p.HasAsk {
			askText = formatFloat(p.Ask)
		}
		askVolText := "-"
		if p.HasAskVol {
			askVolText = formatFloat(p.AskVol)
		}
		vixText := "-"
		if p.HasVIX {
			vixText = formatFloat(p.VIX)
		}
		lines = append(lines, formatAlignedColumns([]string{
			p.Contract,
			formatFloat(p.Forward),
			volumeText,
			oiText,
			bidVolText,
			bidText,
			askText,
			askVolText,
			vixText,
		}, unifiedColumnWidth))
	}
	return strings.Join(lines, "\n")
}

type optionRenderFilter struct {
	Enabled     bool
	DeltaAbsMin float64
	DeltaAbsMax float64
	DefaultMin  float64
	DefaultMax  float64
}

func inferOptionTypeFromContract(contract string) string {
	upper := strings.ToUpper(strings.TrimSpace(contract))
	if len(upper) < 3 {
		return ""
	}
	for i := len(upper) - 2; i >= 1; i-- {
		ch := upper[i]
		if ch != 'C' && ch != 'P' {
			continue
		}
		prev := upper[i-1]
		next := upper[i+1]
		if prev < '0' || prev > '9' || next < '0' || next > '9' {
			continue
		}
		if ch == 'C' {
			return "c"
		}
		return "p"
	}
	return ""
}

func renderOptionsPanel(rows []map[string]any, focusSymbol string, filter optionRenderFilter) string {
	focusText := strings.TrimSpace(focusSymbol)
	if focusText == "" {
		return "Select a contract."
	}
	rows = filterOptionsRows(rows, focusText)
	if len(rows) == 0 {
		return "Select a contract."
	}
	filterText := "Delta|abs|: off"
	if filter.Enabled {
		minThreshold := filter.DeltaAbsMin
		maxThreshold := filter.DeltaAbsMax
		if minThreshold <= 0 || maxThreshold <= 0 || minThreshold > maxThreshold {
			minThreshold = filter.DefaultMin
			maxThreshold = filter.DefaultMax
			if minThreshold <= 0 || maxThreshold <= 0 || minThreshold > maxThreshold {
				minThreshold = defaultOptionsDeltaAbsMin
				maxThreshold = defaultOptionsDeltaAbsMax
			}
		}
		filteredRows, strikeCount, ok := filterOptionsByDeltaStrikeSet(rows, minThreshold, maxThreshold)
		if ok {
			rows = filteredRows
			filterText = fmt.Sprintf("%s<=Delta|abs|<=%s | strikes=%d",
				formatFloat(minThreshold),
				formatFloat(maxThreshold),
				strikeCount,
			)
		} else {
			rows = nil
			filterText = fmt.Sprintf("%s<=Delta|abs|<=%s | no strikes",
				formatFloat(minThreshold),
				formatFloat(maxThreshold),
			)
		}
	}
	if len(rows) == 0 {
		return fmt.Sprintf("Focus: %s\n%s\nNo options data for current focus.", focusText, filterText)
	}

	type optionSideQuote struct {
		Last      float64
		HasLast   bool
		IV        float64
		HasIV     bool
		Delta     float64
		HasDelta  bool
		Volume    float64
		HasVolume bool
		TTE       float64
		HasTTE    bool
	}
	type strikeRow struct {
		Strike  float64
		Call    optionSideQuote
		HasCall bool
		Put     optionSideQuote
		HasPut  bool
	}

	buildSide := func(row map[string]any) optionSideQuote {
		side := optionSideQuote{}
		side.Last, side.HasLast = asOptionalFloat(row["last"])
		side.IV, side.HasIV = asOptionalFloat(row["iv"])
		side.Delta, side.HasDelta = asOptionalFloat(row["delta"])
		side.Volume, side.HasVolume = asOptionalFloat(row["volume"])
		side.TTE, side.HasTTE = asOptionalFloat(row["tte"])
		if side.HasLast && (math.IsNaN(side.Last) || math.IsInf(side.Last, 0)) {
			side.HasLast = false
		}
		if side.HasIV && (math.IsNaN(side.IV) || math.IsInf(side.IV, 0)) {
			side.HasIV = false
		}
		if side.HasDelta && (math.IsNaN(side.Delta) || math.IsInf(side.Delta, 0)) {
			side.HasDelta = false
		}
		if side.HasVolume && (math.IsNaN(side.Volume) || math.IsInf(side.Volume, 0)) {
			side.HasVolume = false
		}
		if side.HasTTE && (math.IsNaN(side.TTE) || math.IsInf(side.TTE, 0)) {
			side.HasTTE = false
		}
		return side
	}

	strikeMap := make(map[string]*strikeRow)
	for _, row := range rows {
		contract := strings.TrimSpace(asString(row["ctp_contract"]))
		strike, strikeOK := asOptionalFloat(row["strike"])
		if contract == "" || !strikeOK || math.IsNaN(strike) || math.IsInf(strike, 0) {
			continue
		}
		cp := strings.ToLower(strings.TrimSpace(asString(row["option_type"])))
		if cp == "" {
			cp = inferOptionTypeFromContract(contract)
		}
		if cp != "c" && cp != "p" {
			continue
		}
		key := fmt.Sprintf("%.6f", strike)
		entry, exists := strikeMap[key]
		if !exists {
			entry = &strikeRow{Strike: strike}
			strikeMap[key] = entry
		}
		side := buildSide(row)
		if cp == "c" {
			entry.Call = side
			entry.HasCall = true
		} else {
			entry.Put = side
			entry.HasPut = true
		}
	}

	if len(strikeMap) == 0 {
		return fmt.Sprintf("Focus: %s\n%s\nNo valid option rows.", focusText, filterText)
	}

	strikes := make([]strikeRow, 0, len(strikeMap))
	callRows := 0
	putRows := 0
	for _, row := range strikeMap {
		if row.HasCall {
			callRows++
		}
		if row.HasPut {
			putRows++
		}
		strikes = append(strikes, *row)
	}
	sort.Slice(strikes, func(i, j int) bool { return strikes[i].Strike < strikes[j].Strike })

	const sideColWidth = unifiedColumnWidth
	const strikeColWidth = unifiedColumnWidth
	leftHeader := []string{"TTE", "VOL", "DELTA", "IV", "LAST"}
	rightHeader := []string{"LAST", "IV", "DELTA", "VOL", "TTE"}
	leftHeaderLine := formatAlignedColumns(leftHeader, sideColWidth)
	rightHeaderLine := formatAlignedColumns(rightHeader, sideColWidth)
	leftBlockWidth := len(leftHeaderLine)
	rightBlockWidth := len(rightHeaderLine)
	lines := []string{
		"Focus: " + focusText,
		filterText,
		fmt.Sprintf("Rows: %d | Strikes: %d | Call rows: %d | Put rows: %d", len(rows), len(strikes), callRows, putRows),
		"",
		fmt.Sprintf("%-*s | %-*s | %-*s", leftBlockWidth, "CALL", strikeColWidth, "", rightBlockWidth, "PUT"),
		fmt.Sprintf("%s | %-*s | %s", leftHeaderLine, strikeColWidth, "STRIKE", rightHeaderLine),
	}
	for _, row := range strikes {
		leftValues := []string{
			formatOptionalFloat(row.Call.TTE, row.HasCall && row.Call.HasTTE),
			formatOptionalFloat(row.Call.Volume, row.HasCall && row.Call.HasVolume),
			formatOptionalFloat(row.Call.Delta, row.HasCall && row.Call.HasDelta),
			formatOptionalFloat(row.Call.IV, row.HasCall && row.Call.HasIV),
			formatOptionalFloat(row.Call.Last, row.HasCall && row.Call.HasLast),
		}
		rightValues := []string{
			formatOptionalFloat(row.Put.Last, row.HasPut && row.Put.HasLast),
			formatOptionalFloat(row.Put.IV, row.HasPut && row.Put.HasIV),
			formatOptionalFloat(row.Put.Delta, row.HasPut && row.Put.HasDelta),
			formatOptionalFloat(row.Put.Volume, row.HasPut && row.Put.HasVolume),
			formatOptionalFloat(row.Put.TTE, row.HasPut && row.Put.HasTTE),
		}
		lines = append(lines, fmt.Sprintf("%s | %-*s | %s",
			formatAlignedColumns(leftValues, sideColWidth),
			strikeColWidth,
			formatFloat(row.Strike),
			formatAlignedColumns(rightValues, sideColWidth),
		))
	}
	return strings.Join(lines, "\n")
}

func formatAlignedColumns(values []string, width int) string {
	if width <= 0 {
		width = 1
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		cell := strings.TrimSpace(value)
		if len(cell) > width {
			cell = cell[:width]
		}
		parts = append(parts, fmt.Sprintf("%-*s", width, cell))
	}
	return strings.Join(parts, "")
}

func filterOptionsByDeltaStrikeSet(rows []map[string]any, minThreshold, maxThreshold float64) ([]map[string]any, int, bool) {
	if minThreshold <= 0 || maxThreshold <= 0 || minThreshold > maxThreshold || len(rows) == 0 {
		return nil, 0, false
	}
	strikeSet := make(map[string]struct{})
	for _, row := range rows {
		delta, hasDelta := asOptionalFloat(row["delta"])
		strike, hasStrike := asOptionalFloat(row["strike"])
		if !hasDelta || !hasStrike || math.IsNaN(delta) || math.IsInf(delta, 0) || math.IsNaN(strike) || math.IsInf(strike, 0) {
			continue
		}
		deltaAbs := math.Abs(delta)
		if deltaAbs < minThreshold || deltaAbs > maxThreshold {
			continue
		}
		strikeKey := fmt.Sprintf("%.6f", strike)
		strikeSet[strikeKey] = struct{}{}
	}
	if len(strikeSet) == 0 {
		return nil, 0, false
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		strike, hasStrike := asOptionalFloat(row["strike"])
		if !hasStrike || math.IsNaN(strike) || math.IsInf(strike, 0) {
			continue
		}
		strikeKey := fmt.Sprintf("%.6f", strike)
		if _, ok := strikeSet[strikeKey]; ok {
			filtered = append(filtered, row)
		}
	}
	return filtered, len(strikeSet), true
}

func parsePositiveRange(minValue, maxValue string, defaultMin, defaultMax float64) (float64, float64, bool) {
	minThreshold, minErr := strconv.ParseFloat(strings.TrimSpace(minValue), 64)
	maxThreshold, maxErr := strconv.ParseFloat(strings.TrimSpace(maxValue), 64)
	if minErr == nil && maxErr == nil &&
		minThreshold > 0 && maxThreshold > 0 &&
		!math.IsNaN(minThreshold) && !math.IsInf(minThreshold, 0) &&
		!math.IsNaN(maxThreshold) && !math.IsInf(maxThreshold, 0) &&
		minThreshold <= maxThreshold {
		return minThreshold, maxThreshold, true
	}
	fallbackMin := defaultMin
	fallbackMax := defaultMax
	if fallbackMin <= 0 || math.IsNaN(fallbackMin) || math.IsInf(fallbackMin, 0) {
		fallbackMin = defaultOptionsDeltaAbsMin
	}
	if fallbackMax <= 0 || math.IsNaN(fallbackMax) || math.IsInf(fallbackMax, 0) {
		fallbackMax = defaultOptionsDeltaAbsMax
	}
	if fallbackMin > fallbackMax {
		fallbackMin = defaultOptionsDeltaAbsMin
		fallbackMax = defaultOptionsDeltaAbsMax
	}
	return fallbackMin, fallbackMax, false
}

func filterOptionsRows(rows []map[string]any, focus string) []map[string]any {
	target := strings.TrimSpace(focus)
	if target == "" {
		return nil
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		contract := strings.TrimSpace(asString(row["ctp_contract"]))
		underlying := strings.TrimSpace(asString(row["underlying"]))
		symbol := strings.TrimSpace(asString(row["symbol"]))
		if strings.EqualFold(contract, target) || strings.EqualFold(underlying, target) || strings.EqualFold(symbol, target) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func (ui *UI) currentFocusSymbol() string {
	if symbol := strings.TrimSpace(ui.focusSymbol); symbol != "" {
		return symbol
	}
	if ui.liveMarket == nil {
		return ""
	}
	row, _ := ui.liveMarket.GetSelection()
	if row <= 0 {
		return ""
	}
	cell := ui.liveMarket.GetCell(row, 0)
	if cell == nil {
		return ""
	}
	return strings.TrimSpace(cell.Text)
}

func convertUnusualTrades(rows []map[string]any) []TradeRow {
	out := make([]TradeRow, 0, len(rows))
	for _, row := range rows {
		contract := strings.TrimSpace(asString(row["ctp_contract"]))
		if contract == "" {
			continue
		}
		price, hasPrice := asOptionalFloat(row["price"])
		size, hasSize := asOptionalFloat(row["turnover_chg"])
		ratio, hasRatio := asOptionalFloat(row["turnover_ratio"])
		strike, hasStrike := asOptionalFloat(row["strike"])
		tte, hasTTE := asOptionalFloat(row["tte"])
		timeText := strings.TrimSpace(asString(row["time"]))
		if parsed, err := time.Parse(time.RFC3339, timeText); err == nil {
			timeText = parsed.Format("15:04:05")
		} else if parsed, err := time.Parse(time.RFC3339Nano, timeText); err == nil {
			timeText = parsed.Format("15:04:05")
		}
		if timeText == "" {
			if ts, ok := asOptionalFloat(row["ts"]); ok {
				timeText = time.UnixMilli(int64(ts)).Format("15:04:05")
			}
		}
		if timeText == "" {
			timeText = "-"
		}
		ratioText := "-"
		if hasRatio {
			ratioText = fmt.Sprintf("%.1f%%", ratio*100)
		}
		out = append(out, TradeRow{
			Time:   timeText,
			Sym:    contract,
			CP:     strings.ToUpper(strings.TrimSpace(asString(row["cp"]))),
			Strike: formatOptionalFloat(strike, hasStrike),
			TTE:    formatOptionalFloat(tte, hasTTE),
			Price:  formatOptionalFloat(price, hasPrice),
			Size:   formatOptionalFloat(size, hasSize),
			IV:     ratioText,
			Tag:    "TURNOVER",
		})
	}
	return out
}

func optionsRowsKey(rows []map[string]any) string {
	if len(rows) == 0 {
		return ""
	}
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, strings.ToLower(strings.TrimSpace(asString(row["ctp_contract"]))))
	}
	return strings.Join(parts, "|")
}

func convertMarketRows(raw []map[string]any) []MarketRow {
	rows := make([]MarketRow, 0, len(raw))
	for _, item := range raw {
		symbol := strings.TrimSpace(asString(item["ctp_contract"]))
		if symbol == "" {
			continue
		}
		exchange := strings.TrimSpace(asString(item["exchange"]))
		if exchange == "" {
			exchange = "-"
		}
		last, hasLast := asOptionalFloat(item["last"])
		pre, hasPre := asOptionalFloat(item["pre_settlement"])
		bid, hasBid := asOptionalFloat(item["bid1"])
		ask, hasAsk := asOptionalFloat(item["ask1"])
		turnover, hasTurnover := asOptionalFloat(item["turnover"])
		oi, hasOI := asOptionalFloat(item["open_interest"])
		preOI, hasPreOI := asOptionalFloat(item["pre_open_interest"])
		chg := "-"
		if hasLast && hasPre {
			chg = formatChange(last - pre)
		}
		oiChgPct := "-"
		if hasOI && hasPreOI && preOI > 0 {
			oiChgPct = formatPercent(oi/preOI - 1)
		}
		ts := asString(item["datetime"])
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			ts = parsed.Format("15:04:05")
		} else if strings.TrimSpace(ts) == "" {
			ts = "-"
		}
		row := MarketRow{
			Symbol:   symbol,
			Exchange: exchange,
			Last:     formatOptionalFloat(last, hasLast),
			Chg:      chg,
			Bid:      formatOptionalFloat(bid, hasBid),
			Ask:      formatOptionalFloat(ask, hasAsk),
			Vol:      formatOptionalIntLike(item["volume"]),
			Turnover: formatOptionalFloat(turnover, hasTurnover),
			OI:       formatOptionalIntLike(item["open_interest"]),
			OIChgPct: oiChgPct,
			TS:       ts,
		}
		rows = append(rows, row)
	}
	return rows
}

func sortMarketRows(rows []MarketRow, sortBy string, asc bool) {
	key := strings.TrimSpace(strings.ToLower(sortBy))
	if key == "" {
		key = "volume"
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		leftRaw := marketRowFieldValue(left, key)
		rightRaw := marketRowFieldValue(right, key)
		leftVal, leftOK := parseFloat(leftRaw)
		rightVal, rightOK := parseFloat(rightRaw)
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			ls := strings.ToLower(strings.TrimSpace(leftRaw))
			rs := strings.ToLower(strings.TrimSpace(rightRaw))
			if ls == rs {
				return left.Symbol < right.Symbol
			}
			if asc {
				return ls < rs
			}
			return ls > rs
		}
		if leftVal == rightVal {
			return left.Symbol < right.Symbol
		}
		if asc {
			return leftVal < rightVal
		}
		return leftVal > rightVal
	})
}

func marketRowFieldValue(row MarketRow, key string) string {
	switch key {
	case "ctp_contract", "contract", "symbol":
		return row.Symbol
	case "exchange":
		return row.Exchange
	case "last":
		return row.Last
	case "chg":
		return row.Chg
	case "bid", "bid1":
		return row.Bid
	case "ask", "ask1":
		return row.Ask
	case "volume", "vol":
		return row.Vol
	case "turnover":
		return row.Turnover
	case "open_interest", "oi":
		return row.OI
	case "oi_chg", "oi_chg%", "oi_chg_pct":
		return strings.TrimSuffix(row.OIChgPct, "%")
	case "ts", "datetime":
		return row.TS
	default:
		return row.Vol
	}
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}

func asFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed
	default:
		return 0
	}
}

func asOptionalFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func parseFloat(raw string) (float64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "-" {
		return 0, false
	}
	normalized := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(trimmed, "k"), "K"))
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, false
	}
	if strings.HasSuffix(strings.ToLower(trimmed), "k") {
		value *= 1000
	}
	return value, true
}

func formatFloat(value float64) string {
	if value == 0 {
		return "0"
	}
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func formatChange(value float64) string {
	if value > 0 {
		return "+" + formatFloat(value)
	}
	return formatFloat(value)
}

func formatPercent(value float64) string {
	if value > 0 {
		return "+" + strconv.FormatFloat(value*100, 'f', 2, 64) + "%"
	}
	return strconv.FormatFloat(value*100, 'f', 2, 64) + "%"
}

func formatOptionalFloat(value float64, ok bool) string {
	if !ok {
		return "-"
	}
	return formatFloat(value)
}

func formatOptionalIntLike(value any) string {
	switch v := value.(type) {
	case nil:
		return "-"
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case float32:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return "-"
		}
		return trimmed
	default:
		return "-"
	}
}

func (ui *UI) sortDirection() string {
	if ui.marketSortAsc {
		return "asc"
	}
	return "desc"
}

func (ui *UI) startLiveProcessIfNeeded() {
	if ui.liveProc != nil && !ui.liveProc.Done() {
		ui.startOptionsWorkerIfNeeded()
		ui.startUnusualWorkerIfNeeded()
		return
	}
	if strings.TrimSpace(ui.routerAddr) == "" {
		ui.appendLiveLogLine("router addr missing")
		return
	}
	_, cfg, err := configstore.LoadDefault()
	if err != nil {
		ui.appendLiveLogLine("load config failed: " + err.Error())
		return
	}
	if err := cfg.ValidateLiveMD(); err != nil {
		ui.appendLiveLogLine("invalid live config: " + err.Error())
		return
	}

	liveCtx, cancel := context.WithCancel(context.Background())
	proc, err := live.StartDetached(liveCtx, cfg.LiveMD, "", ui.routerAddr, ui.logger)
	if err != nil {
		cancel()
		ui.appendLiveLogLine("start live md failed: " + err.Error())
		return
	}
	ui.liveCancel = cancel
	ui.liveProc = proc
	ui.appendLiveLogLine(fmt.Sprintf("live md started on %s:%d", cfg.LiveMD.Host, cfg.LiveMD.Port))
	ui.startOptionsWorkerIfNeeded()
	ui.startUnusualWorkerIfNeeded()
	if err := ui.pushUnusualThresholds(); err != nil {
		ui.appendLiveLogLine("sync unusual thresholds failed: " + err.Error())
	}

	go func(startedProc *live.Process) {
		err := <-startedProc.Exit()
		ui.app.QueueUpdateDraw(func() {
			if err != nil {
				ui.appendLiveLogLine("live md exited: " + err.Error())
			} else {
				ui.appendLiveLogLine("live md exited")
			}
			if ui.liveProc == startedProc {
				ui.liveProc = nil
				ui.liveCancel = nil
			}
			ui.stopOptionsWorker()
			ui.stopUnusualWorker()
		})
	}(proc)
}

func (ui *UI) stopLiveProcess() {
	if ui.liveCancel != nil {
		ui.liveCancel()
		ui.liveCancel = nil
	}
	if ui.liveProc != nil {
		ui.liveProc.Stop()
		ui.liveProc = nil
	}
	ui.stopOptionsWorker()
	ui.stopUnusualWorker()
}

func (ui *UI) startOptionsWorkerIfNeeded() {
	if ui.optsProc != nil && !ui.optsProc.Done() {
		return
	}
	if strings.TrimSpace(ui.routerAddr) == "" {
		ui.appendLiveLogLine("options worker skipped: router addr missing")
		return
	}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	proc, err := live.StartOptionsWorkerDetached(workerCtx, "", ui.routerAddr, ui.logger)
	if err != nil {
		workerCancel()
		ui.appendLiveLogLine("start options worker failed: " + err.Error())
		return
	}
	ui.optsCancel = workerCancel
	ui.optsProc = proc
	ui.appendLiveLogLine("options worker started")

	go func(startedProc *live.Process) {
		err := <-startedProc.Exit()
		ui.app.QueueUpdateDraw(func() {
			if err != nil {
				ui.appendLiveLogLine("options worker exited: " + err.Error())
			} else {
				ui.appendLiveLogLine("options worker exited")
			}
			if ui.optsProc == startedProc {
				ui.optsProc = nil
				ui.optsCancel = nil
			}
		})
	}(proc)
}

func (ui *UI) stopOptionsWorker() {
	if ui.optsCancel != nil {
		ui.optsCancel()
		ui.optsCancel = nil
	}
	if ui.optsProc != nil {
		ui.optsProc.Stop()
		ui.optsProc = nil
	}
}

func (ui *UI) startUnusualWorkerIfNeeded() {
	if ui.unusualProc != nil && !ui.unusualProc.Done() {
		return
	}
	if strings.TrimSpace(ui.routerAddr) == "" {
		ui.appendLiveLogLine("unusual worker skipped: router addr missing")
		return
	}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	proc, err := live.StartUnusualWorkerDetached(workerCtx, "", ui.routerAddr, ui.logger)
	if err != nil {
		workerCancel()
		ui.appendLiveLogLine("start unusual worker failed: " + err.Error())
		return
	}
	ui.unusualCancel = workerCancel
	ui.unusualProc = proc
	ui.appendLiveLogLine("unusual worker started")

	go func(startedProc *live.Process) {
		err := <-startedProc.Exit()
		ui.app.QueueUpdateDraw(func() {
			if err != nil {
				ui.appendLiveLogLine("unusual worker exited: " + err.Error())
			} else {
				ui.appendLiveLogLine("unusual worker exited")
			}
			if ui.unusualProc == startedProc {
				ui.unusualProc = nil
				ui.unusualCancel = nil
			}
		})
	}(proc)
}

func (ui *UI) stopUnusualWorker() {
	if ui.unusualCancel != nil {
		ui.unusualCancel()
		ui.unusualCancel = nil
	}
	if ui.unusualProc != nil {
		ui.unusualProc.Stop()
		ui.unusualProc = nil
	}
}

func (ui *UI) pushUnusualThresholds() error {
	if ui.rpcClient == nil {
		return fmt.Errorf("router rpc unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return ui.rpcClient.Call(ctx, "ui.set_unusual_threshold", router.SetUnusualThresholdParams{
		TurnoverChgThreshold:   ui.unusualChgThreshold,
		TurnoverRatioThreshold: ui.unusualRatioThreshold,
	}, nil)
}

func (ui *UI) setUnusualThresholds(chgThreshold, ratioThreshold float64) error {
	prevChg := ui.unusualChgThreshold
	prevRatio := ui.unusualRatioThreshold
	ui.unusualChgThreshold = chgThreshold
	ui.unusualRatioThreshold = ratioThreshold
	if err := ui.pushUnusualThresholds(); err != nil {
		ui.unusualChgThreshold = prevChg
		ui.unusualRatioThreshold = prevRatio
		return err
	}
	return nil
}

func (ui *UI) ensureFocusSymbol() {
	current := strings.TrimSpace(ui.focusSymbol)
	if current != "" && ui.hasMarketSymbol(current) {
		if ui.focusSyncPending {
			ui.focusSyncPending = false
			ui.pushFocusSymbol(current)
		}
		return
	}
	symbol := ui.selectedMarketSymbol()
	if symbol == "" && len(ui.marketRows) > 0 {
		symbol = strings.TrimSpace(ui.marketRows[0].Symbol)
		if ui.liveMarket != nil && ui.liveMarket.GetRowCount() > 1 {
			ui.liveMarket.Select(1, 0)
		}
	}
	if symbol == "" {
		if current != "" {
			ui.focusSymbol = ""
			ui.focusSyncPending = false
			ui.pushFocusSymbol("")
		}
		return
	}
	if strings.EqualFold(current, symbol) {
		ui.focusSymbol = symbol
		if ui.focusSyncPending {
			ui.focusSyncPending = false
			ui.pushFocusSymbol(symbol)
		}
		return
	}
	ui.focusSymbol = symbol
	ui.focusSyncPending = false
	ui.pushFocusSymbol(symbol)
}

func (ui *UI) hasMarketSymbol(symbol string) bool {
	target := strings.TrimSpace(symbol)
	if target == "" {
		return false
	}
	for _, row := range ui.marketRows {
		if strings.EqualFold(strings.TrimSpace(row.Symbol), target) {
			return true
		}
	}
	return false
}

func (ui *UI) pushFocusSymbol(symbol string) {
	if ui.rpcClient == nil {
		return
	}
	value := strings.TrimSpace(symbol)
	go func(target string) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = ui.rpcClient.Call(ctx, "ui.set_focus_symbol", router.SetFocusSymbolParams{Symbol: target}, nil)
	}(value)
}

func (ui *UI) appendLiveLogLine(line string) {
	ui.appendLiveLogLineAt(line, 0)
}

func (ui *UI) appendLiveLogLineAt(line string, tsMillis int64) {
	if strings.TrimSpace(line) == "" {
		return
	}
	stampTime := time.Now()
	if tsMillis > 0 {
		stampTime = time.UnixMilli(tsMillis)
	}
	stamped := fmt.Sprintf("[%s] %s", stampTime.Format("15:04:05"), line)
	ui.liveLogLines = append([]string{stamped}, ui.liveLogLines...)
	if len(ui.liveLogLines) > 12 {
		ui.liveLogLines = ui.liveLogLines[:12]
	}
	if ui.liveLog != nil {
		ui.liveLog.SetText(strings.Join(ui.liveLogLines, "\n"))
	}
}

func (ui *UI) stopTicker() {
	if ui.ticker != nil {
		ui.ticker.Stop()
	}
}

func (ui *UI) beforeDraw(screen tcell.Screen) bool {
	width, _ := screen.Size()
	ui.lastWidth = width
	ui.updateLogo(width)
	ui.updateDivider(width)
	return false
}

func setBorderColor(item tview.Primitive, color tcell.Color) {
	if setter, ok := item.(interface {
		SetBorderColor(tcell.Color) *tview.Box
	}); ok {
		setter.SetBorderColor(color)
	}
	if setter, ok := item.(interface {
		SetTitleColor(tcell.Color) *tview.Box
	}); ok {
		setter.SetTitleColor(color)
	}
}

func centerModal(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(p, height, 1, true).
				AddItem(nil, 0, 1, false),
			width, 1, true,
		).
		AddItem(nil, 0, 1, false)
}
