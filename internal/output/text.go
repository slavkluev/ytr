package output

import "github.com/mattn/go-runewidth"

const displayEllipsis = "..."

// TruncateDisplay shortens a string to fit within maxWidth display cells.
// It preserves valid UTF-8 boundaries and appends an ellipsis when possible.
func TruncateDisplay(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}

	if maxWidth <= runewidth.StringWidth(displayEllipsis) {
		return runewidth.Truncate(s, maxWidth, "")
	}

	return runewidth.Truncate(s, maxWidth, displayEllipsis)
}

// FitColumn shortens s to the room the last table column has left once the
// fixed columns took reserved cells, never going below minWidth. Off a TTY
// there is no width to fit, so the value is returned whole.
func FitColumn(s string, reserved, minWidth int) string {
	if !IsTTY() {
		return s
	}
	return TruncateDisplay(s, max(TerminalWidth()-reserved, minWidth))
}
