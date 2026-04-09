package runtime

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

func RunBootstrap() (string, error) {
	return RunBootstrapStream(nil)
}

func RunBootstrapStream(onChunk func(string)) (string, error) {
	script, err := BootstrapScriptPath()
	if err != nil {
		return "", err
	}
	shell, err := resolveShell()
	if err != nil {
		return "", err
	}

	cmd := exec.Command(shell, script)
	output, err := runStreamingCommand(cmd, onChunk)
	if err != nil {
		return output, fmt.Errorf("bootstrap failed: %w", err)
	}
	return output, nil
}

func runStreamingCommand(cmd *exec.Cmd, onChunk func(string)) (string, error) {
	collector := newStreamingCollector(onChunk)
	cmd.Stdout = collector
	cmd.Stderr = collector
	err := cmd.Run()
	return collector.String(), err
}

type streamingCollector struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	onChunk func(string)
}

func newStreamingCollector(onChunk func(string)) *streamingCollector {
	return &streamingCollector{onChunk: onChunk}
}

func (c *streamingCollector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, err := c.buf.Write(p)
	if err != nil {
		return n, err
	}
	if c.onChunk != nil && len(p) > 0 {
		c.onChunk(string(p))
	}
	return n, nil
}

func (c *streamingCollector) String() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func BootstrapScriptPath() (string, error) {
	if path, ok := findBootstrapScript(); ok {
		return path, nil
	}
	return "", fmt.Errorf("bootstrap script not found (expected scripts/bootstrap_python.sh)")
}

func resolveShell() (string, error) {
	if shell, err := exec.LookPath("bash"); err == nil {
		return shell, nil
	}
	return "", fmt.Errorf("bash not found; scripts/bootstrap_python.sh requires bash")
}

func findBootstrapScript() (string, bool) {
	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, "scripts", "bootstrap_python.sh")
		if fileExists(candidate) {
			return candidate, true
		}
	}

	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "scripts", "bootstrap_python.sh")
		if fileExists(candidate) {
			return candidate, true
		}
	}

	return "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
