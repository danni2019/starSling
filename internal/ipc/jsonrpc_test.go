package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

func TestFrameRoundTrip(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	payload := []byte(`{"k":"v"}`)
	if err := WriteFrame(buf, payload); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	got, err := ReadFrame(buf)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: %q != %q", got, payload)
	}
}

func TestClientCall(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		msg, err := ReadMessage(conn)
		if err != nil {
			return
		}
		respPayload, _ := json.Marshal(map[string]bool{"ok": msg.Method == "ping"})
		resp := Message{
			JSONRPC: JSONRPCVersion,
			ID:      msg.ID,
			Result:  respPayload,
		}
		_ = WriteMessage(conn, resp)
	}()

	client := NewClient(ln.Addr().String())
	client.Timeout = time.Second
	var out struct {
		OK bool `json:"ok"`
	}
	if err := client.Call(context.Background(), "ping", map[string]string{"a": "b"}, &out); err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if !out.OK {
		t.Fatalf("unexpected rpc output: %+v", out)
	}
	<-done
}

func TestReadFrameInvalidSize(t *testing.T) {
	buf := bytes.NewBuffer([]byte{0, 0, 0, 0})
	_, err := ReadFrame(buf)
	if err == nil {
		t.Fatal("expected error for invalid size")
	}
}

func TestWriteMessageAddsVersion(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()

	go func() {
		_ = WriteMessage(w, Message{Method: "m"})
		_ = w.Close()
	}()

	msg, err := ReadMessage(r)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if msg.JSONRPC != JSONRPCVersion {
		t.Fatalf("jsonrpc mismatch: %q", msg.JSONRPC)
	}
}

func TestWriteFrameHandlesShortWrites(t *testing.T) {
	payload := []byte(`{"k":"v","n":123}`)
	w := &chunkWriter{chunkSize: 3}
	if err := WriteFrame(w, payload); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	got, err := ReadFrame(bytes.NewReader(w.buf.Bytes()))
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: %q != %q", got, payload)
	}
}

func TestWriteAllReturnsErrShortWriteOnZeroProgress(t *testing.T) {
	err := writeAll(&zeroWriter{}, []byte("abc"))
	if err != io.ErrShortWrite {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}
}

type chunkWriter struct {
	buf       bytes.Buffer
	chunkSize int
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	if w.chunkSize <= 0 {
		return 0, nil
	}
	if len(p) > w.chunkSize {
		p = p[:w.chunkSize]
	}
	return w.buf.Write(p)
}

type zeroWriter struct{}

func (w *zeroWriter) Write(_ []byte) (int, error) {
	return 0, nil
}
