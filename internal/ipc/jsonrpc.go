package ipc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	JSONRPCVersion = "2.0"
	maxFrameBytes  = 32 * 1024 * 1024
	DefaultTimeout = 2 * time.Second
)

var idCounter uint64

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func (m Message) IsNotification() bool {
	return m.Method != "" && len(m.ID) == 0
}

func (m Message) IsRequest() bool {
	return m.Method != "" && len(m.ID) > 0
}

func (m Message) IsResponse() bool {
	return m.Method == "" && len(m.ID) > 0
}

func ReadFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header)
	if size == 0 || size > maxFrameBytes {
		return nil, fmt.Errorf("invalid frame size %d", size)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > maxFrameBytes {
		return fmt.Errorf("invalid payload size %d", len(payload))
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if err := writeAll(w, header); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func ReadMessage(r io.Reader) (Message, error) {
	body, err := ReadFrame(r)
	if err != nil {
		return Message{}, err
	}
	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func WriteMessage(w io.Writer, msg Message) error {
	if msg.JSONRPC == "" {
		msg.JSONRPC = JSONRPCVersion
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return WriteFrame(w, payload)
}

type Client struct {
	Addr    string
	Timeout time.Duration
}

func NewClient(addr string) *Client {
	return &Client{Addr: addr, Timeout: DefaultTimeout}
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	if c.Addr == "" {
		return errors.New("rpc addr is empty")
	}
	rawParams, err := marshalParams(params)
	if err != nil {
		return err
	}
	id := json.RawMessage(strconv.FormatUint(atomic.AddUint64(&idCounter, 1), 10))
	req := Message{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}
	resp, err := c.roundTrip(ctx, req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	if result == nil || len(resp.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Result, result); err != nil {
		return fmt.Errorf("decode rpc result: %w", err)
	}
	return nil
}

func (c *Client) Notify(ctx context.Context, method string, params any) error {
	if c.Addr == "" {
		return errors.New("rpc addr is empty")
	}
	rawParams, err := marshalParams(params)
	if err != nil {
		return err
	}
	req := Message{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  rawParams,
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return WriteMessage(conn, req)
}

func (c *Client) roundTrip(ctx context.Context, req Message) (Message, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return Message{}, err
	}
	defer conn.Close()
	if err := WriteMessage(conn, req); err != nil {
		return Message{}, err
	}
	resp, err := ReadMessage(conn)
	if err != nil {
		return Message{}, err
	}
	if len(resp.ID) == 0 {
		return Message{}, errors.New("rpc response id missing")
	}
	return resp, nil
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	dialCtx := ctx
	if dialCtx == nil {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(dialCtx, "tcp", c.Addr)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := dialCtx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return json.RawMessage(`{}`), nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return raw, nil
}

func writeAll(w io.Writer, buf []byte) error {
	for len(buf) > 0 {
		n, err := w.Write(buf)
		if n > 0 {
			buf = buf[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
