package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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

func TestBuildLiveScreenUsesTableRowColorForRightPanels(t *testing.T) {
	ui := &UI{useArbMonitor: true}
	ui.buildLiveScreen()

	curveColor := firstInnerTextColor(t, ui.liveCurve)
	if curveColor != colorTableRow {
		t.Fatalf("expected liveCurve text color %v, got %v", colorTableRow, curveColor)
	}
	optsColor := firstInnerTextColor(t, ui.liveOpts)
	if optsColor != colorTableRow {
		t.Fatalf("expected liveOpts text color %v, got %v", colorTableRow, optsColor)
	}
}

func TestFillOverviewTableReordersCPColumnsByPairs(t *testing.T) {
	table := tview.NewTable()
	fillOverviewTable(table, []overviewFuturesDisplayRow{
		{
			Symbol:        "ma605",
			HasOIChgPct:   true,
			OIChgPct:      0.1,
			HasTurnover:   true,
			Turnover:      10,
			HasCGammaInv:  true,
			CGammaInv:     1,
			HasCGammaFnt:  true,
			CGammaFnt:     2,
			HasCGammaMid:  true,
			CGammaMid:     3,
			HasCGammaBack: true,
			CGammaBack:    4,
			HasPGammaInv:  true,
			PGammaInv:     5,
			HasPGammaFnt:  true,
			PGammaFnt:     6,
			HasPGammaMid:  true,
			PGammaMid:     7,
			HasPGammaBack: true,
			PGammaBack:    8,
		},
	})

	expectedHeaders := []string{
		"SYMBOL", "OI_CHG%", "TURN",
		"C_INV", "P_INV", "C_FNT", "P_FNT", "C_MID", "P_MID", "C_BAK", "P_BAK",
	}
	for col, want := range expectedHeaders {
		got := strings.TrimSpace(table.GetCell(0, col).Text)
		if got != want {
			t.Fatalf("unexpected header at col %d: got %q want %q", col, got, want)
		}
	}

	expectedValues := []string{
		"1.00e+00", "5.00e+00", "2.00e+00", "6.00e+00",
		"3.00e+00", "7.00e+00", "4.00e+00", "8.00e+00",
	}
	for idx, want := range expectedValues {
		col := idx + 3
		got := strings.TrimSpace(table.GetCell(1, col).Text)
		if got != want {
			t.Fatalf("unexpected value at col %d: got %q want %q", col, got, want)
		}
	}
}

func TestFillArbitrageTableAddsOpenHighLowColumns(t *testing.T) {
	table := tview.NewTable()
	fillArbitrageTable(table, []ArbitrageMonitorRow{
		{
			Name:      "pair1",
			Value:     "1",
			High:      "3",
			Low:       "0",
			Open:      "2",
			PreClose:  "4",
			PreSettle: "5",
			Status:    "READY",
			Missing:   "-",
			UpdatedAt: "10:00:00",
			Formula:   "a-b",
		},
	})

	expectedHeaders := []string{
		"NAME", "VALUE", "HIGH", "LOW", "OPEN", "PRE_CLOSE", "PRE_SETTLE", "STATUS", "MISSING", "UPDATED_AT", "FORMULA",
	}
	for col, want := range expectedHeaders {
		got := strings.TrimSpace(table.GetCell(0, col).Text)
		if got != want {
			t.Fatalf("unexpected header at col %d: got %q want %q", col, got, want)
		}
	}
	if got := strings.TrimSpace(table.GetCell(1, 2).Text); got != "3" {
		t.Fatalf("expected HIGH value in col 2, got %q", got)
	}
	if got := strings.TrimSpace(table.GetCell(1, 3).Text); got != "0" {
		t.Fatalf("expected LOW value in col 3, got %q", got)
	}
	if got := strings.TrimSpace(table.GetCell(1, 4).Text); got != "2" {
		t.Fatalf("expected OPEN value in col 4, got %q", got)
	}
	if got := strings.TrimSpace(table.GetCell(1, 5).Text); got != "4" {
		t.Fatalf("expected PRE_CLOSE value in col 5, got %q", got)
	}
	if got := strings.TrimSpace(table.GetCell(1, 6).Text); got != "5" {
		t.Fatalf("expected PRE_SETTLE value in col 6, got %q", got)
	}
	if got := strings.TrimSpace(table.GetCell(1, 10).Text); got != "a-b" {
		t.Fatalf("expected FORMULA value in final column, got %q", got)
	}
}

func firstInnerTextColor(t *testing.T, view *tview.TextView) tcell.Color {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()

	screen.SetSize(120, 20)
	view.SetRect(0, 0, 80, 6)
	view.Draw(screen)

	for y := 1; y < 5; y++ {
		for x := 1; x < 79; x++ {
			main, _, style, _ := screen.GetContent(x, y)
			if main == ' ' || main == 0 {
				continue
			}
			fg, _, _ := style.Decompose()
			return fg
		}
	}
	t.Fatalf("unable to find text rune inside text view")
	return tcell.ColorDefault
}
