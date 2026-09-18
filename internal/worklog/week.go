package worklog

import (
	"errors"
	"fmt"
	"time"
)

// MaxWorkDays is the number of distinct working days a single work week may
// contain. Reaching it means the week has to be ended before work can start on
// another day.
const MaxWorkDays = 5

var (
	ErrNoActiveWeek  = errors.New("no work week is active")
	ErrWeekActive    = errors.New("a work week is already active")
	ErrWeekFull      = fmt.Errorf("a work week cannot cover more than %d working days", MaxWorkDays)
	ErrWeekEndsEarly = errors.New("a work week cannot end before it started")
)

// Week is an explicitly opened stretch of working days. Boundaries are whole
// calendar days, so a week runs from the first date through the last one
// inclusive. A week without an end is still running.
type Week struct {
	Start string `json:"start"`
	End   string `json:"end,omitempty"`
}

// NewWeek opens a week beginning on the calendar date of t.
func NewWeek(t time.Time) *Week { return &Week{Start: t.Format(DateLayout)} }

// Active reports whether the week is still running.
func (w *Week) Active() bool { return w.End == "" }

// StartDate is the first calendar day of the week. Weeks that came from the
// store always carry valid dates, because the store rejects the rest.
func (w *Week) StartDate() time.Time { return parseDate(w.Start) }

// EndDate is the last calendar day of the week, or the zero time while active.
func (w *Week) EndDate() time.Time { return parseDate(w.End) }

// Covers reports whether an ISO date belongs to the week. An active week
// covers every date from its start up to today, which is exactly the range its
// days are read from.
func (w *Week) Covers(date string, now time.Time) bool {
	if date < w.Start {
		return false
	}
	if w.Active() {
		return date <= now.Format(DateLayout)
	}
	return date <= w.End
}

// Close ends the week on the calendar date of t.
func (w *Week) Close(t time.Time) error {
	if !w.Active() {
		return ErrNoActiveWeek
	}
	end := t.Format(DateLayout)
	if end < w.Start {
		return ErrWeekEndsEarly
	}
	w.End = end
	return nil
}

// Validate checks that the dates are well formed and in order.
func (w *Week) Validate() error {
	if parseDate(w.Start).IsZero() {
		return fmt.Errorf("invalid week start date %q", w.Start)
	}
	if !w.Active() {
		if parseDate(w.End).IsZero() {
			return fmt.Errorf("invalid week end date %q", w.End)
		}
		if w.End < w.Start {
			return ErrWeekEndsEarly
		}
	}
	return nil
}

func parseDate(s string) time.Time {
	t, err := time.ParseInLocation(DateLayout, s, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// WeekReport is a week together with the days worked during it.
type WeekReport struct {
	Week   *Week
	Days   []*Day
	Worked time.Duration
	Breaks time.Duration
}

// NewWeekReport totals up the given days, which are expected to be the started
// days that fall inside the week, in chronological order.
func NewWeekReport(week *Week, days []*Day, now time.Time) WeekReport {
	r := WeekReport{Week: week, Days: days}
	for _, d := range days {
		r.Worked += d.Worked(now)
		r.Breaks += d.BreakTime(now)
	}
	return r
}

// WorkDays is the number of distinct days worked during the week.
func (r WeekReport) WorkDays() int { return len(r.Days) }

// DaysLeft is how many more working days the week has room for.
func (r WeekReport) DaysLeft() int {
	if left := MaxWorkDays - r.WorkDays(); left > 0 {
		return left
	}
	return 0
}

// Full reports whether the week has reached MaxWorkDays.
func (r WeekReport) Full() bool { return r.WorkDays() >= MaxWorkDays }

// Average is the mean worked time per working day.
func (r WeekReport) Average() time.Duration {
	if len(r.Days) == 0 {
		return 0
	}
	return r.Worked / time.Duration(len(r.Days))
}

// HasDay reports whether the calendar date of t is already a working day of
// this week, which is what makes a second session on the same day exempt from
// the MaxWorkDays limit.
func (r WeekReport) HasDay(t time.Time) bool {
	date := t.Format(DateLayout)
	for _, d := range r.Days {
		if d.Date == date {
			return true
		}
	}
	return false
}
