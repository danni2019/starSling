package router

import (
	"math"
	"testing"
	"time"
)

func TestGetLatestMarketMinSeq(t *testing.T) {
	state := NewState()
	state.UpdateMarket(MarketSnapshot{
		Rows: []map[string]any{{"ctp_contract": "cu2604"}},
	})

	snapshot, unchanged := state.GetLatestMarket(0)
	if unchanged {
		t.Fatalf("expected changed snapshot for min_seq=0")
	}
	if snapshot.Seq == 0 {
		t.Fatalf("expected seq > 0")
	}

	snapshot, unchanged = state.GetLatestMarket(snapshot.Seq)
	if !unchanged {
		t.Fatalf("expected unchanged snapshot for matching min_seq")
	}
	if snapshot.Seq == 0 {
		t.Fatalf("expected unchanged response to carry latest seq")
	}
}

func TestGetViewSnapshotMarksStale(t *testing.T) {
	state := NewState()
	state.UpdateMarket(MarketSnapshot{
		Rows: []map[string]any{{"ctp_contract": "cu2604"}},
	})
	state.UpdateOptions(OptionsSnapshot{
		Rows: []map[string]any{{"ctp_contract": "cu2604C70000", "underlying": "cu2604"}},
	})

	state.mu.Lock()
	state.lastMarket = time.Now().Add(-3 * time.Second)
	state.lastOptions = time.Now().Add(-3 * time.Second)
	state.mu.Unlock()

	view := state.GetViewSnapshot("cu2604")
	if !view.Market.Stale {
		t.Fatalf("expected market stale flag")
	}
	if !view.Options.Stale {
		t.Fatalf("expected options stale flag")
	}
}

func TestGetViewSnapshotReusesCachedOverviewWhenInputsUnchanged(t *testing.T) {
	state := NewState()
	state.UpdateMarket(MarketSnapshot{
		Rows: []map[string]any{
			{
				"ctp_contract":      "IF2503",
				"product_class":     "1",
				"symbol":            "IF",
				"open_interest":     100.0,
				"pre_open_interest": 80.0,
				"turnover":          1000.0,
			},
		},
	})

	first := state.GetViewSnapshot("")
	if first.Overview.Seq == 0 {
		t.Fatalf("expected overview seq > 0 after market update")
	}
	if first.Overview.TS == 0 {
		t.Fatalf("expected overview ts to be populated")
	}

	time.Sleep(5 * time.Millisecond)

	second := state.GetViewSnapshot("")
	if second.Overview.Seq != first.Overview.Seq {
		t.Fatalf("expected stable overview seq across repeated polls, got %d -> %d", first.Overview.Seq, second.Overview.Seq)
	}
	if second.Overview.TS != first.Overview.TS {
		t.Fatalf("expected cached overview ts to be reused across repeated polls, got %d -> %d", first.Overview.TS, second.Overview.TS)
	}
}

func TestGetViewSnapshotFiltersOptionRowsButLatestKeepsAll(t *testing.T) {
	state := NewState()
	state.UpdateMarket(MarketSnapshot{
		Rows: []map[string]any{
			{"ctp_contract": "cu2604", "product_class": "1"},
			{"ctp_contract": "cu2604C72000", "product_class": "2"},
		},
	})

	view := state.GetViewSnapshot("")
	if len(view.Market.Rows) != 1 {
		t.Fatalf("expected non-option rows only in view snapshot, got %d rows", len(view.Market.Rows))
	}
	if got := toString(view.Market.Rows[0]["ctp_contract"]); got != "cu2604" {
		t.Fatalf("unexpected market row in view snapshot: %s", got)
	}

	latest, unchanged := state.GetLatestMarket(0)
	if unchanged {
		t.Fatalf("expected changed latest market snapshot")
	}
	if len(latest.Rows) != 2 {
		t.Fatalf("expected latest market snapshot to keep full rows, got %d", len(latest.Rows))
	}
}

