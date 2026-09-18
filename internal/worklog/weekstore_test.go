package worklog

import (
	"testing"
	"time"
)

func TestReportCoversTheWholeCalendarWeek(t *testing.T) {
	s := NewStore(t.TempDir())

	// One day before the week, three inside it, one after.
	for _, day := range []int{8, 9, 11, 13, 16} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}

	report, err := s.Report(WeekOf(onDay(11, 8)), onDay(16, 20))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if got, want := report.WorkDays(), 3; got != want {
		t.Fatalf("work days = %d, want %d", got, want)
	}
	if got, want := report.Worked, 24*time.Hour; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	if got, want := report.Days[0].Date, "2026-03-09"; got != want {
		t.Errorf("first day = %q, want %q", got, want)
	}
	if got, want := report.Days[2].Date, "2026-03-13"; got != want {
		t.Errorf("last day = %q, want %q", got, want)
	}
}

func TestCurrentWeekIsTheOneTodayFallsIn(t *testing.T) {
	s := NewStore(t.TempDir())
	mustDo(t, s.Save(workedDay(t, 9, 8)))
	mustDo(t, s.Save(workedDay(t, 16, 8)))

	report, err := s.CurrentWeek(onDay(17, 20))
	if err != nil {
		t.Fatalf("current week: %v", err)
	}
	if got, want := report.Week.Start, "2026-03-16"; got != want {
		t.Errorf("week start = %q, want %q", got, want)
	}
	if got, want := report.WorkDays(), 1; got != want {
		t.Errorf("work days = %d, want %d - last week must not be counted in", got, want)
	}
}

func TestHistoryGroupsDaysIntoCalendarWeeks(t *testing.T) {
	s := NewStore(t.TempDir())

	// Two days in the week of the 9th, one in the week of the 16th.
	for _, day := range []int{9, 13, 16} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}

	weeks, err := s.History(onDay(16, 20), "", "")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := len(weeks), 2; got != want {
		t.Fatalf("weeks = %d, want %d", got, want)
	}
	if got, want := weeks[0].Week.Start, "2026-03-09"; got != want {
		t.Errorf("first week = %q, want %q", got, want)
	}
	if got, want := weeks[0].WorkDays(), 2; got != want {
		t.Errorf("days in the first week = %d, want %d", got, want)
	}
	if got, want := weeks[1].Week.Start, "2026-03-16"; got != want {
		t.Errorf("second week = %q, want %q", got, want)
	}
	// A week with nothing recorded in it is not reported at all.
	for _, w := range weeks {
		if w.WorkDays() == 0 {
			t.Errorf("empty week reported: %+v", w.Week)
		}
	}
}

func TestHistoryRangeCutsWeeksAtBothEnds(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, day := range []int{9, 10, 11, 12, 13, 16} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}

	weeks, err := s.History(onDay(16, 20), "2026-03-10", "2026-03-12")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := len(weeks), 1; got != want {
		t.Fatalf("weeks = %d, want %d", got, want)
	}
	if got, want := weeks[0].WorkDays(), 3; got != want {
		t.Errorf("days left = %d, want %d - both ends are inclusive", got, want)
	}
	// The totals have to follow the days that are left, or they would not add
	// up to what is being reported.
	if got, want := weeks[0].Worked, 24*time.Hour; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	// The week itself is still the whole calendar week.
	if got, want := weeks[0].Week.End, "2026-03-15"; got != want {
		t.Errorf("week end = %q, want %q", got, want)
	}
}

func TestGapBefore(t *testing.T) {
	tests := []struct {
		name     string
		recorded []int
		start    int
		want     []string
	}{
		{
			name:     "back on tuesday with no monday",
			recorded: []int{6}, // Friday
			start:    10,       // Tuesday
			want:     []string{"2026-03-09"},
		},
		{
			name:     "back on monday with no friday before it",
			recorded: []int{5}, // Thursday
			start:    9,        // Monday
			want:     []string{"2026-03-06"},
		},
		{
			// The weekend in between is not a gap.
			name:     "straight from friday to monday",
			recorded: []int{6},
			start:    9,
			want:     nil,
		},
		{
			name:     "a day off midweek",
			recorded: []int{9, 10},
			start:    12,
			want:     []string{"2026-03-11"},
		},
		{
			name:     "yesterday was logged",
			recorded: []int{9},
			start:    10,
			want:     nil,
		},
		{
			name:     "nothing on record at all",
			recorded: nil,
			start:    10,
			want:     nil,
		},
		{
			name:     "a longer absence",
			recorded: []int{6},
			start:    16,
			want:     []string{"2026-03-09", "2026-03-10", "2026-03-11", "2026-03-12", "2026-03-13"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore(t.TempDir())
			for _, day := range tc.recorded {
				mustDo(t, s.Save(workedDay(t, day, 8)))
			}

			gap, err := s.GapBefore(onDay(tc.start, 8))
			if err != nil {
				t.Fatalf("gap: %v", err)
			}
			var got []string
			for _, day := range gap.Weekdays {
				got = append(got, day.Format(DateLayout))
			}
			if len(got) != len(tc.want) {
				t.Fatalf("missing = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("missing[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestGapBeforeIgnoresTodaysOwnRecord(t *testing.T) {
	s := NewStore(t.TempDir())
	mustDo(t, s.Save(workedDay(t, 6, 8)))  // Friday
	mustDo(t, s.Save(workedDay(t, 10, 8))) // today, Tuesday

	gap, err := s.GapBefore(onDay(10, 18))
	if err != nil {
		t.Fatalf("gap: %v", err)
	}
	if got, want := len(gap.Weekdays), 1; got != want {
		t.Fatalf("missing = %v, want just Monday", gap.Weekdays)
	}
	if got, want := gap.Weekdays[0].Format(DateLayout), "2026-03-09"; got != want {
		t.Errorf("missing = %q, want %q", got, want)
	}
	// The summary counts off the last day that was logged, not off the gap.
	if got, want := gap.Since.Format(DateLayout), "2026-03-06"; got != want {
		t.Errorf("gap since = %q, want %q", got, want)
	}
}

func TestWeekFilesAreNotReadAsDays(t *testing.T) {
	s := NewStore(t.TempDir())

	// Left behind by the versions that recorded weeks by hand.
	mustDo(t, s.write(s.dir+"/week-2026-03-09.json", map[string]string{"start": "2026-03-09"}))
	mustDo(t, s.Save(workedDay(t, 9, 8)))

	days, err := s.Days("2026-03-01", "2026-03-31")
	if err != nil {
		t.Fatalf("days: %v", err)
	}
	if got, want := len(days), 1; got != want {
		t.Fatalf("days = %d, want %d (week file leaked in?)", got, want)
	}
	if got, want := days[0].Date, "2026-03-09"; got != want {
		t.Errorf("date = %q, want %q", got, want)
	}
}
