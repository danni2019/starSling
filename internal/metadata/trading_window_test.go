package metadata

import (
	"testing"
	"time"
)

func TestInTradingWindowAppliesSymmetricPaddingForDaySession(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	seg := TradeSegment{
		Start: mustTimeOfDay(t, "09:00:00"),
		End:   mustTimeOfDay(t, "10:15:00"),
	}

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{
			name: "before padded window",
			now:  time.Date(2026, 3, 12, 8, 44, 59, 0, loc),
			want: false,
		},
		{
			name: "at padded open",
			now:  time.Date(2026, 3, 12, 8, 45, 0, 0, loc),
			want: true,
		},
		{
			name: "at padded close",
			now:  time.Date(2026, 3, 12, 10, 30, 0, 0, loc),
			want: true,
		},
		{
			name: "after padded close",
			now:  time.Date(2026, 3, 12, 10, 30, 1, 0, loc),
			want: false,
		},
	}

	for _, tc := range cases {
		if got := InTradingWindow(tc.now, []TradeSegment{seg}); got != tc.want {
			t.Fatalf("%s: InTradingWindow() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestInTradingWindowAppliesSymmetricPaddingForOvernightSession(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	seg := TradeSegment{
		Start: mustTimeOfDay(t, "21:00:00"),
		End:   mustTimeOfDay(t, "02:30:00"),
	}

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{
			name: "before padded overnight open",
			now:  time.Date(2026, 3, 11, 20, 44, 59, 0, loc),
			want: false,
		},
		{
			name: "at padded overnight open",
			now:  time.Date(2026, 3, 11, 20, 45, 0, 0, loc),
			want: true,
		},
		{
			name: "inside overnight from next day clock",
			now:  time.Date(2026, 3, 12, 1, 0, 0, 0, loc),
			want: true,
		},
		{
			name: "at padded overnight close",
			now:  time.Date(2026, 3, 12, 2, 45, 0, 0, loc),
			want: true,
		},
		{
			name: "after padded overnight close",
			now:  time.Date(2026, 3, 12, 2, 45, 1, 0, loc),
			want: false,
		},
	}

	for _, tc := range cases {
		if got := InTradingWindow(tc.now, []TradeSegment{seg}); got != tc.want {
			t.Fatalf("%s: InTradingWindow() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func mustTimeOfDay(t *testing.T, raw string) time.Duration {
	t.Helper()
	value, err := parseTimeOfDay(raw)
	if err != nil {
		t.Fatalf("parseTimeOfDay(%q): %v", raw, err)
	}
	return value
}
