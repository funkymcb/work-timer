package worklog

import (
	"testing"
	"time"
)

func TestActiveWeekRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())

	active, err := s.ActiveWeek()
	if err != nil {
		t.Fatalf("active week on empty store: %v", err)
	}
	if active != nil {
		t.Fatalf("empty store has an active week: %+v", active)
	}

	mustDo(t, s.SaveWeek(NewWeek(onDay(9, 8))))

	active, err = s.ActiveWeek()
	if err != nil {
		t.Fatalf("active week: %v", err)
	}
	if active == nil {
		t.Fatal("saved week is not reported as active")
	}
	if got, want := active.Start, "2026-03-09"; got != want {
		t.Errorf("start = %q, want %q", got, want)
	}

	mustDo(t, active.Close(onDay(13, 17)))
	mustDo(t, s.SaveWeek(active))

	active, err = s.ActiveWeek()
	if err != nil {
		t.Fatalf("active week after close: %v", err)
	}
	if active != nil {
		t.Errorf("closed week still reported as active: %+v", active)
	}
}

func TestLastWeekIsTheLatestStarted(t *testing.T) {
	s := NewStore(t.TempDir())

	first := NewWeek(onDay(2, 8))
	mustDo(t, first.Close(onDay(6, 17)))
	mustDo(t, s.SaveWeek(first))
	mustDo(t, s.SaveWeek(NewWeek(onDay(9, 8))))

	last, err := s.LastWeek()
	if err != nil {
		t.Fatalf("last week: %v", err)
	}
	if got, want := last.Start, "2026-03-09"; got != want {
		t.Errorf("start = %q, want %q", got, want)
	}
}

func TestWeekDaysOnlyCoversTheWeek(t *testing.T) {
	s := NewStore(t.TempDir())

	// One day before the week, three inside it, one after.
	for _, day := range []int{6, 9, 10, 11, 16} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}
	week := NewWeek(onDay(9, 8))
	mustDo(t, week.Close(onDay(13, 17)))
	mustDo(t, s.SaveWeek(week))

	report, err := s.Report(week, onDay(16, 20))
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
	if got, want := report.Days[2].Date, "2026-03-11"; got != want {
		t.Errorf("last day = %q, want %q", got, want)
	}
}

func TestActiveWeekReportStopsAtToday(t *testing.T) {
	s := NewStore(t.TempDir())

	mustDo(t, s.Save(workedDay(t, 9, 8)))
	mustDo(t, s.Save(workedDay(t, 10, 8)))
	// A stray file from a later date must not be pulled into a running week.
	mustDo(t, s.Save(workedDay(t, 20, 8)))
	week := NewWeek(onDay(9, 8))
	mustDo(t, s.SaveWeek(week))

	report, err := s.Report(week, onDay(10, 18))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if got, want := report.WorkDays(), 2; got != want {
		t.Errorf("work days = %d, want %d", got, want)
	}
	if got, want := report.Worked, 16*time.Hour; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
}

func TestHistoryGroupsDaysByWeek(t *testing.T) {
	s := NewStore(t.TempDir())

	// A day from before weeks were kept, a closed week of three days, and a
	// running week with today in it.
	mustDo(t, s.Save(workedDay(t, 6, 2)))
	closed := NewWeek(onDay(9, 8))
	mustDo(t, closed.Close(onDay(11, 17)))
	mustDo(t, s.SaveWeek(closed))
	for _, day := range []int{9, 10, 11} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}
	mustDo(t, s.SaveWeek(NewWeek(onDay(16, 8))))
	mustDo(t, s.Save(workedDay(t, 16, 6)))

	weeks, loose, err := s.History(onDay(16, 20), "", "")
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	if got, want := len(weeks), 2; got != want {
		t.Fatalf("weeks = %d, want %d", got, want)
	}
	if got, want := weeks[0].WorkDays(), 3; got != want {
		t.Errorf("days in the closed week = %d, want %d", got, want)
	}
	if got, want := weeks[0].Worked, 24*time.Hour; got != want {
		t.Errorf("worked in the closed week = %v, want %v", got, want)
	}
	if got, want := weeks[1].WorkDays(), 1; got != want {
		t.Errorf("days in the running week = %d, want %d", got, want)
	}
	if got, want := len(loose), 1; got != want {
		t.Fatalf("days outside every week = %d, want %d", got, want)
	}
	if got, want := loose[0].Date, "2026-03-06"; got != want {
		t.Errorf("loose day = %q, want %q", got, want)
	}
}

