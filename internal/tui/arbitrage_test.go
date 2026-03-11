package tui

import (
	"math"
	"strings"
	"testing"
)

func TestCompileArbitrageExpressionRejectsEmpty(t *testing.T) {
	_, err := compileArbitrageExpression("   ")
	if err == nil {
		t.Fatalf("expected empty formula to fail")
	}
}

func TestCompileArbitrageExpressionSupportsComplexFormula(t *testing.T) {
	expr, err := compileArbitrageExpression("contract1 * 4 / contract2 * 3 - (contract3 / 2 + 100)")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if len(expr.Contracts) != 3 {
		t.Fatalf("expected 3 contracts, got %v", expr.Contracts)
	}
}

func TestCompileArbitrageExpressionSupportsFullWidthOperators(t *testing.T) {
	expr, err := compileArbitrageExpression("contract1 × 4 ÷ contract2 －（contract3 ÷ 2 ＋ 100）")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if len(expr.Contracts) != 3 {
		t.Fatalf("expected 3 contracts, got %v", expr.Contracts)
	}
}

func TestCompileArbitrageExpressionSupportsQuotedContract(t *testing.T) {
	expr, err := compileArbitrageExpression("'cu2605-C-72000' * 2 - ma605")
	if err != nil {
		t.Fatalf("compile with quoted contract failed: %v", err)
	}
	joined := strings.Join(expr.Contracts, ",")
	if !strings.Contains(joined, "cu2605-c-72000") {
		t.Fatalf("expected quoted contract to be extracted, got %v", expr.Contracts)
	}
}

func TestCompileArbitrageExpressionFailsOnMismatchedParen(t *testing.T) {
	_, err := compileArbitrageExpression("ma605 * (3 - eg2605")
	if err == nil {
		t.Fatalf("expected mismatched paren formula to fail")
	}
}

func TestEvaluateArbitrageExpressionComplexFormula(t *testing.T) {
	expr, err := compileArbitrageExpression("contract1 * 4 / contract2 * 3 - (contract3 / 2 + 100)")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	result := evaluateArbitrageExpression(expr, map[string]float64{
		"contract1": 10,
		"contract2": 5,
		"contract3": 20,
	})
	if result.HasError {
		t.Fatalf("expected no error, got %v", result.Err)
	}
	if !result.Known {
		t.Fatalf("expected known result")
	}
	if math.Abs(result.Value-(-86)) > 1e-9 {
		t.Fatalf("unexpected value: got %.6f want -86", result.Value)
	}
}

func TestEvaluateArbitrageExpressionPartialWhenMissingContracts(t *testing.T) {
	expr, err := compileArbitrageExpression("ma605 * 3 - eg2605 * 2 + 100")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	result := evaluateArbitrageExpression(expr, map[string]float64{
		"ma605": 2500,
	})
	if result.HasError {
		t.Fatalf("expected no runtime error, got %v", result.Err)
	}
	if result.Known {
		t.Fatalf("expected unknown result when legs are missing")
	}
	if len(result.Missing) != 1 || result.Missing[0] != "eg2605" {
		t.Fatalf("unexpected missing set: %v", result.Missing)
	}
}

func TestEvaluateArbitrageExpressionDivisionByZero(t *testing.T) {
	expr, err := compileArbitrageExpression("ma605 / (eg2605 - eg2605)")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	result := evaluateArbitrageExpression(expr, map[string]float64{
		"ma605":  10,
		"eg2605": 5,
	})
	if !result.HasError {
		t.Fatalf("expected division by zero runtime error")
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "division by zero") {
		t.Fatalf("unexpected error: %v", result.Err)
	}
}

func TestBuildMarketPriceMapsUseCaseInsensitiveContractKeys(t *testing.T) {
	rows := []MarketRow{
		{Symbol: "MA605", Last: "2500.5", Open: "2450", High: "2550", Low: "2400", PreClose: "2440", PreSettle: "2430"},
		{Symbol: "eg2605", Last: "-", Open: "-", High: "-", Low: "-", PreClose: "-", PreSettle: "-"},
		{Symbol: "rb2605", Last: "3800", Open: "3750", High: "3900", Low: "3700", PreClose: "3720", PreSettle: "3710"},
	}

	lastPrices := buildMarketLastPriceMap(rows)
	if got := lastPrices["ma605"]; math.Abs(got-2500.5) > 1e-9 {
		t.Fatalf("unexpected ma605 price: %v", got)
	}
	if _, ok := lastPrices["eg2605"]; ok {
		t.Fatalf("expected missing eg2605 because last is non-numeric")
	}
	if got := lastPrices["rb2605"]; math.Abs(got-3800) > 1e-9 {
		t.Fatalf("unexpected rb2605 price: %v", got)
	}

	openPrices := buildMarketOpenPriceMap(rows)
	if got := openPrices["ma605"]; math.Abs(got-2450) > 1e-9 {
		t.Fatalf("unexpected ma605 open: %v", got)
	}
	if got := openPrices["rb2605"]; math.Abs(got-3750) > 1e-9 {
		t.Fatalf("unexpected rb2605 open: %v", got)
	}

	highPrices := buildMarketHighPriceMap(rows)
	if got := highPrices["ma605"]; math.Abs(got-2550) > 1e-9 {
		t.Fatalf("unexpected ma605 high: %v", got)
	}
	if got := highPrices["rb2605"]; math.Abs(got-3900) > 1e-9 {
		t.Fatalf("unexpected rb2605 high: %v", got)
	}

	lowPrices := buildMarketLowPriceMap(rows)
	if got := lowPrices["ma605"]; math.Abs(got-2400) > 1e-9 {
		t.Fatalf("unexpected ma605 low: %v", got)
	}
	if got := lowPrices["rb2605"]; math.Abs(got-3700) > 1e-9 {
		t.Fatalf("unexpected rb2605 low: %v", got)
	}

	preClosePrices := buildMarketPreClosePriceMap(rows)
	if got := preClosePrices["ma605"]; math.Abs(got-2440) > 1e-9 {
		t.Fatalf("unexpected ma605 pre_close: %v", got)
	}
	if got := preClosePrices["rb2605"]; math.Abs(got-3720) > 1e-9 {
		t.Fatalf("unexpected rb2605 pre_close: %v", got)
	}

	preSettlePrices := buildMarketPreSettlePriceMap(rows)
	if got := preSettlePrices["ma605"]; math.Abs(got-2430) > 1e-9 {
		t.Fatalf("unexpected ma605 pre_settle: %v", got)
	}
	if got := preSettlePrices["rb2605"]; math.Abs(got-3710) > 1e-9 {
		t.Fatalf("unexpected rb2605 pre_settle: %v", got)
	}
}
