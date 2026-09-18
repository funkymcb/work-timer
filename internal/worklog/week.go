package worklog

import "time"

// WeekdaysPerWeek is how many weekdays, Monday through Friday, a week holds.
// It is what the day count of a week report is measured against.
const WeekdaysPerWeek = 5

// Week is a calendar week, Monday through Sunday. Weeks are not recorded
// anywhere: they are worked out from the dates of the day files, so there is
// no week to open, to close, or to forget.
type Week struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// WeekOf returns the calendar week the given time falls in.
func WeekOf(t time.Time) Week {
	monday := startOfWeek(t)
	return Week{
		Start: monday.Format(DateLayout),
		End:   monday.AddDate(0, 0, 6).Format(DateLayout),
	}
}

// startOfWeek is midnight on the Monday of the week t falls in. Go counts
// Sunday as weekday zero, so Sunday has to be pushed back into the week that
// began six days earlier rather than starting a new one.
func startOfWeek(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7
	year, month, day := t.Date()
	return time.Date(year, month, day-offset, 0, 0, 0, 0, t.Location())
}

// StartDate is the Monday the week begins on.
func (w Week) StartDate() time.Time { return parseDate(w.Start) }

// EndDate is the Sunday the week ends on.
func (w Week) EndDate() time.Time { return parseDate(w.End) }

// Contains reports whether an ISO date falls in the week.
func (w Week) Contains(date string) bool { return date >= w.Start && date <= w.End }

// Current reports whether this is the week t falls in.
func (w Week) Current(t time.Time) bool { return w.Contains(t.Format(DateLayout)) }

// isWeekend reports whether a date falls on a Saturday or a Sunday, which are
// counted apart from the five weekdays.
func isWeekend(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

func parseDate(s string) time.Time {
	t, err := time.ParseInLocation(DateLayout, s, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// WeekReport is a calendar week together with the days worked during it.
type WeekReport struct {
	Week   Week
	Days   []*Day
	Worked time.Duration
	Breaks time.Duration
}

// NewWeekReport totals up the given days, which are expected to be the started
// days that fall inside the week, in chronological order.
func NewWeekReport(week Week, days []*Day, now time.Time) WeekReport {
	r := WeekReport{Week: week, Days: days}
	for _, d := range days {
		r.Worked += d.Worked(now)
		r.Breaks += d.BreakTime(now)
	}
	return r
}

// WorkDays is the number of days worked during the week, weekend days counted.
func (r WeekReport) WorkDays() int { return len(r.Days) }

// Weekdays is how many of the five weekdays were worked.
func (r WeekReport) Weekdays() int {
	n := 0
	for _, d := range r.Days {
		if !isWeekend(d.CalendarDate()) {
			n++
		}
	}
	return n
}

// WeekendDays is how many Saturdays and Sundays were worked, which is normally
// none and is worth pointing out when it is not.
func (r WeekReport) WeekendDays() int { return r.WorkDays() - r.Weekdays() }

// Average is the mean worked time per day worked.
func (r WeekReport) Average() time.Duration {
	if len(r.Days) == 0 {
		return 0
	}
	return r.Worked / time.Duration(len(r.Days))
}
