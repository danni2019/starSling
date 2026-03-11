package settingsstore

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	schemaVersion            = 1
	settingsFileName         = "global_settings.json"
	DefaultRiskFreeRate      = 0.01
	DefaultDaysInYear        = 365
	DefaultGammaBucketFront  = 30
	DefaultGammaBucketMid    = 90
	minPositiveDays          = 1
	maxReasonableDaysInYear  = 370
	maxReasonableGammaBucket = 370
)

var userConfigDirFn = os.UserConfigDir

type Settings struct {
	SchemaVersion int `json:"schema_version"`

	RiskFreeRate         float64 `json:"risk_free_rate"`
	DaysInYear           int     `json:"days_in_year"`
	GammaBucketFrontDays int     `json:"gamma_bucket_front_days"`
	GammaBucketMidDays   int     `json:"gamma_bucket_mid_days"`

	Overview  SettingsOverview  `json:"settings_overview"`
	Market    SettingsMarket    `json:"settings_market"`
	Options   SettingsOptions   `json:"settings_options"`
	Unusual   SettingsUnusual   `json:"settings_unusual"`
	Flow      SettingsFlow      `json:"settings_flow"`
	Arbitrage SettingsArbitrage `json:"settings_arbitrage"`
}

type SettingsOverview struct {
	SortBy         string `json:"sort_by"`
	SortAsc        bool   `json:"sort_asc"`
	RequireOptions bool   `json:"require_options"`
}

type SettingsMarket struct {
	Exchange string `json:"exchange"`
	Class    string `json:"class"`
	Symbol   string `json:"symbol"`
	Contract string `json:"contract"`
	MainOnly bool   `json:"main_only"`
	SortBy   string `json:"sort_by"`
	SortAsc  bool   `json:"sort_asc"`
}

type SettingsOptions struct {
	DeltaEnabled bool    `json:"delta_enabled"`
	DeltaAbsMin  float64 `json:"delta_abs_min"`
	DeltaAbsMax  float64 `json:"delta_abs_max"`
}

type SettingsUnusual struct {
	TurnoverChgThreshold   float64 `json:"turnover_chg_threshold"`
	TurnoverRatioThreshold float64 `json:"turnover_ratio_threshold"`
	OIRatioThreshold       float64 `json:"oi_ratio_threshold"`
	Symbol                 string  `json:"symbol"`
	Contract               string  `json:"contract"`
}

type SettingsFlow struct {
	WindowSeconds      int  `json:"window_seconds"`
	MinAnalysisSeconds int  `json:"min_analysis_seconds"`
	OnlySelected       bool `json:"only_selected"`
	OnlyFocused        bool `json:"only_focused"`
}

type SettingsArbitrage struct {
	Formula string                  `json:"formula"`
	Pairs   []SettingsArbitragePair `json:"pairs"`
}

type SettingsArbitragePair struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Formula string `json:"formula"`
}

func Default() Settings {
	return Settings{
		SchemaVersion:        schemaVersion,
		RiskFreeRate:         DefaultRiskFreeRate,
		DaysInYear:           DefaultDaysInYear,
		GammaBucketFrontDays: DefaultGammaBucketFront,
		GammaBucketMidDays:   DefaultGammaBucketMid,
		Overview:             SettingsOverview{SortBy: "turnover", SortAsc: false, RequireOptions: false},
		Market:               SettingsMarket{SortBy: "vol", SortAsc: false},
		Options:              SettingsOptions{DeltaEnabled: false, DeltaAbsMin: 0.25, DeltaAbsMax: 0.5},
		Unusual:              SettingsUnusual{TurnoverChgThreshold: 100000.0, TurnoverRatioThreshold: 0.05, OIRatioThreshold: 0.05, Symbol: "", Contract: ""},
		Flow:                 SettingsFlow{WindowSeconds: 120, MinAnalysisSeconds: 30, OnlySelected: false, OnlyFocused: false},
		Arbitrage:            SettingsArbitrage{Formula: "", Pairs: nil},
	}
}

func Dir() (string, error) {
	if base, err := userConfigDirFn(); err == nil && strings.TrimSpace(base) != "" {
		return filepath.Join(base, "starsling", "metadata"), nil
	}
	return filepath.Join("runtime", "metadata"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, settingsFileName), nil
}

func Load() (Settings, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	return LoadPath(path)
}

func LoadPath(path string) (Settings, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read settings: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parse settings: %w", err)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		// Return normalized defaults if persisted values are still invalid after normalization.
		def := Default()
		return def, fmt.Errorf("invalid settings: %w", err)
	}
	return cfg, nil
}

func Save(cfg Settings) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SavePath(path, cfg)
}

