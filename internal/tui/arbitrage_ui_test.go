package tui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

func TestRenderArbitrageMonitorReady(t *testing.T) {
	ui := &UI{
		useArbMonitor: true,
		liveFlow:      tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "ma605", Last: "2500"},
			{Symbol: "eg2605", Last: "2000"},
		},
	}
	ui.setArbitrageFormula("ma605 * 3 - eg2605 * 2")
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetTitle()); !strings.Contains(got, "Arbitrage Monitor") {
		t.Fatalf("unexpected panel title: %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 7).Text); got != "READY" {
		t.Fatalf("expected READY status, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 1).Text); got != "3500" {
		t.Fatalf("expected value 3500, got %q", got)
	}
}

func TestRenderArbitrageMonitorPartialWithMissingContracts(t *testing.T) {
	ui := &UI{
		useArbMonitor: true,
		liveFlow:      tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "ma605", Last: "2500"},
		},
	}
	ui.setArbitrageFormula("ma605 * 3 - eg2605 * 2")
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 7).Text); got != "PARTIAL" {
		t.Fatalf("expected PARTIAL status, got %q", got)
	}
	missing := strings.TrimSpace(ui.liveFlow.GetCell(1, 8).Text)
	if !strings.Contains(missing, "eg2605") {
		t.Fatalf("expected missing contracts to include eg2605, got %q", missing)
	}
}

func TestRenderArbitrageMonitorRuntimeError(t *testing.T) {
	ui := &UI{
		useArbMonitor: true,
		liveFlow:      tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "ma605", Last: "2500"},
			{Symbol: "eg2605", Last: "2000"},
		},
	}
	ui.setArbitrageFormula("ma605 / (eg2605 - eg2605)")
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 7).Text); got != "RUNTIME_ERR" {
		t.Fatalf("expected RUNTIME_ERR status, got %q", got)
	}
	if msg := strings.TrimSpace(ui.liveFlow.GetCell(1, 8).Text); !strings.Contains(msg, "division by zero") {
		t.Fatalf("expected division by zero message, got %q", msg)
	}
}

func TestRenderArbitrageMonitorInvalidFormula(t *testing.T) {
	ui := &UI{
		useArbMonitor: true,
		liveFlow:      tview.NewTable(),
	}
	ui.setArbitrageFormula("ma605 * (")
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 7).Text); got != "INVALID" {
		t.Fatalf("expected INVALID status, got %q", got)
	}
}

func TestUpsertArbitrageMonitorCreateEditDelete(t *testing.T) {
	ui := &UI{useArbMonitor: true}

	if err := ui.upsertArbitrageMonitor("", "pairA", "ma605 * 3 - eg2605 * 2"); err != nil {
		t.Fatalf("create pair failed: %v", err)
	}
	if len(ui.arbMonitors) != 1 {
		t.Fatalf("expected one monitor after create, got %d", len(ui.arbMonitors))
	}
	id := ui.arbMonitors[0].ID
	if strings.TrimSpace(id) == "" {
		t.Fatalf("expected generated ID for created pair")
	}

	if err := ui.upsertArbitrageMonitor(id, "pairA-edited", "rb2605 - rb2610"); err != nil {
		t.Fatalf("edit pair failed: %v", err)
	}
	if ui.arbMonitors[0].Name != "pairA-edited" {
		t.Fatalf("expected edited name, got %q", ui.arbMonitors[0].Name)
	}
	if ui.arbMonitors[0].Formula != "rb2605 - rb2610" {
		t.Fatalf("expected edited formula, got %q", ui.arbMonitors[0].Formula)
	}

	if deleted := ui.deleteArbitrageMonitor(id); !deleted {
		t.Fatalf("expected monitor delete to succeed")
	}
	if len(ui.arbMonitors) != 0 {
		t.Fatalf("expected no monitors after delete, got %+v", ui.arbMonitors)
	}
}

func TestRenderArbitrageMonitorMultiplePairs(t *testing.T) {
	ui := &UI{
		useArbMonitor: true,
		liveFlow:      tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "ma605", Last: "2500"},
			{Symbol: "eg2605", Last: "2000"},
			{Symbol: "rb2605", Last: "3500"},
			{Symbol: "rb2610", Last: "3400"},
		},
	}
	if err := ui.upsertArbitrageMonitor("", "pair1", "ma605 * 3 - eg2605 * 2"); err != nil {
		t.Fatalf("create pair1 failed: %v", err)
	}
	if err := ui.upsertArbitrageMonitor("", "pair2", "rb2605 - rb2610"); err != nil {
		t.Fatalf("create pair2 failed: %v", err)
	}
	ui.renderArbitrageMonitor()

	if got := ui.liveFlow.GetRowCount(); got != 3 {
		t.Fatalf("expected header + 2 rows, got %d", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 0).Text); got != "pair1" {
		t.Fatalf("expected first row name pair1, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(2, 0).Text); got != "pair2" {
		t.Fatalf("expected second row name pair2, got %q", got)
	}
}

