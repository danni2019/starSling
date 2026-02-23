package router

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const staleAfter = 2 * time.Second
const logBufferSize = 200
const defaultTurnoverChgThreshold = 100000.0
const defaultTurnoverRatioThreshold = 0.05
const defaultOIRatioThreshold = 0.05

type State struct {
	mu       sync.RWMutex
	market   MarketSnapshot
	curve    CurveSnapshot
	options  OptionsSnapshot
	overview OverviewSnapshot
	unusual  UnusualSnapshot
	logs     []LogLine

	focusSymbol            string
	turnoverChgThreshold   float64
	turnoverRatioThreshold float64
	oiRatioThreshold       float64

	marketSeq   int64
	curveSeq    int64
	optionsSeq  int64
	unusualSeq  int64
	logSeq      int64
	overviewSeq int64

	lastMarket  time.Time
	lastCurve   time.Time
	lastOptions time.Time
	lastUnusual time.Time
	lastLogs    time.Time
}

func NewState() *State {
	return &State{
		market: MarketSnapshot{
			SchemaVersion: 1,
			RowKey:        "ctp_contract",
			Columns:       []string{},
			Rows:          []map[string]any{},
		},
		options: OptionsSnapshot{
			SchemaVersion: 1,
			Rows:          []map[string]any{},
		},
		curve: CurveSnapshot{
			SchemaVersion: 1,
			Rows:          []map[string]any{},
		},
		overview: OverviewSnapshot{
			SchemaVersion: 1,
			Rows:          []OverviewRow{},
		},
		unusual: UnusualSnapshot{
			SchemaVersion: 1,
			Rows:          []map[string]any{},
		},
		logs:                   []LogLine{},
		turnoverChgThreshold:   defaultTurnoverChgThreshold,
		turnoverRatioThreshold: defaultTurnoverRatioThreshold,
		oiRatioThreshold:       defaultOIRatioThreshold,
	}
}

func (s *State) UpdateMarket(snapshot MarketSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.marketSeq++
	s.overviewSeq++
	s.market = snapshot
	s.market.SchemaVersion = 1
	s.market.Seq = s.marketSeq
	if s.market.RowKey == "" {
		s.market.RowKey = "ctp_contract"
	}
	s.market.Stale = false
	s.lastMarket = time.Now()
	s.rebuildOverviewLocked()
}

func (s *State) GetViewSnapshot(focusSymbol string) ViewSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(focusSymbol) == "" {
		focusSymbol = s.focusSymbol
	}

	market := s.market
	market.Rows = filterNonOptionMarketRows(s.market.Rows)
	if !s.lastMarket.IsZero() && time.Since(s.lastMarket) > staleAfter {
		market.Stale = true
	}
	options := s.options
	options.Rows = filterOptionsRows(s.options.Rows, focusSymbol)
	if !s.lastOptions.IsZero() && time.Since(s.lastOptions) > staleAfter {
		options.Stale = true
	}
	curve := s.curve
	if !s.lastCurve.IsZero() && time.Since(s.lastCurve) > staleAfter {
		curve.Stale = true
	}
	unusual := s.unusual
	if !s.lastUnusual.IsZero() && time.Since(s.lastUnusual) > staleAfter {
		unusual.Stale = true
	}
	overview := s.overview
	if market.Stale || options.Stale {
		overview.Stale = true
	}
	logSnapshot := LogSnapshot{
		SchemaVersion: 1,
		Seq:           s.logSeq,
		Items:         append([]LogLine(nil), s.logs...),
	}
	if !s.lastLogs.IsZero() {
		logSnapshot.TS = s.lastLogs.UnixMilli()
		if time.Since(s.lastLogs) > staleAfter {
			logSnapshot.Stale = true
		}
	}
	return ViewSnapshot{
		Market:   market,
		Curve:    curve,
		Options:  options,
		Unusual:  unusual,
		Overview: overview,
		Logs:     logSnapshot,
	}
}

func (s *State) GetLatestMarket(minSeq int64) (MarketSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.market.Seq <= minSeq {
		return MarketSnapshot{Seq: s.market.Seq}, true
	}
	return s.market, false
}

func (s *State) SetFocusSymbol(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focusSymbol = strings.TrimSpace(symbol)
}

func (s *State) GetUIState() UIState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return UIState{
		FocusSymbol:            s.focusSymbol,
		TurnoverChgThreshold:   s.turnoverChgThreshold,
		TurnoverRatioThreshold: s.turnoverRatioThreshold,
		OIRatioThreshold:       s.oiRatioThreshold,
	}
}

