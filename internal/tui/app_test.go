package tui

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/ipc"
	"github.com/danni2019/starSling/internal/live"
	"github.com/danni2019/starSling/internal/metadata"
	"github.com/danni2019/starSling/internal/router"
)

type testContractResolver struct {
	optionUnderlying map[string]string
	contractSymbol   map[string]string
	optionTypeCP     map[string]string
	inferUnderlying  map[string]string
	inferSymbol      map[string]string
	inferOptionType  map[string]string
}

func (r testContractResolver) ResolveOptionUnderlying(contract string) (string, bool) {
	if r.optionUnderlying == nil {
		return "", false
	}
	value, ok := r.optionUnderlying[normalizeToken(contract)]
	return value, ok
}

func (r testContractResolver) ResolveContractSymbol(contract string) (string, bool) {
	if r.contractSymbol == nil {
		return "", false
	}
	value, ok := r.contractSymbol[normalizeToken(contract)]
	return value, ok
}

func (r testContractResolver) ResolveOptionTypeCP(contract string) (string, bool) {
	if r.optionTypeCP == nil {
		return "", false
	}
	value, ok := r.optionTypeCP[normalizeToken(contract)]
	return value, ok
}

func (r testContractResolver) InferOptionUnderlying(contract string) (string, bool) {
	if r.inferUnderlying == nil {
		return "", false
	}
	value, ok := r.inferUnderlying[normalizeToken(contract)]
	return value, ok
}

func (r testContractResolver) InferContractSymbol(contract string) (string, bool) {
	if r.inferSymbol == nil {
		return "", false
	}
	value, ok := r.inferSymbol[normalizeToken(contract)]
	return value, ok
}

func (r testContractResolver) InferOptionTypeCP(contract string) (string, bool) {
	if r.inferOptionType == nil {
		return "", false
	}
	value, ok := r.inferOptionType[normalizeToken(contract)]
	return value, ok
}

func testClassifiableFlowEvent(key, contract, symbol, underlying string, ts int64) flowEvent {
	return flowEvent{
		Key:             key,
		TS:              ts,
		Contract:        contract,
		Symbol:          symbol,
		Underlying:      underlying,
		CP:              "c",
		Strike:          90.0,
		HasStrike:       true,
		DirectionScore:  0.8,
		VolScore:        0.7,
		GammaScore:      0.6,
		ThetaScore:      -0.8,
		PositionScore:   0.5,
		Delta:           0.5,
		Vega:            0.2,
		Gamma:           0.1,
		Theta:           -0.2,
		WeightDirection: 10,
		WeightVol:       10,
		WeightGamma:     10,
		WeightTheta:     10,
		WeightPosition:  10,
		QDirection:      1,
		QVol:            1,
		QGamma:          1,
		QTheta:          1,
		QPosition:       1,
		GDirection:      1,
		GVol:            1,
		GGamma:          1,
		GTheta:          1,
		GPosition:       1,
	}
}

func TestApplyMarketSnapshotHandlesStaleTransitionWithoutSeqChange(t *testing.T) {
	ui := &UI{
		liveMarket:   tview.NewTable(),
		marketSortBy: "volume",
	}

	ui.applyMarketSnapshot(router.MarketSnapshot{
		Seq: 1,
		Rows: []map[string]any{
			{
				"ctp_contract":   "cu2604",
				"last":           100,
				"pre_settlement": 99,
				"volume":         1000,
				"open_interest":  5000,
			},
		},
	})
	if len(ui.liveLogLines) != 0 {
		t.Fatalf("unexpected log lines after initial snapshot: %v", ui.liveLogLines)
	}

	ui.applyMarketSnapshot(router.MarketSnapshot{Seq: 1, Stale: true})
	if len(ui.liveLogLines) != 1 || !strings.Contains(ui.liveLogLines[0], "market snapshot stale") {
		t.Fatalf("expected stale log line, got: %v", ui.liveLogLines)
	}

	ui.applyMarketSnapshot(router.MarketSnapshot{Seq: 1, Stale: false})
	if len(ui.liveLogLines) != 2 || !strings.Contains(ui.liveLogLines[0], "market snapshot resumed") {
		t.Fatalf("expected resumed log line, got: %v", ui.liveLogLines)
	}
}

func TestClassifyLiveProcessExitReasonDetectsDisconnectTimeoutToken(t *testing.T) {
	err := errors.New("exit status 2: exit_reason=front_disconnected_timeout")
	if got := classifyLiveProcessExitReason(err); got != liveExitReasonDisconnectTimeoutToken {
		t.Fatalf("classifyLiveProcessExitReason() = %q, want %q", got, liveExitReasonDisconnectTimeoutToken)
	}
	if got := classifyLiveProcessExitReason(errors.New("exit status 2: login timeout")); got != "" {
		t.Fatalf("classifyLiveProcessExitReason() = %q, want empty for unrelated errors", got)
	}
}

func TestObserveLiveHeartbeatTracksLatestHeartbeatLog(t *testing.T) {
	ui := &UI{}
	ts := time.Date(2026, 3, 12, 14, 30, 0, 0, time.UTC).UnixMilli()
	ui.observeLiveHeartbeat(router.LogSnapshot{
		Items: []router.LogLine{
			{TS: ts, Level: "DEBUG", Source: "live_md", Message: "heartbeat"},
		},
	})
	if !ui.liveHeartbeatSeen {
		t.Fatalf("expected heartbeat to be observed")
	}
	if got := ui.liveHeartbeatAt.UnixMilli(); got != ts {
		t.Fatalf("heartbeat timestamp mismatch: got %d want %d", got, ts)
	}
}

func TestObserveLiveHeartbeatIgnoresStaleHeartbeatBeforeLiveStart(t *testing.T) {
	startedAt := time.Date(2026, 3, 12, 14, 0, 0, 0, time.UTC)
	ui := &UI{liveStartedAt: startedAt}

	ui.observeLiveHeartbeat(router.LogSnapshot{
		Items: []router.LogLine{
			{TS: startedAt.Add(-time.Minute).UnixMilli(), Level: "DEBUG", Source: "live_md", Message: "heartbeat"},
		},
	})
	if ui.liveHeartbeatSeen {
		t.Fatalf("expected stale pre-start heartbeat to be ignored")
	}
	if !ui.liveHeartbeatAt.IsZero() {
		t.Fatalf("expected no heartbeat timestamp, got %s", ui.liveHeartbeatAt.String())
	}

	freshTS := startedAt.Add(10 * time.Second).UnixMilli()
	ui.observeLiveHeartbeat(router.LogSnapshot{
		Items: []router.LogLine{
			{TS: freshTS, Level: "DEBUG", Source: "live_md", Message: "heartbeat"},
		},
	})
	if !ui.liveHeartbeatSeen {
		t.Fatalf("expected post-start heartbeat to be observed")
	}
	if got := ui.liveHeartbeatAt.UnixMilli(); got != freshTS {
		t.Fatalf("heartbeat timestamp mismatch: got %d want %d", got, freshTS)
	}
}

func TestHandleLiveKeysEscStopsLiveAndReturnsMain(t *testing.T) {
	ui := &UI{
		app:                 tview.NewApplication(),
		pages:               tview.NewPages(),
		menu:                tview.NewList(),
		liveReconnectActive: true,
	}
	ui.pages.AddPage(string(screenMain), tview.NewBox(), true, true)
	ui.pages.AddPage(string(screenLive), tview.NewBox(), true, false)
	ui.setCurrentScreen(screenLive)

	ui.handleLiveKeys(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))

	if ui.currentScreen() != screenMain {
		t.Fatalf("expected ESC to return to main screen, got %s", ui.currentScreen())
	}
	if ui.liveReconnectActive {
		t.Fatalf("expected ESC path to clear reconnect state")
	}
}

func TestDriveLiveReconnectExhaustedFallsBackToMain(t *testing.T) {
	ui := &UI{
		app:                   tview.NewApplication(),
		pages:                 tview.NewPages(),
		menu:                  tview.NewList(),
		liveReconnectActive:   true,
		liveReconnectAttempts: liveReconnectMaxAttempts,
	}
	ui.pages.AddPage(string(screenMain), tview.NewBox(), true, true)
	ui.pages.AddPage(string(screenLive), tview.NewBox(), true, false)
	ui.setCurrentScreen(screenLive)

	ui.driveLiveReconnect(time.Now())

	if ui.currentScreen() != screenMain {
		t.Fatalf("expected exhausted reconnect to fallback to main, got %s", ui.currentScreen())
	}
	if ui.liveReconnectActive {
		t.Fatalf("expected reconnect state to reset after fallback")
	}
}

func TestDriveLiveReconnectPausesOutsideTradingWindow(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	ui := &UI{
		liveReconnectActive:      true,
		liveReconnectAttempts:    0,
		liveReconnectNextAttempt: time.Time{},
		liveTradeTimeLocation:    loc,
		liveTradeSegments: []metadata.TradeSegment{
			{
				Start: 9 * time.Hour,
				End:   10 * time.Hour,
			},
		},
	}
	ui.setCurrentScreen(screenLive)
	now := time.Date(2026, 3, 12, 3, 0, 0, 0, loc)

	ui.driveLiveReconnect(now)

	if !ui.liveReconnectPausedOutsideWindow {
		t.Fatalf("expected reconnect to pause outside trading window")
	}
	if ui.liveReconnectAttempts != 0 {
		t.Fatalf("expected no reconnect attempts outside trading window, got %d", ui.liveReconnectAttempts)
	}
}

func TestHandleLiveProcessExitIgnoresStaleProcessCallback(t *testing.T) {
	currentProc := &live.Process{}
	startedAt := time.Date(2026, 3, 12, 14, 0, 0, 0, time.UTC)
	heartbeatAt := startedAt.Add(10 * time.Second)
	ui := &UI{
		liveProc:          currentProc,
		liveCancel:        func() {},
		liveStartedAt:     startedAt,
		liveHeartbeatSeen: true,
		liveHeartbeatAt:   heartbeatAt,
	}

	ui.handleLiveProcessExit(&live.Process{}, errors.New("stale process exited"))

	if ui.liveProc != currentProc {
		t.Fatalf("expected active live process to be preserved on stale callback")
	}
	if ui.liveCancel == nil {
		t.Fatalf("expected active live cancel func to be preserved on stale callback")
	}
	if !ui.liveStartedAt.Equal(startedAt) {
		t.Fatalf("expected liveStartedAt unchanged, got %s want %s", ui.liveStartedAt, startedAt)
	}
	if !ui.liveHeartbeatSeen {
		t.Fatalf("expected heartbeat seen flag unchanged")
	}
	if !ui.liveHeartbeatAt.Equal(heartbeatAt) {
		t.Fatalf("expected heartbeat timestamp unchanged, got %s want %s", ui.liveHeartbeatAt, heartbeatAt)
	}
}

func TestHandleLiveSnapshotPollErrorSkipsHangFallback(t *testing.T) {
	ui := &UI{
		app:                           tview.NewApplication(),
		pages:                         tview.NewPages(),
		menu:                          tview.NewList(),
		liveProc:                      &live.Process{},
		liveStartedAt:                 time.Now().Add(-2 * time.Second),
		liveProcessHangTimeoutSeconds: 1,
		liveReconnectActive:           true,
	}
	ui.pages.AddPage(string(screenMain), tview.NewBox(), true, true)
	ui.pages.AddPage(string(screenLive), tview.NewBox(), true, false)
	ui.setCurrentScreen(screenLive)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("poll error path should not trigger hang fallback/stop: %v", recovered)
		}
	}()
	ui.handleLiveSnapshotPollError(time.Now(), errors.New("router unavailable"))

	if ui.currentScreen() != screenLive {
		t.Fatalf("expected poll error path to keep live screen active, got %s", ui.currentScreen())
	}
	if ui.liveProc == nil {
		t.Fatalf("expected live process to remain untouched on poll error")
	}
	if ui.liveReconnectActive {
		t.Fatalf("expected reconnect scheduler to run and reset active flag when live proc is running")
	}
	if len(ui.liveLogLines) == 0 || !strings.Contains(ui.liveLogLines[0], "router poll failed:") {
		t.Fatalf("expected router poll failure log, got %v", ui.liveLogLines)
	}
}

func TestApplyMarketSnapshotClearsSeededRowsWhenRouterHasNoData(t *testing.T) {
	ui := &UI{
		liveMarket:   tview.NewTable(),
		marketSortBy: "volume",
	}
	fillMarketTable(ui.liveMarket, []MarketRow{
		{Symbol: "cu2604", Vol: "100"},
		{Symbol: "ag2604", Vol: "80"},
	})
	if got := ui.liveMarket.GetRowCount(); got <= 1 {
		t.Fatalf("expected seeded rows before clear, got row count %d", got)
	}

	ui.applyMarketSnapshot(router.MarketSnapshot{Seq: 0})

	if got := ui.liveMarket.GetRowCount(); got != 1 {
		t.Fatalf("expected table to reset to header-only state, got row count %d", got)
	}
	if len(ui.marketRows) != 0 {
		t.Fatalf("expected cached market rows to be cleared, got %+v", ui.marketRows)
	}
}

