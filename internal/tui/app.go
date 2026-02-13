package tui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/configstore"
	"github.com/danni2019/starSling/internal/ipc"
	"github.com/danni2019/starSling/internal/live"
	"github.com/danni2019/starSling/internal/logging"
	"github.com/danni2019/starSling/internal/metadata"
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
const defaultFlowWindowSeconds = 120
const defaultFlowMinAnalysisSeconds = 30
const maxVoiceQueueSize = 64
const internalDebugUIEnv = "STARSLING_INTERNAL_DEBUG_UI"

var marketDisplaySortFields = []string{
	"contract",
	"exchange",
	"last",
	"chg",
	"chg_pct",
	"bidv",
	"bid",
	"ask",
	"askv",
	"vol",
	"turnover",
	"oi",
	"oi_chg_pct",
	"ts",
}

func runtimeDebugUIEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(internalDebugUIEnv)))
	return value == "1" || value == "true" || value == "yes"
}

type UI struct {
	app   *tview.Application
	pages *tview.Pages

	logger     *slog.Logger
	routerAddr string
	rpcClient  *ipc.Client
	metadata   *metadata.ContractMappings

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
	liveFlow   *tview.Table

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

	lastMarketSeq             int64
	lastMarketStale           bool
	marketSortBy              string
	marketSortAsc             bool
	marketRawRows             []map[string]any
	marketRows                []MarketRow
	filterExchange            string
	filterClass               string
	filterSymbol              string
	filterContract            string
	filterMainOnly            bool
	focusSymbol               string
	focusSyncPending          bool
	lastOptionsSeq            int64
	lastOptionsStale          bool
	lastOptionsKey            string
	optionsRawRows            []map[string]any
	optionsDeltaAbsMin        float64
	optionsDeltaAbsMax        float64
	optionsDeltaEnabled       bool
	voiceEnabled              bool
	voiceContracts            map[string]struct{}
	voiceLastSpoken           map[string]time.Time
	voiceLastPrice            map[string]float64
	voiceUnavailable          bool
	voicePlaybackEnabled      atomic.Bool
	voiceMutedAt              time.Time
	lastCurveContracts        []string
	lastCurveSeq              int64
	lastCurveStale            bool
	lastUnusualSeq            int64
	lastUnusualStale          bool
	lastLogsSeq               int64
	unusualChgThreshold       float64
	unusualRatioThreshold     float64
	unusualOIRatioThreshold   float64
	liveLogLines              []string
	flowWindowSeconds         int
	flowMinAnalysisSeconds    int
	flowOnlySelectedContracts bool
	flowOnlyFocusedSymbol     bool
	flowSortBy                string
	flowSortAsc               bool
	flowEvents                []flowEvent
	flowSeen                  map[string]int64
	flowPrevByContract        map[string]optionFrame
	flowCurrByContract        map[string]optionFrame
	flowHasResult             bool
	voiceQueue                chan string
	logoTitleWidth            int
	logoFrame                 int
	lastWidth                 int
}

func newUI(routerAddr string, logger *slog.Logger) *UI {
	if logger == nil {
		logger = logging.New("INFO")
	}
	mappings, err := metadata.LoadContractMappings()
	if err != nil {
		logger.Warn("load contract metadata mappings failed", "error", err)
	}
	ui := &UI{
		app:                     tview.NewApplication(),
		pages:                   tview.NewPages(),
		data:                    mockData(),
		logger:                  logger,
		routerAddr:              strings.TrimSpace(routerAddr),
		metadata:                mappings,
		screen:                  screenMain,
		marketSortBy:            "vol",
		marketSortAsc:           false,
		unusualChgThreshold:     100000.0,
		unusualRatioThreshold:   0.05,
		unusualOIRatioThreshold: 0.05,
		flowWindowSeconds:       defaultFlowWindowSeconds,
		flowMinAnalysisSeconds:  defaultFlowMinAnalysisSeconds,
		flowSortBy:              "total_turnover_sum",
		flowSortAsc:             false,
		optionsDeltaAbsMin:      defaultOptionsDeltaAbsMin,
		optionsDeltaAbsMax:      defaultOptionsDeltaAbsMax,
		voiceContracts:          make(map[string]struct{}),
		voiceLastSpoken:         make(map[string]time.Time),
		voiceLastPrice:          make(map[string]float64),
		flowSeen:                make(map[string]int64),
		flowPrevByContract:      make(map[string]optionFrame),
		flowCurrByContract:      make(map[string]optionFrame),
		voiceQueue:              make(chan string, maxVoiceQueueSize),
	}
	if ui.routerAddr != "" {
		ui.rpcClient = ipc.NewClient(ui.routerAddr)
	}

	ui.buildScreens()
	ui.bindKeys()
	ui.startVoiceWorker()
	ui.startTicker()

	ui.app.SetRoot(ui.pages, true)
	ui.app.SetBeforeDrawFunc(ui.beforeDraw)
	ui.app.SetFocus(ui.menu)

	return ui
}

func (ui *UI) Run() error {
	defer ui.stopTicker()
	defer ui.stopLiveProcess()
	defer ui.stopVoiceWorker()
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
		} else if ui.app.GetFocus() == ui.liveCurve {
			ui.openVoiceSettings()
		} else if ui.app.GetFocus() == ui.liveTrades {
			ui.openUnusualThresholdSettings()
		} else if ui.app.GetFocus() == ui.liveFlow {
			ui.openFlowSettings()
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
	mainOnlyCheckbox := tview.NewCheckbox().
		SetLabel("Main contract only").
		SetChecked(ui.filterMainOnly)
	sortOptions := displayMarketSortColumns()
	sortIdx := indexOfFold(sortOptions, ui.marketSortBy)
	if sortIdx < 0 {
		sortIdx = indexOfFold(sortOptions, "vol")
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
		AddFormItem(mainOnlyCheckbox).
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
		ui.filterMainOnly = mainOnlyCheckbox.IsChecked()
		sortBy := strings.TrimSpace(strings.ToLower(selectedSortBy))
		if sortBy == "" {
			sortBy = "vol"
		}
		ui.marketSortBy = sortBy
		order := strings.TrimSpace(strings.ToLower(selectedOrder))
		ui.marketSortAsc = order == "asc"
		ui.renderMarketRows()
		ui.ensureFocusSymbol()
		ui.renderFlowAggregation()
		ui.closeDrilldown()
	})
	form.AddButton("Reset", func() {
		ui.resetMarketFilters()
		ui.renderMarketRows()
		ui.ensureFocusSymbol()
		ui.renderFlowAggregation()
		ui.closeDrilldown()
	})
	form.AddButton("Cancel", func() {
		ui.closeDrilldown()
	})

	ui.pages.AddPage(string(screenDrilldown), centerModal(form, 68, 16), true, true)
	ui.app.SetFocus(form)
}

func (ui *UI) resetMarketFilters() {
	ui.filterExchange = ""
	ui.filterClass = ""
	ui.filterSymbol = ""
	ui.filterContract = ""
	ui.filterMainOnly = false
	ui.marketSortBy = "vol"
	ui.marketSortAsc = false
}

