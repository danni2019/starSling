package router

type MarketSnapshot struct {
	SchemaVersion int              `json:"schema_version"`
	TS            int64            `json:"ts"`
	Seq           int64            `json:"seq"`
	RowKey        string           `json:"row_key"`
	Columns       []string         `json:"columns"`
	Rows          []map[string]any `json:"rows"`
	Stale         bool             `json:"stale,omitempty"`
}

type ViewSnapshot struct {
	Market MarketSnapshot `json:"market"`
}

type UIState struct {
	FocusSymbol string `json:"focus_symbol"`
}

type GetLatestMarketParams struct {
	MinSeq int64 `json:"min_seq"`
}

type GetViewSnapshotParams struct {
	FocusSymbol string         `json:"focus_symbol"`
	Limits      map[string]int `json:"limits"`
}

type SetFocusSymbolParams struct {
	Symbol string `json:"symbol"`
}