func TestHandleLiveKeysResortsMarketWithoutNewSnapshot(t *testing.T) {
	ui := &UI{
		liveMarket:    tview.NewTable(),
		marketSortBy:  "volume",
		marketSortAsc: false,
		marketRows: []MarketRow{
			{Symbol: "ag2604", Vol: "10"},
			{Symbol: "cu2604", Vol: "20"},
		},
	}
	ui.renderMarketRows()
	if got := strings.TrimSpace(ui.liveMarket.GetCell(1, 0).Text); got != "cu2604" {
		t.Fatalf("expected initial desc row to be cu2604, got %q", got)
	}

	ui.handleLiveKeys(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModNone))
	if got := strings.TrimSpace(ui.liveMarket.GetCell(1, 0).Text); got != "ag2604" {
		t.Fatalf("expected asc row to be ag2604 after sort toggle, got %q", got)
	}
}

func TestApplyMarketSnapshotKeepsSelectionBySymbol(t *testing.T) {
	ui := &UI{
		liveMarket:    tview.NewTable(),
		marketSortBy:  "volume",
		marketSortAsc: false,
	}

	ui.applyMarketSnapshot(router.MarketSnapshot{
		Seq: 1,
		Rows: []map[string]any{
			{"ctp_contract": "cu2604", "last": 100.0, "pre_settlement": 99.0, "volume": 1000.0, "open_interest": 10.0},
			{"ctp_contract": "ag2604", "last": 100.0, "pre_settlement": 99.0, "volume": 500.0, "open_interest": 10.0},
		},
	})
	ui.liveMarket.Select(1, 0)

	ui.applyMarketSnapshot(router.MarketSnapshot{
		Seq: 2,
		Rows: []map[string]any{
			{"ctp_contract": "cu2604", "last": 100.0, "pre_settlement": 99.0, "volume": 100.0, "open_interest": 10.0},
			{"ctp_contract": "ag2604", "last": 100.0, "pre_settlement": 99.0, "volume": 2000.0, "open_interest": 10.0},
		},
	})

	row, _ := ui.liveMarket.GetSelection()
	if row != 2 {
		t.Fatalf("expected cu2604 selection to move to row 2 after reorder, got row %d", row)
	}
	if got := ui.selectedMarketSymbol(); got != "cu2604" {
		t.Fatalf("expected selected symbol to remain cu2604, got %q", got)
	}
}

func TestEnsureFocusSymbolReplacesMissingFocusedContract(t *testing.T) {
	ui := &UI{
		liveMarket: tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "ag2604"},
			{Symbol: "cu2604"},
		},
		focusSymbol: "zn2604",
	}
	fillMarketTable(ui.liveMarket, ui.marketRows)
	ui.liveMarket.Select(1, 0)

	ui.ensureFocusSymbol()

	if ui.focusSymbol != "ag2604" {
		t.Fatalf("expected focus symbol to fall back to selected market row, got %q", ui.focusSymbol)
	}
}

func TestEnsureFocusSymbolClearsWhenMarketBecomesEmpty(t *testing.T) {
	ui := &UI{
		liveMarket:   tview.NewTable(),
		focusSymbol:  "cu2604",
		marketRows:   nil,
		marketSortBy: "volume",
	}
	fillMarketTable(ui.liveMarket, nil)

	ui.ensureFocusSymbol()

	if ui.focusSymbol != "" {
		t.Fatalf("expected focus symbol to clear when market rows are empty, got %q", ui.focusSymbol)
	}
}

func TestEnsureFocusSymbolResyncsWhenPending(t *testing.T) {
	ui := &UI{
		liveMarket: tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "cu2604"},
		},
		focusSymbol:      "cu2604",
		focusSyncPending: true,
	}
	fillMarketTable(ui.liveMarket, ui.marketRows)
	ui.liveMarket.Select(1, 0)

	ui.ensureFocusSymbol()

	if ui.focusSyncPending {
		t.Fatalf("expected focus sync pending to clear after resync")
	}
	if ui.focusSymbol != "cu2604" {
		t.Fatalf("expected focus symbol to stay cu2604, got %q", ui.focusSymbol)
	}
}

func TestApplyMarketSnapshotMarksFocusSyncPendingOnSeqReset(t *testing.T) {
	ui := &UI{
		liveMarket:    tview.NewTable(),
		marketSortBy:  "volume",
		focusSymbol:   "cu2604",
		lastMarketSeq: 3,
		marketRows: []MarketRow{
			{Symbol: "cu2604"},
		},
	}
	fillMarketTable(ui.liveMarket, ui.marketRows)

	ui.applyMarketSnapshot(router.MarketSnapshot{Seq: 0})

	if !ui.focusSyncPending {
		t.Fatalf("expected focus sync pending after market seq reset")
	}
}

