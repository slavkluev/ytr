package output_test

import (
	"testing"
	"time"

	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
)

func TestTimeAgo_JustNow(t *testing.T) {
	got := output.TimeAgo(time.Now())
	if got != "just now" {
		t.Errorf("TimeAgo(now) = %q, want %q", got, "just now")
	}
}

func TestTimeAgo_MinutesAgo(t *testing.T) {
	got := output.TimeAgo(time.Now().Add(-30 * time.Minute))
	if got != "30m ago" {
		t.Errorf("TimeAgo(30m ago) = %q, want %q", got, "30m ago")
	}
}

func TestTimeAgo_HoursAgo(t *testing.T) {
	got := output.TimeAgo(time.Now().Add(-5 * time.Hour))
	if got != "5h ago" {
		t.Errorf("TimeAgo(5h ago) = %q, want %q", got, "5h ago")
	}
}

func TestTimeAgo_DaysAgo(t *testing.T) {
	got := output.TimeAgo(time.Now().Add(-3 * 24 * time.Hour))
	if got != "3d ago" {
		t.Errorf("TimeAgo(3d ago) = %q, want %q", got, "3d ago")
	}
}

func TestTimeAgo_OlderDate(t *testing.T) {
	// 60 days ago -- should show month+day format
	target := time.Now().Add(-60 * 24 * time.Hour)
	got := output.TimeAgo(target)
	want := target.Format("Jan 2")
	if got != want {
		t.Errorf("TimeAgo(60d ago) = %q, want %q", got, want)
	}
}

func TestTimeAgo_YearOld(t *testing.T) {
	// 400 days ago -- should show full date with year
	target := time.Now().Add(-400 * 24 * time.Hour)
	got := output.TimeAgo(target)
	want := target.Format("Jan 2, 2006")
	if got != want {
		t.Errorf("TimeAgo(400d ago) = %q, want %q", got, want)
	}
}

func TestFormatTimeOffTTYIsRFC3339(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	moment := time.Date(2026, 9, 19, 14, 22, 31, 0, time.FixedZone("MSK", 3*60*60))
	if got, want := output.FormatTime(moment), "2026-09-19T14:22:31+03:00"; got != want {
		t.Errorf("FormatTime off a TTY = %q, want %q", got, want)
	}
}

func TestFormatTimeOffTTYKeepsTheServerOffset(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(false)

	moment := time.Date(2026, 9, 19, 14, 22, 31, 0, time.FixedZone("", -7*60*60))
	if got, want := output.FormatTime(moment), "2026-09-19T14:22:31-07:00"; got != want {
		t.Errorf("FormatTime off a TTY = %q, want %q", got, want)
	}
}

func TestFormatTimeOnTTYIsRelative(t *testing.T) {
	testutil.ResetOutputFlags(t)
	output.SetTTY(true)

	if got, want := output.FormatTime(time.Now().Add(-3*time.Hour)), "3h ago"; got != want {
		t.Errorf("FormatTime on a TTY = %q, want %q", got, want)
	}
}
