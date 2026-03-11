package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsStale(t *testing.T) {
	now := time.Date(2026, 1, 30, 12, 0, 0, 0, time.UTC)

	if !IsStale(time.Time{}, now, RefreshAfter) {
		t.Fatalf("expected zero time to be stale")
	}
	if IsStale(now.Add(-1*time.Hour), now, RefreshAfter) {
		t.Fatalf("expected 1h old metadata to be fresh")
	}
	if !IsStale(now.Add(-2*time.Hour), now, RefreshAfter) {
		t.Fatalf("expected 2h old metadata to be stale")
	}
}

func TestSourceRequestURLsAndTimeout(t *testing.T) {
	source := Source{
		Name:       "contract",
		URL:        "http://ignored.example",
		URLs:       []string{" https://a.example ", " ", "https://b.example"},
		TimeoutSec: 60,
	}
	gotURLs := source.requestURLs()
	if len(gotURLs) != 2 || gotURLs[0] != "https://a.example" || gotURLs[1] != "https://b.example" {
		t.Fatalf("requestURLs() = %#v, want [https://a.example https://b.example]", gotURLs)
	}
	if got := source.timeout(); got != 60*time.Second {
		t.Fatalf("timeout() = %s, want 60s", got)
	}

	fallback := Source{Name: "spec", URL: " http://single.example "}
	gotURLs = fallback.requestURLs()
	if len(gotURLs) != 1 || gotURLs[0] != "http://single.example" {
		t.Fatalf("single requestURLs() = %#v, want [http://single.example]", gotURLs)
	}
	if got := fallback.timeout(); got != DefaultFetchTimeout {
		t.Fatalf("fallback timeout() = %s, want %s", got, DefaultFetchTimeout)
	}
}

func TestAggregatePayloadsMergesArrayAndEnvelope(t *testing.T) {
	payloadA := []byte(`{"rsp_code":0,"data":[{"InstrumentID":"IF2503"}]}`)
	payloadB := []byte(`[{"InstrumentID":"IO2503-C-4000"}]`)
	merged, err := aggregatePayloads([][]byte{payloadA, payloadB})
	if err != nil {
		t.Fatalf("aggregatePayloads error: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(merged, &rows); err != nil {
		t.Fatalf("unmarshal merged payload: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("merged rows = %d, want 2", len(rows))
	}
}

func TestAggregatePayloadsRejectsNestedObjectWithoutData(t *testing.T) {
	valid := []byte(`[{"InstrumentID":"IF2503"}]`)
	invalid := []byte(`{"rsp_code":0,"data":{"rsp_code":0}}`)
	_, err := aggregatePayloads([][]byte{valid, invalid})
	if err == nil {
		t.Fatalf("expected aggregatePayloads error for nested object without data")
	}
	if !strings.Contains(err.Error(), "no data field") {
		t.Fatalf("aggregatePayloads error = %v, want nested no-data error", err)
	}
}

func TestFetchAndStoreBatchWritesMergedContractCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/futures":
			_, _ = w.Write([]byte(`{"rsp_code":0,"data":[{"InstrumentID":"IF2503","ProductID":"IF","UnderlyingInstrID":"","ProductClass":"1","OptionsType":""}]}`))
		case "/option":
			_, _ = w.Write([]byte(`[{"InstrumentID":"IO2503-C-4000","ProductID":"IOC","UnderlyingInstrID":"IF2503","ProductClass":"2","OptionsType":"1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	source := Source{
		Name:       "contract",
		URLs:       []string{srv.URL + "/futures", srv.URL + "/option"},
		TimeoutSec: 5,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &http.Client{Timeout: source.timeout()}

	if err := fetchAndStore(context.Background(), client, cacheDir, source, logger); err != nil {
		t.Fatalf("fetchAndStore error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(cacheDir, "contract.json"))
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	var cached Cached
	if err := json.Unmarshal(raw, &cached); err != nil {
		t.Fatalf("unmarshal cache file: %v", err)
	}
	if cached.LastError != "" {
		t.Fatalf("LastError = %q, want empty", cached.LastError)
	}
	if !strings.Contains(cached.URL, "/futures") || !strings.Contains(cached.URL, "/option") {
		t.Fatalf("cached.URL = %q, want joined batch urls", cached.URL)
	}
	rows, err := parseContractRows(cached.Data)
	if err != nil {
		t.Fatalf("parseContractRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
}

func TestFetchAndStoreBatchFailureKeepsPreviousData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			_, _ = w.Write([]byte(`[{"InstrumentID":"IF2503"}]`))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	existing := Cached{
		Name:        "contract",
		URL:         "http://old.example",
		LastUpdated: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		LastError:   "",
		Data:        json.RawMessage(`[{"InstrumentID":"OLD1"}]`),
	}
	encoded, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatalf("marshal existing cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "contract.json"), encoded, 0o644); err != nil {
		t.Fatalf("seed cache file: %v", err)
	}

	source := Source{
		Name:       "contract",
		URLs:       []string{srv.URL + "/ok", srv.URL + "/fail"},
		TimeoutSec: 5,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &http.Client{Timeout: source.timeout()}
	err = fetchAndStore(context.Background(), client, cacheDir, source, logger)
	if err == nil {
		t.Fatalf("expected fetchAndStore error, got nil")
	}

	raw, err := os.ReadFile(filepath.Join(cacheDir, "contract.json"))
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	var cached Cached
	if err := json.Unmarshal(raw, &cached); err != nil {
		t.Fatalf("unmarshal cache: %v", err)
	}
	if cached.LastError == "" {
		t.Fatalf("LastError should be recorded on batch failure")
	}
	var gotRows []map[string]any
	if err := json.Unmarshal(cached.Data, &gotRows); err != nil {
		t.Fatalf("unmarshal cached data: %v", err)
	}
	var wantRows []map[string]any
	if err := json.Unmarshal(existing.Data, &wantRows); err != nil {
		t.Fatalf("unmarshal existing data: %v", err)
	}
	gotData, _ := json.Marshal(gotRows)
	wantData, _ := json.Marshal(wantRows)
	if string(gotData) != string(wantData) {
		t.Fatalf("Data changed on failure: got %s want %s", gotData, wantData)
	}
}

func TestFetchAndStoreBatchUsesSingleTimeoutBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(700 * time.Millisecond)
		_, _ = w.Write([]byte(`[{"InstrumentID":"IF2503"}]`))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	source := Source{
		Name:       "contract",
		URLs:       []string{srv.URL + "/a", srv.URL + "/b"},
		TimeoutSec: 1,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &http.Client{Timeout: source.timeout()}

	start := time.Now()
	err := fetchAndStore(context.Background(), client, cacheDir, source, logger)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected fetchAndStore timeout error for cumulative batch budget")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fetchAndStore error = %v, want context deadline exceeded", err)
	}
	if elapsed >= 1400*time.Millisecond {
		t.Fatalf("batch fetch took %s, expected cumulative timeout budget shorter than full sequential duration", elapsed)
	}
}