func TestConvertMarketRowsLeavesChangeUnknownWithoutPreSettlement(t *testing.T) {
	rows := convertMarketRows([]map[string]any{
		{
			"ctp_contract":   "cu2604",
			"last":           100.0,
			"pre_settlement": nil,
		},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Chg != "-" {
		t.Fatalf("expected unknown change marker '-', got %q", rows[0].Chg)
	}
}

func TestConvertMarketRowsRendersMissingQuotesAsUnknown(t *testing.T) {
	rows := convertMarketRows([]map[string]any{
		{
			"ctp_contract":   "cu2604",
			"last":           nil,
			"open":           nil,
			"high":           nil,
			"low":            nil,
			"pre_close":      nil,
			"pre_settlement": nil,
			"bid1":           nil,
			"ask1":           nil,
			"volume":         nil,
			"open_interest":  nil,
			"datetime":       nil,
		},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Last != "-" || rows[0].Open != "-" || rows[0].High != "-" || rows[0].Low != "-" ||
		rows[0].PreClose != "-" || rows[0].PreSettle != "-" ||
		rows[0].Bid != "-" || rows[0].Ask != "-" || rows[0].Vol != "-" || rows[0].OI != "-" {
		t.Fatalf("expected missing fields to render as '-', got %+v", rows[0])
	}
	if rows[0].TS != "-" {
		t.Fatalf("expected missing timestamp to render as '-', got %q", rows[0].TS)
	}
}

func TestConvertMarketRowsMapsOpenHighLowValues(t *testing.T) {
	rows := convertMarketRows([]map[string]any{
		{
			"ctp_contract":   "cu2604",
			"last":           101.0,
			"open":           100.0,
			"high":           110.0,
			"low":            95.0,
			"pre_close":      99.0,
			"pre_settlement": 98.0,
		},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Last != "101" || rows[0].Open != "100" || rows[0].High != "110" || rows[0].Low != "95" ||
		rows[0].PreClose != "99" || rows[0].PreSettle != "98" {
		t.Fatalf("unexpected mapped open/high/low fields: %+v", rows[0])
	}
}

func TestSortMarketRowsPlacesUnknownValuesLast(t *testing.T) {
	rows := []MarketRow{
		{Symbol: "missing", Vol: "-"},
		{Symbol: "zero", Vol: "0"},
		{Symbol: "ten", Vol: "10"},
	}

	sortMarketRows(rows, "volume", false)
	if rows[0].Symbol != "ten" || rows[1].Symbol != "zero" || rows[2].Symbol != "missing" {
		t.Fatalf("unexpected desc sort order: %+v", rows)
	}

	sortMarketRows(rows, "volume", true)
	if rows[0].Symbol != "zero" || rows[1].Symbol != "ten" || rows[2].Symbol != "missing" {
		t.Fatalf("unexpected asc sort order: %+v", rows)
	}
}

func TestSortMarketRawRowsPlacesUnknownValuesLast(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "missing", "volume": nil},
		{"ctp_contract": "zero", "volume": 0.0},
		{"ctp_contract": "ten", "volume": 10.0},
	}

	sortMarketRawRows(rows, "volume", false)
	if got := []string{
		asString(rows[0]["ctp_contract"]),
		asString(rows[1]["ctp_contract"]),
		asString(rows[2]["ctp_contract"]),
	}; got[0] != "ten" || got[1] != "zero" || got[2] != "missing" {
		t.Fatalf("unexpected desc raw sort order: %+v", got)
	}

	sortMarketRawRows(rows, "volume", true)
	if got := []string{
		asString(rows[0]["ctp_contract"]),
		asString(rows[1]["ctp_contract"]),
		asString(rows[2]["ctp_contract"]),
	}; got[0] != "zero" || got[1] != "ten" || got[2] != "missing" {
		t.Fatalf("unexpected asc raw sort order: %+v", got)
	}
}

func TestApplyMarketSnapshotClearsRowsOnEmptySeqUpdate(t *testing.T) {
	ui := &UI{
		liveMarket:   tview.NewTable(),
		marketSortBy: "volume",
	}

	ui.applyMarketSnapshot(router.MarketSnapshot{
		Seq: 1,
		Rows: []map[string]any{
			{"ctp_contract": "cu2604", "last": 100.0, "pre_settlement": 99.0, "volume": 1000.0, "open_interest": 10.0},
			{"ctp_contract": "ag2604", "last": 100.0, "pre_settlement": 99.0, "volume": 500.0, "open_interest": 10.0},
		},
	})
	if got := ui.liveMarket.GetRowCount(); got != 3 {
		t.Fatalf("expected 2 data rows before empty update, got row count %d", got)
	}

	ui.applyMarketSnapshot(router.MarketSnapshot{
		Seq:  2,
		Rows: []map[string]any{},
	})

	if got := ui.liveMarket.GetRowCount(); got != 1 {
		t.Fatalf("expected table to clear on empty update, got row count %d", got)
	}
	if len(ui.marketRows) != 0 {
		t.Fatalf("expected cached market rows to be cleared, got %+v", ui.marketRows)
	}
}

func TestApplyOptionsSnapshotRefreshesRowsOnSameSeq(t *testing.T) {
	ui := &UI{
		liveOpts:    tview.NewTextView(),
		focusSymbol: "cu2604",
	}

	ui.applyOptionsSnapshot(router.OptionsSnapshot{
		Seq: 1,
		Rows: []map[string]any{
			{
				"ctp_contract": "cu2604C72000",
				"option_type":  "c",
				"strike":       72000.0,
			},
		},
	})
	if got := asString(ui.optionsRawRows[0]["ctp_contract"]); got != "cu2604C72000" {
		t.Fatalf("expected initial options row to be cu2604C72000, got %q", got)
	}

	ui.focusSymbol = "ag2604"
	ui.applyOptionsSnapshot(router.OptionsSnapshot{
		Seq: 1,
		Rows: []map[string]any{
			{
				"ctp_contract": "ag2604C30000",
				"option_type":  "c",
				"strike":       30000.0,
			},
		},
	})
	if got := asString(ui.optionsRawRows[0]["ctp_contract"]); got != "ag2604C30000" {
		t.Fatalf("expected options rows to refresh on same seq, got %q", got)
	}
}

func TestApplyUnusualSnapshotResetsFlowAggregationOnSeqRegression(t *testing.T) {
	oldKey := "old-session-event"
	ui := &UI{
		liveTrades:        tview.NewTable(),
		liveFlow:          tview.NewTable(),
		lastUnusualSeq:    5,
		flowWindowSeconds: defaultFlowWindowSeconds,
		flowEvents: []flowEvent{
			{
				Key:      oldKey,
				TS:       1000,
				Contract: "cu2604C72000",
			},
		},
		flowSeen: map[string]int64{
			oldKey: 1000,
		},
	}

	ui.applyUnusualSnapshot(router.UnusualSnapshot{
		Seq:  1,
		Rows: []map[string]any{},
	})

	if len(ui.flowEvents) != 0 {
		t.Fatalf("expected flow events to clear on seq regression, got %+v", ui.flowEvents)
	}
	if len(ui.flowSeen) != 0 {
		t.Fatalf("expected flow seen cache to clear on seq regression, got %+v", ui.flowSeen)
	}
	if ui.lastUnusualSeq != 1 {
		t.Fatalf("expected last unusual seq to update to 1, got %d", ui.lastUnusualSeq)
	}
}

func TestFilterUnusualRowsSupportsSymbolCSVFilters(t *testing.T) {
	rows := []map[string]any{
		{"symbol": "cu", "ctp_contract": "cu2604C72000"},
		{"symbol": "ag", "ctp_contract": "ag2604P5200"},
		{"symbol": "zn", "ctp_contract": "zn2604C23000"},
	}

	filtered := filterUnusualRows(rows, "CU, ag", nil)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 rows after csv filter, got %d", len(filtered))
	}
	if got := strings.TrimSpace(asString(filtered[0]["ctp_contract"])); got != "cu2604C72000" {
		t.Fatalf("unexpected first filtered contract: %q", got)
	}
	if got := strings.TrimSpace(asString(filtered[1]["ctp_contract"])); got != "ag2604P5200" {
		t.Fatalf("unexpected second filtered contract: %q", got)
	}
}

func TestFilterUnusualRowsMatchesByContractRootAndResolverSymbol(t *testing.T) {
	rows := []map[string]any{
		{"symbol": "", "ctp_contract": "sc2605C420"},
		{"symbol": "", "ctp_contract": "px2609P3000"},
		{"symbol": "", "ctp_contract": "zn2604C23000"},
		{"symbol": "mo", "ctp_contract": "mo2604-C-8000"},
	}

	filtered := filterUnusualRows(rows, "sc,px", nil)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 rows matched by contract roots, got %d", len(filtered))
	}
	if got := strings.TrimSpace(asString(filtered[0]["ctp_contract"])); got != "sc2605C420" {
		t.Fatalf("unexpected first contract matched by root: %q", got)
	}
	if got := strings.TrimSpace(asString(filtered[1]["ctp_contract"])); got != "px2609P3000" {
		t.Fatalf("unexpected second contract matched by root: %q", got)
	}

	resolver := testContractResolver{
		contractSymbol: map[string]string{
			"mo2604-c-8000": "IM",
		},
	}
	filtered = filterUnusualRows(rows, "im", resolver)
	if len(filtered) != 1 {
		t.Fatalf("expected resolver symbol mapping to match one row, got %d", len(filtered))
	}
	if got := strings.TrimSpace(asString(filtered[0]["ctp_contract"])); got != "mo2604-C-8000" {
		t.Fatalf("unexpected resolver-matched contract: %q", got)
	}
}

func TestApplyUnusualSnapshotAppliesSymbolFiltersToTradesTable(t *testing.T) {
	ui := &UI{
		liveTrades:          tview.NewTable(),
		useArbMonitor:       true,
		unusualFilterSymbol: "cu,ag",
	}
	rows := []map[string]any{
		{"symbol": "cu", "ctp_contract": "cu2604C72000", "cp": "c"},
		{"symbol": "ag", "ctp_contract": "ag2604P5200", "cp": "p"},
		{"symbol": "zn", "ctp_contract": "zn2604C23000", "cp": "c"},
		{"symbol": "cu", "ctp_contract": "cu2605C72000", "cp": "c"},
	}
	ui.applyUnusualSnapshot(router.UnusualSnapshot{
		Seq:  1,
		Rows: rows,
	})

	if len(ui.unusualRawRows) != 4 {
		t.Fatalf("expected unusual raw rows to cache all 4 items, got %d", len(ui.unusualRawRows))
	}
	if got := ui.liveTrades.GetRowCount(); got != 4 {
		t.Fatalf("expected header + 3 filtered rows, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveTrades.GetCell(1, 1).Text); got != "cu2604C72000" {
		t.Fatalf("unexpected first filtered table contract: %q", got)
	}
	if got := strings.TrimSpace(ui.liveTrades.GetCell(2, 1).Text); got != "ag2604P5200" {
		t.Fatalf("unexpected second filtered table contract: %q", got)
	}
	if got := strings.TrimSpace(ui.liveTrades.GetCell(3, 1).Text); got != "cu2605C72000" {
		t.Fatalf("unexpected third filtered table contract: %q", got)
	}
}

func TestRenderFlowAggregationPrunesEventsAfterWindowShrink(t *testing.T) {
	oldKey := "old-window-event"
	newKey := "new-window-event"
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      300,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowEvents: []flowEvent{
			{Key: oldKey, TS: 100000, Contract: "cu2604C72000"},
			{Key: newKey, TS: 300000, Contract: "cu2604C72000"},
		},
		flowSeen: map[string]int64{
			oldKey: 100000,
			newKey: 300000,
		},
	}

	ui.renderFlowAggregation()
	if len(ui.flowEvents) != 2 {
		t.Fatalf("expected both events within initial 300s window, got %d", len(ui.flowEvents))
	}

	ui.flowWindowSeconds = 60
	ui.renderFlowAggregation()

	if len(ui.flowEvents) != 1 {
		t.Fatalf("expected one event after shrinking to 60s window, got %d", len(ui.flowEvents))
	}
	if ui.flowEvents[0].Key != newKey {
		t.Fatalf("expected newest event to remain after pruning, got key %q", ui.flowEvents[0].Key)
	}
	if len(ui.flowSeen) != 1 {
		t.Fatalf("expected flow seen map to be pruned to one entry, got %d", len(ui.flowSeen))
	}
	if _, ok := ui.flowSeen[newKey]; !ok {
		t.Fatalf("expected flow seen map to retain newest key")
	}
	if _, ok := ui.flowSeen[oldKey]; ok {
		t.Fatalf("expected old key to be removed after pruning")
	}
}

func TestRenderFlowAggregationSkipsUnclassifiableTurnoverInTotals(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowEvents: []flowEvent{
			{
				Key:             "valid",
				TS:              100000,
				Contract:        "cu2604C72000",
				Symbol:          "CU",
				Underlying:      "cu2604",
				CP:              "c",
				Strike:          90.0,
				HasStrike:       true,
				DirectionScore:  0.8,
				VolScore:        0.7,
				GammaScore:      0.6,
				ThetaScore:      -0.8,
				PositionScore:   0.5,
				Delta:           0.5,
				Vega:            0.2,
				Gamma:           0.1,
				Theta:           -0.2,
				WeightDirection: 10,
				WeightVol:       10,
				WeightGamma:     10,
				WeightTheta:     10,
				WeightPosition:  10,
				QDirection:      1,
				QVol:            1,
				QGamma:          1,
				QTheta:          1,
				QPosition:       1,
				GDirection:      1,
				GVol:            1,
				GGamma:          1,
				GTheta:          1,
				GPosition:       1,
			},
			{
				Key:        "invalid-cp",
				TS:         132000,
				Contract:   "cu2604X72000",
				Symbol:     "CU",
				Underlying: "cu2604",
				CP:         "x",
				Strike:     90.0,
				HasStrike:  true,
			},
		},
		flowSeen: map[string]int64{
			"valid":      100000,
			"invalid-cp": 132000,
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got < 2 {
		t.Fatalf("expected at least one aggregated row, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "cu2604" {
		t.Fatalf("expected underlying cu2604, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 1).Text); got != "BULL" {
		t.Fatalf("expected direction BULL, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 2).Text); got != "LONG_VOL" {
		t.Fatalf("expected vol LONG_VOL, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 8).Text); got != "cu2604C72000" {
		t.Fatalf("expected top contract to contain valid contract, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(0, 9).Text); got != "TIME_WINDOW" {
		t.Fatalf("expected TIME_WINDOW header at column 9, got %q", got)
	}
	expectedWindow := time.UnixMilli(100000).Format("15:04:05") + " ~ " + time.UnixMilli(132000).Format("15:04:05")
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 9).Text); got != expectedWindow {
		t.Fatalf("expected flow row time window %q, got %q", expectedWindow, got)
	}
}

func TestRenderFlowAggregationDoesNotCreateRowsForOnlyUnclassifiableEvents(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		marketRawRows: []map[string]any{
			{"ctp_contract": "cu2604", "last": 100.0},
		},
		flowEvents: []flowEvent{
			{
				Key:         "invalid-cp-1",
				TS:          100000,
				Contract:    "cu2604X72000",
				Symbol:      "CU",
				Underlying:  "cu2604",
				CP:          "x",
				Strike:      90.0,
				HasStrike:   true,
				Turnover:    100.0,
				HasTurnover: true,
			},
			{
				Key:         "invalid-cp-2",
				TS:          132000,
				Contract:    "cu2604X72100",
				Symbol:      "CU",
				Underlying:  "cu2604",
				CP:          "x",
				Strike:      91.0,
				HasStrike:   true,
				Turnover:    200.0,
				HasTurnover: true,
			},
		},
		flowSeen: map[string]int64{
			"invalid-cp-1": 100000,
			"invalid-cp-2": 132000,
		},
	}

	ui.renderFlowAggregation()

	if ui.flowHasResult {
		t.Fatalf("expected flowHasResult=false when no events are classifiable")
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "Waiting for unusual events..." {
		t.Fatalf("expected waiting placeholder for unclassifiable-only window, got %q", got)
	}
	if title := ui.liveFlow.GetTitle(); !strings.Contains(title, "no classifiable events") {
		t.Fatalf("expected title to report no classifiable events, got %q", title)
	}
}

func TestNormalizeExclusiveFlowFilters(t *testing.T) {
	tests := []struct {
		name         string
		onlySelected bool
		onlyFocused  bool
		wantSelected bool
		wantFocused  bool
	}{
		{name: "both off", onlySelected: false, onlyFocused: false, wantSelected: false, wantFocused: false},
		{name: "selected only", onlySelected: true, onlyFocused: false, wantSelected: true, wantFocused: false},
		{name: "focused only", onlySelected: false, onlyFocused: true, wantSelected: false, wantFocused: true},
		{name: "both on", onlySelected: true, onlyFocused: true, wantSelected: false, wantFocused: true},
	}
	for _, tc := range tests {
		gotSelected, gotFocused := normalizeExclusiveFlowFilters(tc.onlySelected, tc.onlyFocused)
		if gotSelected != tc.wantSelected || gotFocused != tc.wantFocused {
			t.Fatalf("%s: normalizeExclusiveFlowFilters(%v,%v)=(%v,%v), want (%v,%v)",
				tc.name,
				tc.onlySelected, tc.onlyFocused,
				gotSelected, gotFocused,
				tc.wantSelected, tc.wantFocused,
			)
		}
	}
}

func TestRenderFlowAggregationFiltersBySelectedContracts(t *testing.T) {
	ui := &UI{
		liveFlow:                  tview.NewTable(),
		flowWindowSeconds:         defaultFlowWindowSeconds,
		flowMinAnalysisSeconds:    defaultFlowMinAnalysisSeconds,
		flowOnlySelectedContracts: true,
		marketRows:                []MarketRow{{Symbol: "ag2604C72000"}},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "AG", "ag2604", 100000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "AG", "ag2604", 132000),
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 101000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 133000),
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got != 2 {
		t.Fatalf("expected one aggregated row after selected-contract filtering, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "ag2604" {
		t.Fatalf("expected only ag2604 row after selected-contract filtering, got %q", got)
	}
}

func TestRenderFlowAggregationFiltersBySelectedUnderlyingWhenEventLacksUnderlying(t *testing.T) {
	ui := &UI{
		liveFlow:                  tview.NewTable(),
		flowWindowSeconds:         defaultFlowWindowSeconds,
		flowMinAnalysisSeconds:    defaultFlowMinAnalysisSeconds,
		flowOnlySelectedContracts: true,
		marketRows:                []MarketRow{{Symbol: "cu2604"}},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "", "", 100000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "", "", 132000),
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "", "", 101000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "", "", 133000),
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got != 2 {
		t.Fatalf("expected one aggregated row after selected-contract fallback matching, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "cu2604C72000" {
		t.Fatalf("expected only cu2604 option row after selected-contract fallback matching, got %q", got)
	}
}

func TestRenderFlowAggregationFiltersByFocusedSymbol(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowOnlyFocusedSymbol:  true,
		focusSymbol:            "ag2604",
		lastCurveContracts:     []string{"ag2604", "ag2605"},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "AG", "ag2604", 100000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "AG", "ag2604", 132000),
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 101000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 133000),
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got != 2 {
		t.Fatalf("expected one aggregated row after focused-symbol filtering, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "ag2604" {
		t.Fatalf("expected only ag2604 row after focused-symbol filtering, got %q", got)
	}
}

func TestRenderFlowAggregationFiltersByFocusedUnderlyingWhenEventLacksContext(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowOnlyFocusedSymbol:  true,
		focusSymbol:            "cu2604",
		lastCurveContracts:     []string{"cu2604", "cu2605"},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "", "", 100000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "", "", 132000),
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "", "", 101000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "", "", 133000),
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got != 2 {
		t.Fatalf("expected one aggregated row after focused-symbol fallback matching, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "cu2604C72000" {
		t.Fatalf("expected only cu2604 option row after focused-symbol fallback matching, got %q", got)
	}
}

func TestRenderFlowAggregationShowsFocusedMessageWhenCurveContractsMismatch(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowOnlyFocusedSymbol:  true,
		focusSymbol:            "cu2604",
		lastCurveContracts:     []string{"ag2604", "ag2605"},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "", "", 100000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "", "", 132000),
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "", "", 101000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "", "", 133000),
		},
	}

	ui.renderFlowAggregation()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "Currently no focused symbol" {
		t.Fatalf("expected focused-symbol message when curve contracts mismatch focus, got %q", got)
	}
}

func TestRenderFlowAggregationFocusedFilterFallsBackToMarketContracts(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowOnlyFocusedSymbol:  true,
		focusSymbol:            "cu2604",
		marketRows: []MarketRow{
			{Symbol: "cu2604"},
			{Symbol: "ag2604"},
		},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 100000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 132000),
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "AG", "ag2604", 101000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "AG", "ag2604", 133000),
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got != 2 {
		t.Fatalf("expected one aggregated row after market-contract fallback matching, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "cu2604" {
		t.Fatalf("expected only cu2604 row after market-contract fallback matching, got %q", got)
	}
}

