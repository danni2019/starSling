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

	liveProc   *live.Process
	liveCancel context.CancelFunc
	optsProc   *live.Process
	optsCancel context.CancelFunc

	lastMarketSeq    int64
	lastMarketStale  bool
	marketSortBy     string
	marketSortAsc    bool
	marketRawRows    []map[string]any
	marketRows       []MarketRow
	filterExchange   string
	filterClass      string
	filterSymbol     string
	lastOptionsSeq   int64
	lastOptionsStale bool
	lastOptionsKey   string
	liveLogLines     []string
	logoTitleWidth   int
	logoFrame        int
	lastWidth        int
}

func newUI(routerAddr string, logger *slog.Logger) *UI {
	if logger == nil {
		logger = logging.New("INFO")
	}
	ui := &UI{
		app:           tview.NewApplication(),
		pages:         tview.NewPages(),
		data:          mockData(),
		logger:        logger,
		routerAddr:    strings.TrimSpace(routerAddr),
		screen:        screenMain,
		marketSortBy:  "volume",
		marketSortAsc: false,
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
		} else {
			ui.openDrilldown()
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
	sortOptions := marketNumericColumns(ui.marketRawRows)
	if len(sortOptions) == 0 {
		sortOptions = []string{"volume"}
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
		sortBy := strings.TrimSpace(strings.ToLower(selectedSortBy))
		if sortBy == "" {
			sortBy = "volume"
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
		ui.marketSortBy = "volume"
		ui.marketSortAsc = false
		ui.renderMarketRows()
		ui.closeDrilldown()
	})
	form.AddButton("Cancel", func() {
		ui.closeDrilldown()
	})

	ui.pages.AddPage(string(screenDrilldown), centerModal(form, 68, 15), true, true)
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
	err := ui.rpcClient.Call(ctx, "router.get_view_snapshot", router.GetViewSnapshotParams{}, &view)
	ui.app.QueueUpdateDraw(func() {
		ui.logoFrame = (ui.logoFrame + 1) % 2
		ui.updateLogo(ui.lastWidth)
		if err != nil {
			ui.appendLiveLogLine("router poll failed: " + err.Error())
			return
		}
		ui.applyMarketSnapshot(view.Market)
		ui.applyOptionsSnapshot(view.Options)
	})
}

func (ui *UI) applyMarketSnapshot(snapshot router.MarketSnapshot) {
	if snapshot.Seq == 0 {
		shouldClear := ui.lastMarketSeq != 0 || ui.lastMarketStale || len(ui.marketRows) > 0 ||
			(ui.liveMarket != nil && ui.liveMarket.GetRowCount() > 1)
		if !shouldClear {
			return
		}
		ui.marketRawRows = nil
		ui.marketRows = nil
		ui.lastMarketSeq = 0
		ui.lastMarketStale = false
		ui.renderMarketRows()
		return
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
		}
		return
	}
	optionsKey := optionsRowsKey(snapshot.Rows)
	seqChanged := snapshot.Seq != ui.lastOptionsSeq
	staleChanged := snapshot.Stale != ui.lastOptionsStale
	keyChanged := optionsKey != ui.lastOptionsKey
	if !seqChanged && !staleChanged && !keyChanged {
		return
	}
	if seqChanged || keyChanged {
		ui.liveOpts.SetText(renderOptionsPanel(snapshot.Rows))
		ui.lastOptionsSeq = snapshot.Seq
		ui.lastOptionsKey = optionsKey
	}
	if staleChanged && snapshot.Stale {
		ui.appendLiveLogLine("options snapshot stale")
	}
	ui.lastOptionsStale = snapshot.Stale
}

