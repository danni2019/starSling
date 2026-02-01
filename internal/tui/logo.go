package tui

import (
	"math"
	"strings"
)

const (
	logoHeight   = 18
	curveHeight  = 10
	titleHeight  = 8
	maxLogoWidth = 110
)

func RenderLogo(width int) []string {
	logoWidth := min(width, maxLogoWidth)
	if logoWidth <= 0 {
		return blankLogo()
	}

	curve := renderCurve(logoWidth, curveHeight)
	titleLines := strings.Split(strings.Trim(titleArt, "\n"), "\n")
	lines := make([]string, 0, logoHeight)
	for _, line := range curve {
		lines = append(lines, padRight(line, logoWidth))
	}
	for i := 0; i < titleHeight; i++ {
		line := ""
		if i < len(titleLines) {
			line = titleLines[i]
		}
		lines = append(lines, padRight(line, logoWidth))
	}
	for len(lines) < logoHeight {
		lines = append(lines, strings.Repeat(" ", logoWidth))
	}
	return lines
}

func blankLogo() []string {
	lines := make([]string, logoHeight)
	for i := range lines {
		lines[i] = ""
	}
	return lines
}

func renderCurve(width, height int) []string {
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", width))
	}

	p0x := int(math.Round(0.12 * float64(width)))
	p0y := int(math.Round(0.80 * float64(height)))
	pmx := int(math.Round(0.60 * float64(width)))
	pmy := int(math.Round(0.10 * float64(height)))
	p1x := int(math.Round(0.88 * float64(width)))
	p1y := int(math.Round(0.30 * float64(height)))

	steps := max(1, 2*width)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := quadBezier(p0x, pmx, p1x, t)
		y := quadBezier(p0y, pmy, p1y, t)
		ix := int(math.Round(x))
		iy := int(math.Round(y))
		if ix < 0 || ix >= width || iy < 0 || iy >= height {
			continue
		}
		grid[iy][ix] = '·'
	}

	if p1x >= 0 && p1x < width && p1y >= 0 && p1y < height {
		grid[p1y][p1x] = '✶'
	}

	lines := make([]string, height)
	for i, row := range grid {
		lines[i] = string(row)
	}
	return lines
}

func quadBezier(p0, p1, p2 int, t float64) float64 {
	return math.Pow(1-t, 2)*float64(p0) + 2*(1-t)*t*float64(p1) + math.Pow(t, 2)*float64(p2)
}
