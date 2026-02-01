package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func RunBootstrap() (string, error) {
	script, err := resolveBootstrapScript()
	if err != nil {
		return "", err
	}
	shell, err := resolveShell()
	if err != nil {
		return "", err
	}

	cmd := exec.Command(shell, script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("bootstrap failed: %w", err)
	}
	return string(output), nil
}

func resolveShell() (string, error) {
	if shell, err := exec.LookPath("bash"); err == nil {
		return shell, nil
	}
	return "", fmt.Errorf("bash not found; scripts/bootstrap_python.sh requires bash")
}

func resolveBootstrapScript() (string, error) {
	if path, ok := findBootstrapScript(); ok {
		return path, nil
	}
	return "", fmt.Errorf("bootstrap script not found (expected scripts/bootstrap_python.sh)")
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
