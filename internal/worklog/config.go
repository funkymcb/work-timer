package worklog

import (
	"math"
	"time"
)

// DefaultWeeklyHours is the contracted week assumed when nothing was
// configured: the full time week most contracts are written for.
const DefaultWeeklyHours = 40

// Config holds what cannot be worked out from the day records: the week the
// contract is written for, where the overtime balance was last set back to
// zero, and the overtime that has been taken out of it since. It is stored as
// one JSON file next to the days.
type Config struct {
	// WeeklyHours is the contracted week in hours, e.g. 40 or 37.5.
	WeeklyHours float64 `json:"weekly_hours"`
	// OvertimeSince is the ISO date the balance is counted from, always a
	// Monday, and empty while it covers everything on record.
	OvertimeSince string `json:"overtime_since,omitempty"`
	// OvertimeUsed are the withdrawals from the balance, in the order they
	// were made. A reset clears them, because it zeroes what they were
	// taken out of.
	OvertimeUsed []UsedUp `json:"overtime_used,omitempty"`
}

// UsedUp is overtime taken out of the balance rather than worked off, a day
// off paid for out of the hours that were built up.
type UsedUp struct {
	// Date is the ISO date the withdrawal was recorded on.
	Date string `json:"date"`
	// Hours is what was taken off, and puts hours back when negative, which
	// is how a withdrawal made by mistake is undone.
	Hours float64 `json:"hours"`
}

// DefaultConfig is what a work log that was never configured runs on.
func DefaultConfig() Config { return Config{WeeklyHours: DefaultWeeklyHours} }

// WeeklyTarget is the contracted week as a duration. A file that was hand
// edited down to zero or below falls back on the default rather than making
// every week infinite overtime.
func (c Config) WeeklyTarget() time.Duration {
	if c.WeeklyHours <= 0 {
		return hoursDuration(DefaultWeeklyHours)
	}
	return hoursDuration(c.WeeklyHours)
}

// Used is the total taken out of the balance so far.
func (c Config) Used() time.Duration {
	var total time.Duration
	for _, used := range c.OvertimeUsed {
		total += hoursDuration(used.Hours)
	}
	return total
}

// UseUp records a withdrawal from the balance on the given day.
func (c *Config) UseUp(hours float64, on time.Time) {
	c.OvertimeUsed = append(c.OvertimeUsed, UsedUp{Date: on.Format(DateLayout), Hours: hours})
}

// ResetOvertime sets the balance back to zero: the count starts again at the
// Monday of the week t falls in, and the withdrawals from the old balance go
// with it.
func (c *Config) ResetOvertime(t time.Time) Week {
	week := WeekOf(t)
	c.OvertimeSince = week.Start
	c.OvertimeUsed = nil
	return week
}

// hoursDuration turns a number of hours into a duration, rounded to whole
// minutes, which is the granularity everything here is reported in.
func hoursDuration(hours float64) time.Duration {
	return time.Duration(math.Round(hours*60)) * time.Minute
}

// OvertimeReport is the balance between the hours worked and the hours owed,
// added up over the weeks it covers, less what has been taken out of it.
type OvertimeReport struct {
	// Since is the ISO date the count starts at, empty for everything on
	// record.
	Since string
	// Weeks is how many weeks went into the balance. The week that is
	// running is not among them, nor are weeks with nothing logged at all.
	Weeks int
	// First and Last are the ends of the stretch counted, both zero when no
	// week was.
	First, Last Week
	Worked      time.Duration
	Target      time.Duration
	// Used is the overtime taken out of the balance rather than worked off.
	Used time.Duration
}

// Balance is the overtime: positive when more was worked than owed, negative
// when less.
func (r OvertimeReport) Balance() time.Duration { return r.Worked - r.Target - r.Used }

// Empty reports whether there is nothing to show: no finished week counted and
// nothing taken out of the balance either.
func (r OvertimeReport) Empty() bool { return r.Weeks == 0 && r.Used == 0 }

// SinceDate is the date the count starts at, and the zero time when it covers
// everything on record.
func (r OvertimeReport) SinceDate() time.Time { return parseDate(r.Since) }
