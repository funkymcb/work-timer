// Package worklog models a working day as a set of work sessions with breaks
// and provides the state transitions behind the start/pause/resume/stop
// commands.
package worklog

import (
	"errors"
	"time"
)

// DateLayout is the layout used for the date key of a day (and its file name).
const DateLayout = "2006-01-02"

var (
	ErrAlreadyStarted = errors.New("work has already been started")
	ErrNotStarted     = errors.New("work has not been started yet")
	ErrAlreadyPaused  = errors.New("you are already on a break")
	ErrNotPaused      = errors.New("you are not on a break")
	ErrOutOfOrder     = errors.New("entries have to be recorded in order")
)

// Interval is a span of time. An interval without an end is still running.
type Interval struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end,omitempty"`
}

// Open reports whether the interval is still running.
func (i Interval) Open() bool { return i.End == nil }

// Duration returns the length of the interval, using now as the end for open
// intervals. Negative durations (clock jumps) are clamped to zero.
func (i Interval) Duration(now time.Time) time.Duration {
	end := now
	if i.End != nil {
		end = *i.End
	}
	if d := end.Sub(i.Start); d > 0 {
		return d
	}
	return 0
}

// Session is one continuous stretch of work, possibly interrupted by breaks.
type Session struct {
	Start  time.Time  `json:"start"`
	End    *time.Time `json:"end,omitempty"`
	Breaks []Interval `json:"breaks,omitempty"`
}

// Open reports whether the session is still running.
func (s Session) Open() bool { return s.End == nil }

// Elapsed is the wall clock time covered by the session, breaks included.
func (s Session) Elapsed(now time.Time) time.Duration {
	return Interval{Start: s.Start, End: s.End}.Duration(now)
}

// BreakTime is the total time spent on breaks during the session.
func (s Session) BreakTime(now time.Time) time.Duration {
	var total time.Duration
	for _, b := range s.Breaks {
		total += b.Duration(now)
	}
	return total
}

// Worked is the elapsed time of the session minus its breaks.
func (s Session) Worked(now time.Time) time.Duration {
	if d := s.Elapsed(now) - s.BreakTime(now); d > 0 {
		return d
	}
	return 0
}

// State describes what the timer is currently doing.
type State int

const (
	// Idle means no session is running: either nothing was started today or
	// the day has been stopped.
	Idle State = iota
	// Working means a session is running and no break is active.
	Working
	// OnBreak means a session is running and a break is active.
	OnBreak
)

func (s State) String() string {
	switch s {
	case Working:
		return "working"
	case OnBreak:
		return "on break"
	default:
		return "idle"
	}
}

// Day holds every session of a single calendar day.
type Day struct {
	Date     string    `json:"date"`
	Sessions []Session `json:"sessions,omitempty"`
}

// NewDay returns an empty day for the calendar date of t.
func NewDay(t time.Time) *Day {
	return &Day{Date: t.Format(DateLayout)}
}

// current returns the running session, or nil when the day is idle.
func (d *Day) current() *Session {
	if len(d.Sessions) == 0 {
		return nil
	}
	s := &d.Sessions[len(d.Sessions)-1]
	if !s.Open() {
		return nil
	}
	return s
}

// openBreak returns the running break of the running session, or nil.
func (d *Day) openBreak() *Interval {
	s := d.current()
	if s == nil || len(s.Breaks) == 0 {
		return nil
	}
	b := &s.Breaks[len(s.Breaks)-1]
	if !b.Open() {
		return nil
	}
	return b
}

// State reports what the day is currently doing.
func (d *Day) State() State {
	switch {
	case d.current() == nil:
		return Idle
	case d.openBreak() != nil:
		return OnBreak
	default:
		return Working
	}
}

// Started reports whether any work was recorded for the day.
func (d *Day) Started() bool { return len(d.Sessions) > 0 }

// CalendarDate is the day's date as a time. Days that came from the store
// always carry a valid date.
func (d *Day) CalendarDate() time.Time { return parseDate(d.Date) }

// FirstStart is the time work began on this day. Only valid if Started.
func (d *Day) FirstStart() time.Time { return d.Sessions[0].Start }