func SavePath(path string, cfg Settings) error {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	cfg.SchemaVersion = schemaVersion
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

func (s *Settings) Normalize() {
	if s == nil {
		return
	}
	defaults := Default()
	if s.SchemaVersion == 0 {
		s.SchemaVersion = schemaVersion
	}
	if !isFinite(s.RiskFreeRate) {
		s.RiskFreeRate = defaults.RiskFreeRate
	}
	if s.DaysInYear <= 0 || s.DaysInYear > maxReasonableDaysInYear {
		s.DaysInYear = defaults.DaysInYear
	}
	if s.GammaBucketFrontDays <= 0 || s.GammaBucketFrontDays > maxReasonableGammaBucket {
		s.GammaBucketFrontDays = defaults.GammaBucketFrontDays
	}
	if s.GammaBucketMidDays <= 0 || s.GammaBucketMidDays > maxReasonableGammaBucket || s.GammaBucketMidDays <= s.GammaBucketFrontDays {
		s.GammaBucketMidDays = defaults.GammaBucketMidDays
		if s.GammaBucketMidDays <= s.GammaBucketFrontDays {
			s.GammaBucketFrontDays = defaults.GammaBucketFrontDays
		}
	}

	s.Overview.SortBy = strings.ToLower(strings.TrimSpace(s.Overview.SortBy))
	if s.Overview.SortBy != "turnover" && s.Overview.SortBy != "oi_chg" {
		s.Overview.SortBy = defaults.Overview.SortBy
	}

	s.Market.Exchange = strings.TrimSpace(s.Market.Exchange)
	s.Market.Class = strings.TrimSpace(s.Market.Class)
	s.Market.Symbol = strings.TrimSpace(s.Market.Symbol)
	s.Market.Contract = strings.TrimSpace(s.Market.Contract)
	s.Market.SortBy = strings.ToLower(strings.TrimSpace(s.Market.SortBy))
	if s.Market.SortBy == "" {
		s.Market.SortBy = defaults.Market.SortBy
	}

	if !isFinite(s.Options.DeltaAbsMin) || s.Options.DeltaAbsMin <= 0 {
		s.Options.DeltaAbsMin = defaults.Options.DeltaAbsMin
	}
	if !isFinite(s.Options.DeltaAbsMax) || s.Options.DeltaAbsMax <= 0 {
		s.Options.DeltaAbsMax = defaults.Options.DeltaAbsMax
	}
	if s.Options.DeltaAbsMax < s.Options.DeltaAbsMin {
		s.Options.DeltaAbsMin = defaults.Options.DeltaAbsMin
		s.Options.DeltaAbsMax = defaults.Options.DeltaAbsMax
	}

	if !isFinite(s.Unusual.TurnoverChgThreshold) || s.Unusual.TurnoverChgThreshold <= 0 {
		s.Unusual.TurnoverChgThreshold = defaults.Unusual.TurnoverChgThreshold
	}
	if !isFinite(s.Unusual.TurnoverRatioThreshold) || s.Unusual.TurnoverRatioThreshold <= 0 {
		s.Unusual.TurnoverRatioThreshold = defaults.Unusual.TurnoverRatioThreshold
	}
	if !isFinite(s.Unusual.OIRatioThreshold) || s.Unusual.OIRatioThreshold <= 0 {
		s.Unusual.OIRatioThreshold = defaults.Unusual.OIRatioThreshold
	}
	s.Unusual.Symbol = strings.TrimSpace(s.Unusual.Symbol)
	s.Unusual.Contract = strings.TrimSpace(s.Unusual.Contract)

	if s.Flow.WindowSeconds < 60 || s.Flow.WindowSeconds > 300 {
		s.Flow.WindowSeconds = defaults.Flow.WindowSeconds
	}
	if s.Flow.MinAnalysisSeconds < 15 || s.Flow.MinAnalysisSeconds > 60 || s.Flow.MinAnalysisSeconds > s.Flow.WindowSeconds {
		s.Flow.MinAnalysisSeconds = defaults.Flow.MinAnalysisSeconds
		if s.Flow.MinAnalysisSeconds > s.Flow.WindowSeconds {
			s.Flow.WindowSeconds = defaults.Flow.WindowSeconds
		}
	}
	if s.Flow.OnlySelected && s.Flow.OnlyFocused {
		s.Flow.OnlyFocused = false
	}

	s.Arbitrage.Formula = strings.TrimSpace(s.Arbitrage.Formula)
	normalizedPairs := make([]SettingsArbitragePair, 0, len(s.Arbitrage.Pairs))
	usedIDs := make(map[string]struct{}, len(s.Arbitrage.Pairs))
	for _, pair := range s.Arbitrage.Pairs {
		formula := strings.TrimSpace(pair.Formula)
		if formula == "" {
			continue
		}
		name := strings.TrimSpace(pair.Name)
		id := strings.TrimSpace(pair.ID)
		if id == "" {
			id = fmt.Sprintf("arb-%03d", len(normalizedPairs)+1)
		}
		baseID := id
		suffix := 2
		for {
			key := strings.ToLower(strings.TrimSpace(id))
			if _, exists := usedIDs[key]; !exists && key != "" {
				usedIDs[key] = struct{}{}
				break
			}
			id = fmt.Sprintf("%s-%d", baseID, suffix)
			suffix++
		}
		normalizedPairs = append(normalizedPairs, SettingsArbitragePair{
			ID:      id,
			Name:    name,
			Formula: formula,
		})
	}
	s.Arbitrage.Pairs = normalizedPairs
}

func (s Settings) Validate() error {
	if !isFinite(s.RiskFreeRate) {
		return fmt.Errorf("risk_free_rate must be a finite number")
	}
	if s.DaysInYear < minPositiveDays || s.DaysInYear > maxReasonableDaysInYear {
		return fmt.Errorf("days_in_year must be in (0, %d]", maxReasonableDaysInYear)
	}
	if s.GammaBucketFrontDays < minPositiveDays || s.GammaBucketFrontDays > maxReasonableGammaBucket {
		return fmt.Errorf("gamma_bucket_front_days must be in (0, %d]", maxReasonableGammaBucket)
	}
	if s.GammaBucketMidDays < minPositiveDays || s.GammaBucketMidDays > maxReasonableGammaBucket {
		return fmt.Errorf("gamma_bucket_mid_days must be in (0, %d]", maxReasonableGammaBucket)
	}
	if s.GammaBucketMidDays <= s.GammaBucketFrontDays {
		return fmt.Errorf("gamma_bucket_mid_days must be > gamma_bucket_front_days")
	}
	return nil
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