func TestHistorySinceCutsWeeksAndDays(t *testing.T) {
	s := NewStore(t.TempDir())

	// The same shape as above: a loose day, a closed week, a running one.
	mustDo(t, s.Save(workedDay(t, 6, 2)))
	closed := NewWeek(onDay(9, 8))
	mustDo(t, closed.Close(onDay(11, 17)))
	mustDo(t, s.SaveWeek(closed))
	for _, day := range []int{9, 10, 11} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}
	mustDo(t, s.SaveWeek(NewWeek(onDay(16, 8))))
	mustDo(t, s.Save(workedDay(t, 16, 6)))

	// From the middle of the closed week on: its first day and the loose day
	// before it are gone, the rest stands.
	weeks, loose, err := s.History(onDay(16, 20), "2026-03-10", "")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := len(weeks), 2; got != want {
		t.Fatalf("weeks = %d, want %d", got, want)
	}
	if got, want := weeks[0].WorkDays(), 2; got != want {
		t.Errorf("days left of the closed week = %d, want %d", got, want)
	}
	// The totals have to follow the days that are left, or they would not add
	// up to what is being reported.
	if got, want := weeks[0].Worked, 16*time.Hour; got != want {
		t.Errorf("worked in the closed week = %v, want %v", got, want)
	}
	if len(loose) != 0 {
		t.Errorf("loose days = %+v, want none", loose)
	}

	// From after the closed week, only the running one is left.
	weeks, _, err = s.History(onDay(16, 20), "2026-03-16", "")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := len(weeks), 1; got != want {
		t.Fatalf("weeks = %d, want %d", got, want)
	}
	if got, want := weeks[0].Week.Start, "2026-03-16"; got != want {
		t.Errorf("week = %q, want %q", got, want)
	}

	// Past everything on record, nothing is left at all.
	weeks, loose, err = s.History(onDay(16, 20), "2026-03-20", "")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(weeks) != 0 || len(loose) != 0 {
		t.Errorf("weeks = %+v, loose = %+v, want neither past the last record", weeks, loose)
	}
}

func TestHistoryKeepsAnEmptyWeekInsideTheRange(t *testing.T) {
	s := NewStore(t.TempDir())

	// A week opened today, before the day was started: it has nothing to
	// report yet, but it is not over either.
	mustDo(t, s.SaveWeek(NewWeek(onDay(16, 8))))

	weeks, _, err := s.History(onDay(16, 20), "2026-03-16", "")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := len(weeks), 1; got != want {
		t.Fatalf("weeks = %d, want %d - a week opened inside the range should stay", got, want)
	}
	if got, want := weeks[0].WorkDays(), 0; got != want {
		t.Errorf("work days = %d, want %d", got, want)
	}

	// From the day after, it falls out of the range.
	weeks, _, err = s.History(onDay(16, 20), "2026-03-17", "")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(weeks) != 0 {
		t.Errorf("weeks = %+v, want none", weeks)
	}
}

func TestWeekFilesAreNotReadAsDays(t *testing.T) {
	s := NewStore(t.TempDir())

	mustDo(t, s.SaveWeek(NewWeek(onDay(9, 8))))
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

func TestHistoryUntilCutsTheOtherEnd(t *testing.T) {
	s := NewStore(t.TempDir())

	closed := NewWeek(onDay(9, 8))
	mustDo(t, closed.Close(onDay(11, 17)))
	mustDo(t, s.SaveWeek(closed))
	for _, day := range []int{9, 10, 11} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}
	mustDo(t, s.SaveWeek(NewWeek(onDay(16, 8))))
	mustDo(t, s.Save(workedDay(t, 16, 6)))

	// Up to the middle of the closed week: its last day is gone, and the week
	// that starts after the cutoff is dropped whole.
	weeks, _, err := s.History(onDay(16, 20), "", "2026-03-10")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := len(weeks), 1; got != want {
		t.Fatalf("weeks = %d, want %d", got, want)
	}
	if got, want := weeks[0].WorkDays(), 2; got != want {
		t.Errorf("days left of the closed week = %d, want %d", got, want)
	}
	if got, want := weeks[0].Worked, 16*time.Hour; got != want {
		t.Errorf("worked in the closed week = %v, want %v", got, want)
	}

	// The cutoff day itself is part of the range.
	weeks, _, err = s.History(onDay(16, 20), "2026-03-11", "2026-03-11")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got, want := len(weeks), 1; got != want {
		t.Fatalf("weeks = %d, want %d", got, want)
	}
	if got, want := weeks[0].WorkDays(), 1; got != want {
		t.Errorf("work days = %d, want %d - both ends are inclusive", got, want)
	}
	if got, want := weeks[0].Days[0].Date, "2026-03-11"; got != want {
		t.Errorf("day = %q, want %q", got, want)
	}
}