// LastActivity is the most recent recorded timestamp of the day, i.e. the end
// of the last session or, while running, the start of the current session or
// break. Only valid if Started.
func (d *Day) LastActivity() time.Time {
	s := d.Sessions[len(d.Sessions)-1]
	if s.End != nil {
		return *s.End
	}
	if len(s.Breaks) > 0 {
		b := s.Breaks[len(s.Breaks)-1]
		if b.End != nil {
			return *b.End
		}
		return b.Start
	}
	return s.Start
}

// Worked is the total net working time of the day.
func (d *Day) Worked(now time.Time) time.Duration {
	var total time.Duration
	for _, s := range d.Sessions {
		total += s.Worked(now)
	}
	return total
}

// BreakTime is the total break time of the day. Time between a stop and a
// later start does not count as a break.
func (d *Day) BreakTime(now time.Time) time.Duration {
	var total time.Duration
	for _, s := range d.Sessions {
		total += s.BreakTime(now)
	}
	return total
}

// Span is the wall clock time between the first start and the last activity.
func (d *Day) Span(now time.Time) time.Duration {
	if !d.Started() {
		return 0
	}
	end := now
	if d.State() == Idle {
		end = d.LastActivity()
	}
	if s := end.Sub(d.FirstStart()); s > 0 {
		return s
	}
	return 0
}

// CurrentBreak returns the running break and true while on a break.
func (d *Day) CurrentBreak() (Interval, bool) {
	if b := d.openBreak(); b != nil {
		return *b, true
	}
	return Interval{}, false
}

// LastBreak returns the most recent break of the running session and true if
// there is one.
func (d *Day) LastBreak() (Interval, bool) {
	s := d.current()
	if s == nil || len(s.Breaks) == 0 {
		return Interval{}, false
	}
	return s.Breaks[len(s.Breaks)-1], true
}

// checkOrder rejects a timestamp that would land out of order, which only a
// backdated entry can produce: it has to sit at or after the last thing
// recorded, and on a day without entries it has to fall on that day or later.
// Without this, a backwards timestamp would silently create a negative
// interval, which the duration math clamps away.
func (d *Day) checkOrder(at time.Time) error {
	if d.Started() {
		if at.Before(d.LastActivity()) {
			return ErrOutOfOrder
		}
		return nil
	}
	if at.Format(DateLayout) < d.Date {
		return ErrOutOfOrder
	}
	return nil
}

// Start begins a new session at the given time. It fails if a session is
// already running or if the time lies before the last entry of the day.
func (d *Day) Start(at time.Time) error {
	if d.State() != Idle {
		return ErrAlreadyStarted
	}
	if err := d.checkOrder(at); err != nil {
		return err
	}
	d.Sessions = append(d.Sessions, Session{Start: at})
	return nil
}

// Pause begins a break at the given time. It fails unless work is currently
// running, and rejects a time before the last entry of the day.
func (d *Day) Pause(at time.Time) error {
	switch d.State() {
	case Idle:
		return ErrNotStarted
	case OnBreak:
		return ErrAlreadyPaused
	}
	if err := d.checkOrder(at); err != nil {
		return err
	}
	s := d.current()
	s.Breaks = append(s.Breaks, Interval{Start: at})
	return nil
}

// Resume ends the running break at the given time. It fails unless a break is
// active, and rejects a time before that break started.
func (d *Day) Resume(at time.Time) error {
	b := d.openBreak()
	if b == nil {
		if d.State() == Idle {
			return ErrNotStarted
		}
		return ErrNotPaused
	}
	if err := d.checkOrder(at); err != nil {
		return err
	}
	b.End = &at
	return nil
}

// Stop ends the running session at the given time, closing an active break
// first. It rejects a time before the last entry of the day.
func (d *Day) Stop(at time.Time) error {
	s := d.current()
	if s == nil {
		return ErrNotStarted
	}
	if err := d.checkOrder(at); err != nil {
		return err
	}
	if b := d.openBreak(); b != nil {
		b.End = &at
	}
	s.End = &at
	return nil
}
