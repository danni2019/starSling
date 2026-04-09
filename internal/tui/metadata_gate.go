package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/danni2019/starSling/internal/metadata"
)

var (
	metadataLoadSourcesFn          = metadata.LoadSources
	metadataLoadFn                 = metadata.Load
	metadataRefreshIfStaleFn       = metadata.RefreshIfStale
	metadataRefreshAllFn           = metadata.RefreshAll
	metadataLoadContractMappingsFn = metadata.LoadContractMappings
	metadataLoadTradeSegmentsFn    = metadata.LoadTradeSegments
	metadataNowFn                  = time.Now
	ensureLiveMetadataReadyFn      = func(ui *UI) error { return ui.ensureLiveMetadataReady() }
	liveMetadataStateFn            = func(ui *UI) (liveMetadataState, error) { return ui.liveMetadataState() }
)

var requiredLiveMetadataNames = []string{"contract", "trade_time"}

type liveMetadataState struct {
	Missing []string
	Stale   []string
}

func (s liveMetadataState) Ready() bool {
	return len(s.Missing) == 0 && len(s.Stale) == 0
}

func (s liveMetadataState) Message() string {
	parts := []string{"Market metadata is required before entering Live Market Data."}
	if len(s.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("Missing: %s", strings.Join(s.Missing, ", ")))
	}
	if len(s.Stale) > 0 {
		parts = append(parts, fmt.Sprintf("Stale: %s", strings.Join(s.Stale, ", ")))
	}
	parts = append(parts, "Open Metadata Setup to download or refresh metadata now.")
	return strings.Join(parts, "\n\n")
}

func (ui *UI) liveMetadataState() (liveMetadataState, error) {
	if _, err := metadataLoadSourcesFn(); err != nil {
		return liveMetadataState{}, fmt.Errorf("load metadata sources: %w", err)
	}
	now := metadataNowFn().UTC()
	state := liveMetadataState{}
	for _, name := range requiredLiveMetadataNames {
		cached, err := metadataLoadFn(name)
		if err != nil {
			state.Missing = append(state.Missing, name)
			continue
		}
		if metadata.IsStale(cached.LastUpdated, now, metadata.RefreshAfter) {
			state.Stale = append(state.Stale, name)
		}
	}
	return state, nil
}

func (ui *UI) ensureLiveMetadataReady() error {
	logger := ui.logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	now := metadataNowFn().UTC()

	sources, sourcesErr := metadataLoadSourcesFn()
	var refreshErr error
	if sourcesErr == nil {
		_, _, refreshErr = metadataRefreshIfStaleFn(context.Background(), logger, sources, now)
		if refreshErr != nil && logger != nil {
			logger.Warn("metadata refresh before live startup failed", "error", refreshErr)
		}
	}

	mappings, mapErr := metadataLoadContractMappingsFn()
	segments, segErr := metadataLoadTradeSegmentsFn()

	if (mapErr != nil || segErr != nil) && sourcesErr == nil {
		forceErr := metadataRefreshAllFn(context.Background(), logger, sources)
		if forceErr != nil && logger != nil {
			logger.Warn("forced metadata refresh before live startup failed", "error", forceErr)
		}
		if mappings, mapErr = metadataLoadContractMappingsFn(); mapErr == nil {
			ui.metadata = mappings
		}
		segments, segErr = metadataLoadTradeSegmentsFn()
		if forceErr != nil && refreshErr == nil {
			refreshErr = forceErr
		}
	}

	if mapErr != nil {
		return buildMetadataReadyError(mapErr, sourcesErr, refreshErr)
	}

	ui.metadata = mappings
	ui.liveTradeSegments = segments
	ui.liveTradeSegmentsErr = segErr
	return nil
}

func buildMetadataReadyError(mapErr, sourcesErr, refreshErr error) error {
	parts := []string{fmt.Sprintf("load contract metadata: %v", mapErr)}
	if sourcesErr != nil {
		parts = append(parts, fmt.Sprintf("load metadata sources: %v", sourcesErr))
	}
	if refreshErr != nil {
		parts = append(parts, fmt.Sprintf("refresh metadata: %v", refreshErr))
	}
	return errors.New(strings.Join(parts, "; "))
}