func TestRenderFlowAggregationBothEnabledUsesFocusedFilter(t *testing.T) {
	ui := &UI{
		liveFlow:                  tview.NewTable(),
		flowWindowSeconds:         defaultFlowWindowSeconds,
		flowMinAnalysisSeconds:    defaultFlowMinAnalysisSeconds,
		flowOnlySelectedContracts: true,
		flowOnlyFocusedSymbol:     true,
		focusSymbol:               "ag2604",
		lastCurveContracts:        []string{"ag2604", "ag2605"},
		marketRows:                []MarketRow{{Symbol: "cu2604C72000"}},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "AG", "ag2604", 100000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "AG", "ag2604", 132000),
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 101000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 133000),
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got != 2 {
		t.Fatalf("expected focused filter to win when both enabled, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "ag2604" {
		t.Fatalf("expected focused-only result ag2604 when both enabled, got %q", got)
	}
}

func TestRenderFlowAggregationBothEnabledIgnoresSelectedMismatch(t *testing.T) {
	ui := &UI{
		liveFlow:                  tview.NewTable(),
		flowWindowSeconds:         defaultFlowWindowSeconds,
		flowMinAnalysisSeconds:    defaultFlowMinAnalysisSeconds,
		flowOnlySelectedContracts: true,
		flowOnlyFocusedSymbol:     true,
		focusSymbol:               "lc2605",
		lastCurveContracts:        []string{"lc2603", "lc2604", "lc2605", "lc2606"},
		marketRows: []MarketRow{
			{Symbol: "sc2604"},
		},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("lc2605-a", "lc2605C11000", "LC", "lc2605", 100000),
			testClassifiableFlowEvent("lc2605-b", "lc2605P11000", "LC", "lc2605", 132000),
			testClassifiableFlowEvent("lc2604-a", "lc2604C11000", "LC", "lc2604", 101000),
			testClassifiableFlowEvent("lc2604-b", "lc2604P11000", "LC", "lc2604", 133000),
			testClassifiableFlowEvent("sc2604-a", "sc2604C500", "SC", "sc2604", 102000),
			testClassifiableFlowEvent("sc2604-b", "sc2604P500", "SC", "sc2604", 134000),
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got != 3 {
		t.Fatalf("expected focused filter rows (lc2604/lc2605) when both enabled, got row count %d", got)
	}
	rowA := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text)
	rowB := strings.TrimSpace(ui.liveFlow.GetCell(2, 0).Text)
	if !(rowA == "lc2604" || rowA == "lc2605") || !(rowB == "lc2604" || rowB == "lc2605") || rowA == rowB {
		t.Fatalf("expected focused rows lc2604 and lc2605, got %q and %q", rowA, rowB)
	}
}

func TestRenderFlowAggregationFocusedFilterExcludesForeignSymbolContracts(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowOnlyFocusedSymbol:  true,
		focusSymbol:            "lc2605",
		lastCurveContracts:     []string{"lc2603", "lc2604", "lc2605", "sc2604"},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("lc-a", "lc2605C11000", "LC", "lc2605", 100000),
			testClassifiableFlowEvent("lc-b", "lc2605P11000", "LC", "lc2605", 132000),
			testClassifiableFlowEvent("sc-a", "sc2604C500", "SC", "sc2604", 101000),
			testClassifiableFlowEvent("sc-b", "sc2604P500", "SC", "sc2604", 133000),
		},
	}

	ui.renderFlowAggregation()

	if got := ui.liveFlow.GetRowCount(); got != 2 {
		t.Fatalf("expected only focused-symbol row after filtering foreign symbol contracts, got row count %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "lc2605" {
		t.Fatalf("expected focused-symbol filter to keep lc2605 only, got %q", got)
	}
}

func TestRenderFlowAggregationShowsSelectedContractsEmptyMessage(t *testing.T) {
	ui := &UI{
		liveFlow:                  tview.NewTable(),
		flowOnlySelectedContracts: true,
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 100000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 132000),
		},
	}

	ui.renderFlowAggregation()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "Currently no contracts are selected" {
		t.Fatalf("expected selected-contracts empty message, got %q", got)
	}
}

func TestRenderFlowAggregationShowsFocusedSymbolEmptyMessage(t *testing.T) {
	ui := &UI{
		liveFlow:              tview.NewTable(),
		flowOnlyFocusedSymbol: true,
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 100000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 132000),
		},
	}

	ui.renderFlowAggregation()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "Currently no focused symbol" {
		t.Fatalf("expected focused-symbol empty message, got %q", got)
	}
}

func TestRenderFlowAggregationShowsFocusedSymbolEmptyMessageForStaleFocus(t *testing.T) {
	ui := &UI{
		liveFlow:              tview.NewTable(),
		liveMarket:            tview.NewTable(),
		flowOnlyFocusedSymbol: true,
		focusSymbol:           "ag2604",
		marketRows: []MarketRow{
			{Symbol: "cu2604"},
		},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "AG", "ag2604", 100000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "AG", "ag2604", 132000),
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 101000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 133000),
		},
	}
	fillMarketTable(ui.liveMarket, ui.marketRows)
	ui.liveMarket.Select(1, 0)

	ui.renderFlowAggregation()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "Currently no focused symbol" {
		t.Fatalf("expected stale focused-symbol message, got %q", got)
	}
}

func TestRenderFlowAggregationPrioritizesFocusedSymbolMessageWhenBothMissing(t *testing.T) {
	ui := &UI{
		liveFlow:                  tview.NewTable(),
		flowOnlySelectedContracts: true,
		flowOnlyFocusedSymbol:     true,
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 100000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 132000),
		},
	}

	ui.renderFlowAggregation()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "Currently no focused symbol" {
		t.Fatalf("expected focused-symbol message to have higher priority, got %q", got)
	}
}

func TestRenderFlowAggregationCollectingClearsPreviousRows(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 100000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 132000),
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "AG", "ag2604", 133000),
		},
	}

	ui.renderFlowAggregation()
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got == "Waiting for unusual events..." {
		t.Fatalf("expected initial render to produce aggregated rows, got waiting placeholder")
	}

	ui.flowOnlyFocusedSymbol = true
	ui.focusSymbol = "ag2604"
	ui.lastCurveContracts = []string{"ag2604", "ag2605"}
	ui.renderFlowAggregation()

	if title := ui.liveFlow.GetTitle(); !strings.Contains(title, "collecting") {
		t.Fatalf("expected collecting title after focused filtering to one fresh event, got %q", title)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "Waiting for unusual events..." {
		t.Fatalf("expected collecting placeholder after focused filtering, got %q", got)
	}
}

func TestEnsureFocusSymbolRerendersFlowForFocusedFilter(t *testing.T) {
	ui := &UI{
		liveFlow:               tview.NewTable(),
		liveMarket:             tview.NewTable(),
		flowWindowSeconds:      defaultFlowWindowSeconds,
		flowMinAnalysisSeconds: defaultFlowMinAnalysisSeconds,
		flowOnlyFocusedSymbol:  true,
		focusSymbol:            "cu2604",
		lastCurveContracts:     []string{"ag2604", "ag2605"},
		marketRows: []MarketRow{
			{Symbol: "ag2604"},
		},
		flowEvents: []flowEvent{
			testClassifiableFlowEvent("ag-1", "ag2604C72000", "AG", "ag2604", 100000),
			testClassifiableFlowEvent("ag-2", "ag2604C72000", "AG", "ag2604", 132000),
			testClassifiableFlowEvent("cu-1", "cu2604C72000", "CU", "cu2604", 101000),
			testClassifiableFlowEvent("cu-2", "cu2604C72000", "CU", "cu2604", 133000),
		},
	}
	fillMarketTable(ui.liveMarket, ui.marketRows)
	ui.liveMarket.Select(1, 0)

	ui.ensureFocusSymbol()

	if ui.focusSymbol != "ag2604" {
		t.Fatalf("expected ensureFocusSymbol to switch focus to ag2604, got %q", ui.focusSymbol)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "ag2604" {
		t.Fatalf("expected flow table to rerender with ag2604 focus, got %q", got)
	}
}

func TestRenderOptionsPanelShowsFullChainWithoutTruncation(t *testing.T) {
	rows := make([]map[string]any, 0, 30)
	for i := 0; i < 30; i++ {
		optionType := "c"
		if i%2 == 1 {
			optionType = "p"
		}
		rows = append(rows, map[string]any{
			"ctp_contract": "cu2604C" + strconv.Itoa(70000+i*100),
			"symbol":       "CU",
			"underlying":   "cu2604",
			"option_type":  optionType,
			"strike":       float64(70000 + i*100),
			"iv":           0.2 + float64(i)*0.001,
			"last":         100 + float64(i),
			"delta":        -0.2 + float64(i)*0.01,
			"gamma":        0.01,
			"theta":        -0.03,
			"vega":         0.12,
			"volume":       float64(100 + i),
			"tte":          30.0,
		})
	}
	panel := renderOptionsPanel(nil, rows, "cu2604", optionRenderFilter{})
	if !strings.Contains(panel, "Rows: 30") {
		t.Fatalf("expected full row count in panel, got: %s", panel)
	}
	if !strings.Contains(panel, "STRIKE") || !strings.Contains(panel, "CALL") || !strings.Contains(panel, "PUT") {
		t.Fatalf("expected T-quote style headers in panel, got: %s", panel)
	}
	if strings.Contains(panel, "GAMMA") || strings.Contains(panel, "THETA") || strings.Contains(panel, "VEGA") {
		t.Fatalf("expected GAMMA/THETA/VEGA columns removed, got panel: %s", panel)
	}
	if !strings.Contains(panel, "70000") || !strings.Contains(panel, "72900") {
		t.Fatalf("expected first and last strikes to be present, got panel: %s", panel)
	}
}

func TestInferOptionTypeFromContract(t *testing.T) {
	tests := []struct {
		contract string
		want     string
	}{
		{contract: "CU2604P72000", want: "p"},
		{contract: "CU2604C72000", want: "c"},
		{contract: "cu2604p72000", want: "p"},
		{contract: "MO2604-C-8000", want: "c"},
		{contract: "MO2604-P-8000", want: "p"},
		{contract: "C2406", want: ""},
		{contract: "", want: ""},
	}
	for _, tc := range tests {
		if got := inferOptionTypeFromContract(tc.contract); got != tc.want {
			t.Fatalf("inferOptionTypeFromContract(%q) = %q, want %q", tc.contract, got, tc.want)
		}
	}
}

func TestInferOptionUnderlyingFromContract(t *testing.T) {
	tests := []struct {
		contract string
		want     string
	}{
		{contract: "CU2604P72000", want: "CU2604"},
		{contract: "cu2604c72000", want: "cu2604"},
		{contract: "MO2604-C-8000", want: "MO2604"},
		{contract: "MO2604-P-8000", want: "MO2604"},
		{contract: "C2406", want: ""},
		{contract: "", want: ""},
	}
	for _, tc := range tests {
		if got := inferOptionUnderlyingFromContract(tc.contract); got != tc.want {
			t.Fatalf("inferOptionUnderlyingFromContract(%q) = %q, want %q", tc.contract, got, tc.want)
		}
	}
}

func TestResolveOptionUnderlyingPrefersExistingWhenResolverNil(t *testing.T) {
	got, ok := resolveOptionUnderlying("MO2605-C-1234", "IM2605", nil)
	if !ok || got != "IM2605" {
		t.Fatalf("resolveOptionUnderlying() = (%q,%v), want (IM2605,true)", got, ok)
	}
}

func TestResolveOptionUnderlyingPrefersExistingOverResolverInfer(t *testing.T) {
	resolver := testContractResolver{
		inferUnderlying: map[string]string{
			"mo2605-c-1234": "MO2605",
		},
	}
	got, ok := resolveOptionUnderlying("MO2605-C-1234", "IM2605", resolver)
	if !ok || got != "IM2605" {
		t.Fatalf("resolveOptionUnderlying() = (%q,%v), want (IM2605,true)", got, ok)
	}
}

func TestResolveContractSymbolPrefersExistingWhenResolverNil(t *testing.T) {
	got, ok := resolveContractSymbol("MO2605-C-1234", "IM", nil)
	if !ok || got != "IM" {
		t.Fatalf("resolveContractSymbol() = (%q,%v), want (IM,true)", got, ok)
	}
}

func TestResolveContractSymbolPrefersExistingOverResolverInfer(t *testing.T) {
	resolver := testContractResolver{
		inferSymbol: map[string]string{
			"mo2605-c-1234": "MO",
		},
	}
	got, ok := resolveContractSymbol("MO2605-C-1234", "IM", resolver)
	if !ok || got != "IM" {
		t.Fatalf("resolveContractSymbol() = (%q,%v), want (IM,true)", got, ok)
	}
}

func TestRenderOptionsPanelInfersPutFromContract(t *testing.T) {
	panel := renderOptionsPanel(nil, []map[string]any{
		{
			"ctp_contract": "CU2604P72000",
			"underlying":   "CU2604",
			"option_type":  "",
			"strike":       72000.0,
			"last":         99.0,
			"iv":           0.25,
			"volume":       120.0,
		},
	}, "CU2604", optionRenderFilter{})
	if !strings.Contains(panel, "Put rows: 1") {
		t.Fatalf("expected inferred put row to be counted in put rows, got panel: %s", panel)
	}
}

func TestRenderOptionsPanelRequiresFocus(t *testing.T) {
	panel := renderOptionsPanel(nil, []map[string]any{
		{
			"ctp_contract": "CU2604P72000",
			"underlying":   "CU2604",
			"option_type":  "p",
			"strike":       72000.0,
			"iv":           0.25,
			"volume":       120.0,
		},
	}, "", optionRenderFilter{})
	if !strings.Contains(panel, "Select a contract") {
		t.Fatalf("expected select prompt when focus empty, got: %s", panel)
	}
}

func TestRenderOptionsPanelFiltersToFocusChain(t *testing.T) {
	rows := []map[string]any{
		{
			"ctp_contract": "CU2604P72000",
			"underlying":   "CU2604",
			"symbol":       "CU",
			"option_type":  "p",
			"strike":       72000.0,
			"iv":           0.25,
			"volume":       120.0,
		},
		{
			"ctp_contract": "AL2604P19000",
			"underlying":   "AL2604",
			"symbol":       "AL",
			"option_type":  "p",
			"strike":       19000.0,
			"iv":           0.31,
			"volume":       80.0,
		},
	}
	panel := renderOptionsPanel(nil, rows, "CU2604", optionRenderFilter{})
	if strings.Contains(panel, "19000") {
		t.Fatalf("expected non-focused chain to be excluded, got: %s", panel)
	}
	if !strings.Contains(panel, "72000") {
		t.Fatalf("expected focused chain to remain, got: %s", panel)
	}
}

func TestFilterOptionsRowsWithResolverUsesMetadataUnderlying(t *testing.T) {
	rows := []map[string]any{
		{
			"ctp_contract": "MO2604-C-8000",
			"underlying":   "MO2604",
			"symbol":       "MO",
			"option_type":  "c",
			"strike":       8000.0,
		},
	}
	resolver := testContractResolver{
		optionUnderlying: map[string]string{
			"mo2604-c-8000": "IM2604",
		},
	}
	filtered := filterOptionsRowsWithResolver(rows, "IM2604", resolver)
	if len(filtered) != 1 {
		t.Fatalf("expected metadata-based focus match, got %d rows", len(filtered))
	}
}

func TestFilterOptionsByDeltaStrikeSetKeepsOnlyMatchedStrikes(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "o1", "strike": 100.0, "delta": -0.6},
		{"ctp_contract": "o2", "strike": 110.0, "delta": -0.2},
		{"ctp_contract": "o3", "strike": 110.0, "delta": nil},
		{"ctp_contract": "o4", "strike": 115.0, "delta": nil},
		{"ctp_contract": "o5", "strike": 120.0, "delta": 0.25},
		{"ctp_contract": "o6", "strike": 120.0, "delta": nil},
		{"ctp_contract": "o7", "strike": 130.0, "delta": 0.8},
	}
	filtered, strikeCount, ok := filterOptionsByDeltaStrikeSet(rows, 0.2, 0.25)
	if !ok {
		t.Fatalf("expected strike set match")
	}
	if strikeCount != 2 {
		t.Fatalf("expected strike count 2, got %d", strikeCount)
	}
	if len(filtered) != 4 {
		t.Fatalf("expected 4 rows in matched strikes, got %d", len(filtered))
	}
	gotContracts := map[string]bool{}
	for _, row := range filtered {
		gotContracts[asString(row["ctp_contract"])] = true
	}
	for _, contract := range []string{"o2", "o3", "o5", "o6"} {
		if !gotContracts[contract] {
			t.Fatalf("expected contract %s in filtered rows, got %v", contract, gotContracts)
		}
	}
	if gotContracts["o4"] {
		t.Fatalf("expected non-matched strike 115 row to be excluded, got %v", gotContracts)
	}
}

