package tui

import "github.com/gdamore/tcell/v2"

var (
	colorBackground = tcell.NewRGBColor(8, 11, 13)
	colorAccent     = tcell.NewRGBColor(135, 215, 255)
	colorMuted      = tcell.NewRGBColor(111, 143, 143)
	colorBorder     = tcell.NewRGBColor(127, 179, 177)
	colorFocus      = tcell.NewRGBColor(155, 211, 255)
	colorHighlight  = tcell.NewRGBColor(31, 59, 58)

	colorTitle = tcell.NewRGBColor(155, 211, 255)
	colorCurve = tcell.NewRGBColor(243, 207, 107)
	colorStar  = tcell.NewRGBColor(255, 232, 154)

	colorMenu         = tcell.NewRGBColor(140, 183, 180)
	colorMenuSelected = tcell.NewRGBColor(183, 227, 216)

	colorTableHeader = tcell.NewRGBColor(124, 199, 255)
	colorTableRow    = tcell.NewRGBColor(157, 214, 211)
	colorLogText     = tcell.NewRGBColor(143, 184, 180)
)
