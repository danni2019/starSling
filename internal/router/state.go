package router

import (
	"strings"
	"sync"
	"time"
)

const staleAfter = 2 * time.Second

type State struct {
	mu          sync.RWMutex
	market      MarketSnapshot
	focusSymbol string
	marketSeq   int64
	lastMarket  time.Time
}

func NewState() *State {
	return &State{
		market: MarketSnapshot{
			SchemaVersion: 1,
			RowKey:        "ctp_contract",
			Columns:       []string{},
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

	market := s.market
	if !s.lastMarket.IsZero() && time.Since(s.lastMarket) > staleAfter {
		market.Stale = true
	}
	return ViewSnapshot{Market: market}
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
