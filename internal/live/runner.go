package live

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/danni2019/starSling/internal/config"
)

//go:embed live_md.py
var liveScript []byte

func Run(ctx context.Context, cfg config.LiveMDConfig, pythonPath string, logger *slog.Logger) error {
	proc, err := Start(ctx, cfg, pythonPath, logger)
	if err != nil {
		return err
	}
	if err := <-proc.Exit(); err != nil {
		return fmt.Errorf("run live md: %w", err)
	}
	return nil
}

func buildArgs(cfg config.LiveMDConfig, scriptPath string) []string {
	args := []string{
		scriptPath,
		"--api", cfg.API,
		"--protocol", cfg.Protocol,
		"--host", cfg.Host,
		"--port", strconv.Itoa(cfg.Port),
	}
	if cfg.Username != "" {
		args = append(args, "--username", cfg.Username)
	}
	if cfg.Password != "" {
		args = append(args, "--password", cfg.Password)
	}
	if len(cfg.Instruments) > 0 {
		args = append(args, "--instruments", strings.Join(cfg.Instruments, ","))
	}
	return args
}

func resolvePython(pythonPath string) (string, error) {
	candidate := strings.TrimSpace(pythonPath)
	if candidate == "" {
		bundled := BundledPythonPath()
		if bundled != "" {
			candidate = bundled
		} else {
			candidate = "python3.11"
		}
	}
	resolved, err := exec.LookPath(candidate)
	if err != nil {
		return "", fmt.Errorf("python not found: %s (run scripts/bootstrap_python.sh)", candidate)
	}
	return resolved, nil
}

func BundledPythonPath() string {
	platform := RuntimePlatform()
	if platform == "" {
		return ""
	}
	candidates := []string{
		filepath.Join("runtime", platform, "venv", "bin", "python"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "runtime", platform, "venv", "bin", "python"))
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func RuntimePlatform() string {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "macos-arm64"
		}
		return "macos-x86_64"
	case "linux":
		if runtime.GOARCH == "amd64" {
			return "linux-x86_64"
		}
	}
	return ""
}

func writeTempScript() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "starsling-live-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	scriptPath := filepath.Join(tempDir, "live_md.py")
	if err := os.WriteFile(scriptPath, liveScript, 0o700); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write script: %w", err)
	}

	return scriptPath, cleanup, nil
}
