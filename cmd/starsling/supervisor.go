package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/live"
	"github.com/danni2019/starSling/internal/metadata"
)

const (
	loginFailureWindow = 30 * time.Minute
	supervisorTick     = 15 * time.Second
)

var errSessionStopped = errors.New("live session stopped")

func superviseLive(ctx context.Context, cfg config.LiveMDConfig, pythonPath string, sources []metadata.Source, logger *slog.Logger) error {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}

	segments, segErr := metadata.LoadTradeSegments()
	if segErr != nil {
		logger.Warn("trade_time unavailable", "error", segErr)
	}

	ticker := time.NewTicker(supervisorTick)
	defer ticker.Stop()
	keyCtx, cancelKeys := context.WithCancel(ctx)
	defer cancelKeys()
	keyCh := watchKeys(keyCtx)

	var proc *live.Process
	var exitCh <-chan error
	var failureStart time.Time

	for {
		select {
		case <-ctx.Done():
			if proc != nil {
				proc.Stop()
			}
			return ctx.Err()
		case key, ok := <-keyCh:
			if !ok {
				keyCh = nil
				continue
			}
			if proc != nil {
				proc.Stop()
				proc = nil
				exitCh = nil
			}
			if key == keyCtrlC {
				return errForceExit
			}
			return errUserAborted
		case err := <-exitCh:
			if proc == nil {
				continue
			}
			logger.Warn("live-md exited", "error", err)
			proc = nil
			exitCh = nil
			if shouldStopAfterFailure(&failureStart, time.Now(), logger) {
				return errSessionStopped
			}
		case <-ticker.C:
			now := time.Now().In(location)
			inWindow := metadata.InTradingWindow(now, segments)
			if segErr != nil || len(segments) == 0 {
				inWindow = true
			}

			if !inWindow {
				if proc != nil {
					logger.Info("trading window closed; stopping live md")
					proc.Stop()
					proc = nil
					exitCh = nil
				}
				failureStart = time.Time{}
				continue
			}

			if proc != nil {
				continue
			}

			warnings, refreshed, err := metadata.RefreshIfStale(ctx, logger, sources, time.Now().UTC())
			if err != nil {
				logger.Warn("metadata refresh failed", "error", err)
			}
			if refreshed {
				segments, segErr = metadata.LoadTradeSegments()
				if segErr != nil {
					logger.Warn("trade_time reload failed", "error", segErr)
				}
			}
			if err := promptMetadataWarning(warnings); err != nil {
				if errors.Is(err, errForceExit) {
					return errForceExit
				}
				if errors.Is(err, errUserAborted) {
					return errUserAborted
				}
				logger.Error("warning prompt error", "error", err)
				continue
			}

			proc, err = live.Start(ctx, cfg, pythonPath, "", logger)
			if err != nil {
				logger.Warn("live-md start failed", "error", err)
				proc = nil
				if shouldStopAfterFailure(&failureStart, time.Now(), logger) {
					return errSessionStopped
				}
				continue
			}
			exitCh = proc.Exit()
			failureStart = time.Time{}
		}
	}
}

func shouldStopAfterFailure(failureStart *time.Time, now time.Time, logger *slog.Logger) bool {
	if failureStart.IsZero() {
		*failureStart = now
		return false
	}
	if now.Sub(*failureStart) >= loginFailureWindow {
		logger.Warn("live-md login failures exceeded window", "window", loginFailureWindow.String())
		return true
	}
	return false
}
