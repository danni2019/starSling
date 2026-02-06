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
	Market  MarketSnapshot  `json:"market"`
	Curve   CurveSnapshot   `json:"curve"`
	Options OptionsSnapshot `json:"options"`
	Unusual UnusualSnapshot `json:"unusual"`
	Logs    LogSnapshot     `json:"logs"`
}

type UIState struct {
	FocusSymbol            string  `json:"focus_symbol"`
	TurnoverChgThreshold   float64 `json:"turnover_chg_threshold"`
	TurnoverRatioThreshold float64 `json:"turnover_ratio_threshold"`
}

type OptionsSnapshot struct {
	SchemaVersion int              `json:"schema_version"`
	TS            int64            `json:"ts"`
	Seq           int64            `json:"seq"`
	Rows          []map[string]any `json:"rows"`
	Stale         bool             `json:"stale,omitempty"`
}

type CurveSnapshot struct {
	SchemaVersion int              `json:"schema_version"`
	TS            int64            `json:"ts"`
	Seq           int64            `json:"seq"`
	Rows          []map[string]any `json:"rows"`
	Stale         bool             `json:"stale,omitempty"`
}

type UnusualSnapshot struct {
	SchemaVersion int              `json:"schema_version"`
	TS            int64            `json:"ts"`
	Seq           int64            `json:"seq"`
	Rows          []map[string]any `json:"rows"`
	Stale         bool             `json:"stale,omitempty"`
}

type LogLine struct {
	TS      int64  `json:"ts"`
	Level   string `json:"level"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

type LogSnapshot struct {
	SchemaVersion int       `json:"schema_version"`
	TS            int64     `json:"ts"`
	Seq           int64     `json:"seq"`
	Items         []LogLine `json:"items"`
	Stale         bool      `json:"stale,omitempty"`
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

type SetUnusualThresholdParams struct {
	TurnoverChgThreshold   float64 `json:"turnover_chg_threshold"`
	TurnoverRatioThreshold float64 `json:"turnover_ratio_threshold"`
}
