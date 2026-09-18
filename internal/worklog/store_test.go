package worklog

import (
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())

	day := NewDay(at(8, 0))
	mustDo(t, day.Start(at(8, 0)))
	mustDo(t, day.Pause(at(12, 0)))
	mustDo(t, day.Resume(at(12, 30)))
	mustDo(t, s.Save(day))

	loaded, err := s.Load(at(23, 0))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, want := loaded.State(), Working; got != want {
		t.Errorf("state = %v, want %v", got, want)
	}
	if got, want := loaded.Worked(at(17, 0)), 8*time.Hour+30*time.Minute; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	if !loaded.FirstStart().Equal(at(8, 0)) {
		t.Errorf("first start = %v, want %v", loaded.FirstStart(), at(8, 0))
	}
}

func TestLoadMissingDayIsEmpty(t *testing.T) {
	s := NewStore(t.TempDir())

	day, err := s.Load(at(8, 0))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if day.Started() {
		t.Errorf("missing day reported as started: %+v", day)
	}
	if got, want := day.Date, "2026-03-10"; got != want {
		t.Errorf("date = %q, want %q", got, want)
	}
}

func TestCurrentFollowsSessionAcrossMidnight(t *testing.T) {
	s := NewStore(t.TempDir())

	// Started yesterday at 21:00 and never stopped.
	yesterday := NewDay(at(21, 0))
	mustDo(t, yesterday.Start(at(21, 0)))
	mustDo(t, s.Save(yesterday))

	// It is now 00:30 the next day.
	now := at(21, 0).Add(3*time.Hour + 30*time.Minute)
	current, err := s.Current(now)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if got, want := current.Date, "2026-03-10"; got != want {
		t.Fatalf("date = %q, want yesterday %q", got, want)
	}
	if got, want := current.Worked(now), 3*time.Hour+30*time.Minute; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
}

func TestCurrentIgnoresFinishedYesterday(t *testing.T) {
	s := NewStore(t.TempDir())

	yesterday := NewDay(at(9, 0))
	mustDo(t, yesterday.Start(at(9, 0)))
	mustDo(t, yesterday.Stop(at(17, 0)))
	mustDo(t, s.Save(yesterday))

	now := at(9, 0).AddDate(0, 0, 1)
	current, err := s.Current(now)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if got, want := current.Date, "2026-03-11"; got != want {
		t.Errorf("date = %q, want today %q", got, want)
	}
	if current.Started() {
		t.Errorf("today reported as started: %+v", current)
	}
}