func TestFilterOptionsRowsCaseInsensitive(t *testing.T) {
	rows := []map[string]any{
		{"ctp_contract": "CU2604C70000", "underlying": "cu2604", "symbol": "CU"},
		{"ctp_contract": "ag2604C30000", "underlying": "ag2604", "symbol": "AG"},
	}
	filtered := filterOptionsRows(rows, "cu2604")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered row by underlying, got %d", len(filtered))
	}
	filtered = filterOptionsRows(rows, "cu")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered row by symbol, got %d", len(filtered))
	}
	filtered = filterOptionsRows(rows, "cu2604c70000")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered row by contract, got %d", len(filtered))
	}
}

func TestStateThresholdUpdate(t *testing.T) {
	state := NewState()
	uiState := state.GetUIState()
	if uiState.TurnoverChgThreshold != 100000.0 {
		t.Fatalf("unexpected default chg threshold: %v", uiState.TurnoverChgThreshold)
	}
	if uiState.TurnoverRatioThreshold != 0.05 {
		t.Fatalf("unexpected default ratio threshold: %v", uiState.TurnoverRatioThreshold)
	}
	if uiState.OIRatioThreshold != 0.05 {
		t.Fatalf("unexpected default oi ratio threshold: %v", uiState.OIRatioThreshold)
	}

	state.SetUnusualThresholds(200000, 0.1, 0.2)
	uiState = state.GetUIState()
	if uiState.TurnoverChgThreshold != 200000 {
		t.Fatalf("unexpected updated chg threshold: %v", uiState.TurnoverChgThreshold)
	}
	if uiState.TurnoverRatioThreshold != 0.1 {
		t.Fatalf("unexpected updated ratio threshold: %v", uiState.TurnoverRatioThreshold)
	}
	if uiState.OIRatioThreshold != 0.2 {
		t.Fatalf("unexpected updated oi ratio threshold: %v", uiState.OIRatioThreshold)
	}
}

func TestUpdateUnusualEnrichesRowsWithGreeks(t *testing.T) {
	state := NewState()
	state.UpdateOptions(OptionsSnapshot{
		Rows: []map[string]any{
			{
				"ctp_contract": "cu2604C72000",
				"iv":           0.22,
				"delta":        0.41,
				"gamma":        0.01,
				"theta":        -0.02,
				"vega":         0.15,
			},
		},
	})
	state.UpdateUnusual(UnusualSnapshot{
		Rows: []map[string]any{
			{"ctp_contract": "cu2604C72000", "turnover_chg": 120000.0},
			{"ctp_contract": "ag2604C30000", "turnover_chg": 50000.0},
		},
	})

	view := state.GetViewSnapshot("")
	if len(view.Unusual.Rows) != 2 {
		t.Fatalf("expected 2 unusual rows, got %d", len(view.Unusual.Rows))
	}

	rowsByContract := make(map[string]map[string]any, len(view.Unusual.Rows))
	for _, row := range view.Unusual.Rows {
		rowsByContract[toString(row["ctp_contract"])] = row
	}

	matched, ok := rowsByContract["cu2604C72000"]
	if !ok {
		t.Fatalf("expected matched unusual row for cu2604C72000")
	}
	for key, want := range map[string]float64{
		"iv":    0.22,
		"delta": 0.41,
		"gamma": 0.01,
		"theta": -0.02,
		"vega":  0.15,
	} {
		got, ok := matched[key].(float64)
		if !ok {
			t.Fatalf("expected %s to be float64 in merged unusual row, got %#v", key, matched[key])
		}
		if got != want {
			t.Fatalf("unexpected %s value: got %v want %v", key, got, want)
		}
	}

	unmatched, ok := rowsByContract["ag2604C30000"]
	if !ok {
		t.Fatalf("expected unmatched unusual row for ag2604C30000")
	}
	if _, exists := unmatched["iv"]; exists {
		t.Fatalf("expected no greek fields on unmatched unusual row, got %#v", unmatched["iv"])
	}
}

