package router

import (
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
