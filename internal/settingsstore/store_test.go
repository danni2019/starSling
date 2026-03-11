package settingsstore

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPathReturnsDefaultsWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "missing.json")
	got, err := LoadPath(path)
	if err != nil {
		t.Fatalf("LoadPath missing: %v", err)
	}
	want := Default()
	if got.DaysInYear != want.DaysInYear || got.RiskFreeRate != want.RiskFreeRate {
		t.Fatalf("defaults mismatch: got %+v want %+v", got, want)
	}
	if got.GammaBucketFrontDays != want.GammaBucketFrontDays || got.GammaBucketMidDays != want.GammaBucketMidDays {
		t.Fatalf("gamma bucket defaults mismatch: got %+v want %+v", got, want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	cfg := Default()
	cfg.RiskFreeRate = 0.015
	cfg.DaysInYear = 252
	cfg.GammaBucketFrontDays = 20
	cfg.GammaBucketMidDays = 60
	cfg.Overview.SortBy = "oi_chg"
	cfg.Overview.SortAsc = true
	cfg.Overview.RequireOptions = true
	cfg.Market.Symbol = "cu"
	cfg.Options.DeltaEnabled = true
	cfg.Options.DeltaAbsMin = 0.2
	cfg.Options.DeltaAbsMax = 0.6
	cfg.Unusual.TurnoverChgThreshold = 200000
	cfg.Unusual.Symbol = "cu,ag"
	cfg.Unusual.Contract = "cu2604,ag2604"
	cfg.Flow.WindowSeconds = 180
	cfg.Flow.MinAnalysisSeconds = 45
	cfg.Flow.OnlySelected = true
	cfg.Arbitrage.Formula = "legacy-single"
	cfg.Arbitrage.Pairs = []SettingsArbitragePair{
		{ID: "spread-a", Name: "A", Formula: "ma605 * 3 - eg2605 * 2"},
		{ID: "spread-b", Name: "B", Formula: "rb2605 - rb2610"},
	}

	if err := SavePath(path, cfg); err != nil {
		t.Fatalf("SavePath: %v", err)
	}
	got, err := LoadPath(path)
	if err != nil {
		t.Fatalf("LoadPath: %v", err)
	}
	if got.RiskFreeRate != cfg.RiskFreeRate || got.DaysInYear != cfg.DaysInYear {
		t.Fatalf("global settings mismatch: got %+v want %+v", got, cfg)
	}
	if got.GammaBucketFrontDays != cfg.GammaBucketFrontDays || got.GammaBucketMidDays != cfg.GammaBucketMidDays {
		t.Fatalf("gamma buckets mismatch: got %+v want %+v", got, cfg)
	}
	if got.Overview.SortBy != "oi_chg" || !got.Overview.SortAsc || !got.Overview.RequireOptions {
		t.Fatalf("overview settings mismatch: %+v", got.Overview)
	}
	if got.Flow.WindowSeconds != 180 || got.Flow.MinAnalysisSeconds != 45 || !got.Flow.OnlySelected {
		t.Fatalf("flow settings mismatch: %+v", got.Flow)
	}
	if got.Unusual.Symbol != cfg.Unusual.Symbol || got.Unusual.Contract != cfg.Unusual.Contract {
		t.Fatalf("unusual filter mismatch: got=%+v want=%+v", got.Unusual, cfg.Unusual)
	}
	if got.Arbitrage.Formula != cfg.Arbitrage.Formula {
		t.Fatalf("arbitrage legacy formula mismatch: got=%q want=%q", got.Arbitrage.Formula, cfg.Arbitrage.Formula)
	}
	if len(got.Arbitrage.Pairs) != 2 {
		t.Fatalf("expected 2 arbitrage pairs, got %+v", got.Arbitrage.Pairs)
	}
	if got.Arbitrage.Pairs[0].ID != "spread-a" || got.Arbitrage.Pairs[0].Name != "A" {
		t.Fatalf("unexpected first arbitrage pair: %+v", got.Arbitrage.Pairs[0])
	}
	if got.Arbitrage.Pairs[1].Formula != "rb2605 - rb2610" {
		t.Fatalf("unexpected second arbitrage formula: %+v", got.Arbitrage.Pairs[1])
	}
}

func TestLoadPathInvalidJSONReturnsDefaultsAndError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	got, err := LoadPath(path)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse settings") {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Default()
	if got.DaysInYear != want.DaysInYear || got.RiskFreeRate != want.RiskFreeRate {
		t.Fatalf("expected defaults on parse error, got %+v", got)
	}
}