func TestUnderlyingPriceForGammaFallbackOrder(t *testing.T) {
	vwapRow := map[string]any{
		"bid1":     100.0,
		"ask1":     102.0,
		"bid_vol1": 1.0,
		"ask_vol1": 3.0,
		"last":     999.0,
	}
	if got, ok := underlyingPriceForGamma(vwapRow); !ok || math.Abs(got-101.5) > 1e-9 {
		t.Fatalf("expected vwap fallback first, got (%v,%v)", got, ok)
	}

	midRow := map[string]any{
		"bid1":     100.0,
		"ask1":     104.0,
		"bid_vol1": 0.0,
		"ask_vol1": 0.0,
		"last":     999.0,
	}
	if got, ok := underlyingPriceForGamma(midRow); !ok || math.Abs(got-102.0) > 1e-9 {
		t.Fatalf("expected mid fallback second, got (%v,%v)", got, ok)
	}

	lastRow := map[string]any{
		"last": 88.0,
	}
	if got, ok := underlyingPriceForGamma(lastRow); !ok || math.Abs(got-88.0) > 1e-9 {
		t.Fatalf("expected last fallback third, got (%v,%v)", got, ok)
	}
}

func TestToOptionalFiniteFloatRejectsPartialStringParses(t *testing.T) {
	for _, input := range []string{"1,234", "12abc"} {
		if got, ok := toOptionalFiniteFloat(input); ok {
			t.Fatalf("expected invalid parse for %q, got %v", input, got)
		}
	}
	if got, ok := toOptionalFiniteFloat("1e3"); !ok || got != 1000 {
		t.Fatalf("expected scientific notation to parse, got (%v,%v)", got, ok)
	}
}

func TestBuildOverviewRowsAggregatesFuturesAndOptions(t *testing.T) {
	marketRows := []map[string]any{
		{
			"ctp_contract":      "IF2503",
			"product_class":     "1",
			"symbol":            "IF",
			"open_interest":     100.0,
			"pre_open_interest": 80.0,
			"turnover":          1000.0,
			"bid1":              100.0,
			"ask1":              102.0,
			"bid_vol1":          1.0,
			"ask_vol1":          3.0,
		},
		{
			"ctp_contract":      "IF2504",
			"product_class":     "1",
			"symbol":            "IF",
			"open_interest":     50.0,
			"pre_open_interest": 50.0,
			"turnover":          500.0,
		},
		{
			"ctp_contract":      "IH2503",
			"product_class":     "1",
			"symbol":            "IH",
			"open_interest":     20.0,
			"pre_open_interest": 25.0,
			"turnover":          200.0,
		},
		{
			"ctp_contract":  "IO2503-C-4000",
			"product_class": "2",
			"symbol":        "IF",
			"underlying":    "IF2503",
			"open_interest": 10.0,
			"multiplier":    100.0,
		},
		{
			"ctp_contract":  "IO2503-P-4000",
			"product_class": "2",
			"symbol":        "IF",
			"underlying":    "IF2503",
			"open_interest": 20.0,
			"multiplier":    100.0,
		},
	}
	optionsRows := []map[string]any{
		{
			"ctp_contract": "IO2503-C-4000",
			"symbol":       "IF",
			"underlying":   "IF2503",
			"option_type":  "c",
			"tte":          20.0,
			"gamma":        0.01,
		},
		{
			"ctp_contract": "IO2503-P-4000",
			"symbol":       "IF",
			"underlying":   "IF2503",
			"option_type":  "p",
			"tte":          20.0,
			"gamma":        0.02,
		},
	}

	rows := buildOverviewRows(marketRows, optionsRows)
	if len(rows) != 2 {
		t.Fatalf("expected 2 merged overview rows, got %d", len(rows))
	}
	bySymbol := map[string]OverviewRow{}
	for _, row := range rows {
		bySymbol[row.Symbol] = row
	}
	ifRow, ok := bySymbol["IF"]
	if !ok {
		t.Fatalf("missing IF overview row")
	}
	if ifRow.OIChgPct == nil {
		t.Fatalf("expected IF oi_chg_pct")
	}
	wantOIChg := (150.0 / 130.0) - 1.0
	if math.Abs(*ifRow.OIChgPct-wantOIChg) > 1e-9 {
		t.Fatalf("unexpected IF oi_chg_pct: got %v want %v", *ifRow.OIChgPct, wantOIChg)
	}
	if ifRow.Turnover == nil || math.Abs(*ifRow.Turnover-1500.0) > 1e-9 {
		t.Fatalf("unexpected IF turnover: %#v", ifRow.Turnover)
	}
	S := 101.5
	wantCall := 0.01 * 10.0 * 100.0 * S * S
	wantPut := 0.02 * 20.0 * 100.0 * S * S
	if ifRow.CGammaInv == nil || math.Abs(*ifRow.CGammaInv-wantCall) > 1e-6 {
		t.Fatalf("unexpected call gamma inventory: got %#v want %v", ifRow.CGammaInv, wantCall)
	}
	if ifRow.PGammaInv == nil || math.Abs(*ifRow.PGammaInv-wantPut) > 1e-6 {
		t.Fatalf("unexpected put gamma inventory: got %#v want %v", ifRow.PGammaInv, wantPut)
	}
	if ifRow.CGammaFnt == nil || math.Abs(*ifRow.CGammaFnt-wantCall) > 1e-6 {
		t.Fatalf("unexpected call front gamma inventory: got %#v want %v", ifRow.CGammaFnt, wantCall)
	}
	if ifRow.PGammaFnt == nil || math.Abs(*ifRow.PGammaFnt-wantPut) > 1e-6 {
		t.Fatalf("unexpected put front gamma inventory: got %#v want %v", ifRow.PGammaFnt, wantPut)
	}
	if ifRow.CGammaMid != nil || ifRow.CGammaBack != nil || ifRow.PGammaMid != nil || ifRow.PGammaBack != nil {
		t.Fatalf("expected only front bucket values, got IF row %#v", ifRow)
	}
}

