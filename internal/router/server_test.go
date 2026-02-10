package router

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/danni2019/starSling/internal/ipc"
)

func TestRouterMarketSnapshotRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := NewState()
	server, err := Start(ctx, "127.0.0.1:0", state, nil)
	if err != nil {
		t.Fatalf("start router: %v", err)
	}
	defer server.Stop(context.Background())

	client := ipc.NewClient(server.Addr())
	client.Timeout = time.Second

	payload := MarketSnapshot{
		SchemaVersion: 1,
		TS:            time.Now().UnixMilli(),
		RowKey:        "ctp_contract",
		Columns:       []string{"ctp_contract", "product_class", "last", "volume"},
		Rows: []map[string]any{
			{"ctp_contract": "cu2604", "product_class": "1", "last": 104550.0, "volume": 55848},
			{"ctp_contract": "ag2604C30000", "product_class": "2", "last": 31490.0, "volume": 1200},
		},
	}
	if err := client.Notify(context.Background(), "market.snapshot", payload); err != nil {
		t.Fatalf("notify market.snapshot: %v", err)
	}

	view, err := waitViewSnapshot(client)
	if err != nil {
		t.Fatalf("get_view_snapshot: %v", err)
	}
	if view.Market.Seq == 0 {
		t.Fatalf("expected seq > 0")
	}
	var latestChanged MarketSnapshot
	if err := client.Call(context.Background(), "router.get_latest_market", GetLatestMarketParams{MinSeq: 0}, &latestChanged); err != nil {
		t.Fatalf("get_latest_market changed: %v", err)
	}
	if latestChanged.Seq != view.Market.Seq {
		t.Fatalf("expected latest seq %d, got %d", view.Market.Seq, latestChanged.Seq)
	}
	var latestUnchanged map[string]any
	if err := client.Call(context.Background(), "router.get_latest_market", GetLatestMarketParams{MinSeq: view.Market.Seq}, &latestUnchanged); err != nil {
		t.Fatalf("get_latest_market unchanged: %v", err)
	}
	if unchanged, ok := latestUnchanged["unchanged"].(bool); !ok || !unchanged {
		t.Fatalf("expected unchanged=true, got %#v", latestUnchanged)
	}
	if len(view.Market.Rows) != 1 {
		t.Fatalf("expected non-option rows in view snapshot, got %d rows", len(view.Market.Rows))
	}
	if got := view.Market.Rows[0]["ctp_contract"]; got != "cu2604" {
		t.Fatalf("unexpected row in view snapshot: %v", got)
	}
	if len(latestChanged.Rows) != 2 {
		t.Fatalf("expected latest market rows to be full set, got %d", len(latestChanged.Rows))
	}

	optPayload := OptionsSnapshot{
		SchemaVersion: 1,
		TS:            time.Now().UnixMilli(),
		Rows: []map[string]any{
			{"ctp_contract": "cu2604C72000", "underlying": "cu2604", "symbol": "cu", "strike": 72000.0, "iv": 0.22},
			{"ctp_contract": "ag2604C30000", "underlying": "ag2604", "symbol": "ag", "strike": 30000.0, "iv": 0.30},
		},
	}
	if err := client.Notify(context.Background(), "options.snapshot", optPayload); err != nil {
		t.Fatalf("notify options.snapshot: %v", err)
	}
	var filtered ViewSnapshot
	if err := client.Call(context.Background(), "router.get_view_snapshot", GetViewSnapshotParams{FocusSymbol: "cu2604"}, &filtered); err != nil {
		t.Fatalf("get_view_snapshot with focus: %v", err)
	}
	if len(filtered.Options.Rows) != 1 {
		t.Fatalf("expected 1 option row for focus, got %d", len(filtered.Options.Rows))
	}
	if err := client.Notify(context.Background(), "curve.snapshot", CurveSnapshot{
		SchemaVersion: 1,
		TS:            time.Now().UnixMilli(),
		Rows:          []map[string]any{{"ctp_contract": "cu2604", "forward": 104550.0, "vix": 0.2}},
	}); err != nil {
		t.Fatalf("notify curve.snapshot: %v", err)
	}
	if err := client.Notify(context.Background(), "unusual.snapshot", UnusualSnapshot{
		SchemaVersion: 1,
		TS:            time.Now().UnixMilli(),
		Rows:          []map[string]any{{"ctp_contract": "cu2604C72000", "turnover_chg": 120000.0, "turnover_ratio": 0.1}},
	}); err != nil {
		t.Fatalf("notify unusual.snapshot: %v", err)
	}
	if err := client.Notify(context.Background(), "log.append", LogLine{
		TS:      time.Now().UnixMilli(),
		Level:   "INFO",
		Source:  "worker",
		Message: "test log",
	}); err != nil {
		t.Fatalf("notify log.append: %v", err)
	}
	var withSections ViewSnapshot
	if err := client.Call(context.Background(), "router.get_view_snapshot", GetViewSnapshotParams{}, &withSections); err != nil {
		t.Fatalf("get_view_snapshot full sections: %v", err)
	}
	if len(withSections.Curve.Rows) != 1 {
		t.Fatalf("expected 1 curve row, got %d", len(withSections.Curve.Rows))
	}
	if len(withSections.Unusual.Rows) != 1 {
		t.Fatalf("expected 1 unusual row, got %d", len(withSections.Unusual.Rows))
	}
	if len(withSections.Logs.Items) == 0 {
		t.Fatalf("expected log items")
	}

	if err := client.Call(context.Background(), "ui.set_focus_symbol", SetFocusSymbolParams{Symbol: "cu2604"}, nil); err != nil {
		t.Fatalf("ui.set_focus_symbol: %v", err)
	}
	if err := client.Call(context.Background(), "ui.set_unusual_threshold", SetUnusualThresholdParams{
		TurnoverChgThreshold:   200000,
		TurnoverRatioThreshold: 0.2,
		OIRatioThreshold:       0.15,
	}, nil); err != nil {
		t.Fatalf("ui.set_unusual_threshold: %v", err)
	}
	var uiState UIState
	if err := client.Call(context.Background(), "router.get_ui_state", map[string]any{}, &uiState); err != nil {
		t.Fatalf("router.get_ui_state: %v", err)
	}
	if uiState.FocusSymbol != "cu2604" {
		t.Fatalf("unexpected focus symbol: %q", uiState.FocusSymbol)
	}
	if uiState.TurnoverChgThreshold != 200000 {
		t.Fatalf("unexpected threshold chg: %v", uiState.TurnoverChgThreshold)
	}
	if uiState.TurnoverRatioThreshold != 0.2 {
		t.Fatalf("unexpected threshold ratio: %v", uiState.TurnoverRatioThreshold)
	}
	if uiState.OIRatioThreshold != 0.15 {
		t.Fatalf("unexpected threshold oi ratio: %v", uiState.OIRatioThreshold)
	}
}