func TestParsePositiveRangeFallsBackToDefaults(t *testing.T) {
	minThreshold, maxThreshold, valid := parsePositiveRange("bad-value", "0.4", 0.25, 0.5)
	if valid {
		t.Fatalf("expected invalid parse")
	}
	if minThreshold != 0.25 || maxThreshold != 0.5 {
		t.Fatalf("expected fallback range [0.25,0.5], got [%v,%v]", minThreshold, maxThreshold)
	}
	minThreshold, maxThreshold, valid = parsePositiveRange("0.3", "0.45", 0.25, 0.5)
	if !valid {
		t.Fatalf("expected valid parse")
	}
	if minThreshold != 0.3 || maxThreshold != 0.45 {
		t.Fatalf("expected range [0.3,0.45], got [%v,%v]", minThreshold, maxThreshold)
	}
	minThreshold, maxThreshold, valid = parsePositiveRange("0.6", "0.4", 0.25, 0.5)
	if valid {
		t.Fatalf("expected invalid range when min > max")
	}
	if minThreshold != 0.25 || maxThreshold != 0.5 {
		t.Fatalf("expected fallback range [0.25,0.5], got [%v,%v]", minThreshold, maxThreshold)
	}
}

func TestParseUnusualThresholdInputs(t *testing.T) {
	chg, ratio, oiRatio, ok := parseUnusualThresholdInputs("200000", "0.2", "0.15")
	if !ok {
		t.Fatalf("expected parseUnusualThresholdInputs to succeed")
	}
	if chg != 200000 || ratio != 0.2 || oiRatio != 0.15 {
		t.Fatalf("unexpected parsed values: chg=%v ratio=%v oiRatio=%v", chg, ratio, oiRatio)
	}

	if _, _, _, ok := parseUnusualThresholdInputs("bad", "0.2", "0.15"); ok {
		t.Fatalf("expected invalid parse for non-numeric value")
	}
	if _, _, _, ok := parseUnusualThresholdInputs("200000", "0", "0.15"); ok {
		t.Fatalf("expected invalid parse for non-positive ratio")
	}
	if _, _, _, ok := parseUnusualThresholdInputs("NaN", "0.2", "0.15"); ok {
		t.Fatalf("expected invalid parse for NaN input")
	}
}

func TestApplyFlowSettingsFallsBackAndAppliesImmediately(t *testing.T) {
	ui := &UI{
		flowWindowSeconds:      300,
		flowMinAnalysisSeconds: 45,
	}

	valid := ui.applyFlowSettings("bad", "999")
	if valid {
		t.Fatalf("expected applyFlowSettings to report invalid input")
	}
	if ui.flowWindowSeconds != defaultFlowWindowSeconds {
		t.Fatalf("expected fallback window %d, got %d", defaultFlowWindowSeconds, ui.flowWindowSeconds)
	}
	if ui.flowMinAnalysisSeconds != defaultFlowMinAnalysisSeconds {
		t.Fatalf("expected fallback min window %d, got %d", defaultFlowMinAnalysisSeconds, ui.flowMinAnalysisSeconds)
	}

	valid = ui.applyFlowSettings("60", "70")
	if valid {
		t.Fatalf("expected applyFlowSettings to reject min_analysis > window_size")
	}
	if ui.flowWindowSeconds != 60 {
		t.Fatalf("expected window_size to keep valid value 60, got %d", ui.flowWindowSeconds)
	}
	if ui.flowMinAnalysisSeconds != defaultFlowMinAnalysisSeconds {
		t.Fatalf("expected min window fallback %d when min>window, got %d", defaultFlowMinAnalysisSeconds, ui.flowMinAnalysisSeconds)
	}
}

func TestAxisQualityEnforcesCapWithoutRenormalization(t *testing.T) {
	got := axisQuality(1, 0, 0, true, true, false, 0.70)
	if math.Abs(got-0.70) > 1e-9 {
		t.Fatalf("expected trigger contribution capped at 0.70 when book quality missing, got %v", got)
	}

	got = axisQuality(1, 0, 0, true, false, false, 0.70)
	if math.Abs(got-0.70) > 1e-9 {
		t.Fatalf("expected single-source quality capped at 0.70, got %v", got)
	}

	got = axisQuality(1, 0, 0, true, true, true, 0.70)
	if math.Abs(got-0.60) > 1e-9 {
		t.Fatalf("expected uncapped full-data blend to stay at 0.60, got %v", got)
	}
}

func TestRuntimeDebugUIHiddenByDefault(t *testing.T) {
	t.Setenv(internalDebugUIEnv, "")
	ui := &UI{}
	_ = ui.buildLiveScreen()
	if ui.liveLog != nil {
		t.Fatalf("expected runtime log panel to stay hidden by default")
	}
}

func TestFlowEventIDNormalizationStable(t *testing.T) {
	rowA := map[string]any{
		"ts":           float64(1738789200000),
		"tag":          "TURNOVER",
		"price":        1.12345674,
		"turnover_chg": 100.12345674,
		"oi_chg":       -2.12345674,
	}
	rowB := map[string]any{
		"ts":           float64(1738789200000),
		"tag":          "turnover",
		"price":        "1.12345675",
		"turnover_chg": "100.12345675",
		"oi_chg":       "-2.12345675",
	}
	idA := flowEventID(rowA, "CU2604C72000", "c", 72000.12345674, true)
	idB := flowEventID(rowB, "cu2604c72000", "C", 72000.12345675, true)
	if idA != idB {
		t.Fatalf("expected normalized event ids to match, got %q vs %q", idA, idB)
	}
}