func (ui *UI) openUnusualThresholdSettings() {
	ui.setCurrentScreen(screenDrilldown)
	chgInput := tview.NewInputField().
		SetLabel("Turnover Chg >= ").
		SetText(formatFloat(ui.unusualChgThreshold))
	ratioInput := tview.NewInputField().
		SetLabel("Turnover Ratio >= ").
		SetText(strconv.FormatFloat(ui.unusualRatioThreshold, 'f', 4, 64))
	oiRatioInput := tview.NewInputField().
		SetLabel("OI Ratio >= ").
		SetText(strconv.FormatFloat(ui.unusualOIRatioThreshold, 'f', 4, 64))
	hint := tview.NewTextView().
		SetTextColor(colorMuted).
		SetText(" ")

	form := tview.NewForm().
		AddFormItem(chgInput).
		AddFormItem(ratioInput).
		AddFormItem(oiRatioInput)
	form.SetBorder(true).SetTitle("Unusual thresholds")
	form.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	form.SetBackgroundColor(colorBackground)
	form.SetFieldBackgroundColor(colorBackground)
	form.SetFieldTextColor(colorTableRow)
	form.SetButtonBackgroundColor(colorHighlight)
	form.SetButtonTextColor(colorMenuSelected)
	form.AddButton("Apply", func() {
		chg, ratio, oiRatio, ok := parseUnusualThresholdInputs(
			chgInput.GetText(),
			ratioInput.GetText(),
			oiRatioInput.GetText(),
		)
		if !ok {
			hint.SetText("invalid input: all thresholds must be positive numbers")
			return
		}
		if err := ui.setUnusualThresholds(chg, ratio, oiRatio); err != nil {
			hint.SetText("failed to update thresholds: " + err.Error())
			ui.appendLiveLogLine("failed to update unusual thresholds: " + err.Error())
			return
		}
		hint.SetText(" ")
		ui.appendLiveLogLine(fmt.Sprintf("unusual thresholds updated: chg>=%.0f turnover_ratio>=%.2f%% oi_ratio>=%.2f%%", chg, ratio*100, oiRatio*100))
		ui.closeDrilldown()
	})
	form.AddButton("Cancel", func() {
		ui.closeDrilldown()
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(hint, 1, 0, false)
	ui.pages.AddPage(string(screenDrilldown), centerModal(layout, 62, 14), true, true)
	ui.app.SetFocus(form)
}

func (ui *UI) openFlowSettings() {
	ui.setCurrentScreen(screenDrilldown)
	selectedOnly, focusedOnly := normalizeExclusiveFlowFilters(ui.flowOnlySelectedContracts, ui.flowOnlyFocusedSymbol)
	ui.flowOnlySelectedContracts = selectedOnly
	ui.flowOnlyFocusedSymbol = focusedOnly

	windowInput := tview.NewInputField().
		SetLabel("window_size(sec) [60,300]: ").
		SetText(strconv.Itoa(ui.flowWindowSeconds))
	minWindowInput := tview.NewInputField().
		SetLabel("min_analysis(sec) [15,60]: ").
		SetText(strconv.Itoa(ui.flowMinAnalysisSeconds))
	selectedContractsBox := tview.NewCheckbox().
		SetLabel("Only selected contracts").
		SetChecked(selectedOnly)
	focusedSymbolBox := tview.NewCheckbox().
		SetLabel("Only focused symbol").
		SetChecked(focusedOnly)
	syncingExclusive := false
	selectedContractsBox.SetChangedFunc(func(checked bool) {
		if syncingExclusive || !checked {
			return
		}
		syncingExclusive = true
		focusedSymbolBox.SetChecked(false)
		syncingExclusive = false
	})
	focusedSymbolBox.SetChangedFunc(func(checked bool) {
		if syncingExclusive || !checked {
			return
		}
		syncingExclusive = true
		selectedContractsBox.SetChecked(false)
		syncingExclusive = false
	})
	hint := tview.NewTextView().
		SetTextColor(colorMuted).
		SetText(" ")

	form := tview.NewForm().
		AddFormItem(windowInput).
		AddFormItem(minWindowInput).
		AddFormItem(selectedContractsBox).
		AddFormItem(focusedSymbolBox)
	form.SetBorder(true).SetTitle("Flow Aggregation Settings")
	form.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	form.SetBackgroundColor(colorBackground)
	form.SetFieldBackgroundColor(colorBackground)
	form.SetFieldTextColor(colorTableRow)
	form.SetButtonBackgroundColor(colorHighlight)
	form.SetButtonTextColor(colorMenuSelected)

	form.AddButton("Apply", func() {
		selectedOnly, focusedOnly := normalizeExclusiveFlowFilters(
			selectedContractsBox.IsChecked(),
			focusedSymbolBox.IsChecked(),
		)
		ui.flowOnlySelectedContracts = selectedOnly
		ui.flowOnlyFocusedSymbol = focusedOnly
		valid := ui.applyFlowSettings(windowInput.GetText(), minWindowInput.GetText())
		windowInput.SetText(strconv.Itoa(ui.flowWindowSeconds))
		minWindowInput.SetText(strconv.Itoa(ui.flowMinAnalysisSeconds))
		if !valid {
			hint.SetText("invalid input detected, reset to defaults")
			return
		}
		hint.SetText(" ")
		ui.closeDrilldown()
	})
	form.AddButton("Cancel", func() {
		ui.closeDrilldown()
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(hint, 1, 0, false)
	ui.pages.AddPage(string(screenDrilldown), centerModal(layout, 66, 15), true, true)
	ui.app.SetFocus(form)
}

func parseIntInRange(raw string, minValue, maxValue int) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	if value < minValue || value > maxValue {
		return 0, false
	}
	return value, true
}

func parseUnusualThresholdInputs(chgRaw, ratioRaw, oiRatioRaw string) (float64, float64, float64, bool) {
	chg, errChg := strconv.ParseFloat(strings.TrimSpace(chgRaw), 64)
	ratio, errRatio := strconv.ParseFloat(strings.TrimSpace(ratioRaw), 64)
	oiRatio, errOIRatio := strconv.ParseFloat(strings.TrimSpace(oiRatioRaw), 64)
	if errChg != nil || errRatio != nil || errOIRatio != nil {
		return 0, 0, 0, false
	}
	if math.IsNaN(chg) || math.IsInf(chg, 0) ||
		math.IsNaN(ratio) || math.IsInf(ratio, 0) ||
		math.IsNaN(oiRatio) || math.IsInf(oiRatio, 0) {
		return 0, 0, 0, false
	}
	if chg <= 0 || ratio <= 0 || oiRatio <= 0 {
		return 0, 0, 0, false
	}
	return chg, ratio, oiRatio, true
}

func (ui *UI) applyFlowSettings(windowRaw, minRaw string) bool {
	windowSeconds, okWindow := parseIntInRange(windowRaw, 60, 300)
	minSeconds, okMin := parseIntInRange(minRaw, 15, 60)
	if !okWindow {
		windowSeconds = defaultFlowWindowSeconds
	}
	if !okMin {
		minSeconds = defaultFlowMinAnalysisSeconds
	}
	if minSeconds > windowSeconds {
		minSeconds = defaultFlowMinAnalysisSeconds
		okMin = false
	}
	ui.flowWindowSeconds = windowSeconds
	ui.flowMinAnalysisSeconds = minSeconds
	ui.renderFlowAggregation()
	return okWindow && okMin
}

func (ui *UI) openVoiceSettings() {
	ui.setCurrentScreen(screenDrilldown)
	enabled := ui.voiceEnabled
	contracts := make([]string, 0, len(ui.lastCurveContracts))
	contracts = append(contracts, ui.lastCurveContracts...)
	sort.Strings(contracts)
	localContracts := make(map[string]struct{}, len(ui.voiceContracts))
	for contract := range ui.voiceContracts {
		localContracts[contract] = struct{}{}
	}

	form := tview.NewForm()
	enabledBox := tview.NewCheckbox().SetLabel("Quote voice reporting").SetChecked(enabled)
	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetUseStyleTags(false, false)
	list.SetBorder(true).SetTitle("Contracts")
	list.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	for idx, contract := range contracts {
		text := contract
		if _, ok := localContracts[contract]; ok {
			text = "[x] " + contract
		} else {
			text = "[ ] " + contract
		}
		contractIdx := idx
		list.AddItem(text, "", 0, func() {
			name := contracts[contractIdx]
			if _, ok := localContracts[name]; ok {
				delete(localContracts, name)
			} else {
				localContracts[name] = struct{}{}
			}
			if _, ok := localContracts[name]; ok {
				list.SetItemText(contractIdx, "[x] "+name, "")
			} else {
				list.SetItemText(contractIdx, "[ ] "+name, "")
			}
		})
	}

	form.AddFormItem(enabledBox)
	form.AddButton("Apply", func() {
		ui.voiceEnabled = enabledBox.IsChecked()
		if !ui.voiceEnabled {
			ui.voicePlaybackEnabled.Store(false)
			ui.voiceContracts = make(map[string]struct{})
			ui.voiceLastSpoken = make(map[string]time.Time)
			ui.voiceLastPrice = make(map[string]float64)
			ui.voiceUnavailable = false
			ui.voiceMutedAt = time.Time{}
			ui.drainVoiceQueue()
		} else {
			ui.voicePlaybackEnabled.Store(true)
			ui.voiceUnavailable = false
			ui.voiceContracts = make(map[string]struct{}, len(localContracts))
			for contract := range localContracts {
				ui.voiceContracts[contract] = struct{}{}
			}
		}
		ui.closeDrilldown()
	})
	form.AddButton("Cancel", func() {
		ui.closeDrilldown()
	})
	form.SetBorder(true).SetTitle("Quote voice reporting")
	form.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	form.SetBackgroundColor(colorBackground)
	form.SetFieldBackgroundColor(colorBackground)
	form.SetFieldTextColor(colorTableRow)
	form.SetButtonBackgroundColor(colorHighlight)
	form.SetButtonTextColor(colorMenuSelected)

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			_, buttonIndex := form.GetFocusedItemIndex()
			if buttonIndex == form.GetButtonCount()-1 {
				ui.app.SetFocus(list)
				return nil
			}
		}
		return event
	})

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyBacktab:
			form.SetFocus(form.GetFormItemCount() + form.GetButtonCount() - 1)
			ui.app.SetFocus(form)
			return nil
		}
		return event
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 7, 1, true).
		AddItem(list, 0, 1, false)

	ui.pages.AddPage(string(screenDrilldown), centerModal(layout, 64, 16), true, true)
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
		ui.renderFlowAggregation()
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
		ui.metadata,
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
		ui.lastCurveContracts = nil
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
		ui.lastCurveContracts = extractCurveContracts(snapshot.Rows)
		ui.maybeSpeakQuotes(snapshot.Rows)
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
			ui.resetFlowAggregation()
			ui.lastUnusualSeq = 0
			ui.lastUnusualStale = false
		}
		return
	}
	if snapshot.Seq < ui.lastUnusualSeq {
		ui.resetFlowAggregation()
	}
	seqChanged := snapshot.Seq != ui.lastUnusualSeq
	staleChanged := snapshot.Stale != ui.lastUnusualStale
	if !seqChanged && !staleChanged {
		return
	}
	if seqChanged {
		fillTradesTable(ui.liveTrades, convertUnusualTrades(snapshot.Rows))
		ui.ingestFlowEvents(snapshot.Rows)
		ui.renderFlowAggregation()
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
		if strings.EqualFold(level, "debug") {
			continue
		}
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
		if ui.filterMainOnly {
			filtered = filterMainContractsBySymbol(filtered)
		}
		ui.marketRows = convertMarketRows(filtered)
		sortMarketRows(ui.marketRows, ui.marketSortBy, ui.marketSortAsc)
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

func filterMainContractsBySymbol(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}
	bestBySymbol := make(map[string]map[string]any)
	for _, row := range rows {
		symbol := strings.ToLower(strings.TrimSpace(asString(row["symbol"])))
		if symbol == "" {
			continue
		}
		current, exists := bestBySymbol[symbol]
		if !exists {
			bestBySymbol[symbol] = row
			continue
		}
		if shouldReplaceMainContract(current, row) {
			bestBySymbol[symbol] = row
		}
	}
	if len(bestBySymbol) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(bestBySymbol))
	for _, row := range bestBySymbol {
		out = append(out, row)
	}
	return out
}

