package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

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
	if got := ui.liveMarket.GetCell(1, 0).Text; got != "cu2604" {
		t.Fatalf("expected initial desc row to be cu2604, got %q", got)
	}

	ui.handleLiveKeys(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModNone))
	if got := ui.liveMarket.GetCell(1, 0).Text; got != "ag2604" {
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

func TestRenderOptionsPanelSkipsRowsWithoutIV(t *testing.T) {
	panel := renderOptionsPanel([]map[string]any{
		{"ctp_contract": "cu2604C70000", "strike": 70000.0, "iv": nil, "volume": 50.0},
		{"ctp_contract": "cu2604C71000", "strike": 71000.0, "iv": 0.22, "volume": 80.0},
	})
	if !strings.Contains(panel, "Options points: 1") {
		t.Fatalf("expected only valid IV points to be rendered, got panel: %s", panel)
	}
	if strings.Contains(panel, "cu2604C70000") {
		t.Fatalf("expected row without IV to be excluded, got panel: %s", panel)
	}
	if !strings.Contains(panel, "cu2604C71000") {
		t.Fatalf("expected row with IV to remain, got panel: %s", panel)
	}
}

func TestFilterMarketRowsSupportsCSVAndCaseInsensitive(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "cu2604", "exchange": "SHFE", "product_class": "1", "symbol": "CU"},
		{"ctp_contract": "rb2605", "exchange": "dce", "product_class": "1", "symbol": "rb"},
		{"ctp_contract": "ta2605", "exchange": "CZCE", "product_class": "1", "symbol": "TA"},
	}

	filtered := filterMarketRows(rows, "shfe,DCE", "1", "cu,RB")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 rows after csv + case-insensitive filter, got %d", len(filtered))
	}
}

func TestFilterMarketRowsSymbolStrictMatch(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "ta2605", "exchange": "CZCE", "product_class": "1", "symbol": "TA"},
		{"ctp_contract": "t2605", "exchange": "CFFEX", "product_class": "1", "symbol": "T"},
	}
	filtered := filterMarketRows(rows, "", "", "t")
	if len(filtered) != 1 {
		t.Fatalf("expected strict symbol match to return one row, got %d", len(filtered))
	}
	if got := asString(filtered[0]["symbol"]); !strings.EqualFold(got, "T") {
		t.Fatalf("expected strict symbol match to keep only T, got %q", got)
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
	if strings.Contains(joined, "symbol") {
		t.Fatalf("did not expect string-only column in numeric set, got %v", columns)
	}
}
