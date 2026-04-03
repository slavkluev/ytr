package output

import (
	"fmt"
	"time"
)

const (
	hoursPerDay = 24
	daysPerWeek = 7
	daysPerYear = 365
)

// TimeAgo formats a time.Time as a human-readable relative duration for table output.
// Returns "just now" for <1 minute, "Xm ago" for <1 hour, "Xh ago" for <24 hours,
// "Xd ago" for <7 days, "Jan 2" for <365 days, and "Jan 2, 2006" for older dates.
// JSON output always uses ISO 8601; this function is for table rendering only.
func TimeAgo(t time.Time) string {
	elapsed := time.Since(t)

	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		minutes := int(elapsed.Minutes())
		return fmt.Sprintf("%dm ago", minutes)
	case elapsed < hoursPerDay*time.Hour:
		hours := int(elapsed.Hours())
		return fmt.Sprintf("%dh ago", hours)
	case elapsed < daysPerWeek*hoursPerDay*time.Hour:
		days := int(elapsed.Hours() / hoursPerDay)
		return fmt.Sprintf("%dd ago", days)
	case elapsed < daysPerYear*hoursPerDay*time.Hour:
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}
