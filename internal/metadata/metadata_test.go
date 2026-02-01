package metadata

import (
	"testing"
	"time"
)

func TestIsStale(t *testing.T) {
	now := time.Date(2026, 1, 30, 12, 0, 0, 0, time.UTC)

	if !IsStale(time.Time{}, now, RefreshAfter) {
		t.Fatalf("expected zero time to be stale")
	}
	if IsStale(now.Add(-1*time.Hour), now, RefreshAfter) {
		t.Fatalf("expected 1h old metadata to be fresh")
	}
	if !IsStale(now.Add(-2*time.Hour), now, RefreshAfter) {
		t.Fatalf("expected 2h old metadata to be stale")
	}
}
