package worklog

import (
	"testing"
	"time"
)

// onDay returns a time on the given day of March 2026 at the given hour.
// 2026-03-09 is a Monday, so the week runs 9 to 15 March.
func onDay(day, hour int) time.Time {
	return time.Date(2026, 3, day, hour, 0, 0, 0, time.Local)
}

// workedDay builds a finished day with the given number of worked hours.
func workedDay(t *testing.T, day int, hours int) *Day {
	t.Helper()
	d := NewDay(onDay(day, 8))
	mustDo(t, d.Start(onDay(day, 8)))
	mustDo(t, d.Stop(onDay(day, 8+hours)))
	return d
}

func TestWeekOf(t *testing.T) {
	tests := []struct {
		name  string
		day   int
		start string
		end   string
	}{
		{"monday opens the week", 9, "2026-03-09", "2026-03-15"},
		{"midweek", 11, "2026-03-09", "2026-03-15"},
		{"friday", 13, "2026-03-09", "2026-03-15"},
		// Go counts Sunday as weekday zero; it has to close the week it
		// followed rather than open the next one.
		{"sunday closes it", 15, "2026-03-09", "2026-03-15"},
		{"the monday after opens the next", 16, "2026-03-16", "2026-03-22"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			week := WeekOf(onDay(tc.day, 8))
			if week.Start != tc.start || week.End != tc.end {
				t.Errorf("WeekOf(%d Mar) = %s..%s, want %s..%s", tc.day, week.Start, week.End, tc.start, tc.end)
			}
		})
	}
}

func TestWeekOfCrossesAMonth(t *testing.T) {
	// Sunday 1 March 2026 belongs to the week that started in February.
	week := WeekOf(time.Date(2026, 3, 1, 12, 0, 0, 0, time.Local))
	if got, want := week.Start, "2026-02-23"; got != want {
		t.Errorf("start = %q, want %q", got, want)
	}
	if got, want := week.End, "2026-03-01"; got != want {
		t.Errorf("end = %q, want %q", got, want)
	}
}

func TestWeekContainsAndCurrent(t *testing.T) {
	week := WeekOf(onDay(11, 8))

	for _, date := range []string{"2026-03-09", "2026-03-12", "2026-03-15"} {
		if !week.Contains(date) {
			t.Errorf("Contains(%q) = false, want true", date)
		}
	}
	for _, date := range []string{"2026-03-08", "2026-03-16"} {
		if week.Contains(date) {
			t.Errorf("Contains(%q) = true, want false", date)
		}
	}
	if !week.Current(onDay(13, 20)) {
		t.Error("the week is not current on a day inside it")
	}
	if week.Current(onDay(16, 8)) {
		t.Error("the week is still current a week later")
	}
}

func TestWeekReportCountsWeekdaysApartFromWeekends(t *testing.T) {
	week := WeekOf(onDay(9, 8))
	// Monday, Wednesday and Saturday.
	days := []*Day{workedDay(t, 9, 8), workedDay(t, 11, 7), workedDay(t, 14, 3)}

	r := NewWeekReport(week, days, onDay(15, 20))

	if got, want := r.Worked, 18*time.Hour; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	if got, want := r.WorkDays(), 3; got != want {
		t.Errorf("work days = %d, want %d", got, want)
	}
	if got, want := r.Weekdays(), 2; got != want {
		t.Errorf("weekdays = %d, want %d", got, want)
	}
	if got, want := r.WeekendDays(), 1; got != want {
		t.Errorf("weekend days = %d, want %d", got, want)
	}
	if got, want := r.Average(), 6*time.Hour; got != want {
		t.Errorf("average = %v, want %v", got, want)
	}
}

func TestEmptyWeekReport(t *testing.T) {
	r := NewWeekReport(WeekOf(onDay(9, 8)), nil, onDay(9, 20))

	if r.WorkDays() != 0 || r.Weekdays() != 0 || r.WeekendDays() != 0 {
		t.Errorf("empty week counts days: %+v", r)
	}
	if r.Average() != 0 {
		t.Errorf("average = %v, want 0", r.Average())
	}
}