func shouldReplaceMainContract(current, candidate map[string]any) bool {
	currentOI, currentHasOI := asOptionalFloat(current["open_interest"])
	candidateOI, candidateHasOI := asOptionalFloat(candidate["open_interest"])
	if currentHasOI && (math.IsNaN(currentOI) || math.IsInf(currentOI, 0)) {
		currentHasOI = false
	}
	if candidateHasOI && (math.IsNaN(candidateOI) || math.IsInf(candidateOI, 0)) {
		candidateHasOI = false
	}
	if !candidateHasOI {
		candidateOI = math.Inf(-1)
	}
	if !currentHasOI {
		currentOI = math.Inf(-1)
	}
	if candidateOI > currentOI {
		return true
	}
	if candidateOI < currentOI {
		return false
	}
	currentContract := strings.ToLower(strings.TrimSpace(asString(current["ctp_contract"])))
	candidateContract := strings.ToLower(strings.TrimSpace(asString(candidate["ctp_contract"])))
	if currentContract == "" {
		return true
	}
	if candidateContract == "" {
		return false
	}
	return candidateContract < currentContract
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

func displayMarketSortColumns() []string {
	out := make([]string, len(marketDisplaySortFields))
	copy(out, marketDisplaySortFields)
	return out
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
	Contract    string
	Forward     float64
	Volume      float64
	OI          float64
	BidVol      float64
	Bid         float64
	Ask         float64
	AskVol      float64
	VIX         float64
	CallSkew    float64
	PutSkew     float64
	HasVolume   bool
	HasOI       bool
	HasBid      bool
	HasAsk      bool
	HasBidVol   bool
	HasAskVol   bool
	HasVIX      bool
	HasCallSkew bool
	HasPutSkew  bool
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
		callSkew, hasCallSkew := asOptionalFloat(row["call_skew"])
		putSkew, hasPutSkew := asOptionalFloat(row["put_skew"])
		points = append(points, curvePoint{
			Contract:    contract,
			Forward:     forward,
			Volume:      volume,
			OI:          oi,
			BidVol:      bidVol,
			Bid:         bid,
			Ask:         ask,
			AskVol:      askVol,
			VIX:         vix,
			CallSkew:    callSkew,
			PutSkew:     putSkew,
			HasVolume:   hasVolume,
			HasOI:       hasOI,
			HasBidVol:   hasBidVol,
			HasBid:      hasBid,
			HasAsk:      hasAsk,
			HasAskVol:   hasAskVol,
			HasVIX:      hasVIX,
			HasCallSkew: hasCallSkew,
			HasPutSkew:  hasPutSkew,
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
		formatAlignedColumns([]string{"CNTRCT", "FWD", "VOL", "OI", "BIDV", "BID", "ASK", "ASKV", "VIX", "CALL_SKW", "PUT_SKW"}, unifiedColumnWidth),
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
		callSkewText := "-"
		if p.HasCallSkew {
			callSkewText = formatFloat(p.CallSkew)
		}
		putSkewText := "-"
		if p.HasPutSkew {
			putSkewText = formatFloat(p.PutSkew)
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
			callSkewText,
			putSkewText,
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

type contractResolver interface {
	ResolveOptionUnderlying(contract string) (string, bool)
	ResolveContractSymbol(contract string) (string, bool)
	ResolveOptionTypeCP(contract string) (string, bool)
}

type contractFallbackResolver interface {
	InferOptionUnderlying(contract string) (string, bool)
	InferContractSymbol(contract string) (string, bool)
	InferOptionTypeCP(contract string) (string, bool)
}

func inferOptionTypeFromContract(contract string) string {
	trimmed := strings.TrimSpace(contract)
	upper := strings.ToUpper(trimmed)
	if len(upper) < 3 {
		return ""
	}
	idx := optionContractCPIndex(trimmed)
	if idx < 0 {
		return ""
	}
	if upper[idx] == 'C' {
		return "c"
	}
	return "p"
}

func inferOptionUnderlyingFromContract(contract string) string {
	trimmed := strings.TrimSpace(contract)
	idx := optionContractCPIndex(trimmed)
	if idx <= 0 {
		return ""
	}
	underlying := strings.TrimSpace(trimmed[:idx])
	underlying = strings.TrimRight(underlying, "-_")
	if token := leadingContractToken(underlying); token != "" {
		return token
	}
	return underlying
}

func optionContractCPIndex(contract string) int {
	upper := strings.ToUpper(strings.TrimSpace(contract))
	if len(upper) < 3 {
		return -1
	}
	if idx := strings.LastIndex(upper, "-C-"); idx > 0 {
		return idx + 1
	}
	if idx := strings.LastIndex(upper, "-P-"); idx > 0 {
		return idx + 1
	}
	for i := len(upper) - 2; i >= 1; i-- {
		ch := upper[i]
		if ch != 'C' && ch != 'P' {
			continue
		}
		suffix := upper[i+1:]
		if suffix == "" {
			continue
		}
		allDigits := true
		for _, c := range suffix {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}
		prefix := upper[:i]
		if !strings.ContainsAny(prefix, "0123456789") {
			continue
		}
		return i
	}
	return -1
}

func leadingContractToken(contract string) string {
	contract = strings.TrimSpace(contract)
	if contract == "" {
		return ""
	}
	idx := 0
	for idx < len(contract) {
		ch := contract[idx]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			idx++
			continue
		}
		break
	}
	if idx == 0 {
		return ""
	}
	digitStart := idx
	for idx < len(contract) {
		ch := contract[idx]
		if ch >= '0' && ch <= '9' {
			idx++
			continue
		}
		break
	}
	if idx == digitStart {
		return ""
	}
	return contract[:idx]
}

func resolveOptionTypeCP(contract string, resolver contractResolver) (string, bool) {
	if resolver == nil {
		cp := strings.ToLower(strings.TrimSpace(inferOptionTypeFromContract(contract)))
		if cp == "c" || cp == "p" {
			return cp, true
		}
		return "", false
	}
	cp, ok := resolver.ResolveOptionTypeCP(contract)
	if !ok || strings.TrimSpace(cp) == "" {
		if fallback, ok := resolver.(contractFallbackResolver); ok {
			if inferred, ok := fallback.InferOptionTypeCP(contract); ok {
				cp = inferred
				ok = true
			}
		}
	}
	if !ok || strings.TrimSpace(cp) == "" {
		cp = inferOptionTypeFromContract(contract)
	}
	cp = strings.ToLower(strings.TrimSpace(cp))
	if cp != "c" && cp != "p" {
		return "", false
	}
	return cp, true
}

func resolveOptionUnderlying(contract, existing string, resolver contractResolver) (string, bool) {
	if resolver != nil {
		if underlying, ok := resolver.ResolveOptionUnderlying(contract); ok {
			underlying = strings.TrimSpace(underlying)
			if underlying != "" {
				return underlying, true
			}
		}
	}
	existing = strings.TrimSpace(existing)
	if existing != "" {
		return existing, true
	}
	if resolver != nil {
		if fallback, ok := resolver.(contractFallbackResolver); ok {
			if underlying, ok := fallback.InferOptionUnderlying(contract); ok {
				underlying = strings.TrimSpace(underlying)
				if underlying != "" {
					return underlying, true
				}
			}
		}
	}
	inferred := inferOptionUnderlyingFromContract(contract)
	if inferred != "" {
		return inferred, true
	}
	return "", false
}

func resolveContractSymbol(contract, existing string, resolver contractResolver) (string, bool) {
	if resolver != nil {
		if symbol, ok := resolver.ResolveContractSymbol(contract); ok {
			symbol = strings.TrimSpace(symbol)
			if symbol != "" {
				return symbol, true
			}
		}
	}
	existing = strings.TrimSpace(existing)
	if existing != "" {
		return existing, true
	}
	if resolver != nil {
		if fallback, ok := resolver.(contractFallbackResolver); ok {
			if symbol, ok := fallback.InferContractSymbol(contract); ok {
				symbol = strings.TrimSpace(symbol)
				if symbol != "" {
					return symbol, true
				}
			}
		}
	}
	inferred := inferContractRoot(contract)
	if inferred != "" {
		return inferred, true
	}
	return "", false
}

func renderOptionsPanel(resolver contractResolver, rows []map[string]any, focusSymbol string, filter optionRenderFilter) string {
	focusText := strings.TrimSpace(focusSymbol)
	if focusText == "" {
		return "Select a contract."
	}
	rows = filterOptionsRowsWithResolver(rows, focusText, resolver)
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
			if resolvedCP, ok := resolveOptionTypeCP(contract, resolver); ok {
				cp = resolvedCP
			} else {
				cp = inferOptionTypeFromContract(contract)
			}
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

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func matchesFocusFields(contract, underlying, symbol, focus string, resolver contractResolver) bool {
	target := normalizeToken(focus)
	if target == "" {
		return false
	}
	for _, candidate := range focusMatchCandidates(contract, underlying, symbol, resolver) {
		if normalizeToken(candidate) == target {
			return true
		}
	}
	return false
}

func focusMatchCandidates(contract, underlying, symbol string, resolver contractResolver) []string {
	candidates := make([]string, 0, 5)
	seen := make(map[string]struct{}, 5)
	appendIfPresent := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		key := normalizeToken(trimmed)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, trimmed)
	}
	appendIfPresent(contract)
	resolvedUnderlying := ""
	if value, ok := resolveOptionUnderlying(contract, underlying, resolver); ok {
		resolvedUnderlying = value
		appendIfPresent(resolvedUnderlying)
	} else {
		appendIfPresent(underlying)
	}
	if resolvedContractSymbol, ok := resolveContractSymbol(contract, symbol, resolver); ok {
		appendIfPresent(resolvedContractSymbol)
	} else {
		appendIfPresent(symbol)
	}
	if resolvedUnderlying != "" {
		if resolvedUnderlyingSymbol, ok := resolveContractSymbol(resolvedUnderlying, "", resolver); ok {
			appendIfPresent(resolvedUnderlyingSymbol)
		}
	}
	return candidates
}

func filterOptionsRows(rows []map[string]any, focus string) []map[string]any {
	return filterOptionsRowsWithResolver(rows, focus, nil)
}

func filterOptionsRowsWithResolver(rows []map[string]any, focus string, resolver contractResolver) []map[string]any {
	target := strings.TrimSpace(focus)
	if target == "" {
		return nil
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		contract := strings.TrimSpace(asString(row["ctp_contract"]))
		underlying := strings.TrimSpace(asString(row["underlying"]))
		symbol := strings.TrimSpace(asString(row["symbol"]))
		if matchesFocusFields(contract, underlying, symbol, target, resolver) {
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
		tag := strings.ToUpper(strings.TrimSpace(asString(row["tag"])))
		if tag == "" {
			tag = "TURNOVER"
		}
		size, hasSize := asOptionalFloat(row["turnover_chg"])
		ratio, hasRatio := asOptionalFloat(row["turnover_ratio"])
		if tag == "OI" {
			size, hasSize = asOptionalFloat(row["oi_chg"])
			ratio, hasRatio = asOptionalFloat(row["oi_ratio"])
		}
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
			Tag:    tag,
		})
	}
	return out
}

const (
	flowNeutralThreshold = 0.15
	flowQualityCap       = 0.70
	flowPositionCap      = 0.80
)

type optionFrame struct {
	TS               int64
	Last             float64
	HasLast          bool
	High             float64
	HasHigh          bool
	Low              float64
	HasLow           bool
	Volume           float64
	HasVolume        bool
	Turnover         float64
	HasTurnover      bool
	OpenInterest     float64
	HasOpenInterest  bool
	Bid1             float64
	HasBid1          bool
	Ask1             float64
	HasAsk1          bool
	BidVol1          float64
	HasBidVol1       bool
	AskVol1          float64
	HasAskVol1       bool
	ExpiryDate       string
	TTE              float64
	HasTTE           bool
	TurnoverChg      float64
	HasTurnoverChg   bool
	TurnoverRatio    float64
	HasTurnoverRatio bool
	OIChg            float64
	HasOIChg         bool
	OIRatio          float64
	HasOIRatio       bool
}

type flowEvent struct {
	Key         string
	TS          int64
	Contract    string
	Symbol      string
	Underlying  string
	CP          string
	Strike      float64
	HasStrike   bool
	Turnover    float64
	HasTurnover bool
	OIChg       float64
	HasOIChg    bool

	TTE      float64
	HasTTE   bool
	Expiry   string
	Tag      string
	WTurn    float64
	WOI      float64
	WTrigger float64
	QData    float64
	QBook    float64
	QBookOK  bool

	DirectionScore float64
	VolScore       float64
	GammaScore     float64
	ThetaScore     float64
	PositionScore  float64
	OrderbookScore float64

	Delta    float64
	Gamma    float64
	Vega     float64
	Theta    float64
	HasDelta bool
	HasGamma bool
	HasVega  bool
	HasTheta bool

	GreeksReady bool

	GDirection float64
	GVol       float64
	GGamma     float64
	GTheta     float64
	GPosition  float64

	QDirection float64
	QVol       float64
	QGamma     float64
	QTheta     float64
	QPosition  float64

	WeightDirection float64
	WeightVol       float64
	WeightGamma     float64
	WeightTheta     float64
	WeightPosition  float64
}

type flowUnderlyingAgg struct {
	Underlying    string
	WindowStartTS int64
	WindowEndTS   int64

	UnderDirection float64
	UnderVol       float64
	UnderGamma     float64
	UnderTheta     float64
	UnderPosition  float64

	DirConfNum   float64
	DirConfDen   float64
	VolConfNum   float64
	VolConfDen   float64
	GammaConfNum float64
	GammaConfDen float64
	ThetaConfNum float64
	ThetaConfDen float64
	PosConfNum   float64
	PosConfDen   float64

	ContractImpact map[string]float64
	PatternHint    string
}

type flowPairCandidate struct {
	A         int
	B         int
	PairType  string
	ComboID   string
	SizeRaw   float64
	TimeGapMS int64
	Balance   float64
	TimeScore float64
	SizeScore float64
	PairScore float64
	EventIDA  string
	EventIDB  string
}

func normalizeExclusiveFlowFilters(onlySelected, onlyFocused bool) (bool, bool) {
	if onlySelected && onlyFocused {
		// Focused filter has higher priority because it is the narrower scope.
		return false, true
	}
	return onlySelected, onlyFocused
}

type flowEmptyReason string

const (
	flowEmptyReasonNone            flowEmptyReason = ""
	flowEmptyReasonNoSelected      flowEmptyReason = "no_selected_contracts"
	flowEmptyReasonNoFocusedSymbol flowEmptyReason = "no_focused_symbol"
)

func flowEmptyMessage(reason flowEmptyReason) string {
	switch reason {
	case flowEmptyReasonNoFocusedSymbol:
		return "Currently no focused symbol"
	case flowEmptyReasonNoSelected:
		return "Currently no contracts are selected"
	default:
		return ""
	}
}

func (ui *UI) resetFlowAggregation() {
	ui.flowEvents = nil
	ui.flowSeen = make(map[string]int64)
	ui.flowPrevByContract = make(map[string]optionFrame)
	ui.flowCurrByContract = make(map[string]optionFrame)
	ui.flowHasResult = false
	if ui.liveFlow != nil {
		ui.liveFlow.SetTitle("Flow Aggregation")
		fillFlowTable(ui.liveFlow, nil)
	}
}

func (ui *UI) ingestFlowEvents(rows []map[string]any) {
	if ui.flowSeen == nil {
		ui.flowSeen = make(map[string]int64)
	}
	if ui.flowPrevByContract == nil {
		ui.flowPrevByContract = make(map[string]optionFrame)
	}
	if ui.flowCurrByContract == nil {
		ui.flowCurrByContract = make(map[string]optionFrame)
	}
	// Keep one stable "previous frame" view for this ingest batch so multiple
	// rows for the same contract (e.g. TURNOVER + OI) compare against the same
	// prior tick instead of mutating per row within the batch.
	batchPrevByContract := make(map[string]optionFrame, len(ui.flowCurrByContract))
	for contract, frame := range ui.flowCurrByContract {
		batchPrevByContract[contract] = frame
	}
	optionRowsByContract := buildOptionRowsByContract(ui.optionsRawRows)
	for _, row := range rows {
		contract := strings.ToLower(strings.TrimSpace(asString(row["ctp_contract"])))
		prevFrame, hasPrev := batchPrevByContract[contract]
		if hasPrev {
			rowTS := flowEventTS(row)
			if ui.isFlowFrameStale(prevFrame, rowTS) {
				hasPrev = false
				prevFrame = optionFrame{}
			}
		}
		if hasPrev {
			ui.flowPrevByContract[contract] = prevFrame
		} else {
			delete(ui.flowPrevByContract, contract)
		}
		event, currFrame, ok := toFlowEventWithContext(row, prevFrame, optionRowsByContract[contract], ui.metadata)
		if !ok {
			continue
		}
		shouldAdvanceCurr := true
		if existingCurr, exists := ui.flowCurrByContract[contract]; exists {
			// Guard against replayed history rows rolling the cache backward.
			shouldAdvanceCurr = currFrame.TS >= existingCurr.TS
		}
		if _, exists := ui.flowSeen[event.Key]; exists {
			if idx := ui.findFlowEventByKey(event.Key); idx >= 0 {
				if !ui.flowEvents[idx].GreeksReady && event.GreeksReady {
					ui.flowEvents[idx] = event
				}
			}
			continue
		}
		if shouldAdvanceCurr {
			ui.flowCurrByContract[contract] = currFrame
		}
		ui.flowSeen[event.Key] = event.TS
		ui.flowEvents = append(ui.flowEvents, event)
	}
	ui.pruneFlowEvents()
}

func (ui *UI) flowWindowMillis() int64 {
	windowMillis := int64(ui.flowWindowSeconds) * 1000
	if windowMillis <= 0 {
		windowMillis = int64(defaultFlowWindowSeconds) * 1000
	}
	return windowMillis
}

func (ui *UI) isFlowFrameStale(frame optionFrame, rowTS int64) bool {
	if frame.TS <= 0 || rowTS <= 0 {
		return false
	}
	return rowTS-frame.TS > ui.flowWindowMillis()
}

func (ui *UI) findFlowEventByKey(key string) int {
	for idx := range ui.flowEvents {
		if ui.flowEvents[idx].Key == key {
			return idx
		}
	}
	return -1
}

func (ui *UI) pruneFlowEvents() {
	if len(ui.flowEvents) == 0 {
		return
	}
	maxTS := int64(0)
	for _, event := range ui.flowEvents {
		if event.TS > maxTS {
			maxTS = event.TS
		}
	}
	if maxTS == 0 {
		return
	}
	windowMillis := ui.flowWindowMillis()
	cutoff := maxTS - windowMillis - 5000
	next := make([]flowEvent, 0, len(ui.flowEvents))
	nextSeen := make(map[string]int64, len(ui.flowEvents))
	for _, event := range ui.flowEvents {
		if event.TS < cutoff {
			continue
		}
		next = append(next, event)
		nextSeen[event.Key] = event.TS
	}
	ui.flowEvents = next
	ui.flowSeen = nextSeen
}

func (ui *UI) renderFlowAggregation() {
	if ui.liveFlow == nil {
		return
	}
	ui.pruneFlowEvents()
	filteredEvents, emptyReason := ui.filterFlowEvents(ui.flowEvents)
	if len(filteredEvents) == 0 {
		ui.flowHasResult = false
		ui.liveFlow.SetTitle("Flow Aggregation")
		fillFlowTable(ui.liveFlow, nil, flowEmptyMessage(emptyReason))
		return
	}
	minTS := filteredEvents[0].TS
	maxTS := filteredEvents[0].TS
	for _, event := range filteredEvents {
		if event.TS < minTS {
			minTS = event.TS
		}
		if event.TS > maxTS {
			maxTS = event.TS
		}
	}
	spanMillis := maxTS - minTS
	minAnalysisMillis := int64(ui.flowMinAnalysisSeconds) * 1000
	if minAnalysisMillis <= 0 {
		minAnalysisMillis = int64(defaultFlowMinAnalysisSeconds) * 1000
	}
	if spanMillis < minAnalysisMillis {
		ui.flowHasResult = false
		ui.liveFlow.SetTitle(fmt.Sprintf(
			"Flow Aggregation (%s ~ %s, collecting)",
			time.UnixMilli(minTS).Format("15:04:05"),
			time.UnixMilli(maxTS).Format("15:04:05"),
		))
		fillFlowTable(ui.liveFlow, nil, flowEmptyMessage(flowEmptyReasonNone))
		return
	}

	eventsByUnderlying := make(map[string][]flowEvent)
	for _, event := range filteredEvents {
		underlying := strings.TrimSpace(event.Underlying)
		if underlying == "" {
			underlying = event.Contract
		}
		key := strings.ToLower(underlying)
		eventsByUnderlying[key] = append(eventsByUnderlying[key], event)
	}
	aggRows := make([]flowUnderlyingAgg, 0, len(eventsByUnderlying))
	for _, events := range eventsByUnderlying {
		if len(events) == 0 {
			continue
		}
		agg := flowUnderlyingAgg{
			Underlying:     events[0].Underlying,
			WindowStartTS:  events[0].TS,
			WindowEndTS:    events[0].TS,
			ContractImpact: make(map[string]float64),
		}
		if strings.TrimSpace(agg.Underlying) == "" {
			agg.Underlying = events[0].Contract
		}
		for _, event := range events {
			if event.TS < agg.WindowStartTS {
				agg.WindowStartTS = event.TS
			}
			if event.TS > agg.WindowEndTS {
				agg.WindowEndTS = event.TS
			}
			if event.WeightDirection == 0 &&
				event.WeightVol == 0 &&
				event.WeightGamma == 0 &&
				event.WeightTheta == 0 &&
				event.WeightPosition == 0 {
				continue
			}
			dirContrib := event.WeightDirection * event.DirectionScore * event.Delta
			volContrib := event.WeightVol * event.VolScore * event.Vega
			gammaContrib := event.WeightGamma * event.GammaScore * event.Gamma
			thetaContrib := event.WeightTheta * event.ThetaScore * event.Theta
			posContrib := event.WeightPosition * event.PositionScore

			agg.UnderDirection += dirContrib
			agg.UnderVol += volContrib
			agg.UnderGamma += gammaContrib
			agg.UnderTheta += thetaContrib
			agg.UnderPosition += posContrib

			dirAbs := math.Abs(dirContrib)
			volAbs := math.Abs(volContrib)
			gammaAbs := math.Abs(gammaContrib)
			thetaAbs := math.Abs(thetaContrib)
			posAbs := math.Abs(posContrib)

			agg.DirConfNum += dirAbs * (event.QDirection * event.GDirection)
			agg.DirConfDen += dirAbs
			agg.VolConfNum += volAbs * (event.QVol * event.GVol)
			agg.VolConfDen += volAbs
			agg.GammaConfNum += gammaAbs * (event.QGamma * event.GGamma)
			agg.GammaConfDen += gammaAbs
			agg.ThetaConfNum += thetaAbs * (event.QTheta * event.GTheta)
			agg.ThetaConfDen += thetaAbs
			agg.PosConfNum += posAbs * (event.QPosition * event.GPosition)
			agg.PosConfDen += posAbs

			impact := dirAbs + volAbs + gammaAbs + thetaAbs + posAbs
			agg.ContractImpact[event.Contract] += impact
		}
		agg.PatternHint = applyPatternOverlay(events, &agg)
		totalImpact := math.Abs(agg.UnderDirection) + math.Abs(agg.UnderVol) + math.Abs(agg.UnderGamma) + math.Abs(agg.UnderTheta) + math.Abs(agg.UnderPosition)
		if totalImpact == 0 {
			continue
		}
		aggRows = append(aggRows, agg)
	}
	if len(aggRows) == 0 {
		ui.flowHasResult = false
		ui.liveFlow.SetTitle(fmt.Sprintf(
			"Flow Aggregation (%s ~ %s, no classifiable events)",
			time.UnixMilli(minTS).Format("15:04:05"),
			time.UnixMilli(maxTS).Format("15:04:05"),
		))
		fillFlowTable(ui.liveFlow, nil, flowEmptyMessage(flowEmptyReasonNone))
		return
	}

	sort.SliceStable(aggRows, func(i, j int) bool {
		left := aggregateConfidence(aggRows[i])
		right := aggregateConfidence(aggRows[j])
		if left == right {
			return strings.ToLower(aggRows[i].Underlying) < strings.ToLower(aggRows[j].Underlying)
		}
		return left > right
	})

	displayRows := make([]FlowRow, 0, len(aggRows))
	for _, agg := range aggRows {
		displayRows = append(displayRows, FlowRow{
			Underlying:  agg.Underlying,
			Direction:   classifyDirectionIntent(agg.UnderDirection),
			Vol:         classifyVolIntent(agg.UnderVol),
			Gamma:       classifyGammaIntent(agg.UnderGamma),
			Theta:       classifyThetaIntent(agg.UnderTheta),
			Position:    classifyPositionIntent(agg.UnderPosition),
			Confidence:  fmt.Sprintf("%.2f", aggregateConfidence(agg)),
			PatternHint: defaultDash(agg.PatternHint),
			TopContracts: defaultDash(strings.Join(
				topContracts(agg.ContractImpact, 2),
				",",
			)),
			TimeWindow: formatFlowWindow(agg.WindowStartTS, agg.WindowEndTS),
		})
	}

	ui.flowHasResult = true
	ui.liveFlow.SetTitle(fmt.Sprintf(
		"Flow Aggregation (%s ~ %s)",
		time.UnixMilli(minTS).Format("15:04:05"),
		time.UnixMilli(maxTS).Format("15:04:05"),
	))
	fillFlowTable(ui.liveFlow, displayRows)
}

func (ui *UI) filterFlowEvents(events []flowEvent) ([]flowEvent, flowEmptyReason) {
	onlySelected, onlyFocused := normalizeExclusiveFlowFilters(ui.flowOnlySelectedContracts, ui.flowOnlyFocusedSymbol)
	ui.flowOnlySelectedContracts = onlySelected
	ui.flowOnlyFocusedSymbol = onlyFocused

	focusSymbol := strings.TrimSpace(ui.currentFocusSymbol())
	focusedContracts := map[string]struct{}{}
	if onlyFocused {
		if normalizeToken(focusSymbol) == "" {
			return nil, flowEmptyReasonNoFocusedSymbol
		}
		focusedContracts = buildFocusedContractsSet(
			focusedContractsSource(ui.lastCurveContracts, ui.marketRows),
			focusSymbol,
			ui.metadata,
		)
		if len(focusedContracts) == 0 {
			return nil, flowEmptyReasonNoFocusedSymbol
		}
	}

	selectedContracts := map[string]struct{}{}
	if onlySelected {
		selectedContracts = buildSelectedContractsSet(ui.marketRows)
		if len(selectedContracts) == 0 {
			return nil, flowEmptyReasonNoSelected
		}
	}

	if !onlySelected && !onlyFocused {
		return events, flowEmptyReasonNone
	}

	filtered := make([]flowEvent, 0, len(events))
	for _, event := range events {
		if onlySelected {
			if !eventMatchesContractSet(event, selectedContracts, ui.metadata) {
				continue
			}
		}
		if onlyFocused && !eventMatchesContractSet(event, focusedContracts, ui.metadata) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered, flowEmptyReasonNone
}

func buildFocusedContractsSet(contracts []string, focus string, resolver contractResolver) map[string]struct{} {
	result := make(map[string]struct{}, len(contracts))
	focusNorm := normalizeToken(focus)
	if focusNorm == "" {
		return result
	}
	focusSymbol := ""
	if symbol, ok := resolveContractSymbol(focus, "", resolver); ok {
		focusSymbol = normalizeToken(symbol)
	}
	for _, contract := range contracts {
		key := normalizeToken(contract)
		if key == "" {
			continue
		}
		if key == focusNorm {
			result[key] = struct{}{}
			continue
		}
		contractSymbol := ""
		if symbol, ok := resolveContractSymbol(contract, "", resolver); ok {
			contractSymbol = normalizeToken(symbol)
		}
		if focusSymbol != "" && contractSymbol == focusSymbol {
			result[key] = struct{}{}
		}
	}
	return result
}

func focusedContractsSource(curveContracts []string, marketRows []MarketRow) []string {
	if len(curveContracts) > 0 {
		return curveContracts
	}
	contracts := make([]string, 0, len(marketRows))
	seen := make(map[string]struct{}, len(marketRows))
	for _, row := range marketRows {
		contract := strings.TrimSpace(row.Symbol)
		if contract == "" {
			continue
		}
		key := normalizeToken(contract)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		contracts = append(contracts, contract)
	}
	return contracts
}

func buildSelectedContractsSet(rows []MarketRow) map[string]struct{} {
	result := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		contract := normalizeToken(row.Symbol)
		if contract == "" {
			continue
		}
		result[contract] = struct{}{}
	}
	return result
}

func eventMatchesContractSet(event flowEvent, contracts map[string]struct{}, resolver contractResolver) bool {
	for _, candidate := range selectedContractCandidates(event, resolver) {
		if _, ok := contracts[normalizeToken(candidate)]; ok {
			return true
		}
	}
	return false
}

func selectedContractCandidates(event flowEvent, resolver contractResolver) []string {
	candidates := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	appendIfPresent := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		key := normalizeToken(trimmed)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, trimmed)
	}
	appendIfPresent(event.Contract)
	if underlying, ok := resolveOptionUnderlying(event.Contract, event.Underlying, resolver); ok {
		appendIfPresent(underlying)
	} else {
		appendIfPresent(event.Underlying)
	}
	return candidates
}

