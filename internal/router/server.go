package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/danni2019/starSling/internal/ipc"
)

type Server struct {
	logger  *slog.Logger
	state   *State
	ln      net.Listener
	wg      sync.WaitGroup
	conns   map[net.Conn]struct{}
	closing bool
	connMu  sync.Mutex
	closed  chan struct{}
	stop    sync.Once
}

func Start(ctx context.Context, addr string, state *State, logger *slog.Logger) (*Server, error) {
	if state == nil {
		state = NewState()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	s := &Server{
		logger: logger,
		state:  state,
		ln:     ln,
		conns:  make(map[net.Conn]struct{}),
		closed: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.acceptLoop()
	go func() {
		<-ctx.Done()
		_ = s.Stop(context.Background())
	}()
	return s, nil
}

func (s *Server) Addr() string {
	return s.ln.Addr().String()
}

func (s *Server) Stop(ctx context.Context) error {
	s.stop.Do(func() {
		close(s.closed)
		_ = s.ln.Close()
		s.closeActiveConns()
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				s.logger.Warn("router accept failed", "error", err)
				continue
			}
		}
		if !s.trackConn(conn) {
			_ = conn.Close()
			return
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer s.untrackConn(conn)
	defer conn.Close()
	for {
		msg, err := ipc.ReadMessage(conn)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			s.logger.Debug("router read message failed", "error", err)
			return
		}
		if msg.JSONRPC != "" && msg.JSONRPC != ipc.JSONRPCVersion {
			_ = s.writeError(conn, msg.ID, -32600, "invalid jsonrpc version")
			continue
		}
		if msg.IsNotification() {
			s.handleNotification(msg)
			continue
		}
		if msg.IsRequest() {
			s.handleRequest(conn, msg)
			continue
		}
		_ = s.writeError(conn, msg.ID, -32600, "invalid message")
	}
}

func (s *Server) trackConn(conn net.Conn) bool {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.closing {
		return false
	}
	s.conns[conn] = struct{}{}
	return true
}

func (s *Server) untrackConn(conn net.Conn) {
	s.connMu.Lock()
	delete(s.conns, conn)
	s.connMu.Unlock()
}

func (s *Server) closeActiveConns() {
	s.connMu.Lock()
	s.closing = true
	conns := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.connMu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (s *Server) handleNotification(msg ipc.Message) {
	switch msg.Method {
	case "market.snapshot":
		var snapshot MarketSnapshot
		if err := json.Unmarshal(msg.Params, &snapshot); err != nil {
			s.logger.Warn("market.snapshot decode failed", "error", err)
			return
		}
		s.state.UpdateMarket(snapshot)
	case "options.snapshot":
		var snapshot OptionsSnapshot
		if err := json.Unmarshal(msg.Params, &snapshot); err != nil {
			s.logger.Warn("options.snapshot decode failed", "error", err)
			return
		}
		s.state.UpdateOptions(snapshot)
	case "curve.snapshot":
		var snapshot CurveSnapshot
		if err := json.Unmarshal(msg.Params, &snapshot); err != nil {
			s.logger.Warn("curve.snapshot decode failed", "error", err)
			return
		}
		s.state.UpdateCurve(snapshot)
	case "unusual.snapshot":
		var snapshot UnusualSnapshot
		if err := json.Unmarshal(msg.Params, &snapshot); err != nil {
			s.logger.Warn("unusual.snapshot decode failed", "error", err)
			return
		}
		s.state.UpdateUnusual(snapshot)
	case "log.append":
		var line LogLine
		if err := json.Unmarshal(msg.Params, &line); err != nil {
			s.logger.Warn("log.append decode failed", "error", err)
			return
		}
		s.state.AppendLog(line)
	default:
		s.logger.Debug("ignore unknown notification", "method", msg.Method)
	}
}

func (s *Server) handleRequest(conn net.Conn, msg ipc.Message) {
	switch msg.Method {
	case "router.get_view_snapshot":
		var params GetViewSnapshotParams
		if err := decodeObjectParams(msg.Params, &params); err != nil {
			_ = s.writeError(conn, msg.ID, -32602, "invalid params")
			return
		}
		result := s.state.GetViewSnapshot(params.FocusSymbol)
		_ = s.writeResult(conn, msg.ID, result)
	case "router.get_latest_market":
		var params GetLatestMarketParams
		if err := decodeObjectParams(msg.Params, &params); err != nil {
			_ = s.writeError(conn, msg.ID, -32602, "invalid params")
			return
		}
		snapshot, unchanged := s.state.GetLatestMarket(params.MinSeq)
		if unchanged {
			_ = s.writeResult(conn, msg.ID, map[string]any{
				"unchanged": true,
				"seq":       snapshot.Seq,
			})
			return
		}
		_ = s.writeResult(conn, msg.ID, snapshot)
	case "router.get_ui_state":
		_ = s.writeResult(conn, msg.ID, s.state.GetUIState())
	case "ui.set_focus_symbol":
		var params SetFocusSymbolParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			_ = s.writeError(conn, msg.ID, -32602, "invalid params")
			return
		}
		s.state.SetFocusSymbol(params.Symbol)
		_ = s.writeResult(conn, msg.ID, map[string]bool{"ok": true})
	case "ui.set_unusual_threshold":
		var params SetUnusualThresholdParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			_ = s.writeError(conn, msg.ID, -32602, "invalid params")
			return
		}
		s.state.SetUnusualThresholds(params.TurnoverChgThreshold, params.TurnoverRatioThreshold, params.OIRatioThreshold)
		_ = s.writeResult(conn, msg.ID, map[string]bool{"ok": true})
	case "ui.set_overview_gamma_buckets":
		var params SetOverviewGammaBucketsParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			_ = s.writeError(conn, msg.ID, -32602, "invalid params")
			return
		}
		s.state.SetOverviewGammaBuckets(params.FrontDays, params.MidDays)
		_ = s.writeResult(conn, msg.ID, map[string]bool{"ok": true})
	default:
		_ = s.writeError(conn, msg.ID, -32601, "method not found")
	}
}

func decodeObjectParams(raw json.RawMessage, out any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	return json.Unmarshal(trimmed, out)
}

func (s *Server) writeResult(conn net.Conn, id json.RawMessage, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return ipc.WriteMessage(conn, ipc.Message{
		JSONRPC: ipc.JSONRPCVersion,
		ID:      id,
		Result:  raw,
	})
}

func (s *Server) writeError(conn net.Conn, id json.RawMessage, code int, message string) error {
	return ipc.WriteMessage(conn, ipc.Message{
		JSONRPC: ipc.JSONRPCVersion,
		ID:      id,
		Error: &ipc.RPCError{
			Code:    code,
			Message: message,
		},
	})
}