func TestApplyPatternOverlayGreedyPairSelectionDeterministic(t *testing.T) {
	events := []flowEvent{
		{
			Key:            "a",
			Contract:       "A",
			Underlying:     "cu2604",
			CP:             "c",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.5,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           1,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
		{
			Key:            "b",
			Contract:       "B",
			Underlying:     "cu2604",
			CP:             "p",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          -0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.1,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           10,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
		{
			Key:            "c",
			Contract:       "C",
			Underlying:     "cu2604",
			CP:             "p",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          -0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.1,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           20,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
	}
	agg := flowUnderlyingAgg{}
	for _, event := range events {
		agg.UnderVol += event.WeightVol * event.VolScore * event.Vega
	}
	hint := applyPatternOverlay(events, &agg)
	if hint != "STRADDLE×1" {
		t.Fatalf("expected one deterministic straddle pair, got %q", hint)
	}
	if math.Abs(agg.UnderVol-13.0) > 1e-9 {
		t.Fatalf("expected greedy tie-break to prefer pair a-b, got underVol=%v", agg.UnderVol)
	}
}

func TestApplyPatternOverlayKeepsDistinctPairsInSameTimeBucket(t *testing.T) {
	events := []flowEvent{
		{
			Key:            "a",
			Contract:       "A",
			Underlying:     "cu2604",
			CP:             "c",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.1,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           1,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
		{
			Key:            "b",
			Contract:       "B",
			Underlying:     "cu2604",
			CP:             "p",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          -0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.1,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           1,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
		{
			Key:            "c",
			Contract:       "C",
			Underlying:     "cu2604",
			CP:             "c",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.1,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           1,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
		{
			Key:            "d",
			Contract:       "D",
			Underlying:     "cu2604",
			CP:             "p",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          -0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.1,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           1,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
	}
	agg := flowUnderlyingAgg{}
	for _, event := range events {
		agg.UnderVol += event.WeightVol * event.VolScore * event.Vega
	}
	hint := applyPatternOverlay(events, &agg)
	if hint != "STRADDLE×2" {
		t.Fatalf("expected two independent straddle pairs in same time bucket, got %q", hint)
	}
	if math.Abs(agg.UnderVol-4.0) > 1e-9 {
		t.Fatalf("expected two pair netting adjustments, got underVol=%v", agg.UnderVol)
	}
}

func TestApplyPatternOverlaySkipsMalformedOptionSide(t *testing.T) {
	events := []flowEvent{
		{
			Key:            "a",
			Contract:       "A",
			Underlying:     "cu2604",
			CP:             "call",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.1,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           1,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
		{
			Key:            "b",
			Contract:       "B",
			Underlying:     "cu2604",
			CP:             "p",
			TS:             1000,
			Expiry:         "2026-03-20",
			HasDelta:       true,
			Delta:          -0.50,
			HasTTE:         true,
			TTE:            30,
			WTrigger:       100,
			QBookOK:        true,
			GreeksReady:    true,
			DirectionScore: 0.8,
			VolScore:       0.1,
			GammaScore:     0.0,
			ThetaScore:     0.0,
			Vega:           1,
			Gamma:          0,
			Theta:          0,
			WeightVol:      1,
			WeightGamma:    1,
			WeightTheta:    1,
		},
	}
	agg := flowUnderlyingAgg{}
	for _, event := range events {
		agg.UnderVol += event.WeightVol * event.VolScore * event.Vega
	}

	hint := applyPatternOverlay(events, &agg)
	if hint != "" {
		t.Fatalf("expected no pattern hint for malformed CP, got %q", hint)
	}
	if math.Abs(agg.UnderVol-0.2) > 1e-9 {
		t.Fatalf("expected no overlay netting with malformed CP, got underVol=%v", agg.UnderVol)
	}
}

func TestIngestFlowEventsUsesStablePrevFrameForSameContractBatch(t *testing.T) {
	const contract = "cu2604c72000"
	ui := &UI{
		flowSeen:           map[string]int64{},
		flowPrevByContract: map[string]optionFrame{},
		flowCurrByContract: map[string]optionFrame{
			contract: {
				TS:              900,
				Last:            100.0,
				HasLast:         true,
				Turnover:        1000000.0,
				HasTurnover:     true,
				OpenInterest:    1000.0,
				HasOpenInterest: true,
			},
		},
	}

	rows := []map[string]any{
		{
			"ts":             float64(1000),
			"ctp_contract":   "cu2604C72000",
			"cp":             "c",
			"symbol":         "CU",
			"underlying":     "cu2604",
			"price":          100.0,
			"turnover":       1100000.0,
			"turnover_chg":   100000.0,
			"turnover_ratio": 0.1,
			"oi":             1100.0,
			"tag":            "TURNOVER",
		},
		{
			"ts":           float64(1000),
			"ctp_contract": "cu2604C72000",
			"cp":           "c",
			"symbol":       "CU",
			"underlying":   "cu2604",
			"price":        100.0,
			"turnover":     1100000.0,
			"oi":           1100.0,
			"oi_chg":       100.0,
			"oi_ratio":     0.1,
			"tag":          "OI",
		},
	}

	ui.ingestFlowEvents(rows)

	if len(ui.flowEvents) != 2 {
		t.Fatalf("expected two flow events from same-contract batch, got %d", len(ui.flowEvents))
	}

	var oiEvent *flowEvent
	for i := range ui.flowEvents {
		if ui.flowEvents[i].Tag == "OI" {
			oiEvent = &ui.flowEvents[i]
			break
		}
	}
	if oiEvent == nil {
		t.Fatalf("expected OI flow event in batch")
	}
	if math.Abs(oiEvent.QData-1.0) > 1e-9 {
		t.Fatalf("expected OI event qData to remain 1.0 when compared to prior tick, got %v", oiEvent.QData)
	}
	if oiEvent.PositionScore <= 0 {
		t.Fatalf("expected positive position score from oi_chg against prior tick, got %v", oiEvent.PositionScore)
	}

	prevFrame, ok := ui.flowPrevByContract[contract]
	if !ok {
		t.Fatalf("expected preserved prior frame for contract %s", contract)
	}
	if !prevFrame.HasOpenInterest || math.Abs(prevFrame.OpenInterest-1000.0) > 1e-9 {
		t.Fatalf("expected prior OI frame to stay at 1000, got %+v", prevFrame)
	}
}

func TestIngestFlowEventsDuplicateHistoryDoesNotRollbackCurrFrame(t *testing.T) {
	const contract = "cu2604c72000"
	oldRow := map[string]any{
		"ts":             float64(1000),
		"ctp_contract":   "cu2604C72000",
		"cp":             "c",
		"symbol":         "CU",
		"underlying":     "cu2604",
		"price":          100.0,
		"turnover":       1000000.0,
		"turnover_chg":   100000.0,
		"turnover_ratio": 0.1,
		"oi":             1000.0,
		"tag":            "TURNOVER",
	}
	oldEvent, ok := toFlowEvent(oldRow)
	if !ok {
		t.Fatalf("expected old row to convert to flow event")
	}
	ui := &UI{
		flowSeen: map[string]int64{
			oldEvent.Key: oldEvent.TS,
		},
		flowPrevByContract: map[string]optionFrame{},
		flowCurrByContract: map[string]optionFrame{
			contract: {
				TS:              2000,
				Last:            120.0,
				HasLast:         true,
				Turnover:        1300000.0,
				HasTurnover:     true,
				OpenInterest:    1300.0,
				HasOpenInterest: true,
			},
		},
		flowEvents: []flowEvent{oldEvent},
	}

	ui.ingestFlowEvents([]map[string]any{oldRow})

	currFrame, exists := ui.flowCurrByContract[contract]
	if !exists {
		t.Fatalf("expected current frame for contract %s", contract)
	}
	if currFrame.TS != 2000 {
		t.Fatalf("expected duplicate history row not to roll back curr frame ts, got %d", currFrame.TS)
	}
	if math.Abs(currFrame.Turnover-1300000.0) > 1e-9 {
		t.Fatalf("expected duplicate history row not to roll back turnover, got %v", currFrame.Turnover)
	}
}

func TestIngestFlowEventsIgnoresStalePrevFrameBeyondWindow(t *testing.T) {
	const contract = "cu2604c72000"
	ui := &UI{
		flowWindowSeconds:  60,
		flowSeen:           map[string]int64{},
		flowPrevByContract: map[string]optionFrame{},
		flowCurrByContract: map[string]optionFrame{},
	}
	ui.flowCurrByContract[contract] = optionFrame{
		TS:          1000,
		Turnover:    1000000.0,
		HasTurnover: true,
	}

	row := map[string]any{
		"ts":             float64(200000),
		"ctp_contract":   "cu2604C72000",
		"cp":             "c",
		"symbol":         "CU",
		"underlying":     "cu2604",
		"price":          100.0,
		"turnover":       1100000.0,
		"turnover_chg":   100000.0,
		"turnover_ratio": 0.1,
		"tag":            "TURNOVER",
	}

	ui.ingestFlowEvents([]map[string]any{row})

	if len(ui.flowEvents) != 1 {
		t.Fatalf("expected one flow event, got %d", len(ui.flowEvents))
	}
	event := ui.flowEvents[0]
	if math.Abs(event.QData-1.0) > 1e-9 {
		t.Fatalf("expected stale prior frame to be ignored, got qData=%v", event.QData)
	}
	if _, ok := ui.flowPrevByContract[contract]; ok {
		t.Fatalf("expected stale prior frame to be dropped from cache")
	}
	currFrame, ok := ui.flowCurrByContract[contract]
	if !ok {
		t.Fatalf("expected current frame for contract %s", contract)
	}
	if currFrame.TS != 200000 {
		t.Fatalf("expected current frame timestamp to advance to new row, got %d", currFrame.TS)
	}
}

func TestSetUnusualThresholdsRollsBackOnRPCFailure(t *testing.T) {
	client := ipc.NewClient("127.0.0.1:1")
	client.Timeout = 50 * time.Millisecond
	ui := &UI{
		rpcClient:               client,
		unusualChgThreshold:     100000,
		unusualRatioThreshold:   0.05,
		unusualOIRatioThreshold: 0.05,
	}

	err := ui.setUnusualThresholds(200000, 0.2, 0.1)
	if err == nil {
		t.Fatalf("expected threshold sync failure")
	}
	if ui.unusualChgThreshold != 100000 {
		t.Fatalf("expected chg threshold rollback to 100000, got %v", ui.unusualChgThreshold)
	}
	if ui.unusualRatioThreshold != 0.05 {
		t.Fatalf("expected ratio threshold rollback to 0.05, got %v", ui.unusualRatioThreshold)
	}
	if ui.unusualOIRatioThreshold != 0.05 {
		t.Fatalf("expected oi ratio threshold rollback to 0.05, got %v", ui.unusualOIRatioThreshold)
	}
}

func TestFilterMarketRowsSupportsCSVAndCaseInsensitive(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "cu2604", "exchange": "SHFE", "product_class": "1", "symbol": "CU"},
		{"ctp_contract": "rb2605", "exchange": "dce", "product_class": "1", "symbol": "rb"},
		{"ctp_contract": "ta2605", "exchange": "CZCE", "product_class": "1", "symbol": "TA"},
	}

	filtered := filterMarketRows(rows, "shfe,DCE", "1", "cu,RB", "")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 rows after csv + case-insensitive filter, got %d", len(filtered))
	}
}

func TestFilterMarketRowsSymbolStrictMatch(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "ta2605", "exchange": "CZCE", "product_class": "1", "symbol": "TA"},
		{"ctp_contract": "t2605", "exchange": "CFFEX", "product_class": "1", "symbol": "T"},
	}
	filtered := filterMarketRows(rows, "", "", "t", "")
	if len(filtered) != 1 {
		t.Fatalf("expected strict symbol match to return one row, got %d", len(filtered))
	}
	if got := asString(filtered[0]["symbol"]); !strings.EqualFold(got, "T") {
		t.Fatalf("expected strict symbol match to keep only T, got %q", got)
	}
}

func TestFilterMarketRowsContractStrictMatchCSV(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "TA2605", "exchange": "CZCE", "product_class": "1", "symbol": "TA"},
		{"ctp_contract": "T2605", "exchange": "CFFEX", "product_class": "1", "symbol": "T"},
		{"ctp_contract": "RB2605", "exchange": "SHFE", "product_class": "1", "symbol": "RB"},
	}
	filtered := filterMarketRows(rows, "", "", "", "t2605,rb2605")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 rows for contract csv filter, got %d", len(filtered))
	}
	for _, row := range filtered {
		contract := strings.ToUpper(asString(row["ctp_contract"]))
		if contract != "T2605" && contract != "RB2605" {
			t.Fatalf("unexpected contract in result: %s", contract)
		}
	}
}

func TestFilterMainContractsBySymbolSelectsMaxOI(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "sc2603", "symbol": "SC", "open_interest": 1000.0},
		{"ctp_contract": "sc2604", "symbol": "SC", "open_interest": 2200.0},
		{"ctp_contract": "sc2605", "symbol": "SC", "open_interest": 1800.0},
		{"ctp_contract": "lc2605", "symbol": "LC", "open_interest": 3000.0},
		{"ctp_contract": "lc2606", "symbol": "LC", "open_interest": 2600.0},
	}

	filtered := filterMainContractsBySymbol(rows)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 rows (one per symbol), got %d", len(filtered))
	}
	contracts := map[string]bool{}
	for _, row := range filtered {
		contracts[strings.ToLower(asString(row["ctp_contract"]))] = true
	}
	if !contracts["sc2604"] || !contracts["lc2605"] {
		t.Fatalf("expected sc2604 and lc2605, got %v", contracts)
	}
}

func TestFilterMainContractsBySymbolUsesContractTieBreak(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "sc2605", "symbol": "SC", "open_interest": 1200.0},
		{"ctp_contract": "sc2604", "symbol": "SC", "open_interest": 1200.0},
	}
	filtered := filterMainContractsBySymbol(rows)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 row, got %d", len(filtered))
	}
	if got := strings.ToLower(asString(filtered[0]["ctp_contract"])); got != "sc2604" {
		t.Fatalf("expected tie-break contract sc2604, got %s", got)
	}
}

func TestFilterMainContractsBySymbolTreatsNonFiniteOIAsMissing(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "sc2603", "symbol": "SC", "open_interest": math.NaN()},
		{"ctp_contract": "sc2604", "symbol": "SC", "open_interest": 1200.0},
		{"ctp_contract": "lc2605", "symbol": "LC", "open_interest": math.Inf(1)},
		{"ctp_contract": "lc2604", "symbol": "LC", "open_interest": 900.0},
	}
	filtered := filterMainContractsBySymbol(rows)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(filtered))
	}
	contracts := map[string]bool{}
	for _, row := range filtered {
		contracts[strings.ToLower(asString(row["ctp_contract"]))] = true
	}
	if !contracts["sc2604"] || !contracts["lc2604"] {
		t.Fatalf("expected finite OI contracts sc2604 and lc2604, got %v", contracts)
	}
}

func TestRenderMarketRowsAppliesMainOnlyAfterOtherFilters(t *testing.T) {
	ui := &UI{
		liveMarket:     tview.NewTable(),
		marketSortBy:   "contract",
		marketSortAsc:  true,
		filterSymbol:   "SC,LC",
		filterMainOnly: true,
		marketRawRows: []map[string]any{
			{"ctp_contract": "sc2603", "symbol": "SC", "open_interest": 1000.0, "last": 10.0},
			{"ctp_contract": "sc2604", "symbol": "SC", "open_interest": 2200.0, "last": 11.0},
			{"ctp_contract": "lc2605", "symbol": "LC", "open_interest": 3000.0, "last": 12.0},
			{"ctp_contract": "cu2604", "symbol": "CU", "open_interest": 9999.0, "last": 13.0},
		},
	}

	ui.renderMarketRows()
	if len(ui.marketRows) != 2 {
		t.Fatalf("expected 2 rendered rows after symbol+main filter, got %d", len(ui.marketRows))
	}
	contracts := []string{
		strings.ToLower(ui.marketRows[0].Symbol),
		strings.ToLower(ui.marketRows[1].Symbol),
	}
	joined := strings.Join(contracts, ",")
	if !strings.Contains(joined, "sc2604") || !strings.Contains(joined, "lc2605") {
		t.Fatalf("expected rendered rows contain sc2604 and lc2605, got %v", contracts)
	}
}

func TestResetMarketFiltersResetsMainOnly(t *testing.T) {
	ui := &UI{
		filterExchange: "SHFE",
		filterClass:    "1",
		filterSymbol:   "SC",
		filterContract: "sc2604",
		filterMainOnly: true,
		marketSortBy:   "last",
		marketSortAsc:  true,
	}

	ui.resetMarketFilters()

	if ui.filterExchange != "" || ui.filterClass != "" || ui.filterSymbol != "" || ui.filterContract != "" {
		t.Fatalf("expected all text filters reset, got exchange=%q class=%q symbol=%q contract=%q",
			ui.filterExchange, ui.filterClass, ui.filterSymbol, ui.filterContract)
	}
	if ui.filterMainOnly {
		t.Fatalf("expected filterMainOnly reset to false")
	}
	if ui.marketSortBy != "vol" || ui.marketSortAsc {
		t.Fatalf("expected market sort reset to vol/desc, got sortBy=%q asc=%v", ui.marketSortBy, ui.marketSortAsc)
	}
}

