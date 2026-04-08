package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	RefreshAfter        = 2 * time.Hour
	WarnAfter           = 12 * time.Hour
	DefaultFetchTimeout = 15 * time.Second
)

type Source struct {
	Name       string   `json:"name"`
	URL        string   `json:"url,omitempty"`
	URLs       []string `json:"urls,omitempty"`
	TimeoutSec int      `json:"timeout_sec,omitempty"`
}

type SourceConfig struct {
	Sources []Source `json:"sources"`
}

type Cached struct {
	Name        string          `json:"name"`
	URL         string          `json:"url"`
	LastUpdated time.Time       `json:"last_updated"`
	LastError   string          `json:"last_error,omitempty"`
	Data        json.RawMessage `json:"data"`
}

type Warning struct {
	Name        string
	LastUpdated time.Time
	Age         time.Duration
	LastError   string
}

func LoadSources() ([]Source, error) {
	path, err := findSourcesFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metadata sources: %w", err)
	}
	var cfg SourceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse metadata sources: %w", err)
	}
	if len(cfg.Sources) == 0 {
		return nil, fmt.Errorf("metadata sources empty")
	}
	for idx := range cfg.Sources {
		source := &cfg.Sources[idx]
		source.Name = strings.TrimSpace(source.Name)
		if source.Name == "" {
			return nil, fmt.Errorf("metadata source[%d] name empty", idx)
		}
		if source.TimeoutSec < 0 {
			return nil, fmt.Errorf("metadata source[%s] timeout_sec must be >= 0", source.Name)
		}
		if len(source.requestURLs()) == 0 {
			return nil, fmt.Errorf("metadata source[%s] has no url", source.Name)
		}
	}
	return cfg.Sources, nil
}

func RefreshAll(ctx context.Context, logger *slog.Logger, sources []Source) error {
	return refreshSources(ctx, logger, sources)
}

func refreshSources(ctx context.Context, logger *slog.Logger, sources []Source) error {
	if len(sources) == 0 {
		return nil
	}
	cacheDir, err := CacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create metadata cache dir: %w", err)
	}

	var errs []error

	for _, source := range sources {
		client := &http.Client{Timeout: source.timeout()}
		if err := fetchAndStore(ctx, client, cacheDir, source, logger); err != nil {
			err = fmt.Errorf("%s: %w", source.Name, err)
			errs = append(errs, err)
			logger.Error("metadata fetch failed", "source", source.Name, "error", err)
		}
	}

	return errors.Join(errs...)
}

func RefreshIfStale(ctx context.Context, logger *slog.Logger, sources []Source, now time.Time) ([]Warning, bool, error) {
	staleSources := make([]Source, 0, len(sources))
	for _, source := range sources {
		cached, err := Load(source.Name)
		if err != nil {
			staleSources = append(staleSources, source)
			continue
		}
		if IsStale(cached.LastUpdated, now, RefreshAfter) {
			staleSources = append(staleSources, source)
		}
	}

	var refreshErr error
	refreshed := false
	if len(staleSources) > 0 {
		refreshErr = refreshSources(ctx, logger, staleSources)
		refreshed = true
	}

	warnings := CollectWarnings(sources, now)
	return warnings, refreshed, refreshErr
}

func CacheDir() (string, error) {
	dirs, err := cacheDirs()
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("no metadata cache directories available")
	}
	return dirs[0], nil
}

func IsStale(lastUpdated time.Time, now time.Time, threshold time.Duration) bool {
	if lastUpdated.IsZero() {
		return true
	}
	return now.Sub(lastUpdated) >= threshold
}

func Load(name string) (Cached, error) {
	dirs, err := cacheDirs()
	if err != nil {
		return Cached{}, err
	}
	var errs []error
	for _, dir := range dirs {
		path := filepath.Join(dir, fmt.Sprintf("%s.json", name))
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var cached Cached
		if err := json.Unmarshal(data, &cached); err != nil {
			errs = append(errs, err)
			continue
		}
		return cached, nil
	}
	return Cached{}, fmt.Errorf("read metadata: %w", errors.Join(errs...))
}

func CollectWarnings(sources []Source, now time.Time) []Warning {
	var warnings []Warning
	for _, source := range sources {
		cached, err := Load(source.Name)
		if err != nil {
			warnings = append(warnings, Warning{
				Name:        source.Name,
				LastUpdated: time.Time{},
				Age:         0,
				LastError:   err.Error(),
			})
			continue
		}
		if IsStale(cached.LastUpdated, now, WarnAfter) {
			warnings = append(warnings, Warning{
				Name:        source.Name,
				LastUpdated: cached.LastUpdated,
				Age:         now.Sub(cached.LastUpdated),
				LastError:   cached.LastError,
			})
		}
	}
	return warnings
}

