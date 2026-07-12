package scheduler

import (
	"testing"
	"time"
)

// mustParse parses a schedule expression or fails the test.
func mustParse(t *testing.T, expr string) Schedule {
	t.Helper()
	s, err := ParseSchedule(expr)
	if err != nil {
		t.Fatalf("ParseSchedule(%q): %v", expr, err)
	}
	return s
}

// TestCronDayOfWeekOnly verifies that a restricted day-of-week with a star
// day-of-month only fires on the matching weekday (standard cron semantics).
func TestCronDayOfWeekOnly(t *testing.T) {
	// "0 2 * * 1" = 02:00 every Monday.
	s := mustParse(t, "0 2 * * 1")
	// Wednesday 2026-07-08 10:00 UTC → next Monday 2026-07-13 02:00 UTC.
	from := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	want := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	if got := s.Next(from); !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
}

// TestCronDayOfMonthOnly verifies that a restricted day-of-month with a star
// day-of-week only fires on the matching day of the month.
func TestCronDayOfMonthOnly(t *testing.T) {
	// "30 4 15 * *" = 04:30 on the 15th of every month.
	s := mustParse(t, "30 4 15 * *")
	from := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	want := time.Date(2026, 8, 15, 4, 30, 0, 0, time.UTC)
	if got := s.Next(from); !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
}

// TestCronDomDowBothRestrictedIsOr verifies vixie-cron OR semantics: when both
// day fields are restricted, the schedule fires when EITHER matches.
func TestCronDomDowBothRestrictedIsOr(t *testing.T) {
	// "0 0 13 * 5" = midnight on the 13th OR any Friday.
	s := mustParse(t, "0 0 13 * 5")
	// From Wed 2026-07-08: Friday 2026-07-10 comes before Monday the 13th.
	from := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	want := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	got := s.Next(from)
	if !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want Friday %v", from, got, want)
	}
	// From Sat 2026-07-11: Monday the 13th comes before the next Friday.
	from = time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	want = time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	if got := s.Next(from); !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want 13th %v", from, got, want)
	}
}

// TestCronBothDayFieldsStar verifies that star day fields match every day.
func TestCronBothDayFieldsStar(t *testing.T) {
	// "0 2 * * *" = 02:00 every day.
	s := mustParse(t, "0 2 * * *")
	from := time.Date(2026, 7, 8, 3, 0, 0, 0, time.UTC)
	want := time.Date(2026, 7, 9, 2, 0, 0, 0, time.UTC)
	if got := s.Next(from); !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
}

// TestCronDayAdvanceKeepsLocation verifies day skipping lands on local
// midnight, not UTC midnight, in a non-UTC location.
func TestCronDayAdvanceKeepsLocation(t *testing.T) {
	loc := time.FixedZone("UTC+9", 9*3600)
	// "0 2 * * 1" = 02:00 every Monday, evaluated in UTC+9.
	s := mustParse(t, "0 2 * * 1")
	from := time.Date(2026, 7, 8, 10, 0, 0, 0, loc)
	want := time.Date(2026, 7, 13, 2, 0, 0, 0, loc)
	if got := s.Next(from); !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", from, got, want)
	}
}