func formatFlowWindow(startTS, endTS int64) string {
	if startTS <= 0 && endTS <= 0 {
		return "-"
	}
	if startTS <= 0 {
		startTS = endTS
	}
	if endTS <= 0 {
		endTS = startTS
	}
	if startTS > endTS {
		startTS, endTS = endTS, startTS
	}
	return fmt.Sprintf(
		"%s ~ %s",
		time.UnixMilli(startTS).Format("15:04:05"),
		time.UnixMilli(endTS).Format("15:04:05"),
	)
}

func aggregateConfidence(agg flowUnderlyingAgg) float64 {
	values := []float64{
		ratioOrZero(agg.DirConfNum, agg.DirConfDen),
		ratioOrZero(agg.VolConfNum, agg.VolConfDen),
		ratioOrZero(agg.GammaConfNum, agg.GammaConfDen),
		ratioOrZero(agg.ThetaConfNum, agg.ThetaConfDen),
		ratioOrZero(agg.PosConfNum, agg.PosConfDen),
	}
	sum := 0.0
	count := 0.0
	for _, v := range values {
		if v <= 0 {
			continue
		}
		sum += v
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / count
}

func ratioOrZero(numerator, denominator float64) float64 {
	if denominator <= 0 {
		return 0
	}
	return numerator / denominator
}

func defaultDash(text string) string {
	if strings.TrimSpace(text) == "" {
		return "-"
	}
	return text
}

func topContracts(contractImpact map[string]float64, limit int) []string {
	if len(contractImpact) == 0 || limit <= 0 {
		return nil
	}
	type kv struct {
		Contract string
		Impact   float64
	}
	items := make([]kv, 0, len(contractImpact))
	for contract, impact := range contractImpact {
		items = append(items, kv{Contract: contract, Impact: impact})
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := math.Abs(items[i].Impact)
		right := math.Abs(items[j].Impact)
		if left == right {
			return strings.ToLower(items[i].Contract) < strings.ToLower(items[j].Contract)
		}
		return left > right
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Contract)
	}
	return out
}