func fetchAndStore(ctx context.Context, client *http.Client, cacheDir string, source Source, logger *slog.Logger) error {
	if ctx == nil {
		ctx = context.Background()
	}
	urls := source.requestURLs()
	if len(urls) == 0 {
		err := fmt.Errorf("source has no request url")
		recordError(cacheDir, source, err)
		return err
	}
	batchCtx, cancel := context.WithTimeout(ctx, source.timeout())
	defer cancel()

	payloads := make([][]byte, 0, len(urls))
	for _, url := range urls {
		payload, err := fetchPayload(batchCtx, client, url)
		if err != nil {
			err = fmt.Errorf("fetch %s: %w", url, err)
			recordError(cacheDir, source, err)
			return err
		}
		payloads = append(payloads, payload)
	}

	payload, err := aggregatePayloads(payloads)
	if err != nil {
		recordError(cacheDir, source, err)
		return err
	}

	cached := Cached{
		Name:        source.Name,
		URL:         source.cacheURL(),
		LastUpdated: time.Now().UTC(),
		LastError:   "",
		Data:        json.RawMessage(payload),
	}

	out, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		recordError(cacheDir, source, err)
		return fmt.Errorf("marshal metadata: %w", err)
	}

	path := filepath.Join(cacheDir, fmt.Sprintf("%s.json", source.Name))
	if err := os.WriteFile(path, out, 0o644); err != nil {
		recordError(cacheDir, source, err)
		return fmt.Errorf("write metadata: %w", err)
	}

	logger.Info("metadata updated", "source", source.Name, "cache", cacheDir)
	return nil
}

func fetchPayload(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("invalid json")
	}
	return payload, nil
}

func aggregatePayloads(payloads [][]byte) ([]byte, error) {
	if len(payloads) == 0 {
		return nil, fmt.Errorf("empty payloads")
	}
	if len(payloads) == 1 {
		return payloads[0], nil
	}

	merged := make([]json.RawMessage, 0)
	for _, payload := range payloads {
		rows, err := extractRows(payload)
		if err != nil {
			return nil, err
		}
		merged = append(merged, rows...)
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("merged payload empty")
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged payload: %w", err)
	}
	return out, nil
}

func extractRows(payload []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	switch trimmed[0] {
	case '[':
		var rows []json.RawMessage
		if err := json.Unmarshal(trimmed, &rows); err != nil {
			return nil, fmt.Errorf("parse payload array: %w", err)
		}
		return rows, nil
	case '{':
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, fmt.Errorf("parse payload envelope: %w", err)
		}
		inner := bytes.TrimSpace(envelope.Data)
		if len(inner) == 0 || bytes.Equal(inner, []byte("null")) {
			return nil, fmt.Errorf("payload has no data field")
		}
		if inner[0] == '[' {
			var rows []json.RawMessage
			if err := json.Unmarshal(inner, &rows); err != nil {
				return nil, fmt.Errorf("parse payload data array: %w", err)
			}
			return rows, nil
		}
		if inner[0] == '{' {
			var wrapped struct {
				Data *json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(inner, &wrapped); err != nil {
				return nil, fmt.Errorf("parse payload nested data: %w", err)
			}
			if wrapped.Data == nil {
				return nil, fmt.Errorf("payload nested object has no data field")
			}
			nested := bytes.TrimSpace(*wrapped.Data)
			if len(nested) == 0 || bytes.Equal(nested, []byte("null")) {
				return nil, fmt.Errorf("payload nested object has no data field")
			}
			var rows []json.RawMessage
			if err := json.Unmarshal(nested, &rows); err != nil {
				return nil, fmt.Errorf("parse payload nested data: %w", err)
			}
			return rows, nil
		}
		return nil, fmt.Errorf("unsupported payload data format")
	default:
		return nil, fmt.Errorf("unsupported payload format")
	}
}

func recordError(cacheDir string, source Source, fetchErr error) {
	cached, err := loadCacheFromDisk(cacheDir, source.Name)
	if err != nil {
		cached = Cached{
			Name:        source.Name,
			URL:         source.cacheURL(),
			LastUpdated: time.Time{},
			Data:        json.RawMessage("null"),
		}
	}
	cached.URL = source.cacheURL()
	cached.LastError = fetchErr.Error()
	out, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(cacheDir, fmt.Sprintf("%s.json", source.Name))
	_ = os.WriteFile(path, out, 0o644)
}

func (s Source) timeout() time.Duration {
	if s.TimeoutSec <= 0 {
		return DefaultFetchTimeout
	}
	return time.Duration(s.TimeoutSec) * time.Second
}

func (s Source) requestURLs() []string {
	out := make([]string, 0, len(s.URLs)+1)
	for _, url := range s.URLs {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		out = append(out, url)
	}
	if len(out) > 0 {
		return out
	}
	if url := strings.TrimSpace(s.URL); url != "" {
		return []string{url}
	}
	return nil
}

func (s Source) cacheURL() string {
	urls := s.requestURLs()
	switch len(urls) {
	case 0:
		return ""
	case 1:
		return urls[0]
	default:
		return strings.Join(urls, ",")
	}
}

func loadCacheFromDisk(cacheDir, name string) (Cached, error) {
	path := filepath.Join(cacheDir, fmt.Sprintf("%s.json", name))
	data, err := os.ReadFile(path)
	if err != nil {
		return Cached{}, err
	}
	var cached Cached
	if err := json.Unmarshal(data, &cached); err != nil {
		return Cached{}, err
	}
	return cached, nil
}

func findSourcesFile() (string, error) {
	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, "config", "metadata.sources.json")
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "config", "metadata.sources.json")
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("metadata sources config not found (config/metadata.sources.json)")
}

func SourcesFilePath() (string, error) {
	return findSourcesFile()
}

func cacheDirs() ([]string, error) {
	var dirs []string
	if base, err := os.UserConfigDir(); err == nil && base != "" {
		dirs = append(dirs, filepath.Join(base, "starsling", "metadata"))
	}
	dirs = append(dirs, filepath.Join("runtime", "metadata"))
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no metadata cache directories available")
	}
	return dirs, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
