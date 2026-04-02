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
