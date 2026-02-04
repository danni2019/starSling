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
		Columns:       []string{"ctp_contract", "last", "volume"},
		Rows: []map[string]any{
			{"ctp_contract": "cu2604", "last": 104550.0, "volume": 55848},
			{"ctp_contract": "ag2604", "last": 31490.0, "volume": 1200},
		},
	}
	if err := client.Notify(context.Background(), "market.snapshot", payload); err != nil {
		t.Fatalf("notify market.snapshot: %v", err)
	}

	var view ViewSnapshot
	if err := client.Call(context.Background(), "router.get_view_snapshot", GetViewSnapshotParams{}, &view); err != nil {
		t.Fatalf("get_view_snapshot: %v", err)
	}
	if view.Market.Seq == 0 {
		t.Fatalf("expected seq > 0")
	}
	if len(view.Market.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(view.Market.Rows))
	}

	if err := client.Call(context.Background(), "ui.set_focus_symbol", SetFocusSymbolParams{Symbol: "cu2604"}, nil); err != nil {
		t.Fatalf("ui.set_focus_symbol: %v", err)
	}
	var uiState UIState
	if err := client.Call(context.Background(), "router.get_ui_state", map[string]any{}, &uiState); err != nil {
		t.Fatalf("router.get_ui_state: %v", err)
	}
	if uiState.FocusSymbol != "cu2604" {
		t.Fatalf("unexpected focus symbol: %q", uiState.FocusSymbol)
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
