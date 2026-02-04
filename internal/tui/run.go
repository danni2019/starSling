package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/danni2019/starSling/internal/metadata"
)

func Run(ctx context.Context, routerAddr string, logger *slog.Logger) int {
	initMetadata(ctx)
	ui := newUI(routerAddr, logger)
	if ctx != nil {
		go func() {
			<-ctx.Done()
			ui.app.Stop()
		}()
	}
	if err := ui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func initMetadata(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sources, err := metadata.LoadSources()
	if err != nil {
		logger.Warn("metadata sources unavailable", "error", err)
		return
	}
	warnings, refreshed, err := metadata.RefreshIfStale(ctx, logger, sources, time.Now().UTC())
	if err != nil {
		logger.Warn("metadata refresh failed", "error", err)
	}
	if refreshed {
		logger.Info("metadata refresh completed")
	}
	for _, warn := range warnings {
		logger.Warn("metadata stale", "source", warn.Name, "age", warn.Age.String(), "error", warn.LastError)
	}
}