func classifyDirectionIntent(value float64) string {
	if math.Abs(value) < flowNeutralThreshold {
		return "NEUTRAL"
	}
	if value > 0 {
		return "BULL"
	}
	return "BEAR"
}

func classifyVolIntent(value float64) string {
	if math.Abs(value) < flowNeutralThreshold {
		return "NEUTRAL"
	}
	if value > 0 {
		return "LONG_VOL"
	}
	return "SHORT_VOL"
}

func classifyGammaIntent(value float64) string {
	if math.Abs(value) < flowNeutralThreshold {
		return "NEUTRAL"
	}
	if value > 0 {
		return "LONG_GAMMA"
	}
	return "SHORT_GAMMA"
}

func classifyThetaIntent(value float64) string {
	if math.Abs(value) < flowNeutralThreshold {
		return "NEUTRAL"
	}
	if value > 0 {
		return "THETA+"
	}
	return "THETA-"
}

func classifyPositionIntent(value float64) string {
	if math.Abs(value) < flowNeutralThreshold {
		return "MIXED"
	}
	if value > 0 {
		return "BUILD"
	}
	return "REDUCE"
}

func applyPatternOverlay(events []flowEvent, agg *flowUnderlyingAgg) string {
	if len(events) < 2 || agg == nil {
		return ""
	}
	candidates := make([]flowPairCandidate, 0)
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			left := events[i]
			right := events[j]
			if !canPairOverlay(left, right) {
				continue
			}
			timeGap := absInt64(left.TS - right.TS)
			balance := clipFloat(1.0-math.Abs(math.Log((left.WTrigger+1e-9)/(right.WTrigger+1e-9)))/math.Log(2), 0, 1)
			timeScore := clipFloat(1.0-float64(timeGap)/2000.0, 0, 1)
			pairType := "STRANGLE"
			if left.HasDelta && right.HasDelta {
				if math.Abs(math.Abs(left.Delta)-math.Abs(right.Delta)) <= 0.10 {
					pairType = "STRADDLE"
				}
			}
			candidates = append(candidates, flowPairCandidate{
				A:         i,
				B:         j,
				PairType:  pairType,
				ComboID:   overlayComboID(left, right),
				SizeRaw:   math.Min(left.WeightVol, right.WeightVol),
				TimeGapMS: timeGap,
				Balance:   balance,
				TimeScore: timeScore,
				EventIDA:  left.Key,
				EventIDB:  right.Key,
			})
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sizes := make([]float64, 0, len(candidates))
	for _, candidate := range candidates {
		sizes = append(sizes, candidate.SizeRaw)
	}
	sizeRef := percentile95(sizes)
	if sizeRef <= 0 {
		sizeRef = 1
	}
	for idx := range candidates {
		candidates[idx].SizeScore = clipFloat(candidates[idx].SizeRaw/sizeRef, 0, 1)
		candidates[idx].PairScore = candidates[idx].SizeScore * candidates[idx].Balance * candidates[idx].TimeScore
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.PairScore == right.PairScore {
			if left.TimeGapMS == right.TimeGapMS {
				leftPair := left.EventIDA + "|" + left.EventIDB
				rightPair := right.EventIDA + "|" + right.EventIDB
				return leftPair < rightPair
			}
			return left.TimeGapMS < right.TimeGapMS
		}
		return left.PairScore > right.PairScore
	})
	used := make(map[int]struct{})
	straddleCount := 0
	strangleCount := 0
	for _, candidate := range candidates {
		if _, ok := used[candidate.A]; ok {
			continue
		}
		if _, ok := used[candidate.B]; ok {
			continue
		}
		used[candidate.A] = struct{}{}
		used[candidate.B] = struct{}{}
		left := events[candidate.A]
		right := events[candidate.B]
		sign := scoreSign(left.DirectionScore)
		if sign == 0 {
			continue
		}
		comboWeightVol := math.Min(left.WeightVol, right.WeightVol)
		comboWeightGamma := math.Min(left.WeightGamma, right.WeightGamma)
		comboWeightTheta := math.Min(left.WeightTheta, right.WeightTheta)

		comboVega := left.Vega + right.Vega
		comboGammaGreek := left.Gamma + right.Gamma
		comboThetaGreek := left.Theta + right.Theta

		legVol := left.WeightVol*left.VolScore*left.Vega + right.WeightVol*right.VolScore*right.Vega
		legGamma := left.WeightGamma*left.GammaScore*left.Gamma + right.WeightGamma*right.GammaScore*right.Gamma
		legTheta := left.WeightTheta*left.ThetaScore*left.Theta + right.WeightTheta*right.ThetaScore*right.Theta

		comboVol := comboWeightVol * float64(sign) * comboVega
		comboGamma := comboWeightGamma * float64(sign) * comboGammaGreek
		comboTheta := comboWeightTheta * float64(-sign) * comboThetaGreek

		agg.UnderVol += comboVol - legVol
		agg.UnderGamma += comboGamma - legGamma
		agg.UnderTheta += comboTheta - legTheta

		if candidate.PairType == "STRADDLE" {
			straddleCount++
		} else {
			strangleCount++
		}
	}
	parts := make([]string, 0, 2)
	if straddleCount > 0 {
		parts = append(parts, fmt.Sprintf("STRADDLE×%d", straddleCount))
	}
	if strangleCount > 0 {
		parts = append(parts, fmt.Sprintf("STRANGLE×%d", strangleCount))
	}
	return strings.Join(parts, " / ")
}

func canPairOverlay(left, right flowEvent) bool {
	leftCP := strings.ToLower(strings.TrimSpace(left.CP))
	rightCP := strings.ToLower(strings.TrimSpace(right.CP))
	if (leftCP != "c" && leftCP != "p") || (rightCP != "c" && rightCP != "p") {
		return false
	}
	if leftCP == rightCP {
		return false
	}
	if !sameExpiryBucket(left, right) {
		return false
	}
	if absInt64(left.TS-right.TS) > 2000 {
		return false
	}
	if !sameSignStrong(left.DirectionScore, right.DirectionScore) {
		return false
	}
	if left.WTrigger <= 0 || right.WTrigger <= 0 {
		return false
	}
	ratio := left.WTrigger / (right.WTrigger + 1e-9)
	if ratio < 0.5 || ratio > 2.0 {
		return false
	}
	if !left.QBookOK || !right.QBookOK {
		return false
	}
	if !left.GreeksReady || !right.GreeksReady {
		return false
	}
	if left.WeightVol <= 0 || right.WeightVol <= 0 {
		return false
	}
	return true
}

func sameExpiryBucket(left, right flowEvent) bool {
	leftExpiry := strings.TrimSpace(left.Expiry)
	rightExpiry := strings.TrimSpace(right.Expiry)
	if leftExpiry != "" && rightExpiry != "" {
		return strings.EqualFold(leftExpiry, rightExpiry)
	}
	if left.HasTTE && right.HasTTE {
		return math.Abs(left.TTE-right.TTE) <= 1.0
	}
	return false
}

