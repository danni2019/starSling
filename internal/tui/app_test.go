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
