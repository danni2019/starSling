package tui

import (
	"strings"
	"testing"
)

func TestRenderLogoTitleTemplate(t *testing.T) {
	logo := RenderLogo(120, 0)
	if len(logo) != logoHeight {
		t.Fatalf("expected logo height %d, got %d", logoHeight, len(logo))
	}

	titleLines := strings.Split(strings.Trim(titleArt, "\n"), "\n")
	if len(titleLines) != titleHeight {
		t.Fatalf("expected title height %d, got %d", titleHeight, len(titleLines))
	}

	for i, line := range titleLines {
		got := strings.TrimSpace(logo[curveHeight+i])
		want := strings.TrimSpace(line)
		if got != want {
			t.Fatalf("title line %d mismatch\nwant: %q\ngot:  %q", i, want, got)
		}
		for _, r := range logo[curveHeight+i] {
			if r == ' ' || r == '█' {
				continue
			}
			t.Fatalf("invalid title rune %q on line %d", r, i)
		}
	}
}

func TestRenderLogoCurve(t *testing.T) {
	logo := RenderLogo(96, 0)
	curve := logo[:curveHeight]

	curveCount := 0
	minX := 1 << 30
	maxX := -1
	startY := -1
	endY := -1
	minY := 1 << 30
	width := 0
	starX := -1
	starY := -1

	for y, line := range curve {
		runes := []rune(line)
		if len(runes) > width {
			width = len(runes)
		}
		for x, r := range runes {
			if r == '█' {
				t.Fatalf("curve area contains forbidden block rune at (%d,%d)", x, y)
			}
			if isStarRune(r) {
				if starX == -1 || x > starX {
					starX = x
					starY = y
				}
				continue
			}
			if !isCurveRune(r) {
				continue
			}
			curveCount++
			if x < minX {
				minX = x
				startY = y
			} else if x == minX && y > startY {
				startY = y
			}
			if x > maxX {
				maxX = x
				endY = y
			} else if x == maxX && y > endY {
				endY = y
			}
		}
	}

	if curveCount < 20 {
		t.Fatalf("expected at least 20 curve runes, got %d", curveCount)
	}
	if width == 0 {
		t.Fatalf("curve width not detected")
	}

	left := int(0.50 * float64(width))
	right := int(0.70 * float64(width))
	for y, line := range curve {
		for x, r := range []rune(line) {
			if !isCurveRune(r) {
				continue
			}
			if x >= left && x <= right && y < minY {
				minY = y
			}
		}
	}

	if startY == -1 || endY == -1 || minY == 1<<30 {
		t.Fatalf("curve points not detected for apex check")
	}
	if minY >= startY-2 {
		t.Fatalf("apex too low: minY=%d startY=%d", minY, startY)
	}
	if endY >= startY {
		t.Fatalf("curve should rise from start to end: startY=%d endY=%d", startY, endY)
	}
	if starX == -1 || starY == -1 {
		t.Fatalf("star not detected on curve")
	}
	if starY >= endY {
		t.Fatalf("star should be above curve end: starY=%d endY=%d", starY, endY)
	}
}

func isCurveRune(r rune) bool {
	if r >= 0x2800 && r <= 0x28FF {
		return r != 0x2800
	}
	switch r {
	case '─', '╱', '╲', '│', '•':
		return true
	default:
		return false
	}
}

func isStarRune(r rune) bool {
	return r == '✶' || r == '✸' || r == '*'
}