func overlayComboID(left, right flowEvent) string {
	underlying := strings.ToLower(strings.TrimSpace(left.Underlying))
	if underlying == "" {
		underlying = strings.ToLower(strings.TrimSpace(right.Underlying))
	}
	expiryBucket := strings.ToLower(strings.TrimSpace(left.Expiry))
	if expiryBucket == "" {
		expiryBucket = strings.ToLower(strings.TrimSpace(right.Expiry))
	}
	if expiryBucket == "" {
		expiryBucket = overlayTTEBucket(left, right)
	}
	minTS := left.TS
	maxTS := right.TS
	if minTS > maxTS {
		minTS, maxTS = maxTS, minTS
	}
	return strings.Join([]string{
		"combo_v1",
		underlying,
		expiryBucket,
		strconv.FormatInt(minTS, 10),
		strconv.FormatInt(maxTS, 10),
		"C+P",
	}, "|")
}

func overlayTTEBucket(left, right flowEvent) string {
	avg := 0.0
	count := 0.0
	if left.HasTTE {
		avg += left.TTE
		count++
	}
	if right.HasTTE {
		avg += right.TTE
		count++
	}
	if count == 0 {
		return "tte_unknown"
	}
	avg = avg / count
	if avg <= 7 {
		return "tte_short"
	}
	if avg <= 30 {
		return "tte_mid"
	}
	return "tte_long"
}

func sameSignStrong(left, right float64) bool {
	if math.Abs(left) < 0.4 || math.Abs(right) < 0.4 {
		return false
	}
	return scoreSign(left) == scoreSign(right)
}

func scoreSign(value float64) int {
	if value > 0 {
		return 1
	}
	if value < 0 {
		return -1
	}
	return 0
}