func TestMarketNumericColumns(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "cu2604", "last": 104550.0, "volume": 55848, "symbol": "CU"},
		{"ctp_contract": "ag2604", "last": 31490.0, "volume": "1200", "exchange": "SHFE"},
	}
	columns := marketNumericColumns(rows)
	joined := strings.Join(columns, ",")
	if !strings.Contains(joined, "last") {
		t.Fatalf("expected numeric column set to include last, got %v", columns)
	}
	if !strings.Contains(joined, "volume") {
		t.Fatalf("expected numeric column set to include volume, got %v", columns)
	}
	if !strings.Contains(joined, "symbol") {
		t.Fatalf("expected sortable column set to include symbol, got %v", columns)
	}
}

func TestRenderCurvePanel(t *testing.T) {
	panel := renderCurvePanel([]map[string]any{
		{"ctp_contract": "cu2604", "forward": 104500.0, "volume": 55848.0, "open_interest": 82000.0, "vix": 0.22, "call_skew": 0.03, "put_skew": -0.02},
		{"ctp_contract": "ag2604", "forward": 31490.0, "volume": 9200.0, "open_interest": 75000.0, "vix": 0.30, "call_skew": 0.01, "put_skew": -0.01},
	})
	if !strings.Contains(panel, "Contracts: 2") {
		t.Fatalf("expected contract count in curve panel, got: %s", panel)
	}
	if !strings.Contains(panel, "cu2604") || !strings.Contains(panel, "ag2604") {
		t.Fatalf("expected contracts in curve panel, got: %s", panel)
	}
	if !strings.Contains(panel, "VOL") || !strings.Contains(panel, "OI") {
		t.Fatalf("expected VOL and OI columns in curve panel, got: %s", panel)
	}
	if !strings.Contains(panel, "CALL_SKW") || !strings.Contains(panel, "PUT_SKW") {
		t.Fatalf("expected CALL_SKW and PUT_SKW columns in curve panel, got: %s", panel)
	}
}

func TestConvertUnusualTrades(t *testing.T) {
	rows := []map[string]any{
		{
			"time":           "2026-02-05T21:01:00+08:00",
			"ctp_contract":   "cu2604C72000",
			"cp":             "c",
			"strike":         72000.0,
			"tte":            35.0,
			"price":          85.0,
			"turnover_chg":   120000.0,
			"turnover_ratio": 0.1,
		},
	}
	trades := convertUnusualTrades(rows)
	if len(trades) != 1 {
		t.Fatalf("expected 1 converted trade, got %d", len(trades))
	}
	if trades[0].Sym != "cu2604C72000" {
		t.Fatalf("unexpected contract: %s", trades[0].Sym)
	}
	if trades[0].Time != "21:01:00" {
		t.Fatalf("expected normalized time 21:01:00, got: %s", trades[0].Time)
	}
	if trades[0].TTE != "35" {
		t.Fatalf("expected tte to be rendered, got: %s", trades[0].TTE)
	}
	if !strings.Contains(trades[0].IV, "%") {
		t.Fatalf("expected ratio text in IV column, got: %s", trades[0].IV)
	}
}

func TestConvertUnusualTradesKeepsMissingNumbersUnknown(t *testing.T) {
	rows := []map[string]any{
		{
			"ts":           float64(1738789200000),
			"ctp_contract": "cu2604P70000",
			"cp":           "p",
			"strike":       nil,
			"price":        nil,
			"turnover_chg": nil,
		},
	}
	trades := convertUnusualTrades(rows)
	if len(trades) != 1 {
		t.Fatalf("expected 1 converted trade, got %d", len(trades))
	}
	if trades[0].Strike != "-" {
		t.Fatalf("expected missing strike to render as '-', got %q", trades[0].Strike)
	}
	if trades[0].Price != "-" {
		t.Fatalf("expected missing price to render as '-', got %q", trades[0].Price)
	}
	if trades[0].Size != "-" {
		t.Fatalf("expected missing turnover_chg to render as '-', got %q", trades[0].Size)
	}
	if trades[0].TTE != "-" {
		t.Fatalf("expected missing tte to render as '-', got %q", trades[0].TTE)
	}
}

func TestConvertUnusualTradesUsesOIRowFieldsWhenTagIsOI(t *testing.T) {
	rows := []map[string]any{
		{
			"time":         "2026-02-05T21:01:00+08:00",
			"ctp_contract": "cu2604C72000",
			"cp":           "c",
			"strike":       72000.0,
			"tte":          35.0,
			"price":        85.0,
			"oi_chg":       -1200.0,
			"oi_ratio":     -0.08,
			"tag":          "OI",
		},
	}
	trades := convertUnusualTrades(rows)
	if len(trades) != 1 {
		t.Fatalf("expected 1 converted trade, got %d", len(trades))
	}
	if trades[0].Tag != "OI" {
		t.Fatalf("expected OI tag, got: %s", trades[0].Tag)
	}
	if trades[0].Size != "-1200" {
		t.Fatalf("expected OI CHG to be used as size, got: %s", trades[0].Size)
	}
	if trades[0].IV != "-8.0%" {
		t.Fatalf("expected OI ratio to be used, got: %s", trades[0].IV)
	}
}

func TestToFlowEventUsesTurnoverChange(t *testing.T) {
	row := map[string]any{
		"ts":           float64(1738789200000),
		"ctp_contract": "cu2604C72000",
		"cp":           "c",
		"strike":       72000.0,
		"turnover":     980000.0,
		"turnover_chg": 120000.0,
		"tag":          "TURNOVER",
	}

	event, ok := toFlowEvent(row)
	if !ok {
		t.Fatalf("expected row to convert into flow event")
	}
	if !event.HasTurnover {
		t.Fatalf("expected turnover_chg to be used as flow turnover")
	}
	if event.Turnover != 120000.0 {
		t.Fatalf("expected flow turnover 120000, got %v", event.Turnover)
	}
}

func TestToFlowEventIgnoresCumulativeTurnoverForOITag(t *testing.T) {
	row := map[string]any{
		"ts":           float64(1738789200000),
		"ctp_contract": "cu2604C72000",
		"cp":           "c",
		"strike":       72000.0,
		"price":        100.0,
		"turnover":     980000.0,
		"oi_chg":       -1200.0,
		"tag":          "OI",
	}

	event, ok := toFlowEvent(row)
	if !ok {
		t.Fatalf("expected row to convert into flow event")
	}
	if event.HasTurnover {
		t.Fatalf("expected OI-only event without turnover_chg to skip turnover aggregation, got %v", event.Turnover)
	}
}

func TestToFlowEventKeepsOIEventWhenPriceIsZero(t *testing.T) {
	row := map[string]any{
		"ts":           float64(1738789200000),
		"ctp_contract": "cu2604C72000",
		"cp":           "c",
		"price":        0.0,
		"oi_chg":       -1200.0,
		"tag":          "OI",
	}

	event, ok := toFlowEvent(row)
	if !ok {
		t.Fatalf("expected OI event with zero price to be kept")
	}
	if event.WOI <= 0 || event.WTrigger <= 0 {
		t.Fatalf("expected positive OI trigger weights, got wOI=%v wTrigger=%v", event.WOI, event.WTrigger)
	}
}

func TestToFlowEventWithContextPrefersMetadataMappings(t *testing.T) {
	row := map[string]any{
		"ts":           float64(1738789200000),
		"ctp_contract": "MO2604-C-8000",
		"cp":           "",
		"price":        120.0,
		"turnover_chg": 1000.0,
		"tag":          "TURNOVER",
	}
	resolver := testContractResolver{
		optionUnderlying: map[string]string{
			"mo2604-c-8000": "IM2604",
		},
		contractSymbol: map[string]string{
			"mo2604-c-8000": "MO",
		},
		optionTypeCP: map[string]string{
			"mo2604-c-8000": "c",
		},
	}

	event, _, ok := toFlowEventWithContext(row, optionFrame{}, nil, resolver)
	if !ok {
		t.Fatalf("expected row to convert into flow event")
	}
	if event.Underlying != "IM2604" {
		t.Fatalf("expected metadata underlying IM2604, got %q", event.Underlying)
	}
	if event.Symbol != "MO" {
		t.Fatalf("expected metadata symbol MO, got %q", event.Symbol)
	}
	if event.CP != "c" {
		t.Fatalf("expected metadata cp c, got %q", event.CP)
	}
}

func TestToFlowEventWithContextOIFallbackSkipsNewerPrevPrice(t *testing.T) {
	prevFrame := optionFrame{
		TS:      2000,
		Last:    100.0,
		HasLast: true,
	}
	row := map[string]any{
		"ts":           float64(1000),
		"ctp_contract": "cu2604C72000",
		"cp":           "c",
		"price":        0.0,
		"oi_chg":       -1200.0,
		"tag":          "OI",
	}

	event, _, ok := toFlowEventWithContext(row, prevFrame, nil, nil)
	if !ok {
		t.Fatalf("expected replayed OI row to convert into flow event")
	}
	if math.Abs(event.WOI-1200.0) > 1e-9 {
		t.Fatalf("expected replayed row not to borrow newer price, got wOI=%v", event.WOI)
	}
}

func TestToFlowEventWithContextOIFallbackIgnoresCrossedBook(t *testing.T) {
	row := map[string]any{
		"ts":           float64(1738789200000),
		"ctp_contract": "cu2604C72000",
		"cp":           "c",
		"price":        0.0,
		"oi_chg":       -100.0,
		"bid1":         101.0,
		"ask1":         100.0,
		"bid_vol1":     10.0,
		"ask_vol1":     10.0,
		"tag":          "OI",
	}

	event, _, ok := toFlowEventWithContext(row, optionFrame{}, nil, nil)
	if !ok {
		t.Fatalf("expected crossed-book OI row to convert into flow event")
	}
	if math.Abs(event.WOI-100.0) > 1e-9 {
		t.Fatalf("expected crossed-book fallback to use safe unit price, got wOI=%v", event.WOI)
	}
}

func TestToFlowEventWithContextCrossedBooksDoNotDriveOrderbookChange(t *testing.T) {
	prevFrame := optionFrame{
		TS:         1000,
		Last:       100.0,
		HasLast:    true,
		Bid1:       101.0,
		HasBid1:    true,
		Ask1:       100.0,
		HasAsk1:    true,
		BidVol1:    10.0,
		HasBidVol1: true,
		AskVol1:    10.0,
		HasAskVol1: true,
	}
	row := map[string]any{
		"ts":           float64(2000),
		"ctp_contract": "cu2604C72000",
		"cp":           "c",
		"price":        105.0,
		"turnover_chg": 100.0,
		"bid1":         103.0,
		"ask1":         102.0,
		"bid_vol1":     10.0,
		"ask_vol1":     10.0,
		"tag":          "TURNOVER",
	}

	event, _, ok := toFlowEventWithContext(row, prevFrame, nil, nil)
	if !ok {
		t.Fatalf("expected crossed-book turnover row to convert into flow event")
	}
	if math.Abs(event.OrderbookScore) > 1e-9 {
		t.Fatalf("expected crossed-book change terms to be ignored, got orderbookScore=%v", event.OrderbookScore)
	}
}

func TestSpreadRejectsLockedOrCrossedQuotes(t *testing.T) {
	if _, ok := spread(100, true, 100, true); ok {
		t.Fatalf("expected locked quotes to be unavailable")
	}
	if _, ok := spread(101, true, 100, true); ok {
		t.Fatalf("expected crossed quotes to be unavailable")
	}
	if got, ok := spread(100, true, 100.5, true); !ok || math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("expected positive spread to remain available, got spread=%v ok=%v", got, ok)
	}
}

func TestBookVWAPReturnsUnavailableOnZeroDepth(t *testing.T) {
	frame := optionFrame{
		Bid1:       100,
		HasBid1:    true,
		Ask1:       101,
		HasAsk1:    true,
		BidVol1:    0,
		HasBidVol1: true,
		AskVol1:    0,
		HasAskVol1: true,
	}
	if _, ok := bookVWAP(frame); ok {
		t.Fatalf("expected zero-depth book VWAP to be unavailable")
	}
	frame.BidVol1 = 3
	frame.AskVol1 = 1
	if got, ok := bookVWAP(frame); !ok || math.Abs(got-100.25) > 1e-9 {
		t.Fatalf("expected valid VWAP for positive depth, got vwap=%v ok=%v", got, ok)
	}
}