func waitViewSnapshot(client *ipc.Client) (ViewSnapshot, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		var view ViewSnapshot
		err := client.Call(context.Background(), "router.get_view_snapshot", GetViewSnapshotParams{}, &view)
		if err == nil && view.Market.Seq > 0 {
			return view, nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return ViewSnapshot{}, err
			}
			return view, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestServerStopConcurrentCallers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, "127.0.0.1:0", NewState(), nil)
	if err != nil {
		t.Fatalf("start router: %v", err)
	}

	const callers = 16
	start := make(chan struct{})
	errCh := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			<-start
			errCh <- server.Stop(context.Background())
		}()
	}
	go func() {
		<-start
		cancel()
	}()
	close(start)

	for i := 0; i < callers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("stop failed: %v", err)
		}
	}
}

func TestServerStopClosesIdleConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, "127.0.0.1:0", NewState(), nil)
	if err != nil {
		t.Fatalf("start router: %v", err)
	}

	conn, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatalf("dial router: %v", err)
	}
	defer conn.Close()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := server.Stop(stopCtx); err != nil {
		t.Fatalf("stop router: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatalf("expected idle connection to be closed")
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatalf("expected closed connection, got read timeout")
	}
}

func TestTrackConnRejectedAfterCloseActiveConns(t *testing.T) {
	s := &Server{
		conns: make(map[net.Conn]struct{}),
	}

	conn, peer := net.Pipe()
	defer peer.Close()
	if !s.trackConn(conn) {
		t.Fatalf("expected initial connection to be tracked")
	}

	s.closeActiveConns()

	lateConn, latePeer := net.Pipe()
	defer lateConn.Close()
	defer latePeer.Close()
	if s.trackConn(lateConn) {
		t.Fatalf("expected late connection to be rejected while closing")
	}
}
