package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/ipc"
	"github.com/danni2019/starSling/internal/router"
)

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
			"ctp_contract":  "cu2604",
			"last":          nil,
			"bid1":          nil,
			"ask1":          nil,
			"volume":        nil,
			"open_interest": nil,
			"datetime":      nil,
		},
	})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Last != "-" || rows[0].Bid != "-" || rows[0].Ask != "-" || rows[0].Vol != "-" || rows[0].OI != "-" {
		t.Fatalf("expected missing fields to render as '-', got %+v", rows[0])
	}
	if rows[0].TS != "-" {
		t.Fatalf("expected missing timestamp to render as '-', got %q", rows[0].TS)
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
	panel := renderOptionsPanel(rows, "cu2604", optionRenderFilter{})
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
		{contract: "C2406", want: ""},
		{contract: "", want: ""},
	}
	for _, tc := range tests {
		if got := inferOptionTypeFromContract(tc.contract); got != tc.want {
			t.Fatalf("inferOptionTypeFromContract(%q) = %q, want %q", tc.contract, got, tc.want)
		}
	}
}

func TestRenderOptionsPanelInfersPutFromContract(t *testing.T) {
	panel := renderOptionsPanel([]map[string]any{
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
	panel := renderOptionsPanel([]map[string]any{
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
	panel := renderOptionsPanel(rows, "CU2604", optionRenderFilter{})
	if strings.Contains(panel, "19000") {
		t.Fatalf("expected non-focused chain to be excluded, got: %s", panel)
	}
	if !strings.Contains(panel, "72000") {
		t.Fatalf("expected focused chain to remain, got: %s", panel)
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

func TestSetUnusualThresholdsRollsBackOnRPCFailure(t *testing.T) {
	client := ipc.NewClient("127.0.0.1:1")
	client.Timeout = 50 * time.Millisecond
	ui := &UI{
		rpcClient:             client,
		unusualChgThreshold:   100000,
		unusualRatioThreshold: 0.05,
	}

	err := ui.setUnusualThresholds(200000, 0.2)
	if err == nil {
		t.Fatalf("expected threshold sync failure")
	}
	if ui.unusualChgThreshold != 100000 {
		t.Fatalf("expected chg threshold rollback to 100000, got %v", ui.unusualChgThreshold)
	}
	if ui.unusualRatioThreshold != 0.05 {
		t.Fatalf("expected ratio threshold rollback to 0.05, got %v", ui.unusualRatioThreshold)
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
		{"ctp_contract": "cu2604", "forward": 104500.0, "volume": 55848.0, "open_interest": 82000.0, "vix": 0.22},
		{"ctp_contract": "ag2604", "forward": 31490.0, "volume": 9200.0, "open_interest": 75000.0, "vix": 0.30},
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