func (ui *UI) renderMarketRows() {
	if ui.liveMarket == nil {
		return
	}
	selectedSymbol := ui.selectedMarketSymbol()
	selectedRow, _ := ui.liveMarket.GetSelection()
	if ui.marketRawRows != nil {
		filtered := filterMarketRows(ui.marketRawRows, ui.filterExchange, ui.filterClass, ui.filterSymbol)
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

func filterMarketRows(rows []map[string]any, exchange, productClass, symbol string) []map[string]any {
	exchangeTokens := csvTokens(exchange)
	classTokens := csvTokens(productClass)
	symbolTokens := csvTokens(symbol)
	if len(exchangeTokens) == 0 && len(classTokens) == 0 && len(symbolTokens) == 0 {
		return rows
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		rowExchange := strings.TrimSpace(asString(row["exchange"]))
		rowClass := strings.TrimSpace(asString(row["product_class"]))
		rowSymbol := strings.TrimSpace(asString(row["symbol"]))
		if !tokenMatch(exchangeTokens, rowExchange) {
			continue
		}
		if !tokenMatch(classTokens, rowClass) {
			continue
		}
		if !tokenMatch(symbolTokens, rowSymbol) {
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

func marketNumericColumns(rows []map[string]any) []string {
	if len(rows) == 0 {
		return nil
	}
	columnSet := make(map[string]struct{})
	for _, row := range rows {
		for key, value := range row {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, ok := asOptionalFloat(value); ok {
				columnSet[strings.ToLower(key)] = struct{}{}
			}
		}
	}
	columns := make([]string, 0, len(columnSet))
	for key := range columnSet {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return columns
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

type optionPoint struct {
	Contract   string
	Underlying string
	Strike     float64
	IV         float64
	Volume     float64
}

func renderOptionsPanel(rows []map[string]any) string {
	if len(rows) == 0 {
		return "No options data for current focus."
	}
	points := make([]optionPoint, 0, len(rows))
	for _, row := range rows {
		strike, strikeOK := asOptionalFloat(row["strike"])
		iv, ivOK := asOptionalFloat(row["iv"])
		vol, volOK := asOptionalFloat(row["volume"])
		contract := strings.TrimSpace(asString(row["ctp_contract"]))
		if contract == "" || !strikeOK || !ivOK || math.IsNaN(iv) || math.IsInf(iv, 0) {
			continue
		}
		point := optionPoint{
			Contract:   contract,
			Underlying: strings.TrimSpace(asString(row["underlying"])),
			Strike:     strike,
			IV:         iv,
			Volume:     0,
		}
		if volOK && !math.IsNaN(vol) && !math.IsInf(vol, 0) {
			point.Volume = vol
		}
		points = append(points, point)
	}
	if len(points) == 0 {
		return "No valid option points."
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Strike < points[j].Strike })
	if len(points) > 24 {
		points = points[:24]
	}

	minIV, maxIV := minMax(func(p optionPoint) float64 { return p.IV }, points)
	minVol, maxVol := minMax(func(p optionPoint) float64 { return p.Volume }, points)
	ivLine := make([]string, 0, len(points))
	volLine := make([]string, 0, len(points))
	strikeLine := make([]string, 0, len(points))
	for _, p := range points {
		ivLine = append(ivLine, levelGlyph(normalize(p.IV, minIV, maxIV)))
		volLine = append(volLine, levelGlyph(normalize(p.Volume, minVol, maxVol)))
		strikeLine = append(strikeLine, shortStrike(p.Strike))
	}

	lines := []string{
		fmt.Sprintf("Options points: %d", len(points)),
		"IV curve (strike asc):  " + strings.Join(ivLine, ""),
		"VOL bars  (strike asc): " + strings.Join(volLine, ""),
		"Strikes: " + strings.Join(strikeLine, " "),
		"",
		"K        IV       VOL      CONTRACT",
	}
	limitRows := len(points)
	if limitRows > 8 {
		limitRows = 8
	}
	for i := 0; i < limitRows; i++ {
		p := points[i]
		lines = append(lines, fmt.Sprintf("%-8s %-8s %-8s %s",
			formatFloat(p.Strike),
			formatFloat(p.IV),
			formatFloat(p.Volume),
			p.Contract,
		))
	}
	return strings.Join(lines, "\n")
}

func minMax(fn func(optionPoint) float64, values []optionPoint) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	minVal := fn(values[0])
	maxVal := minVal
	for _, v := range values[1:] {
		val := fn(v)
		if val < minVal {
			minVal = val
		}
		if val > maxVal {
			maxVal = val
		}
	}
	return minVal, maxVal
}

func normalize(value, minVal, maxVal float64) float64 {
	if maxVal <= minVal {
		return 0
	}
	return (value - minVal) / (maxVal - minVal)
}

func levelGlyph(level float64) string {
	levels := []string{".", ":", "-", "=", "+", "*", "#", "@"}
	if level <= 0 {
		return levels[0]
	}
	if level >= 1 {
		return levels[len(levels)-1]
	}
	idx := int(level * float64(len(levels)-1))
	return levels[idx]
}

func shortStrike(value float64) string {
	raw := formatFloat(value)
	if len(raw) <= 4 {
		return raw
	}
	return raw[len(raw)-4:]
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
		last, hasLast := asOptionalFloat(item["last"])
		pre, hasPre := asOptionalFloat(item["pre_settlement"])
		bid, hasBid := asOptionalFloat(item["bid1"])
		ask, hasAsk := asOptionalFloat(item["ask1"])
		chg := "-"
		if hasLast && hasPre {
			chg = formatChange(last - pre)
		}
		ts := asString(item["datetime"])
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			ts = parsed.Format("15:04:05")
		} else if strings.TrimSpace(ts) == "" {
			ts = "-"
		}
		row := MarketRow{
			Symbol: symbol,
			Last:   formatOptionalFloat(last, hasLast),
			Chg:    chg,
			Bid:    formatOptionalFloat(bid, hasBid),
			Ask:    formatOptionalFloat(ask, hasAsk),
			Vol:    formatOptionalIntLike(item["volume"]),
			OI:     formatOptionalIntLike(item["open_interest"]),
			TS:     ts,
		}
		rows = append(rows, row)
	}
	return rows
}

func sortMarketRows(rows []MarketRow, sortBy string, asc bool) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		var leftRaw string
		var rightRaw string
		switch sortBy {
		case "last":
			leftRaw = left.Last
			rightRaw = right.Last
		case "open_interest":
			leftRaw = left.OI
			rightRaw = right.OI
		default:
			leftRaw = left.Vol
			rightRaw = right.Vol
		}
		leftVal, leftOK := parseFloat(leftRaw)
		rightVal, rightOK := parseFloat(rightRaw)
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return left.Symbol < right.Symbol
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

func (ui *UI) appendLiveLogLine(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	stamped := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), line)
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
