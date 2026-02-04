package router

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const staleAfter = 2 * time.Second

type State struct {
	mu          sync.RWMutex
	market      MarketSnapshot
	options     OptionsSnapshot
	focusSymbol string
	marketSeq   int64
	optionsSeq  int64
	lastMarket  time.Time
	lastOptions time.Time
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
	return ViewSnapshot{
		Market:  market,
		Options: options,
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
		FocusSymbol: s.focusSymbol,
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