func TestRenderArbitrageMonitorDisplaysMetricsAndUpdatesRealtimeForHighLow(t *testing.T) {
	ui := &UI{
		useArbMonitor: true,
		liveFlow:      tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "ma605", Last: "2500", Open: "2400", High: "2550", Low: "2300", PreClose: "2450", PreSettle: "2460"},
			{Symbol: "eg2605", Last: "2000", Open: "1900", High: "2050", Low: "1700", PreClose: "1950", PreSettle: "1960"},
		},
	}
	ui.setArbitrageFormula("ma605 * 3 - eg2605 * 2")
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 1).Text); got != "3500" {
		t.Fatalf("expected initial last value 3500, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 2).Text); got != "3550" {
		t.Fatalf("expected initial high value 3550, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 3).Text); got != "3500" {
		t.Fatalf("expected initial low value 3500, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 4).Text); got != "3400" {
		t.Fatalf("expected initial open value 3400, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 5).Text); got != "3450" {
		t.Fatalf("expected initial pre_close value 3450, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 6).Text); got != "3460" {
		t.Fatalf("expected initial pre_settle value 3460, got %q", got)
	}

	ui.marketRows = []MarketRow{
		{Symbol: "ma605", Last: "2600", Open: "2000", High: "2600", Low: "2250", PreClose: "2200", PreSettle: "2300"},
		{Symbol: "eg2605", Last: "2100", Open: "1000", High: "2100", Low: "1650", PreClose: "1200", PreSettle: "1100"},
	}
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 1).Text); got != "3600" {
		t.Fatalf("expected refreshed last value 3600, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 2).Text); got != "3600" {
		t.Fatalf("expected refreshed high value 3600, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 3).Text); got != "3450" {
		t.Fatalf("expected refreshed low value 3450, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 4).Text); got != "3400" {
		t.Fatalf("expected open to remain frozen at 3400, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 5).Text); got != "3450" {
		t.Fatalf("expected pre_close to remain frozen at 3450, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 6).Text); got != "3460" {
		t.Fatalf("expected pre_settle to remain frozen at 3460, got %q", got)
	}
}

func TestRenderArbitrageMonitorComputesOpenOnceWhenDataBecomesAvailable(t *testing.T) {
	ui := &UI{
		useArbMonitor: true,
		liveFlow:      tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "ma605", Last: "10", Open: "-", High: "11", Low: "9"},
			{Symbol: "eg2605", Last: "4", Open: "-", High: "5", Low: "3"},
		},
	}
	ui.setArbitrageFormula("ma605 - eg2605")
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 4).Text); got != "-" {
		t.Fatalf("expected open to stay unknown before open prices are available, got %q", got)
	}

	ui.marketRows = []MarketRow{
		{Symbol: "ma605", Last: "10", Open: "8", High: "12", Low: "7"},
		{Symbol: "eg2605", Last: "4", Open: "3", High: "6", Low: "2"},
	}
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 4).Text); got != "5" {
		t.Fatalf("expected open to capture first computed value 5, got %q", got)
	}

	ui.marketRows = []MarketRow{
		{Symbol: "ma605", Last: "11", Open: "20", High: "13", Low: "8"},
		{Symbol: "eg2605", Last: "5", Open: "6", High: "7", Low: "3"},
	}
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 4).Text); got != "5" {
		t.Fatalf("expected open to remain frozen after capture, got %q", got)
	}
}

func TestRenderArbitrageMonitorComputesPreCloseAndPreSettleOnceWhenDataBecomesAvailable(t *testing.T) {
	ui := &UI{
		useArbMonitor: true,
		liveFlow:      tview.NewTable(),
		marketRows: []MarketRow{
			{Symbol: "ma605", Last: "10", High: "11", Low: "9", PreClose: "-", PreSettle: "-"},
			{Symbol: "eg2605", Last: "4", High: "5", Low: "3", PreClose: "-", PreSettle: "-"},
		},
	}
	ui.setArbitrageFormula("ma605 - eg2605")
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 5).Text); got != "-" {
		t.Fatalf("expected pre_close unknown before availability, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 6).Text); got != "-" {
		t.Fatalf("expected pre_settle unknown before availability, got %q", got)
	}

	ui.marketRows = []MarketRow{
		{Symbol: "ma605", Last: "10", High: "12", Low: "7", PreClose: "8", PreSettle: "9"},
		{Symbol: "eg2605", Last: "4", High: "6", Low: "2", PreClose: "3", PreSettle: "4"},
	}
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 5).Text); got != "5" {
		t.Fatalf("expected pre_close to capture first computed value 5, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 6).Text); got != "5" {
		t.Fatalf("expected pre_settle to capture first computed value 5, got %q", got)
	}

	ui.marketRows = []MarketRow{
		{Symbol: "ma605", Last: "11", High: "13", Low: "8", PreClose: "20", PreSettle: "19"},
		{Symbol: "eg2605", Last: "5", High: "7", Low: "3", PreClose: "6", PreSettle: "5"},
	}
	ui.renderArbitrageMonitor()

	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 5).Text); got != "5" {
		t.Fatalf("expected pre_close to remain frozen after capture, got %q", got)
	}
	if got := strings.TrimSpace(ui.liveFlow.GetCell(1, 6).Text); got != "5" {
		t.Fatalf("expected pre_settle to remain frozen after capture, got %q", got)
	}
}

func TestOpenArbitrageMonitorManagerKeepsDrilldownForBatchOps(t *testing.T) {
	ui := &UI{
		app:           tview.NewApplication(),
		pages:         tview.NewPages(),
		useArbMonitor: true,
	}
	if err := ui.upsertArbitrageMonitor("", "pair1", "ma605*3-eg2605*2"); err != nil {
		t.Fatalf("create pair failed: %v", err)
	}

	ui.openArbitrageMonitorManager(ui.arbSelectedMonitorID, "batch mode")

	if ui.currentScreen() != screenDrilldown {
		t.Fatalf("expected manager to keep drilldown screen for batch ops, got %q", ui.currentScreen())
	}
}