func (s *State) UpdateOptions(snapshot OptionsSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.optionsSeq++
	s.overviewSeq++
	s.options = snapshot
	s.options.SchemaVersion = 1
	s.options.Seq = s.optionsSeq
	s.options.Stale = false
	s.lastOptions = time.Now()
	s.rebuildOverviewLocked()
}

func (s *State) UpdateCurve(snapshot CurveSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.curveSeq++
	s.curve = snapshot
	s.curve.SchemaVersion = 1
	s.curve.Seq = s.curveSeq
	s.curve.Stale = false
	s.lastCurve = time.Now()
}

func (s *State) UpdateUnusual(snapshot UnusualSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unusualSeq++
	snapshot.Rows = enrichUnusualRowsWithGreeks(snapshot.Rows, s.options.Rows)
	s.unusual = snapshot
	s.unusual.SchemaVersion = 1
	s.unusual.Seq = s.unusualSeq
	s.unusual.Stale = false
	s.lastUnusual = time.Now()
}

var unusualGreekFields = []string{
	"iv",
	"delta",
	"gamma",
	"theta",
	"vega",
}

func enrichUnusualRowsWithGreeks(unusualRows, optionsRows []map[string]any) []map[string]any {
	if len(unusualRows) == 0 || len(optionsRows) == 0 {
		return unusualRows
	}
	optionsByContract := make(map[string]map[string]any, len(optionsRows))
	for _, optionRow := range optionsRows {
		contract := strings.ToLower(strings.TrimSpace(toString(optionRow["ctp_contract"])))
		if contract == "" {
			continue
		}
		if _, exists := optionsByContract[contract]; exists {
			continue
		}
		optionsByContract[contract] = optionRow
	}
	if len(optionsByContract) == 0 {
		return unusualRows
	}
	enriched := make([]map[string]any, 0, len(unusualRows))
	for _, unusualRow := range unusualRows {
		contract := strings.ToLower(strings.TrimSpace(toString(unusualRow["ctp_contract"])))
		optionRow, ok := optionsByContract[contract]
		if !ok {
			enriched = append(enriched, unusualRow)
			continue
		}
		merged := make(map[string]any, len(unusualRow)+len(unusualGreekFields))
		for key, value := range unusualRow {
			merged[key] = value
		}
		for _, greekField := range unusualGreekFields {
			if value, exists := optionRow[greekField]; exists {
				merged[greekField] = value
			}
		}
		enriched = append(enriched, merged)
	}
	return enriched
}

func (s *State) AppendLog(line LogLine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logSeq++
	if line.TS == 0 {
		line.TS = time.Now().UnixMilli()
	}
	s.logs = append([]LogLine{line}, s.logs...)
	if len(s.logs) > logBufferSize {
		s.logs = s.logs[:logBufferSize]
	}
	s.lastLogs = time.Now()
}

func (s *State) SetUnusualThresholds(chgThreshold, ratioThreshold, oiRatioThreshold float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if chgThreshold > 0 {
		s.turnoverChgThreshold = chgThreshold
	}
	if ratioThreshold > 0 {
		s.turnoverRatioThreshold = ratioThreshold
	}
	if oiRatioThreshold > 0 {
		s.oiRatioThreshold = oiRatioThreshold
	}
}

