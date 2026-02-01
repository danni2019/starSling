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

func RenderLogo(width int, frame int) []string {
	logoWidth := min(width, maxLogoWidth)
	if logoWidth <= 0 {
		return blankLogo()
	}

	curve := renderCurve(logoWidth, curveHeight, frame)
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

func renderCurve(width, height int, frame int) []string {
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	subWidth := width * 2
	subHeight := height * 4
	sub := make([][]bool, subHeight)
	for i := range sub {
		sub[i] = make([]bool, subWidth)
	}

	p0sx := int(math.Round(0.12 * float64(subWidth)))
	p0sy := int(math.Round(0.80 * float64(subHeight)))
	pmsx := int(math.Round(0.60 * float64(subWidth)))
	pmsy := int(math.Round(0.08 * float64(subHeight)))
	p1sx := int(math.Round(0.88 * float64(subWidth)))
	p1sy := int(math.Round(0.05 * float64(subHeight)))
	curveYOffset := 0
	if p1sy < 4 {
		curveYOffset = 4 - p1sy
	}

	steps := max(1, 4*width)
	prevX := -1
	prevY := -1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := quadBezier(p0sx, pmsx, p1sx, t)
		y := quadBezier(p0sy, pmsy, p1sy, t) + float64(curveYOffset)
		ix := int(math.Round(x))
		iy := int(math.Round(y))
		if prevX >= 0 {
			drawSubSegment(sub, prevX, prevY, ix, iy)
		}
		prevX = ix
		prevY = iy
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			mask := 0
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					if sub[y*4+dy][x*2+dx] {
						mask |= brailleBit(dx, dy)
					}
				}
			}
			if mask != 0 {
				grid[y][x] = rune(0x2800 + mask)
			}
		}
	}

	starX := int(math.Round(float64(p1sx) / 2.0))
	starY := int(math.Round(float64(p1sy+curveYOffset)/4.0)) - 1
	if starY < 0 {
		starY = 0
	}
	if starX >= 0 && starX < width && starY >= 0 && starY < height {
		grid[starY][starX] = starRune(frame)
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

func drawSubSegment(grid [][]bool, x0, y0, x1, y1 int) {
	height := len(grid)
	if height == 0 {
		return
	}
	width := len(grid[0])
	dx := x1 - x0
	dy := y1 - y0
	steps := max(abs(dx), abs(dy))
	if steps == 0 {
		setSubPixel(grid, width, height, x0, y0)
		return
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + float64(dx)*t))
		y := int(math.Round(float64(y0) + float64(dy)*t))
		setSubPixel(grid, width, height, x, y)
	}
}

func setSubPixel(grid [][]bool, width, height, x, y int) {
	if x < 0 || x >= width || y < 0 || y >= height {
		return
	}
	grid[y][x] = true
}

func brailleBit(dx, dy int) int {
	switch {
	case dx == 0 && dy == 0:
		return 1
	case dx == 0 && dy == 1:
		return 2
	case dx == 0 && dy == 2:
		return 4
	case dx == 0 && dy == 3:
		return 64
	case dx == 1 && dy == 0:
		return 8
	case dx == 1 && dy == 1:
		return 16
	case dx == 1 && dy == 2:
		return 32
	case dx == 1 && dy == 3:
		return 128
	default:
		return 0
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func starRune(frame int) rune {
	if frame%2 == 0 {
		return '✶'
	}
	return '✸'
}

func maxLineWidthTrimmed(lines []string) int {
	maxWidth := 0
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if len(trimmed) > maxWidth {
			maxWidth = len(trimmed)
		}
	}
	return maxWidth
}
