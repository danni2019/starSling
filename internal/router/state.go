package router

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const staleAfter = 2 * time.Second
const logBufferSize = 200
const defaultTurnoverChgThreshold = 100000.0
const defaultTurnoverRatioThreshold = 0.05

type State struct {
	mu      sync.RWMutex
	market  MarketSnapshot
	curve   CurveSnapshot
	options OptionsSnapshot
	unusual UnusualSnapshot
	logs    []LogLine

	focusSymbol            string
	turnoverChgThreshold   float64
	turnoverRatioThreshold float64

	marketSeq  int64
	curveSeq   int64
	optionsSeq int64
	unusualSeq int64
	logSeq     int64

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
		unusual: UnusualSnapshot{
			SchemaVersion: 1,
			Rows:          []map[string]any{},
		},
		logs:                   []LogLine{},
		turnoverChgThreshold:   defaultTurnoverChgThreshold,
		turnoverRatioThreshold: defaultTurnoverRatioThreshold,
	}
}

func (s *State) UpdateMarket(snapshot MarketSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.marketSeq++
	s.market = snapshot
	s.market.SchemaVersion = 1
	s.market.Seq = s.marketSeq
	if s.market.RowKey == "" {
		s.market.RowKey = "ctp_contract"
	}
	s.market.Stale = false
	s.lastMarket = time.Now()
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
		Market:  market,
		Curve:   curve,
		Options: options,
		Unusual: unusual,
		Logs:    logSnapshot,
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
	}
}

func (s *State) UpdateOptions(snapshot OptionsSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.optionsSeq++
	s.options = snapshot
	s.options.SchemaVersion = 1
	s.options.Seq = s.optionsSeq
	s.options.Stale = false
	s.lastOptions = time.Now()
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
	s.unusual = snapshot
	s.unusual.SchemaVersion = 1
	s.unusual.Seq = s.unusualSeq
	s.unusual.Stale = false
	s.lastUnusual = time.Now()
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

func (s *State) SetUnusualThresholds(chgThreshold, ratioThreshold float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if chgThreshold > 0 {
		s.turnoverChgThreshold = chgThreshold
	}
	if ratioThreshold > 0 {
		s.turnoverRatioThreshold = ratioThreshold
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
