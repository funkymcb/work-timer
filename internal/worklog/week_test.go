package worklog

import (
	"errors"
	"testing"
	"time"
)

// onDay returns a time on the given day of March 2026 at the given hour.
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

func TestWeekClose(t *testing.T) {
	w := NewWeek(onDay(9, 8)) // Monday

	if !w.Active() {
		t.Fatal("new week is not active")
	}
	if err := w.Close(onDay(13, 17)); err != nil { // Friday
		t.Fatalf("close: %v", err)
	}
	if w.Active() {
		t.Fatal("week still active after close")
	}
	if got, want := w.End, "2026-03-13"; got != want {
		t.Errorf("end = %q, want %q", got, want)
	}
	if err := w.Close(onDay(14, 8)); !errors.Is(err, ErrNoActiveWeek) {
		t.Errorf("closing twice = %v, want %v", err, ErrNoActiveWeek)
	}
}

func TestWeekCannotEndBeforeItStarted(t *testing.T) {
	w := NewWeek(onDay(9, 8))
	if err := w.Close(onDay(8, 17)); !errors.Is(err, ErrWeekEndsEarly) {
		t.Fatalf("close in the past = %v, want %v", err, ErrWeekEndsEarly)
	}
}

func TestWeekValidate(t *testing.T) {
	tests := []struct {
		name    string
		week    Week
		wantErr bool
	}{
		{"active", Week{Start: "2026-03-09"}, false},
		{"closed", Week{Start: "2026-03-09", End: "2026-03-13"}, false},
		{"same day", Week{Start: "2026-03-09", End: "2026-03-09"}, false},
		{"garbage start", Week{Start: "monday"}, true},
		{"garbage end", Week{Start: "2026-03-09", End: "friday"}, true},
		{"end before start", Week{Start: "2026-03-09", End: "2026-03-08"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.week.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestWeekCovers(t *testing.T) {
	closed := Week{Start: "2026-03-09", End: "2026-03-13"}
	active := Week{Start: "2026-03-09"}
	now := onDay(11, 20)

	tests := []struct {
		name string
		week Week
		date string
		want bool
	}{
		{"the day before a closed week", closed, "2026-03-08", false},
		{"its first day", closed, "2026-03-09", true},
		{"its last day", closed, "2026-03-13", true},
		{"the day after a closed week", closed, "2026-03-14", false},
		{"today of a running week", active, "2026-03-11", true},
		// A file dated ahead of today must not be pulled into a running week.
		{"tomorrow of a running week", active, "2026-03-12", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.week.Covers(tc.date, now); got != tc.want {
				t.Errorf("Covers(%q) = %v, want %v", tc.date, got, tc.want)
			}
		})
	}
}

func TestWeekReportTotals(t *testing.T) {
	w := NewWeek(onDay(9, 8))
	days := []*Day{workedDay(t, 9, 8), workedDay(t, 10, 7), workedDay(t, 11, 6)}

	r := NewWeekReport(w, days, onDay(11, 20))

	if got, want := r.Worked, 21*time.Hour; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	if got, want := r.WorkDays(), 3; got != want {
		t.Errorf("work days = %d, want %d", got, want)
	}
	if got, want := r.DaysLeft(), 2; got != want {
		t.Errorf("days left = %d, want %d", got, want)
	}
	if r.Full() {
		t.Error("three day week reported as full")
	}
	if got, want := r.Average(), 7*time.Hour; got != want {
		t.Errorf("average = %v, want %v", got, want)
	}
	if !r.HasDay(onDay(10, 9)) {
		t.Error("HasDay missed a day that is in the week")
	}
	if r.HasDay(onDay(12, 9)) {
		t.Error("HasDay found a day that is not in the week")
	}
}

func TestWeekReportFullAtFiveDays(t *testing.T) {
	w := NewWeek(onDay(9, 8))
	var days []*Day
	for i := range MaxWorkDays {
		days = append(days, workedDay(t, 9+i, 8))
	}

	r := NewWeekReport(w, days, onDay(13, 20))

	if !r.Full() {
		t.Errorf("week with %d days is not full", MaxWorkDays)
	}
	if got := r.DaysLeft(); got != 0 {
		t.Errorf("days left = %d, want 0", got)
	}
	// The sixth day is the one that must be refused, not a second session on
	// a day that already counts.
	if !r.HasDay(onDay(13, 9)) {
		t.Error("last day of a full week should still be exempt from the limit")
	}
	if r.HasDay(onDay(14, 9)) {
		t.Error("a sixth day must not be part of the week")
	}
}

func TestEmptyWeekReport(t *testing.T) {
	r := NewWeekReport(NewWeek(onDay(9, 8)), nil, onDay(9, 12))

	if r.WorkDays() != 0 || r.Worked != 0 || r.Average() != 0 {
		t.Errorf("empty report = %+v, want zeroes", r)
	}
	if r.Full() {
		t.Error("empty week reported as full")
	}
	if got, want := r.DaysLeft(), MaxWorkDays; got != want {
		t.Errorf("days left = %d, want %d", got, want)
	}
}
