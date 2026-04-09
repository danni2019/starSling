package tui

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/danni2019/starSling/internal/metadata"
)

func TestEnsureLiveMetadataReadyRefreshesWhenMappingsMissing(t *testing.T) {
	origLoadSourcesFn := metadataLoadSourcesFn
	origLoadFn := metadataLoadFn
	origRefreshIfStaleFn := metadataRefreshIfStaleFn
	origRefreshAllFn := metadataRefreshAllFn
	origLoadContractMappingsFn := metadataLoadContractMappingsFn
	origLoadTradeSegmentsFn := metadataLoadTradeSegmentsFn
	origMetadataNowFn := metadataNowFn
	defer func() {
		metadataLoadSourcesFn = origLoadSourcesFn
		metadataLoadFn = origLoadFn
		metadataRefreshIfStaleFn = origRefreshIfStaleFn
		metadataRefreshAllFn = origRefreshAllFn
		metadataLoadContractMappingsFn = origLoadContractMappingsFn
		metadataLoadTradeSegmentsFn = origLoadTradeSegmentsFn
		metadataNowFn = origMetadataNowFn
	}()

	sources := []metadata.Source{{Name: "contract", URL: "http://example.com/contract"}}
	mappings := &metadata.ContractMappings{}
	segments := []metadata.TradeSegment{{Start: time.Hour, End: 2 * time.Hour}}
	mappingsReady := false
	refreshAllCalled := false
	refreshIfStaleCalled := false

	metadataLoadSourcesFn = func() ([]metadata.Source, error) { return sources, nil }
	metadataRefreshIfStaleFn = func(ctx context.Context, logger *slog.Logger, src []metadata.Source, now time.Time) ([]metadata.Warning, bool, error) {
		refreshIfStaleCalled = true
		return nil, false, nil
	}
	metadataRefreshAllFn = func(ctx context.Context, logger *slog.Logger, src []metadata.Source) error {
		refreshAllCalled = true
		mappingsReady = true
		return nil
	}
	metadataLoadContractMappingsFn = func() (*metadata.ContractMappings, error) {
		if !mappingsReady {
			return nil, errors.New("contract metadata missing")
		}
		return mappings, nil
	}
	metadataLoadTradeSegmentsFn = func() ([]metadata.TradeSegment, error) {
		return segments, nil
	}
	metadataNowFn = func() time.Time {
		return time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	}

	ui := &UI{}
	if err := ui.ensureLiveMetadataReady(); err != nil {
		t.Fatalf("ensureLiveMetadataReady returned error: %v", err)
	}
	if !refreshIfStaleCalled {
		t.Fatalf("expected RefreshIfStale to be called")
	}
	if !refreshAllCalled {
		t.Fatalf("expected RefreshAll to be called after missing mappings")
	}
	if ui.metadata != mappings {
		t.Fatalf("expected metadata mappings to be installed")
	}
	if len(ui.liveTradeSegments) != 1 || ui.liveTradeSegments[0] != segments[0] {
		t.Fatalf("expected trade segments to be reloaded, got %+v", ui.liveTradeSegments)
	}
	if ui.liveTradeSegmentsErr != nil {
		t.Fatalf("expected trade segments error to be cleared, got %v", ui.liveTradeSegmentsErr)
	}
}

func TestEnsureLiveMetadataReadyFailsWhenMappingsStillUnavailable(t *testing.T) {
	origLoadSourcesFn := metadataLoadSourcesFn
	origLoadFn := metadataLoadFn
	origRefreshIfStaleFn := metadataRefreshIfStaleFn
	origRefreshAllFn := metadataRefreshAllFn
	origLoadContractMappingsFn := metadataLoadContractMappingsFn
	origLoadTradeSegmentsFn := metadataLoadTradeSegmentsFn
	defer func() {
		metadataLoadSourcesFn = origLoadSourcesFn
		metadataLoadFn = origLoadFn
		metadataRefreshIfStaleFn = origRefreshIfStaleFn
		metadataRefreshAllFn = origRefreshAllFn
		metadataLoadContractMappingsFn = origLoadContractMappingsFn
		metadataLoadTradeSegmentsFn = origLoadTradeSegmentsFn
	}()

	metadataLoadSourcesFn = func() ([]metadata.Source, error) {
		return []metadata.Source{{Name: "contract", URL: "http://example.com/contract"}}, nil
	}
	metadataRefreshIfStaleFn = func(ctx context.Context, logger *slog.Logger, src []metadata.Source, now time.Time) ([]metadata.Warning, bool, error) {
		return nil, false, nil
	}
	metadataRefreshAllFn = func(ctx context.Context, logger *slog.Logger, src []metadata.Source) error {
		return errors.New("network unavailable")
	}
	metadataLoadContractMappingsFn = func() (*metadata.ContractMappings, error) {
		return nil, errors.New("contract metadata missing")
	}
	metadataLoadTradeSegmentsFn = func() ([]metadata.TradeSegment, error) {
		return nil, errors.New("trade_time missing")
	}

	ui := &UI{}
	err := ui.ensureLiveMetadataReady()
	if err == nil {
		t.Fatalf("expected ensureLiveMetadataReady to fail")
	}
	if got := err.Error(); got == "" || !containsAll(got, "load contract metadata", "refresh metadata", "network unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLiveMetadataStateReportsMissingAndStale(t *testing.T) {
	origLoadSourcesFn := metadataLoadSourcesFn
	origLoadFn := metadataLoadFn
	origMetadataNowFn := metadataNowFn
	defer func() {
		metadataLoadSourcesFn = origLoadSourcesFn
		metadataLoadFn = origLoadFn
		metadataNowFn = origMetadataNowFn
	}()

	metadataLoadSourcesFn = func() ([]metadata.Source, error) {
		return []metadata.Source{
			{Name: "contract", URL: "http://example.com/contract"},
			{Name: "trade_time", URL: "http://example.com/trade_time"},
		}, nil
	}
	now := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	metadataNowFn = func() time.Time { return now }
	metadataLoadFn = func(name string) (metadata.Cached, error) {
		switch name {
		case "contract":
			return metadata.Cached{
				Name:        "contract",
				LastUpdated: now.Add(-metadata.RefreshAfter).Add(-time.Minute),
			}, nil
		case "trade_time":
			return metadata.Cached{}, errors.New("missing")
		default:
			return metadata.Cached{}, errors.New("unexpected metadata")
		}
	}

	ui := &UI{}
	state, err := ui.liveMetadataState()
	if err != nil {
		t.Fatalf("liveMetadataState() error = %v", err)
	}
	if !containsAll(strings.Join(state.Missing, ","), "trade_time") {
		t.Fatalf("expected trade_time missing, got %+v", state)
	}
	if !containsAll(strings.Join(state.Stale, ","), "contract") {
		t.Fatalf("expected contract stale, got %+v", state)
	}
	if state.Ready() {
		t.Fatalf("expected metadata state to require refresh")
	}
}

func TestLiveMetadataStateMessageSummarizesIssues(t *testing.T) {
	state := liveMetadataState{
		Missing: []string{"contract"},
		Stale:   []string{"trade_time"},
	}
	msg := state.Message()
	if !containsAll(msg, "Missing: contract", "Stale: trade_time", "Open Metadata Setup") {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