func TestNormalizeFallsBackInvalidFields(t *testing.T) {
	cfg := Default()
	cfg.RiskFreeRate = math.Inf(1)
	cfg.DaysInYear = 999
	cfg.GammaBucketFrontDays = 100
	cfg.GammaBucketMidDays = 90
	cfg.Options.DeltaAbsMin = -1
	cfg.Options.DeltaAbsMax = 0
	cfg.Unusual.TurnoverRatioThreshold = -0.1
	cfg.Unusual.Symbol = "  cu,ag "
	cfg.Unusual.Contract = " cu2604 "
	cfg.Flow.WindowSeconds = 10
	cfg.Flow.MinAnalysisSeconds = 500
	cfg.Flow.OnlySelected = true
	cfg.Flow.OnlyFocused = true
	cfg.Arbitrage.Formula = "  ma605 * 3 - eg2605 * 2  "
	cfg.Arbitrage.Pairs = []SettingsArbitragePair{
		{ID: " ", Name: "  first ", Formula: "  ma605 * 3 - eg2605 * 2  "},
		{ID: "dup", Name: "second", Formula: "rb2605-rb2610"},
		{ID: "dup", Name: "third", Formula: "IF2606-IF2607"},
		{ID: "empty-formula", Name: "drop", Formula: "   "},
	}
	cfg.Normalize()

	if cfg.RiskFreeRate != DefaultRiskFreeRate {
		t.Fatalf("risk_free_rate not normalized: %v", cfg.RiskFreeRate)
	}
	if cfg.DaysInYear != DefaultDaysInYear {
		t.Fatalf("days_in_year not normalized: %d", cfg.DaysInYear)
	}
	if cfg.GammaBucketFrontDays != DefaultGammaBucketFront || cfg.GammaBucketMidDays != DefaultGammaBucketMid {
		t.Fatalf("gamma buckets not normalized: front=%d mid=%d", cfg.GammaBucketFrontDays, cfg.GammaBucketMidDays)
	}
	if cfg.Options.DeltaAbsMin != 0.25 || cfg.Options.DeltaAbsMax != 0.5 {
		t.Fatalf("options deltas not normalized: %+v", cfg.Options)
	}
	if cfg.Flow.OnlySelected && cfg.Flow.OnlyFocused {
		t.Fatalf("exclusive flow flags not normalized: %+v", cfg.Flow)
	}
	if cfg.Unusual.Symbol != "cu,ag" || cfg.Unusual.Contract != "cu2604" {
		t.Fatalf("unexpected unusual filter normalization: %+v", cfg.Unusual)
	}
	if cfg.Arbitrage.Formula != "ma605 * 3 - eg2605 * 2" {
		t.Fatalf("arbitrage formula not normalized: %q", cfg.Arbitrage.Formula)
	}
	if len(cfg.Arbitrage.Pairs) != 3 {
		t.Fatalf("expected 3 normalized arbitrage pairs, got %+v", cfg.Arbitrage.Pairs)
	}
	if cfg.Arbitrage.Pairs[0].ID == "" || cfg.Arbitrage.Pairs[0].Name != "first" {
		t.Fatalf("unexpected normalized first pair: %+v", cfg.Arbitrage.Pairs[0])
	}
	if cfg.Arbitrage.Pairs[1].ID == cfg.Arbitrage.Pairs[2].ID {
		t.Fatalf("expected duplicate IDs to be resolved, got %+v", cfg.Arbitrage.Pairs)
	}
}

func TestDirUsesMetadataPathConvention(t *testing.T) {
	orig := userConfigDirFn
	defer func() { userConfigDirFn = orig }()
	userConfigDirFn = func() (string, error) { return "/tmp/x", nil }
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if dir != filepath.Join("/tmp/x", "starsling", "metadata") {
		t.Fatalf("unexpected dir: %s", dir)
	}
}
