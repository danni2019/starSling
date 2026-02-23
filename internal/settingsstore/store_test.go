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
	cfg.Flow.WindowSeconds = 180
	cfg.Flow.MinAnalysisSeconds = 45
	cfg.Flow.OnlySelected = true

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
	cfg.Flow.WindowSeconds = 10
	cfg.Flow.MinAnalysisSeconds = 500
	cfg.Flow.OnlySelected = true
	cfg.Flow.OnlyFocused = true
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