func TestBuildOverviewRowsMergesOptionGammaWithCaseInsensitiveSymbolKey(t *testing.T) {
	marketRows := []map[string]any{
		{
			"ctp_contract":      "IF2503",
			"product_class":     "1",
			"symbol":            "IF",
			"open_interest":     10.0,
			"pre_open_interest": 10.0,
			"turnover":          100.0,
			"bid1":              100.0,
			"ask1":              100.0,
			"bid_vol1":          1.0,
			"ask_vol1":          1.0,
		},
		{
			"ctp_contract":  "IO2503-C-4000",
			"product_class": "2",
			"symbol":        "IF",
			"underlying":    "IF2503",
			"open_interest": 2.0,
			"multiplier":    10.0,
			"option_type":   "1",
		},
	}
	optionsRows := []map[string]any{
		{
			"ctp_contract": "IO2503-C-4000",
			"symbol":       "if", // mixed case vs futures symbol
			"underlying":   "IF2503",
			"option_type":  "c",
			"tte":          20.0,
			"gamma":        1.0,
		},
	}

	rows := buildOverviewRows(marketRows, optionsRows)
	if len(rows) != 1 {
		t.Fatalf("expected one merged overview row, got %d", len(rows))
	}
	row := rows[0]
	if row.Symbol != "IF" {
		t.Fatalf("expected futures symbol casing to be preserved, got %q", row.Symbol)
	}
	if row.CGammaInv == nil {
		t.Fatalf("expected call gamma inventory to merge despite symbol case mismatch")
	}
	want := 1.0 * 2.0 * 10.0 * 100.0 * 100.0
	if math.Abs(*row.CGammaInv-want) > 1e-9 {
		t.Fatalf("unexpected call gamma inventory: got %v want %v", *row.CGammaInv, want)
	}
}

