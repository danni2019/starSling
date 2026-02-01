package tui

import "time"

type MarketRow struct {
	Symbol string
	Last   string
	Chg    string
	Bid    string
	Ask    string
	Vol    string
	OI     string
	TS     string
}

type TradeRow struct {
	Time   string
	Sym    string
	CP     string
	Strike string
	Price  string
	Size   string
	IV     string
	Tag    string
}

type LogLine struct {
	Time    string
	Message string
}

type MockData struct {
	MarketRows []MarketRow
	Trades     []TradeRow
	Logs       []LogLine
}

func mockData() MockData {
	return MockData{
		MarketRows: []MarketRow{
			{Symbol: "ag2604", Last: "31490", Chg: "+120", Bid: "31480", Ask: "31500", Vol: "9.2k", OI: "82k", TS: "21:07:13"},
			{Symbol: "ag2605", Last: "31340", Chg: "+80", Bid: "31330", Ask: "31350", Vol: "6.1k", OI: "75k", TS: "21:07:13"},
			{Symbol: "ag2606", Last: "31210", Chg: "+40", Bid: "31200", Ask: "31220", Vol: "4.9k", OI: "68k", TS: "21:07:12"},
			{Symbol: "au2604", Last: "482.10", Chg: "+1.25", Bid: "482.05", Ask: "482.15", Vol: "3.4k", OI: "41k", TS: "21:07:12"},
			{Symbol: "sc2603", Last: "502.6", Chg: "+4.8", Bid: "502.5", Ask: "502.7", Vol: "12.7k", OI: "66k", TS: "21:07:12"},
			{Symbol: "cu2603", Last: "72840", Chg: "+210", Bid: "72830", Ask: "72850", Vol: "8.9k", OI: "93k", TS: "21:07:11"},
		},
		Trades: []TradeRow{
			{Time: "21:07:13", Sym: "AG2604", CP: "P", Strike: "31000", Price: "82", Size: "120", IV: "0.46", Tag: ""},
			{Time: "21:07:12", Sym: "AG2604", CP: "C", Strike: "32000", Price: "55", Size: "300", IV: "0.42", Tag: ""},
			{Time: "21:07:11", Sym: "AG2605", CP: "P", Strike: "30000", Price: "90", Size: "80", IV: "0.51", Tag: ""},
			{Time: "21:07:10", Sym: "AG2604", CP: "C", Strike: "33000", Price: "33", Size: "500", IV: "0.39", Tag: ""},
		},
		Logs: []LogLine{
			{Time: "21:07:10", Message: "session-check: connected (gateway ok)"},
			{Time: "21:07:10", Message: "market-stream: subscribed 24 symbols"},
			{Time: "21:07:11", Message: "strategy: curve_flattening_detector started"},
			{Time: "21:07:12", Message: "alert: AG near-month backwardation widened"},
			{Time: "21:07:13", Message: "calc: IV snapshot updated (1.0s)"},
		},
	}
}

func (d MockData) Tick() MockData {
	now := time.Now().Format("15:04:05")
	d.Logs = append([]LogLine{{Time: now, Message: "heartbeat: UI refresh tick"}}, d.Logs...)
	if len(d.Logs) > 8 {
		d.Logs = d.Logs[:8]
	}
	d.Trades = append([]TradeRow{{Time: now, Sym: "AG2604", CP: "P", Strike: "31000", Price: "84", Size: "60", IV: "0.45", Tag: ""}}, d.Trades...)
	if len(d.Trades) > 8 {
		d.Trades = d.Trades[:8]
	}
	return d
}
