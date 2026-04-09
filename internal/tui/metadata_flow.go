package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

var refreshLiveMetadataFn = func(ui *UI, onChunk func(string)) error { return ui.refreshLiveMetadata(onChunk) }

func (ui *UI) openMetadataScreen(autoStart bool, resumeLive bool) {
	ui.metadataAutoStart = autoStart
	ui.metadataResumeLive = resumeLive
	ui.setScreen(screenMetadata)
}

func (ui *UI) refreshLiveMetadata(onChunk func(string)) error {
	logger := ui.logger
	if logger == nil {
		logger = slog.New(newMetadataLogHandler(onChunk))
	} else {
		logger = slog.New(newMetadataLogHandler(onChunk))
	}
	if onChunk != nil {
		onChunk("Refreshing market metadata...\n")
	}
	sources, err := metadataLoadSourcesFn()
	if err != nil {
		return fmt.Errorf("load metadata sources: %w", err)
	}
	if err := metadataRefreshAllFn(context.Background(), logger, sources); err != nil {
		return err
	}
	if err := ensureLiveMetadataReadyFn(ui); err != nil {
		return err
	}
	if onChunk != nil {
		onChunk("Market metadata is ready.\n")
	}
	return nil
}

type metadataLogHandler struct {
	onLine func(string)
}

func newMetadataLogHandler(onLine func(string)) *metadataLogHandler {
	return &metadataLogHandler{onLine: onLine}
}

func (h *metadataLogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *metadataLogHandler) Handle(_ context.Context, record slog.Record) error {
	if h == nil || h.onLine == nil {
		return nil
	}
	var parts []string
	record.Attrs(func(attr slog.Attr) bool {
		parts = append(parts, fmt.Sprintf("%s=%v", attr.Key, attr.Value.Any()))
		return true
	})
	line := record.Message
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, " ") + ")"
	}
	h.onLine(line + "\n")
	return nil
}

func (h *metadataLogHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *metadataLogHandler) WithGroup(_ string) slog.Handler {
	return h
}
