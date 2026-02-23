package live

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/danni2019/starSling/internal/settingsstore"
)

//go:embed options_worker.py
var optionsWorkerScript []byte

func StartOptionsWorkerDetached(ctx context.Context, pythonPath string, routerAddr string, logger *slog.Logger) (*Process, error) {
	return startOptionsWorker(ctx, pythonPath, routerAddr, logger, io.Discard, io.Discard)
}

func startOptionsWorker(ctx context.Context, pythonPath string, routerAddr string, logger *slog.Logger, stdout io.Writer, stderr io.Writer) (*Process, error) {
	if strings.TrimSpace(routerAddr) == "" {
		return nil, fmt.Errorf("router addr missing for options worker")
	}
	resolved, err := resolvePython(pythonPath)
	if err != nil {
		return nil, err
	}
	scriptPath, cleanup, err := writeTempScript("options_worker.py", optionsWorkerScript)
	if err != nil {
		return nil, err
	}
	args := []string{
		scriptPath,
		"--router_addr", strings.TrimSpace(routerAddr),
	}
	settings, settingsErr := settingsstore.Load()
	if settingsErr != nil && logger != nil {
		logger.Warn("load settings for options worker failed; using defaults", "error", settingsErr)
	}
	args = append(args,
		"--risk_free_rate", strconv.FormatFloat(settings.RiskFreeRate, 'g', -1, 64),
		"--days_in_year", strconv.Itoa(settings.DaysInYear),
	)

	cmdCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cmdCtx, resolved, args...)
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if logger != nil {
		logger.Info("starting options worker", "python", resolved, "router", strings.TrimSpace(routerAddr))
	}
	if err := cmd.Start(); err != nil {
		cleanup()
		cancel()
		return nil, fmt.Errorf("start options worker: %w", err)
	}

	proc := &Process{
		cmd:     cmd,
		cancel:  cancel,
		exitCh:  make(chan error, 1),
		cleanup: cleanup,
	}
	go func() {
		err := cmd.Wait()
		proc.setExit(err)
		proc.cleanup()
		proc.exitCh <- err
		close(proc.exitCh)
	}()
	return proc, nil
}