func percentile95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(0.95*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func toFlowEvent(row map[string]any) (flowEvent, bool) {
	event, _, ok := toFlowEventWithContext(row, optionFrame{}, nil, nil)
	return event, ok
}

func toFlowEventWithContext(row map[string]any, prevFrame optionFrame, optionRow map[string]any, resolver contractResolver) (flowEvent, optionFrame, bool) {
	contract := strings.TrimSpace(asString(row["ctp_contract"]))
	if contract == "" {
		return flowEvent{}, optionFrame{}, false
	}
	cp := strings.ToLower(strings.TrimSpace(asString(row["cp"])))
	if cp == "" && optionRow != nil {
		cp = strings.ToLower(strings.TrimSpace(asString(optionRow["option_type"])))
	}
	if cp != "c" && cp != "p" {
		if resolvedCP, ok := resolveOptionTypeCP(contract, resolver); ok {
			cp = resolvedCP
		} else {
			cp = inferOptionTypeFromContract(contract)
		}
	}
	tsMillis := flowEventTS(row)

	currFrame := buildOptionFrame(row, optionRow, tsMillis)
	recoverMissingFrameFields(&currFrame, prevFrame)

	dLast, hasDLast := diffWithFlag(currFrame.Last, currFrame.HasLast, prevFrame.Last, prevFrame.HasLast)
	retLast := 0.0
	if hasDLast && prevFrame.HasLast && prevFrame.Last > 0 {
		retLast = dLast / math.Max(prevFrame.Last, 1e-9)
	}
	dTurnover, hasDTurnover := diffWithFallback(currFrame.Turnover, currFrame.HasTurnover, prevFrame.Turnover, prevFrame.HasTurnover, currFrame.TurnoverChg, currFrame.HasTurnoverChg)
	dVolume, hasDVolume := diffWithFlag(currFrame.Volume, currFrame.HasVolume, prevFrame.Volume, prevFrame.HasVolume)
	dOI, hasDOI := diffWithFallback(currFrame.OpenInterest, currFrame.HasOpenInterest, prevFrame.OpenInterest, prevFrame.HasOpenInterest, currFrame.OIChg, currFrame.HasOIChg)

	spreadCurr, hasSpreadCurr := spread(currFrame.Bid1, currFrame.HasBid1, currFrame.Ask1, currFrame.HasAsk1)
	spreadPrev, hasSpreadPrev := spread(prevFrame.Bid1, prevFrame.HasBid1, prevFrame.Ask1, prevFrame.HasAsk1)
	_, _ = depthImbalance(currFrame.BidVol1, currFrame.HasBidVol1, currFrame.AskVol1, currFrame.HasAskVol1)
	_, _ = depthImbalance(prevFrame.BidVol1, prevFrame.HasBidVol1, prevFrame.AskVol1, prevFrame.HasAskVol1)
	midCurr, hasMidCurr := midPrice(currFrame.Bid1, currFrame.HasBid1, currFrame.Ask1, currFrame.HasAsk1)
	midPrev, hasMidPrev := midPrice(prevFrame.Bid1, prevFrame.HasBid1, prevFrame.Ask1, prevFrame.HasAsk1)
	lastVsMidCurr, hasLastVsMidCurr := 0.0, false
	lastVsMidPrev, hasLastVsMidPrev := 0.0, false
	if currFrame.HasLast && hasMidCurr {
		lastVsMidCurr = currFrame.Last - midCurr
		hasLastVsMidCurr = true
	}
	if prevFrame.HasLast && hasMidPrev {
		lastVsMidPrev = prevFrame.Last - midPrev
		hasLastVsMidPrev = true
	}
	dLastVsMid := 0.0
	hasDLastVsMid := false
	if hasLastVsMidCurr && hasLastVsMidPrev {
		dLastVsMid = lastVsMidCurr - lastVsMidPrev
		hasDLastVsMid = true
	}
	bookVWAPCurr, hasBookVWAPCurr := bookVWAP(currFrame)
	bookVWAPPrev, hasBookVWAPPrev := bookVWAP(prevFrame)
	lastVsBookCurr, hasLastVsBookCurr := 0.0, false
	lastVsBookPrev, hasLastVsBookPrev := 0.0, false
	if currFrame.HasLast && hasBookVWAPCurr {
		lastVsBookCurr = currFrame.Last - bookVWAPCurr
		hasLastVsBookCurr = true
	}
	if prevFrame.HasLast && hasBookVWAPPrev {
		lastVsBookPrev = prevFrame.Last - bookVWAPPrev
		hasLastVsBookPrev = true
	}
	dLastVsBook := 0.0
	hasDLastVsBook := false
	if hasLastVsBookCurr && hasLastVsBookPrev {
		dLastVsBook = lastVsBookCurr - lastVsBookPrev
		hasDLastVsBook = true
	}

	obLoc, obLocOK := weightedAvailable([]float64{
		normBy(lastVsMidCurr, math.Max(spreadCurr, 1e-9)),
		normBy(lastVsBookCurr, math.Max(spreadCurr, 1e-9)),
	}, []float64{0.6, 0.4}, []bool{hasLastVsMidCurr && hasSpreadCurr, hasLastVsBookCurr && hasSpreadCurr})
	obChgSpreadOK := hasSpreadCurr && hasSpreadPrev
	obChgSpreadScale := math.Max(math.Abs(spreadCurr)+math.Abs(spreadPrev), 1e-9)
	obChg, obChgOK := weightedAvailable([]float64{
		normBy(dLastVsMid, obChgSpreadScale),
		normBy(dLastVsBook, obChgSpreadScale),
	}, []float64{0.6, 0.4}, []bool{hasDLastVsMid && obChgSpreadOK, hasDLastVsBook && obChgSpreadOK})
	orderbookScore, _ := weightedAvailable([]float64{obLoc, obChg}, []float64{0.6, 0.4}, []bool{obLocOK, obChgOK})
	orderbookScore = clipFloat(orderbookScore, -1, 1)

	rangePos, hasRangePos := rangePosition(currFrame)
	pxMom, _ := weightedAvailable([]float64{
		normBy(retLast, 0.003),
		normBy(rangePos-0.5, 0.25),
	}, []float64{0.7, 0.3}, []bool{hasDLast && prevFrame.HasLast, hasRangePos})

	flow := 0.0
	if hasDTurnover && dTurnover > 0 {
		scale := math.Max(math.Abs(prevFrame.Turnover), 1)
		if !prevFrame.HasTurnover || scale <= 0 {
			scale = math.Max(math.Abs(dTurnover), 1)
		}
		flow = normBy(dTurnover, scale)
	} else if hasDVolume && dVolume != 0 {
		scale := math.Max(math.Abs(prevFrame.Volume), 1)
		if !prevFrame.HasVolume || scale <= 0 {
			scale = math.Max(math.Abs(dVolume), 1)
		}
		flow = normBy(dVolume, scale)
	}
	directionScore := clipFloat(0.55*orderbookScore+0.25*pxMom+0.20*flow, -1, 1)

	positionScore := 0.0
	if hasDOI {
		den := math.Max(0.5*(math.Abs(currFrame.OpenInterest)+math.Abs(prevFrame.OpenInterest)), 1)
		if !currFrame.HasOpenInterest || !prevFrame.HasOpenInterest {
			den = math.Max(math.Abs(dOI), 1)
		}
		positionScore = clipFloat(normBy(dOI, den), -1, 1)
	}

	strike, hasStrike := asOptionalFloat(row["strike"])
	if !hasStrike && optionRow != nil {
		strike, hasStrike = asOptionalFloat(optionRow["strike"])
	}
	delta, hasDelta := optionalFloatFromSources(row, optionRow, "delta")
	gamma, hasGamma := optionalFloatFromSources(row, optionRow, "gamma")
	vega, hasVega := optionalFloatFromSources(row, optionRow, "vega")
	thetaGreek, hasTheta := optionalFloatFromSources(row, optionRow, "theta")

	absDelta := math.Abs(delta)
	otmLike := 0.0
	itmLike := 0.0
	if hasDelta {
		if absDelta <= 0.35 {
			otmLike = 1
		}
		if absDelta >= 0.65 {
			itmLike = 1
		}
	}
	tte := currFrame.TTE
	hasTTE := currFrame.HasTTE
	shortTTE := 0.0
	if hasTTE && tte <= 7 {
		shortTTE = 1
	}
	gammaScore := 0.0
	volScore := 0.0
	if hasDelta {
		gammaScore = clipFloat(directionScore*(0.7*otmLike+0.3*shortTTE), -1, 1)
		volScore = clipFloat(directionScore*(0.6*otmLike+0.2*shortTTE+0.2*(1-itmLike)), -1, 1)
	}
	thetaScore := clipFloat(-directionScore, -1, 1)

	turnoverChg, hasTurnoverChg := asOptionalFloat(row["turnover_chg"])
	turnoverRatio, hasTurnoverRatio := asOptionalFloat(row["turnover_ratio"])
	oiChg, hasOIChg := asOptionalFloat(row["oi_chg"])
	oiRatio, hasOIRatio := asOptionalFloat(row["oi_ratio"])

	priceForOI := 0.0
	if currFrame.HasLast && currFrame.Last > 0 {
		priceForOI = currFrame.Last
	}
	usePrevPriceFallback := true
	if prevFrame.TS > 0 && currFrame.TS > 0 && prevFrame.TS > currFrame.TS {
		usePrevPriceFallback = false
	}
	if priceForOI <= 0 && usePrevPriceFallback && prevFrame.HasLast && prevFrame.Last > 0 {
		priceForOI = prevFrame.Last
	}
	if priceForOI <= 0 && hasSpreadCurr && hasMidCurr && midCurr > 0 {
		priceForOI = midCurr
	}
	if priceForOI <= 0 && hasSpreadCurr && hasBookVWAPCurr && bookVWAPCurr > 0 {
		priceForOI = bookVWAPCurr
	}
	if priceForOI <= 0 && usePrevPriceFallback && hasSpreadPrev && hasMidPrev && midPrev > 0 {
		priceForOI = midPrev
	}
	if priceForOI <= 0 && usePrevPriceFallback && hasSpreadPrev && hasBookVWAPPrev && bookVWAPPrev > 0 {
		priceForOI = bookVWAPPrev
	}
	if priceForOI <= 0 {
		priceForOI = 1
	}
	wTurn := 0.0
	if hasTurnoverChg {
		wTurn = math.Max(turnoverChg, 0)
	}
	wOI := 0.0
	if hasOIChg {
		wOI = math.Max(math.Abs(oiChg)*priceForOI, 0)
	}
	wTrigger := wTurn + wOI
	if wTrigger <= 0 {
		return flowEvent{}, currFrame, false
	}

	qTurn := 0.0
	wTurnEff := 0.0
	if wTurn > 0 {
		if hasTurnoverRatio {
			if turnoverRatio < 0 {
				wTurnEff = 0
				qTurn = 0
			} else {
				wTurnEff = wTurn
				qTurn = clipFloat(turnoverRatio/5.0, 0, 1)
			}
		} else {
			wTurnEff = wTurn
			qTurn = 1
		}
	}
	qOI := 0.0
	wOIEff := 0.0
	if wOI > 0 {
		if hasOIRatio {
			qOI = clipFloat(math.Abs(oiRatio)/5.0, 0, 1)
			wOIEff = wOI
		} else {
			qOI = 1
			wOIEff = wOI
		}
	}
	qTrig := 0.0
	availTrig := false
	if wTurnEff+wOIEff > 0 {
		availTrig = true
		qTrig = (wTurnEff/(wTurnEff+wOIEff+1e-9))*qTurn + (wOIEff/(wTurnEff+wOIEff+1e-9))*qOI
	}

	inconsistent := false
	qData := 1.0
	if hasTurnoverChg && hasDTurnover {
		if scoreSign(turnoverChg) != scoreSign(dTurnover) {
			inconsistent = true
		} else if math.Abs(turnoverChg) > 0 {
			diffRatio := math.Abs(dTurnover-turnoverChg) / math.Max(math.Abs(turnoverChg), 1e-9)
			if diffRatio > 0.5 {
				inconsistent = true
			}
		}
	}
	if hasOIChg && hasDOI {
		if scoreSign(oiChg) != scoreSign(dOI) {
			inconsistent = true
		}
	}
	if inconsistent {
		qData = 0.5
	}

	qBook := 0.0
	qBookOK := false
	bookCurrComplete := hasSpreadCurr && currFrame.HasBidVol1 && currFrame.HasAskVol1 && (currFrame.BidVol1+currFrame.AskVol1) > 0
	bookPrevComplete := hasSpreadPrev && prevFrame.HasBidVol1 && prevFrame.HasAskVol1 && (prevFrame.BidVol1+prevFrame.AskVol1) > 0
	availBook := false
	if bookCurrComplete && bookPrevComplete {
		qBook = 1
		qBookOK = true
		availBook = true
	} else if (currFrame.HasBid1 && currFrame.HasAsk1) || (currFrame.HasBidVol1 && currFrame.HasAskVol1) {
		qBook = 0.5
		availBook = true
	}

	gDirection := boolToFloat(hasDelta)
	gVol := boolToFloat(hasVega && hasDelta)
	gGamma := boolToFloat(hasGamma && hasDelta)
	gTheta := boolToFloat(hasTheta && hasDelta)
	gPosition := 1.0

	qDirection := axisQuality(qTrig, qData, qBook, availTrig, true, availBook, flowQualityCap)
	qVol := axisQuality(qTrig, qData, qBook, availTrig, true, availBook, flowQualityCap)
	qGamma := axisQuality(qTrig, qData, qBook, availTrig, true, availBook, flowQualityCap)
	qTheta := axisQuality(qTrig, qData, qBook, availTrig, true, availBook, flowQualityCap)
	qPosition := axisQuality(qTrig, qData, qBook, availTrig, true, false, flowPositionCap)

	weightDirection := wTrigger * qDirection * gDirection
	weightVol := wTrigger * qVol * gVol
	weightGamma := wTrigger * qGamma * gGamma
	weightTheta := wTrigger * qTheta * gTheta
	weightPosition := wTrigger * qPosition * gPosition
	resolvedSymbol, _ := resolveContractSymbol(contract, firstNonEmpty(strings.TrimSpace(asString(row["symbol"])), strings.TrimSpace(asString(optionValue(optionRow, "symbol")))), resolver)
	resolvedUnderlying, _ := resolveOptionUnderlying(contract, firstNonEmpty(strings.TrimSpace(asString(row["underlying"])), strings.TrimSpace(asString(optionValue(optionRow, "underlying")))), resolver)
	if resolvedUnderlying == "" {
		resolvedUnderlying = contract
	}

	event := flowEvent{
		Key:             flowEventID(row, contract, cp, strike, hasStrike),
		TS:              tsMillis,
		Contract:        contract,
		Symbol:          resolvedSymbol,
		Underlying:      resolvedUnderlying,
		CP:              cp,
		Strike:          strike,
		HasStrike:       hasStrike,
		Turnover:        turnoverChg,
		HasTurnover:     hasTurnoverChg,
		OIChg:           oiChg,
		HasOIChg:        hasOIChg,
		TTE:             tte,
		HasTTE:          hasTTE,
		Expiry:          firstNonEmpty(strings.TrimSpace(asString(row["expiry_date"])), strings.TrimSpace(asString(optionValue(optionRow, "expiry_date")))),
		Tag:             strings.ToUpper(firstNonEmpty(strings.TrimSpace(asString(row["tag"])), "TURNOVER")),
		WTurn:           wTurn,
		WOI:             wOI,
		WTrigger:        wTrigger,
		QData:           qData,
		QBook:           qBook,
		QBookOK:         qBookOK,
		DirectionScore:  directionScore,
		VolScore:        volScore,
		GammaScore:      gammaScore,
		ThetaScore:      thetaScore,
		PositionScore:   positionScore,
		OrderbookScore:  orderbookScore,
		Delta:           delta,
		Gamma:           gamma,
		Vega:            vega,
		Theta:           thetaGreek,
		HasDelta:        hasDelta,
		HasGamma:        hasGamma,
		HasVega:         hasVega,
		HasTheta:        hasTheta,
		GreeksReady:     hasDelta && hasGamma && hasVega && hasTheta,
		GDirection:      gDirection,
		GVol:            gVol,
		GGamma:          gGamma,
		GTheta:          gTheta,
		GPosition:       gPosition,
		QDirection:      qDirection,
		QVol:            qVol,
		QGamma:          qGamma,
		QTheta:          qTheta,
		QPosition:       qPosition,
		WeightDirection: weightDirection,
		WeightVol:       weightVol,
		WeightGamma:     weightGamma,
		WeightTheta:     weightTheta,
		WeightPosition:  weightPosition,
	}
	return event, currFrame, true
}

func optionValue(row map[string]any, key string) any {
	if row == nil {
		return nil
	}
	return row[key]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildOptionRowsByContract(rows []map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		contract := strings.ToLower(strings.TrimSpace(asString(row["ctp_contract"])))
		if contract == "" {
			continue
		}
		if _, exists := result[contract]; exists {
			continue
		}
		result[contract] = row
	}
	return result
}

func buildOptionFrame(row map[string]any, optionRow map[string]any, ts int64) optionFrame {
	frame := optionFrame{TS: ts}
	frame.Last, frame.HasLast = asOptionalFloat(row["price"])
	if !frame.HasLast {
		frame.Last, frame.HasLast = asOptionalFloat(row["last"])
	}
	if !frame.HasLast && optionRow != nil {
		frame.Last, frame.HasLast = asOptionalFloat(optionRow["last"])
	}
	frame.High, frame.HasHigh = asOptionalFloat(row["high"])
	frame.Low, frame.HasLow = asOptionalFloat(row["low"])
	frame.Volume, frame.HasVolume = asOptionalFloat(row["volume"])
	frame.Turnover, frame.HasTurnover = asOptionalFloat(row["turnover"])
	frame.OpenInterest, frame.HasOpenInterest = asOptionalFloat(row["oi"])
	if !frame.HasOpenInterest {
		frame.OpenInterest, frame.HasOpenInterest = asOptionalFloat(row["open_interest"])
	}
	frame.Bid1, frame.HasBid1 = asOptionalFloat(row["bid1"])
	frame.Ask1, frame.HasAsk1 = asOptionalFloat(row["ask1"])
	frame.BidVol1, frame.HasBidVol1 = asOptionalFloat(row["bid_vol1"])
	frame.AskVol1, frame.HasAskVol1 = asOptionalFloat(row["ask_vol1"])
	frame.ExpiryDate = strings.TrimSpace(asString(row["expiry_date"]))
	frame.TTE, frame.HasTTE = asOptionalFloat(row["tte"])
	if !frame.HasTTE && optionRow != nil {
		frame.TTE, frame.HasTTE = asOptionalFloat(optionRow["tte"])
	}
	frame.TurnoverChg, frame.HasTurnoverChg = asOptionalFloat(row["turnover_chg"])
	frame.TurnoverRatio, frame.HasTurnoverRatio = asOptionalFloat(row["turnover_ratio"])
	frame.OIChg, frame.HasOIChg = asOptionalFloat(row["oi_chg"])
	frame.OIRatio, frame.HasOIRatio = asOptionalFloat(row["oi_ratio"])
	return frame
}

func recoverMissingFrameFields(curr *optionFrame, prev optionFrame) {
	if curr == nil {
		return
	}
	if curr.HasTurnoverChg && !curr.HasTurnover && prev.HasTurnover {
		curr.Turnover = prev.Turnover + curr.TurnoverChg
		curr.HasTurnover = true
	}
	if curr.HasOIChg && !curr.HasOpenInterest && prev.HasOpenInterest {
		curr.OpenInterest = prev.OpenInterest + curr.OIChg
		curr.HasOpenInterest = true
	}
	if !curr.HasHigh && curr.HasLast {
		curr.High = curr.Last
		curr.HasHigh = true
	}
	if !curr.HasLow && curr.HasLast {
		curr.Low = curr.Last
		curr.HasLow = true
	}
}

func diffWithFlag(curr float64, currOK bool, prev float64, prevOK bool) (float64, bool) {
	if !currOK || !prevOK {
		return 0, false
	}
	return curr - prev, true
}

func diffWithFallback(curr float64, currOK bool, prev float64, prevOK bool, fallback float64, fallbackOK bool) (float64, bool) {
	if currOK && prevOK {
		return curr - prev, true
	}
	if fallbackOK {
		return fallback, true
	}
	return 0, false
}

func spread(bid float64, hasBid bool, ask float64, hasAsk bool) (float64, bool) {
	if !hasBid || !hasAsk {
		return 0, false
	}
	if ask <= bid {
		return 0, false
	}
	return ask - bid, true
}

func depthImbalance(bidVol float64, hasBidVol bool, askVol float64, hasAskVol bool) (float64, bool) {
	if !hasBidVol || !hasAskVol {
		return 0, false
	}
	return (bidVol - askVol) / (bidVol + askVol + 1e-9), true
}

func midPrice(bid float64, hasBid bool, ask float64, hasAsk bool) (float64, bool) {
	if !hasBid || !hasAsk {
		return 0, false
	}
	return (bid + ask) / 2.0, true
}

func bookVWAP(frame optionFrame) (float64, bool) {
	if !frame.HasBid1 || !frame.HasAsk1 || !frame.HasBidVol1 || !frame.HasAskVol1 {
		return 0, false
	}
	depth := frame.BidVol1 + frame.AskVol1
	if depth <= 0 {
		return 0, false
	}
	return (frame.Bid1*frame.BidVol1 + frame.Ask1*frame.AskVol1) / depth, true
}

func rangePosition(frame optionFrame) (float64, bool) {
	if !frame.HasHigh || !frame.HasLow || !frame.HasLast {
		return 0, false
	}
	if frame.High <= frame.Low {
		return 0.5, true
	}
	return (frame.Last - frame.Low) / math.Max(frame.High-frame.Low, 1e-9), true
}

func weightedAvailable(values []float64, weights []float64, present []bool) (float64, bool) {
	if len(values) != len(weights) || len(values) != len(present) {
		return 0, false
	}
	sum := 0.0
	weightSum := 0.0
	for i := range values {
		if !present[i] {
			continue
		}
		sum += values[i] * weights[i]
		weightSum += weights[i]
	}
	if weightSum <= 0 {
		return 0, false
	}
	return sum / weightSum, true
}

func normBy(value, scale float64) float64 {
	return math.Tanh(value / math.Max(scale, 1e-9))
}

func optionalFloatFromSources(primary map[string]any, secondary map[string]any, key string) (float64, bool) {
	value, ok := asOptionalFloat(primary[key])
	if ok {
		return value, true
	}
	if secondary != nil {
		return asOptionalFloat(secondary[key])
	}
	return 0, false
}

func axisQuality(qTrig, qData, qBook float64, availTrig, availData, availBook bool, capAxis float64) float64 {
	alphaTrig := 0.6
	alphaData := 0.2
	alphaBook := 0.2
	capAxis = clipFloat(capAxis, 0, 1)
	if capAxis <= 0 {
		return 0
	}
	type component struct {
		alpha float64
		value float64
	}
	components := make([]component, 0, 3)
	if availTrig {
		components = append(components, component{alpha: alphaTrig, value: qTrig})
	}
	if availData {
		components = append(components, component{alpha: alphaData, value: qData})
	}
	if availBook {
		components = append(components, component{alpha: alphaBook, value: qBook})
	}
	if len(components) == 0 {
		return 0
	}
	alphaSum := 0.0
	for _, component := range components {
		alphaSum += component.alpha
	}
	if alphaSum <= 0 {
		return 0
	}
	q := 0.0
	for _, component := range components {
		share := component.alpha / alphaSum
		if share > capAxis {
			share = capAxis
		}
		q += share * component.value
	}
	return clipFloat(q, 0, 1)
}

func flowEventID(row map[string]any, contract, cp string, strike float64, hasStrike bool) string {
	return strings.Join([]string{
		"v2",
		normText(contract),
		normText(strings.TrimSpace(strings.ToUpper(asString(row["tag"])))),
		normTS(row),
		normText(cp),
		normNum(strike, hasStrike),
		normAnyNum(row["price"]),
		normAnyNum(row["turnover_chg"]),
		normAnyNum(row["oi_chg"]),
	}, "|")
}

func normText(value string) string {
	text := strings.TrimSpace(strings.ToLower(value))
	if text == "" {
		return "~"
	}
	return text
}

func normNum(value float64, ok bool) string {
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return "~"
	}
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func normAnyNum(value any) string {
	cast, ok := asOptionalFloat(value)
	return normNum(cast, ok)
}

func normTS(row map[string]any) string {
	ts := flowEventTS(row)
	if ts <= 0 {
		return "0"
	}
	return strconv.FormatInt(ts, 10)
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func clipFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func flowEventTS(row map[string]any) int64 {
	if ts, ok := asOptionalFloat(row["ts"]); ok {
		return int64(ts)
	}
	for _, key := range []string{"time", "datetime"} {
		text := strings.TrimSpace(asString(row[key]))
		if text == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed.UnixMilli()
		}
		if parsed, err := time.Parse(time.RFC3339, text); err == nil {
			return parsed.UnixMilli()
		}
		if parsed, err := time.Parse("2006-01-02 15:04:05", text); err == nil {
			return parsed.UnixMilli()
		}
	}
	return time.Now().UnixMilli()
}

func (ui *UI) buildUnderlyingLastMap() map[string]float64 {
	result := make(map[string]float64)
	for _, raw := range ui.marketRawRows {
		contract := strings.ToLower(strings.TrimSpace(asString(raw["ctp_contract"])))
		last, ok := asOptionalFloat(raw["last"])
		if contract == "" || !ok || last <= 0 {
			continue
		}
		result[contract] = last
	}
	return result
}

func inferContractRoot(contract string) string {
	contract = strings.ToUpper(strings.TrimSpace(contract))
	var b strings.Builder
	for _, r := range contract {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	root := strings.TrimSpace(b.String())
	if root == "" {
		return contract
	}
	return root
}

func extractCurveContracts(rows []map[string]any) []string {
	contracts := make([]string, 0, len(rows))
	seen := make(map[string]struct{})
	for _, row := range rows {
		contract := strings.TrimSpace(asString(row["ctp_contract"]))
		if contract == "" {
			continue
		}
		if _, ok := seen[contract]; ok {
			continue
		}
		seen[contract] = struct{}{}
		contracts = append(contracts, contract)
	}
	sort.Slice(contracts, func(i, j int) bool {
		return strings.ToLower(contracts[i]) < strings.ToLower(contracts[j])
	})
	return contracts
}

func (ui *UI) maybeSpeakQuotes(rows []map[string]any) {
	if !ui.voiceEnabled || ui.voiceUnavailable {
		return
	}
	if len(ui.voiceContracts) == 0 {
		return
	}
	now := time.Now()
	if !ui.voiceMutedAt.IsZero() && now.Before(ui.voiceMutedAt.Add(30*time.Second)) {
		return
	}
	if _, _, err := resolveVoiceCommand("check"); err != nil {
		ui.disableVoiceReporting(err)
		return
	}
	for _, row := range rows {
		contract := strings.TrimSpace(asString(row["ctp_contract"]))
		if contract == "" {
			continue
		}
		if _, ok := ui.voiceContracts[contract]; !ok {
			continue
		}
		last, ok := asOptionalFloat(row["forward"])
		if !ok {
			last, ok = asOptionalFloat(row["last"])
		}
		if !ok {
			continue
		}
		prevPrice, hasPrev := ui.voiceLastPrice[contract]
		if hasPrev && prevPrice == last {
			continue
		}
		prevSpoken, hasSpoken := ui.voiceLastSpoken[contract]
		if hasSpoken && now.Sub(prevSpoken) < 30*time.Second {
			continue
		}
		msg := fmt.Sprintf("%s: %s", spellContract(contract), formatFloat(last))
		ui.enqueueVoice(msg)
		ui.voiceLastPrice[contract] = last
		ui.voiceLastSpoken[contract] = now
	}
}

func (ui *UI) startVoiceWorker() {
	if ui.voiceQueue == nil {
		ui.voiceQueue = make(chan string, maxVoiceQueueSize)
	}
	go func() {
		for msg := range ui.voiceQueue {
			if !ui.voicePlaybackEnabled.Load() {
				continue
			}
			if err := ui.speak(msg); err != nil {
				ui.app.QueueUpdateDraw(func() {
					ui.disableVoiceReporting(err)
				})
			}
		}
	}()
}

func (ui *UI) stopVoiceWorker() {
	if ui.voiceQueue != nil {
		close(ui.voiceQueue)
		ui.voiceQueue = nil
	}
}

func (ui *UI) enqueueVoice(message string) {
	if ui.voiceQueue == nil || strings.TrimSpace(message) == "" {
		return
	}
	select {
	case ui.voiceQueue <- message:
		return
	default:
	}
	select {
	case <-ui.voiceQueue:
	default:
	}
	select {
	case ui.voiceQueue <- message:
	default:
	}
}

func (ui *UI) disableVoiceReporting(err error) {
	if ui.voiceUnavailable && !ui.voiceEnabled && len(ui.voiceContracts) == 0 {
		return
	}
	if err != nil {
		ui.appendLiveLogLine("voice disabled: " + err.Error())
	}
	ui.voiceUnavailable = true
	ui.voiceEnabled = false
	ui.voicePlaybackEnabled.Store(false)
	ui.voiceContracts = make(map[string]struct{})
	ui.voiceMutedAt = time.Time{}
	ui.drainVoiceQueue()
}

func (ui *UI) drainVoiceQueue() {
	if ui.voiceQueue == nil {
		return
	}
	for {
		select {
		case _, ok := <-ui.voiceQueue:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func (ui *UI) speak(text string) error {
	cmd, args, err := resolveVoiceCommand(text)
	if err != nil {
		return err
	}
	command := exec.Command(cmd, args...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

func resolveVoiceCommand(text string) (string, []string, error) {
	if text == "" {
		return "", nil, fmt.Errorf("empty speech text")
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("say"); err == nil {
			return "say", []string{text}, nil
		}
		return "", nil, fmt.Errorf("say not available")
	}
	if _, err := exec.LookPath("espeak"); err == nil {
		return "espeak", []string{text}, nil
	}
	if _, err := exec.LookPath("spd-say"); err == nil {
		return "spd-say", []string{text}, nil
	}
	return "", nil, fmt.Errorf("no speech command available")
}

func spellContract(contract string) string {
	parts := make([]string, 0, len(contract))
	for _, r := range strings.ToUpper(contract) {
		if r >= 'A' && r <= 'Z' {
			parts = append(parts, string(r))
			continue
		}
		if r >= '0' && r <= '9' {
			parts = append(parts, string(r))
		}
	}
	return strings.Join(parts, " ")
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
		bidVol, hasBidVol := asOptionalFloat(item["bid_vol1"])
		askVol, hasAskVol := asOptionalFloat(item["ask_vol1"])
		turnover, hasTurnover := asOptionalFloat(item["turnover"])
		oi, hasOI := asOptionalFloat(item["open_interest"])
		preOI, hasPreOI := asOptionalFloat(item["pre_open_interest"])
		chg := "-"
		chgPct := "-"
		if hasLast && hasPre {
			change := last - pre
			chg = formatChange(change)
			if last != 0 {
				chgPct = formatPercent(change / last)
			}
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
			ChgPct:   chgPct,
			BidVol:   formatOptionalFloat(bidVol, hasBidVol),
			Bid:      formatOptionalFloat(bid, hasBid),
			Ask:      formatOptionalFloat(ask, hasAsk),
			AskVol:   formatOptionalFloat(askVol, hasAskVol),
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
		key = "vol"
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
	case "chg_pct":
		return strings.TrimSuffix(row.ChgPct, "%")
	case "bidv", "bid_vol1":
		return row.BidVol
	case "bid", "bid1":
		return row.Bid
	case "ask", "ask1":
		return row.Ask
	case "askv", "ask_vol1":
		return row.AskVol
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
		OIRatioThreshold:       ui.unusualOIRatioThreshold,
	}, nil)
}

func (ui *UI) setUnusualThresholds(chgThreshold, ratioThreshold, oiRatioThreshold float64) error {
	prevChg := ui.unusualChgThreshold
	prevRatio := ui.unusualRatioThreshold
	prevOIRatio := ui.unusualOIRatioThreshold
	ui.unusualChgThreshold = chgThreshold
	ui.unusualRatioThreshold = ratioThreshold
	ui.unusualOIRatioThreshold = oiRatioThreshold
	if err := ui.pushUnusualThresholds(); err != nil {
		ui.unusualChgThreshold = prevChg
		ui.unusualRatioThreshold = prevRatio
		ui.unusualOIRatioThreshold = prevOIRatio
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
			ui.renderFlowAggregation()
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
	ui.renderFlowAggregation()
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