func TestBuildOverviewRowsBucketsGammaByTTEAndExcludesInvalidTTE(t *testing.T) {
	marketRows := []map[string]any{
		{
			"ctp_contract":      "IF2503",
			"product_class":     "1",
			"symbol":            "IF",
			"open_interest":     1.0,
			"pre_open_interest": 1.0,
			"turnover":          1.0,
			"bid1":              100.0,
			"ask1":              100.0,
			"bid_vol1":          1.0,
			"ask_vol1":          1.0,
		},
		{"ctp_contract": "IO-C-F", "product_class": "2", "symbol": "IF", "underlying": "IF2503", "open_interest": 1.0, "multiplier": 1.0},
		{"ctp_contract": "IO-C-M", "product_class": "2", "symbol": "IF", "underlying": "IF2503", "open_interest": 1.0, "multiplier": 1.0},
		{"ctp_contract": "IO-C-B", "product_class": "2", "symbol": "IF", "underlying": "IF2503", "open_interest": 1.0, "multiplier": 1.0},
		{"ctp_contract": "IO-C-X", "product_class": "2", "symbol": "IF", "underlying": "IF2503", "open_interest": 1.0, "multiplier": 1.0},
		{"ctp_contract": "IO-P-30", "product_class": "2", "symbol": "IF", "underlying": "IF2503", "open_interest": 1.0, "multiplier": 1.0},
		{"ctp_contract": "IO-P-90", "product_class": "2", "symbol": "IF", "underlying": "IF2503", "open_interest": 1.0, "multiplier": 1.0},
	}
	optionsRows := []map[string]any{
		{"ctp_contract": "IO-C-F", "symbol": "IF", "underlying": "IF2503", "option_type": "c", "tte": 10.0, "gamma": 1.0},
		{"ctp_contract": "IO-C-M", "symbol": "IF", "underlying": "IF2503", "option_type": "c", "tte": 30.1, "gamma": 2.0},
		{"ctp_contract": "IO-C-B", "symbol": "IF", "underlying": "IF2503", "option_type": "c", "tte": 90.1, "gamma": 3.0},
		{"ctp_contract": "IO-C-X", "symbol": "IF", "underlying": "IF2503", "option_type": "c", "gamma": 99.0}, // invalid tte excluded
		{"ctp_contract": "IO-P-30", "symbol": "IF", "underlying": "IF2503", "option_type": "p", "tte": 30.0, "gamma": 4.0},
		{"ctp_contract": "IO-P-90", "symbol": "IF", "underlying": "IF2503", "option_type": "p", "tte": 90.0, "gamma": 5.0},
	}

	rows := buildOverviewRows(marketRows, optionsRows)
	if len(rows) != 1 {
		t.Fatalf("expected one merged row, got %d", len(rows))
	}
	row := rows[0]
	// Underlying S should be 100 from vwap/mid/last path on IF2503.
	if row.CGammaFnt == nil || *row.CGammaFnt != 1.0*10000.0 {
		t.Fatalf("unexpected C front bucket: %#v", row.CGammaFnt)
	}
	if row.CGammaMid == nil || *row.CGammaMid != 2.0*10000.0 {
		t.Fatalf("unexpected C mid bucket: %#v", row.CGammaMid)
	}
	if row.CGammaBack == nil || *row.CGammaBack != 3.0*10000.0 {
		t.Fatalf("unexpected C back bucket: %#v", row.CGammaBack)
	}
	wantCInv := 6.0 * 10000.0
	if row.CGammaInv == nil || *row.CGammaInv != wantCInv {
		t.Fatalf("unexpected C total inventory: %#v want %v", row.CGammaInv, wantCInv)
	}
	if row.PGammaFnt == nil || *row.PGammaFnt != 4.0*10000.0 {
		t.Fatalf("unexpected P front bucket boundary(30): %#v", row.PGammaFnt)
	}
	if row.PGammaMid == nil || *row.PGammaMid != 5.0*10000.0 {
		t.Fatalf("unexpected P mid bucket boundary(90): %#v", row.PGammaMid)
	}
	if row.PGammaBack != nil {
		t.Fatalf("unexpected P back bucket: %#v", row.PGammaBack)
	}
	wantPInv := 9.0 * 10000.0
	if row.PGammaInv == nil || *row.PGammaInv != wantPInv {
		t.Fatalf("unexpected P total inventory: %#v want %v", row.PGammaInv, wantPInv)
	}
}