func TestToFlowEventWithContextDowngradesZeroDepthBookQuality(t *testing.T) {
	prevFrame := optionFrame{
		Bid1:       99,
		HasBid1:    true,
		Ask1:       101,
		HasAsk1:    true,
		BidVol1:    0,
		HasBidVol1: true,
		AskVol1:    0,
		HasAskVol1: true,
	}
	row := map[string]any{
		"ts":           float64(1738789200000),
		"ctp_contract": "cu2604C72000",
		"cp":           "c",
		"price":        100.0,
		"turnover_chg": 120000.0,
		"bid1":         99.0,
		"ask1":         101.0,
		"bid_vol1":     0.0,
		"ask_vol1":     0.0,
		"tag":          "TURNOVER",
	}

	event, _, ok := toFlowEventWithContext(row, prevFrame, nil, nil)
	if !ok {
		t.Fatalf("expected row to convert into flow event")
	}
	if event.QBookOK {
		t.Fatalf("expected zero-depth book to be downgraded from full quality")
	}
	if math.Abs(event.QBook-1) < 1e-9 {
		t.Fatalf("expected zero-depth book quality to be less than full quality, got %v", event.QBook)
	}
}

func TestToFlowEventWithContextDowngradesCrossedBookQuality(t *testing.T) {
	prevFrame := optionFrame{
		TS:         1000,
		Bid1:       99,
		HasBid1:    true,
		Ask1:       101,
		HasAsk1:    true,
		BidVol1:    10,
		HasBidVol1: true,
		AskVol1:    12,
		HasAskVol1: true,
	}
	row := map[string]any{
		"ts":           float64(2000),
		"ctp_contract": "cu2604C72000",
		"cp":           "c",
		"price":        100.0,
		"turnover_chg": 120000.0,
		"bid1":         101.0,
		"ask1":         100.0,
		"bid_vol1":     10.0,
		"ask_vol1":     10.0,
		"tag":          "TURNOVER",
	}

	event, _, ok := toFlowEventWithContext(row, prevFrame, nil, nil)
	if !ok {
		t.Fatalf("expected row to convert into flow event")
	}
	if event.QBookOK {
		t.Fatalf("expected crossed book to be downgraded from full quality")
	}
	if math.Abs(event.QBook-0.5) > 1e-9 {
		t.Fatalf("expected crossed book quality to be partial (0.5), got %v", event.QBook)
	}
}

func TestDisableVoiceReportingDisablesPlaybackAndDrainsQueue(t *testing.T) {
	ui := &UI{
		voiceEnabled:   true,
		voiceContracts: map[string]struct{}{"cu2604": {}},
		voiceQueue:     make(chan string, 3),
	}
	ui.voicePlaybackEnabled.Store(true)
	ui.voiceQueue <- "m1"
	ui.voiceQueue <- "m2"

	ui.disableVoiceReporting(nil)

	if ui.voiceEnabled {
		t.Fatalf("expected voice to be disabled")
	}
	if ui.voicePlaybackEnabled.Load() {
		t.Fatalf("expected playback flag to be disabled")
	}
	if len(ui.voiceContracts) != 0 {
		t.Fatalf("expected voice contracts to be cleared, got %+v", ui.voiceContracts)
	}
	if got := len(ui.voiceQueue); got != 0 {
		t.Fatalf("expected voice queue to be drained, got size %d", got)
	}
}

func TestDrainVoiceQueueReturnsOnClosedChannel(t *testing.T) {
	ui := &UI{
		voiceQueue: make(chan string, 1),
	}
	close(ui.voiceQueue)

	done := make(chan struct{})
	go func() {
		ui.drainVoiceQueue()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected drainVoiceQueue to return for closed channel")
	}
}

func TestFillTradesTableIncludesTTEColumn(t *testing.T) {
	table := tview.NewTable()
	fillTradesTable(table, []TradeRow{
		{
			Time:   "21:01:00",
			Sym:    "cu2604C72000",
			CP:     "C",
			Strike: "72000",
			TTE:    "35",
			Price:  "85",
			Size:   "120000",
			IV:     "10.0%",
			Tag:    "TURNOVER",
		},
	})
	header := strings.TrimSpace(table.GetCell(0, 4).Text)
	if header != "TTE" {
		t.Fatalf("expected TTE header at column 4, got %q", header)
	}
	value := strings.TrimSpace(table.GetCell(1, 4).Text)
	if value != "35" {
		t.Fatalf("expected TTE cell value 35, got %q", value)
	}
}

func TestApplyRouterLogsClearsWhenSeqResets(t *testing.T) {
	ui := &UI{
		liveLog: tview.NewTextView(),
	}
	ui.applyRouterLogs(router.LogSnapshot{
		Seq: 1,
		Items: []router.LogLine{
			{Message: "worker started"},
		},
	})
	if len(ui.liveLogLines) != 1 {
		t.Fatalf("expected one runtime log line, got %d", len(ui.liveLogLines))
	}

	ui.applyRouterLogs(router.LogSnapshot{Seq: 0})
	if ui.lastLogsSeq != 0 {
		t.Fatalf("expected lastLogsSeq reset to 0, got %d", ui.lastLogsSeq)
	}
	if len(ui.liveLogLines) != 0 {
		t.Fatalf("expected runtime logs to clear on seq reset, got %v", ui.liveLogLines)
	}
	if got := ui.liveLog.GetText(true); got != "Waiting for runtime logs..." {
		t.Fatalf("expected waiting text after seq reset, got %q", got)
	}
}

func TestApplyRouterLogsKeepsLocalLinesWhenSeqStaysZero(t *testing.T) {
	ui := &UI{
		liveLog: tview.NewTextView(),
	}
	ui.appendLiveLogLine("local startup error")
	if len(ui.liveLogLines) != 1 {
		t.Fatalf("expected one local log line, got %d", len(ui.liveLogLines))
	}

	ui.applyRouterLogs(router.LogSnapshot{Seq: 0})
	if len(ui.liveLogLines) != 1 {
		t.Fatalf("expected local log lines to be preserved while seq stays zero, got %d", len(ui.liveLogLines))
	}
	if got := ui.liveLog.GetText(true); !strings.Contains(got, "local startup error") {
		t.Fatalf("expected local runtime log to remain visible, got %q", got)
	}
}

func TestApplyRouterLogsClearsBufferWhenSeqRegresses(t *testing.T) {
	ui := &UI{
		liveLog: tview.NewTextView(),
	}
	ui.applyRouterLogs(router.LogSnapshot{
		Seq: 3,
		Items: []router.LogLine{
			{Message: "old-1"},
			{Message: "old-2"},
			{Message: "old-3"},
		},
	})
	if len(ui.liveLogLines) != 3 {
		t.Fatalf("expected three log lines, got %d", len(ui.liveLogLines))
	}

	ui.applyRouterLogs(router.LogSnapshot{
		Seq: 1,
		Items: []router.LogLine{
			{Message: "new-session"},
		},
	})
	if ui.lastLogsSeq != 1 {
		t.Fatalf("expected seq to advance from restart snapshot, got %d", ui.lastLogsSeq)
	}
	if len(ui.liveLogLines) != 1 {
		t.Fatalf("expected buffer to reset before appending new session logs, got %d lines", len(ui.liveLogLines))
	}
	text := ui.liveLog.GetText(true)
	if strings.Contains(text, "old-1") || strings.Contains(text, "old-2") || strings.Contains(text, "old-3") {
		t.Fatalf("expected old-session logs to be dropped after seq regression, got %q", text)
	}
	if !strings.Contains(text, "new-session") {
		t.Fatalf("expected new-session log to be present, got %q", text)
	}
}

func TestApplyRouterLogsUsesEventTimestamp(t *testing.T) {
	ui := &UI{
		liveLog: tview.NewTextView(),
	}
	eventTS := time.Date(2026, 2, 5, 9, 30, 15, 0, time.Local).UnixMilli()
	ui.applyRouterLogs(router.LogSnapshot{
		Seq: 1,
		Items: []router.LogLine{
			{
				TS:      eventTS,
				Source:  "worker",
				Level:   "INFO",
				Message: "started",
			},
		},
	})

	got := ui.liveLog.GetText(true)
	wantPrefix := "[" + time.UnixMilli(eventTS).Format("15:04:05") + "] worker INFO started"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("expected runtime log prefix %q, got %q", wantPrefix, got)
	}
}

func TestBuildOverviewDisplayRowsKeepsMergedGammaFieldsAndSortsByTurnover(t *testing.T) {
	rows := buildOverviewDisplayRows(
		[]router.OverviewRow{
			{Symbol: "IH", Turnover: testFloatPtr(100)},
			{
				Symbol:    "IF",
				Turnover:  testFloatPtr(200),
				CGammaInv: testFloatPtr(10),
				PGammaInv: testFloatPtr(20),
			},
		},
		"turnover",
		false,
		false,
	)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Symbol != "IF" || rows[1].Symbol != "IH" {
		t.Fatalf("unexpected sort order: %q, %q", rows[0].Symbol, rows[1].Symbol)
	}
	if !rows[0].HasCGammaInv || rows[0].CGammaInv != 10 {
		t.Fatalf("expected IF call gamma to be preserved, got %+v", rows[0])
	}
	if rows[1].HasCGammaInv || rows[1].HasPGammaInv {
		t.Fatalf("expected IH to keep missing gamma placeholders, got %+v", rows[1])
	}
}

func TestBuildOverviewDisplayRowsOptionAvailabilityUsesAnyValidGammaContribution(t *testing.T) {
	rows := buildOverviewDisplayRows(
		[]router.OverviewRow{
			{Symbol: "IF", Turnover: testFloatPtr(200)},
			{Symbol: "IH", Turnover: testFloatPtr(100), CGammaMid: testFloatPtr(0), CGammaInv: testFloatPtr(0)},
			{Symbol: "IC", Turnover: testFloatPtr(90), PGammaBack: testFloatPtr(5), PGammaInv: testFloatPtr(5)},
		},
		"turnover",
		false,
		true,
	)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows after availability filter, got %d", len(rows))
	}
	if rows[0].Symbol != "IH" || rows[1].Symbol != "IC" {
		t.Fatalf("unexpected filtered order/symbols: %+v", []string{rows[0].Symbol, rows[1].Symbol})
	}
}

func TestBuildOverviewDisplayRowsSortsByOIChgAscending(t *testing.T) {
	rows := buildOverviewDisplayRows(
		[]router.OverviewRow{
			{Symbol: "IF", OIChgPct: testFloatPtr(0.2)},
			{Symbol: "IH", OIChgPct: testFloatPtr(-0.1)},
			{Symbol: "IC", OIChgPct: testFloatPtr(0.05)},
		},
		"oi_chg",
		true,
		false,
	)
	gotOrder := []string{rows[0].Symbol, rows[1].Symbol, rows[2].Symbol}
	wantOrder := []string{"IH", "IC", "IF"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("unexpected sorted order at %d: got=%v want=%v", i, gotOrder, wantOrder)
		}
	}
}

func TestStopLiveProcessCancelsInFlightStartupRequest(t *testing.T) {
	ui := &UI{routerAddr: "127.0.0.1:19090"}

	req, ok := ui.beginLiveStartupRequest()
	if !ok {
		t.Fatalf("expected startup request to begin")
	}
	if req.startCtx == nil {
		t.Fatalf("expected startup request context")
	}
	if !ui.isLiveStartupRequestCurrent(req.seq) {
		t.Fatalf("expected startup request seq=%d to be current", req.seq)
	}
	select {
	case <-req.startCtx.Done():
		t.Fatalf("startup request context canceled before stop")
	default:
	}

	ui.stopLiveProcess()

	if ui.isLiveStartupRequestCurrent(req.seq) {
		t.Fatalf("expected startup request seq=%d to be invalidated after stop", req.seq)
	}
	select {
	case <-req.startCtx.Done():
	default:
		t.Fatalf("expected stopLiveProcess to cancel in-flight startup request")
	}
}

func TestApplyGlobalSettingsRuntimeDefersOptionsRestartDuringInFlightStartup(t *testing.T) {
	ui := &UI{routerAddr: "127.0.0.1:19090"}

	req, ok := ui.beginLiveStartupRequest()
	if !ok {
		t.Fatalf("expected startup request to begin")
	}
	if ui.consumeDeferredOptionsWorkerRestart() {
		t.Fatalf("expected no deferred restart before settings save")
	}

	ui.applyGlobalSettingsRuntime(20, 60, true)

	if !ui.consumeDeferredOptionsWorkerRestart() {
		t.Fatalf("expected settings save to defer options restart during in-flight startup")
	}
	if ui.consumeDeferredOptionsWorkerRestart() {
		t.Fatalf("expected deferred restart marker to be consumed once")
	}

	ui.completeLiveStartupRequest(req.seq)
}

func TestFocusSymbolForPollingUsesLocalAtomicSnapshot(t *testing.T) {
	ui := &UI{}
	if got := ui.focusSymbolForPolling(); got != "" {
		t.Fatalf("expected empty polling focus before snapshot init, got %q", got)
	}

	ui.setFocusSymbolState(" IF ")
	if got := ui.focusSymbolForPolling(); got != "IF" {
		t.Fatalf("expected trimmed polling focus snapshot, got %q", got)
	}

	ui.setFocusSymbolState("")
	if got := ui.focusSymbolForPolling(); got != "" {
		t.Fatalf("expected polling focus snapshot to clear, got %q", got)
	}
}

func testFloatPtr(v float64) *float64 {
	return &v
}
