package runtime

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
)

func TestRunStreamingCommandStreamsAndCollectsCombinedOutput(t *testing.T) {
	var mu sync.Mutex
	var chunks []string
	cmd := exec.Command("sh", "-c", "printf 'hello\\n'; printf 'warn\\n' >&2; printf 'done\\n'")
	output, err := runStreamingCommand(cmd, func(chunk string) {
		mu.Lock()
		chunks = append(chunks, chunk)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("runStreamingCommand() error = %v", err)
	}
	if output == "" {
		t.Fatalf("expected non-empty output")
	}
	if !strings.Contains(output, "hello") || !strings.Contains(output, "warn") || !strings.Contains(output, "done") {
		t.Fatalf("expected combined output to include stdout and stderr, got %q", output)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(chunks) == 0 {
		t.Fatalf("expected streaming callback to receive chunks")
	}
}
