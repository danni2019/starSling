package tui

type MarketRow struct {
	Symbol   string
	Exchange string
	Last     string
	Chg      string
	ChgPct   string
	BidVol   string
	Bid      string
	Ask      string
	AskVol   string
	Vol      string
	Turnover string
	OI       string
	OIChgPct string
	TS       string
}

type TradeRow struct {
	Time   string
	Sym    string
	CP     string
	Strike string
	TTE    string
	Price  string
	Size   string
	IV     string
	Tag    string
}
