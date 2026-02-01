package live

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"

	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/metadata"
)

type Process struct {
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	exitCh  chan error
	cleanup func()

	mu      sync.Mutex
	exitErr error
	done    bool
	stopped bool
}

func Start(ctx context.Context, cfg config.LiveMDConfig, pythonPath string, logger *slog.Logger) (*Process, error) {
	if len(cfg.Instruments) == 0 {
		ids, err := metadata.LoadContractInstrumentIDs()
		if err != nil {
			return nil, fmt.Errorf("load contract instruments: %w", err)
		}
		cfg.Instruments = ids
	}

	resolved, err := resolvePython(pythonPath)
	if err != nil {
		return nil, err
	}

	scriptPath, cleanup, err := writeTempScript()
	if err != nil {
		return nil, err
	}

	args := buildArgs(cfg, scriptPath)
	logger.Info("starting live md", "python", resolved, "host", cfg.Host, "port", cfg.Port)

	cmdCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cmdCtx, resolved, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cleanup()
		cancel()
		return nil, fmt.Errorf("start live md: %w", err)
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

func (p *Process) Exit() <-chan error {
	return p.exitCh
}

func (p *Process) Stop() {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	p.cancel()
}

func (p *Process) Stopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

func (p *Process) Done() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

func (p *Process) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitErr
}

func (p *Process) setExit(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.exitErr = err
	p.done = true
}