func filterOptionsRows(rows []map[string]any, focus string) []map[string]any {
	if strings.TrimSpace(focus) == "" {
		return rows
	}
	focus = strings.TrimSpace(focus)
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		contract := strings.TrimSpace(toString(row["ctp_contract"]))
		underlying := strings.TrimSpace(toString(row["underlying"]))
		symbol := strings.TrimSpace(toString(row["symbol"]))
		if strings.EqualFold(contract, focus) || strings.EqualFold(underlying, focus) || strings.EqualFold(symbol, focus) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterNonOptionMarketRows(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		productClass := strings.TrimSpace(toString(row["product_class"]))
		if strings.EqualFold(productClass, "2") {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func (s *State) buildOverviewSnapshotLocked() OverviewSnapshot {
	rows := buildOverviewRows(s.market.Rows, s.options.Rows)
	return OverviewSnapshot{
		SchemaVersion: 1,
		TS:            time.Now().UnixMilli(),
		Seq:           s.overviewSeq,
		Rows:          rows,
	}
}

func (s *State) rebuildOverviewLocked() {
	s.overview = s.buildOverviewSnapshotLocked()
}

func buildOverviewRows(marketRows, optionsRows []map[string]any) []OverviewRow {
	type futuresAgg struct {
		DisplaySymbol string

		HasTurnover bool
		Turnover    float64
		HasOIPair   bool
		OI          float64
		PreOI       float64
	}
	type optionsAgg struct {
		HasAny bool

		HasCInv  bool
		CInv     float64
		HasCFnt  bool
		CFnt     float64
		HasCMid  bool
		CMid     float64
		HasCBack bool
		CBack    float64

		HasPInv  bool
		PInv     float64
		HasPFnt  bool
		PFnt     float64
		HasPMid  bool
		PMid     float64
		HasPBack bool
		PBack    float64
	}

	futuresBySymbol := make(map[string]*futuresAgg)
	optionsBySymbol := make(map[string]*optionsAgg)
	marketByContract := make(map[string]map[string]any, len(marketRows))

	for _, row := range marketRows {
		contract := strings.ToLower(strings.TrimSpace(toString(row["ctp_contract"])))
		if contract != "" {
			marketByContract[contract] = row
		}

		if !strings.EqualFold(strings.TrimSpace(toString(row["product_class"])), "1") {
			continue
		}
		symbol := strings.TrimSpace(toString(row["symbol"]))
		if symbol == "" {
			continue
		}
		symbolKey := strings.ToLower(symbol)
		agg := futuresBySymbol[symbolKey]
		if agg == nil {
			agg = &futuresAgg{}
			futuresBySymbol[symbolKey] = agg
		}
		if agg.DisplaySymbol == "" {
			agg.DisplaySymbol = symbol
		}
		if turnover, ok := toOptionalFiniteFloat(row["turnover"]); ok {
			agg.HasTurnover = true
			agg.Turnover += turnover
		}
		oi, hasOI := toOptionalFiniteFloat(row["open_interest"])
		preOI, hasPreOI := toOptionalFiniteFloat(row["pre_open_interest"])
		if hasOI && hasPreOI && preOI > 0 {
			agg.HasOIPair = true
			agg.OI += oi
			agg.PreOI += preOI
		}
	}

	for _, row := range optionsRows {
		contract := strings.TrimSpace(toString(row["ctp_contract"]))
		if contract == "" {
			continue
		}
		marketRow := marketByContract[strings.ToLower(contract)]
		if marketRow == nil {
			continue
		}
		symbol := strings.TrimSpace(toString(row["symbol"]))
		if symbol == "" {
			symbol = strings.TrimSpace(toString(marketRow["symbol"]))
		}
		if symbol == "" {
			continue
		}
		symbolKey := strings.ToLower(symbol)
		cp := normalizeOptionTypeCPValue(toString(row["option_type"]))
		if cp == "" {
			cp = normalizeOptionTypeCPValue(toString(marketRow["option_type"]))
		}
		if cp != "c" && cp != "p" {
			continue
		}
		gamma, hasGamma := toOptionalFiniteFloat(row["gamma"])
		oi, hasOI := toOptionalFiniteFloat(marketRow["open_interest"])
		multiplier, hasMultiplier := toOptionalFiniteFloat(marketRow["multiplier"])
		if !hasGamma || !hasOI || !hasMultiplier || multiplier <= 0 {
			continue
		}
		tte, hasTTE := toOptionalFiniteFloat(row["tte"])
		if !hasTTE || tte <= 0 {
			continue
		}
		underlying := strings.TrimSpace(toString(row["underlying"]))
		if underlying == "" {
			underlying = strings.TrimSpace(toString(marketRow["underlying"]))
		}
		if underlying == "" {
			continue
		}
		underRow := marketByContract[strings.ToLower(underlying)]
		S, hasS := underlyingPriceForGamma(underRow)
		if !hasS || S <= 0 {
			continue
		}
		contrib := gamma * oi * multiplier * S * S
		agg := optionsBySymbol[symbolKey]
		if agg == nil {
			agg = &optionsAgg{}
			optionsBySymbol[symbolKey] = agg
		}
		agg.HasAny = true
		bucket := overviewTTEBucket(tte)
		if cp == "c" {
			agg.HasCInv = true
			agg.CInv += contrib
			switch bucket {
			case overviewBucketFront:
				agg.HasCFnt = true
				agg.CFnt += contrib
			case overviewBucketMid:
				agg.HasCMid = true
				agg.CMid += contrib
			case overviewBucketBack:
				agg.HasCBack = true
				agg.CBack += contrib
			}
		} else {
			agg.HasPInv = true
			agg.PInv += contrib
			switch bucket {
			case overviewBucketFront:
				agg.HasPFnt = true
				agg.PFnt += contrib
			case overviewBucketMid:
				agg.HasPMid = true
				agg.PMid += contrib
			case overviewBucketBack:
				agg.HasPBack = true
				agg.PBack += contrib
			}
		}
	}

	rowsOut := make([]OverviewRow, 0, len(futuresBySymbol))
	for symbolKey, agg := range futuresBySymbol {
		displaySymbol := strings.TrimSpace(agg.DisplaySymbol)
		if displaySymbol == "" {
			displaySymbol = symbolKey
		}
		row := OverviewRow{Symbol: displaySymbol}
		if agg.HasTurnover {
			row.Turnover = float64Ptr(agg.Turnover)
		}
		if agg.HasOIPair && agg.PreOI > 0 {
			value := agg.OI/agg.PreOI - 1.0
			row.OIChgPct = float64Ptr(value)
		}
		if optAgg, ok := optionsBySymbol[symbolKey]; ok && optAgg != nil && optAgg.HasAny {
			if optAgg.HasCInv {
				row.CGammaInv = float64Ptr(optAgg.CInv)
			}
			if optAgg.HasCFnt {
				row.CGammaFnt = float64Ptr(optAgg.CFnt)
			}
			if optAgg.HasCMid {
				row.CGammaMid = float64Ptr(optAgg.CMid)
			}
			if optAgg.HasCBack {
				row.CGammaBack = float64Ptr(optAgg.CBack)
			}
			if optAgg.HasPInv {
				row.PGammaInv = float64Ptr(optAgg.PInv)
			}
			if optAgg.HasPFnt {
				row.PGammaFnt = float64Ptr(optAgg.PFnt)
			}
			if optAgg.HasPMid {
				row.PGammaMid = float64Ptr(optAgg.PMid)
			}
			if optAgg.HasPBack {
				row.PGammaBack = float64Ptr(optAgg.PBack)
			}
		}
		rowsOut = append(rowsOut, row)
	}
	sort.Slice(rowsOut, func(i, j int) bool {
		return strings.ToLower(rowsOut[i].Symbol) < strings.ToLower(rowsOut[j].Symbol)
	})

	return rowsOut
}

func underlyingPriceForGamma(row map[string]any) (float64, bool) {
	if row == nil {
		return 0, false
	}
	bid, hasBid := toOptionalFiniteFloat(row["bid1"])
	ask, hasAsk := toOptionalFiniteFloat(row["ask1"])
	bidVol, hasBidVol := toOptionalFiniteFloat(row["bid_vol1"])
	askVol, hasAskVol := toOptionalFiniteFloat(row["ask_vol1"])
	if hasBid && hasAsk && hasBidVol && hasAskVol {
		depth := bidVol + askVol
		if depth > 0 {
			vwap := (bid*bidVol + ask*askVol) / depth
			if vwap > 0 && !math.IsNaN(vwap) && !math.IsInf(vwap, 0) {
				return vwap, true
			}
		}
	}
	if hasBid && hasAsk && bid > 0 && ask > 0 {
		mid := 0.5 * (bid + ask)
		if mid > 0 && !math.IsNaN(mid) && !math.IsInf(mid, 0) {
			return mid, true
		}
	}
	last, hasLast := toOptionalFiniteFloat(row["last"])
	if hasLast && last > 0 {
		return last, true
	}
	return 0, false
}

func normalizeOptionTypeCPValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "c", "call", "1", "认购":
		return "c"
	case "p", "put", "2", "认沽":
		return "p"
	default:
		return ""
	}
}

type overviewExpiryBucket int

const (
	overviewBucketUnknown overviewExpiryBucket = iota
	overviewBucketFront
	overviewBucketMid
	overviewBucketBack
)

func overviewTTEBucket(tte float64) overviewExpiryBucket {
	if math.IsNaN(tte) || math.IsInf(tte, 0) || tte <= 0 {
		return overviewBucketUnknown
	}
	if tte <= 30 {
		return overviewBucketFront
	}
	if tte <= 90 {
		return overviewBucketMid
	}
	return overviewBucketBack
}

func float64Ptr(value float64) *float64 {
	v := value
	return &v
}

func toOptionalFiniteFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", value)
	}
}
