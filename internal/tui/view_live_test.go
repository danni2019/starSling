package tui

import "testing"

func TestMarketSelectionUpdatesPollingFocusSnapshot(t *testing.T) {
	ui := &UI{}
	ui.buildLiveScreen()

	ui.setFocusSymbolState("OLD")
	ui.marketRows = []MarketRow{{Symbol: " IF "}}
	fillMarketTable(ui.liveMarket, ui.marketRows)

	// Prevent the selection callback's deferred flow render path from queueing
	// tview app updates in this unit test.
	ui.liveFlowRenderQueued.Store(true)
	ui.liveMarket.Select(1, 0)

	if got := ui.currentFocusSymbol(); got != "IF" {
		t.Fatalf("expected current focus symbol IF, got %q", got)
	}
	if got := ui.focusSymbolForPolling(); got != "IF" {
		t.Fatalf("expected polling focus snapshot IF, got %q", got)
	}
}
