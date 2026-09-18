// Command work is a small CLI to track working hours: start the day, take
// breaks, stop, and see how much you have worked so far.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/funkymcb/work-timer/internal/worklog"
)

const usage = `work - track your working hours

Usage:
  work start        begin the working day
  work pause        begin a break
  work resume       end the break and continue working
  work stop         end the working day
  work status       show what the timer is doing right now

  work week         show the days and hours of this calendar week

  work history      list every record, down to single sessions and breaks
  work where        print the path of the work log directory

Flags:
  work start|pause|resume|stop [--at HH:MM]   record the entry at that time
                                              instead of now (default now)
  work status [--format text|tmux]            output format (default text)
  work history [--out text|json|yaml]         output format (default text)
  work history [--since DATE] [--until DATE]  only the days in that range,
                                              both dates included

DATE is written 2024-10-01, 01.10.2024 (day first) or 10/01/2024 (month
first) - the separator says which part comes first.

--at only moves an entry backwards, to the most recent time of day that has
already passed, and never before the previous entry.

Weeks run Monday to Sunday and are worked out from the days on record, so
there is no week to open or close. Starting a day points out the weekdays
since your last record that have nothing logged on them.

Data is stored as one JSON file per day in $WORK_TIMER_DIR,
$XDG_DATA_HOME/work-timer or the default data directory.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "work:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, usage)
		return nil
	}

	dir, err := worklog.DefaultDir()
	if err != nil {
		return err
	}
	store := worklog.NewStore(dir)
	now := time.Now()

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "start", "pause", "resume", "stop":
		return timerCommand(cmd, store, now, out, rest)
	case "week":
		return week(store, now, out, rest)
	case "status":
		return status(store, now, out, rest)
	case "history":
		return history(store, now, out, rest)
	case "where":
		fmt.Fprintln(out, store.Dir())
		return nil
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q (try `work help`)", cmd)
	}
}

// timerCommand runs one of the day commands, all of which record an entry and
// therefore share the --at flag.
func timerCommand(cmd string, store *worklog.Store, now time.Time, out io.Writer, args []string) error {
	at, err := parseAtFlag(cmd, args, now, out)
	if err != nil {
		return err
	}
	switch cmd {
	case "start":
		return start(store, at, now, out)
	case "pause":
		_, err := mutate(store, at, now, out, (*worklog.Day).Pause, reportPause)
		return err
	case "resume":
		_, err := mutate(store, at, now, out, (*worklog.Day).Resume, reportResume)
		return err
	default: // stop, the only other command routed here
		_, err := mutate(store, at, now, out, (*worklog.Day).Stop, reportStop)
		return err
	}
}

// parseAtFlag reads the --at flag shared by the day commands and returns the
// time the entry should be recorded at, which is now when the flag is absent.
func parseAtFlag(cmd string, args []string, now time.Time, out io.Writer) (time.Time, error) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(out)
	value := fs.String("at", "", "time of day to record the entry at, as HH:MM (default now)")
	if err := fs.Parse(args); err != nil {
		return time.Time{}, err
	}
	if fs.NArg() > 0 {
		return time.Time{}, fmt.Errorf("unexpected argument %q for `work %s`", fs.Arg(0), cmd)
	}
	if *value == "" {
		return now, nil
	}
	return parseAt(*value, now)
}

// parseAt resolves a clock time like "13:07" to the most recent moment that
// had it, so it is today when it has already passed and yesterday when it has
// not. Entries can only be moved into the past: the time it is typed at is the
// latest one it can mean, and rolling over midnight keeps --at usable for a
// shift that ran into the next day.
func parseAt(value string, now time.Time) (time.Time, error) {
	var (
		t   time.Time
		err error
	)
	for _, layout := range []string{"15:04", "15:04:05"} {
		if t, err = time.Parse(layout, value); err == nil {
			break
		}
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --at time %q - use HH:MM, e.g. --at 13:07", value)
	}

	at := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
	if at.After(now) {
		at = at.AddDate(0, 0, -1)
	}
	return at, nil
}

// mutate loads the current day, applies a state transition at the given time,
// saves the result, prints a report of what happened and returns the day it
// changed. Totals in that report are computed at now, so a backdated entry
// still shows where the day stands.
func mutate(
	store *worklog.Store,
	at, now time.Time,
	out io.Writer,
	apply func(*worklog.Day, time.Time) error,
	report func(io.Writer, *worklog.Day, time.Time, time.Time),
) (*worklog.Day, error) {
	day, err := store.Current(now)
	if err != nil {
		return nil, err
	}
	if err := apply(day, at); err != nil {
		return nil, hint(err, day, at, now)
	}
	if err := store.Save(day); err != nil {
		return nil, err
	}
	report(out, day, at, now)
	return day, nil
}

// start begins the working day and, for the first session of a day, points out
// the weekdays since the last record that have nothing logged on them.
func start(store *worklog.Store, at, now time.Time, out io.Writer) error {
	// Read the gap before the new session lands, or today would close it.
	gap, err := store.GapBefore(at)
	if err != nil {
		return err
	}
	day, err := mutate(store, at, now, out, (*worklog.Day).Start, reportStart)
	if err != nil {
		return err
	}
	if len(day.Sessions) == 1 {
		printGap(out, gap)
	}
	return nil
}

// printGap names the weekdays that were skipped, listing them while there are
// few enough to read and, past that, counting them off the last day on record.
func printGap(out io.Writer, gap worklog.Gap) {
	switch n := len(gap.Weekdays); {
	case gap.Empty():
		return
	case n <= 3:
		labels := make([]string, 0, n)
		for _, day := range gap.Weekdays {
			labels = append(labels, dateLabel(day))
		}
		fmt.Fprintf(out, "  Nothing logged on %s.\n", join(labels))
	default:
		fmt.Fprintf(out, "  Nothing logged on the %d weekdays since %s.\n", n, dateLabel(gap.Since))
	}
}

// join lists names the way a sentence would, e.g. "Mon 14, Tue 15 and Wed 16".
func join(labels []string) string {
	switch len(labels) {
	case 1:
		return labels[0]
	default:
		return strings.Join(labels[:len(labels)-1], ", ") + " and " + labels[len(labels)-1]
	}
}

// week shows the calendar week that is running. It still answers the `start`
// and `end` subcommands of the days when weeks were opened by hand, because
// they are what the fingers remember.
func week(store *worklog.Store, now time.Time, out io.Writer, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "start", "end":
			return fmt.Errorf("`work week %s` is gone - weeks now run Monday to Sunday and are worked out from your day records, so just run `work start`", args[0])
		case "status":
		default:
			return fmt.Errorf("unknown command `work week %s` (try `work week`)", args[0])
		}
	}

	report, err := store.CurrentWeek(now)
	if err != nil {
		return err
	}
	printWeek(out, report, now, weekLayout{})
	return nil
}

// weekLayout says how much of a week to print. Detail lists every session and
// break underneath its day, and from and until carry the range a history was
// limited to, so a week reaching past either end can say that it is only
// partly shown.
type weekLayout struct {
	detail bool
	from   time.Time
	until  time.Time
}

// clip names the ends the history range cuts a week off at, for the week
// header, and is empty for a week that is shown whole. Its days and totals
// cover only what is listed, so a week that loses days has to say so.
func (l weekLayout) clip(week worklog.Week, now time.Time) string {
	// The week that is running reaches no further than today, so a cutoff
	// after today leaves nothing of it out.
	end := week.End
	if week.Current(now) && isoDate(now) < end {
		end = isoDate(now)
	}
	head := !l.from.IsZero() && week.Start < isoDate(l.from)
	tail := !l.until.IsZero() && isoDate(l.until) < end

	switch {
	case head && tail:
		return fmt.Sprintf(" · from %s to %s", dateLabel(l.from), dateLabel(l.until))
	case head:
		return fmt.Sprintf(" · from %s on", dateLabel(l.from))
	case tail:
		return fmt.Sprintf(" · up to %s", dateLabel(l.until))
	default:
		return ""
	}
}

// printWeek renders the day by day breakdown and the totals of a week.
func printWeek(out io.Writer, report worklog.WeekReport, now time.Time, layout weekLayout) {
	detail := layout.detail
	clip := layout.clip(report.Week, now)

	icon := iconStop
	if report.Week.Current(now) {
		icon = iconPlay
	}
	fmt.Fprintf(out, "%s Week %s · %s%s%s\n",
		icon, span(report), weekdayCount(report), weekendNote(report), clip)

	if report.WorkDays() == 0 {
		fmt.Fprintln(out, "  Nothing logged yet. Run `work start` to begin the day.")
		return
	}

	fmt.Fprintln(out)
	for _, d := range report.Days {
		printDay(out, d, now, detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %-10s %8s   breaks %6s\n", "Total", duration(report.Worked), duration(report.Breaks))
	fmt.Fprintf(out, "  %-10s %8s\n", "Average", duration(report.Average()))
}

// printDay renders one line per day and, with detail, the sessions of that day
// with their breaks nested underneath.
func printDay(out io.Writer, d *worklog.Day, now time.Time, detail bool) {
	fmt.Fprintf(out, "  %-10s %8s   breaks %6s%s\n",
		dateLabel(d.CalendarDate()), duration(d.Worked(now)), duration(d.BreakTime(now)), marker(d))
	if !detail {
		return
	}
	for _, s := range d.Sessions {
		fmt.Fprintf(out, "      %s %-15s %8s\n", iconPlay, timeRange(s.Start, s.End), duration(s.Worked(now)))
		for _, b := range s.Breaks {
			fmt.Fprintf(out, "        %s %-13s %8s\n", iconPause, timeRange(b.Start, b.End), duration(b.Duration(now)))
		}
	}
}

// timeRange renders the span of a session or break, e.g. "08:12 - 17:24". An
// entry that is still running reads as open, and one that ended on a later
// calendar day carries the day offset, so a shift over midnight cannot be
// mistaken for one that ran backwards.
func timeRange(start time.Time, end *time.Time) string {
	if end == nil {
		return clock(start) + " - open"
	}
	if days := dayGap(start, *end); days > 0 {
		return fmt.Sprintf("%s - %s+%dd", clock(start), clock(*end), days)
	}
	return clock(start) + " - " + clock(*end)
}

// dayGap is the number of calendar days between two times, counted on the
// local calendar rather than in multiples of 24 hours, so days that a daylight
// saving switch made shorter or longer still count as one.
func dayGap(a, b time.Time) int {
	midnight := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
	return int((midnight(b).Sub(midnight(a)) + 12*time.Hour) / (24 * time.Hour))
}

// history reports everything on record, in the requested output format and
// from the requested date on.
func history(store *worklog.Store, now time.Time, out io.Writer, args []string) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	fs.SetOutput(out)
	format := fs.String("out", "text", "output format: text, json or yaml")
	from := fs.String("since", "", "only report the days from this date on, e.g. 01.10.2024")
	to := fs.String("until", "", "only report the days up to this date, it included")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q for `work history`", fs.Arg(0))
	}

	since, err := optionalDate("since", *from)
	if err != nil {
		return err
	}
	until, err := optionalDate("until", *to)
	if err != nil {
		return err
	}
	if !since.IsZero() && !until.IsZero() && until.Before(since) {
		return fmt.Errorf("--until %s is before --since %s, which leaves no days to report",
			fullDateLabel(until), fullDateLabel(since))
	}

	weeks, err := store.History(now, isoDate(since), isoDate(until))
	if err != nil {
		return err
	}

	switch *format {
	case "text":
		printHistory(out, store.Dir(), weeks, since, until, now)
		return nil
	case "json":
		return writeJSON(out, newHistoryView(weeks, since, until, now))
	case "yaml":
		return writeYAML(out, newHistoryView(weeks, since, until, now))
	default:
		return fmt.Errorf("unknown output %q (use text, json or yaml)", *format)
	}
}

// dateLayouts are the date formats --since and --until take. Dots are read day
// first (01.10.2024), slashes month first (10/01/2024) and dashes year first
// (2024-10-01). Four digit year forms are tried before two digit ones, so a
// full year is never mistaken for a day or a month.
var dateLayouts = []string{
	"2006-01-02",
	"2006-1-2",
	"2006/01/02",
	"2006/1/2",
	"2.1.2006",
	"2.1.06",
	"1/2/2006",
	"1/2/06",
}

// optionalDate reads one end of the range a history is limited to, and returns
// the zero time for a flag that was not given.
func optionalDate(flag, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return parseDate(flag, value)
}

// parseDate reads a date off the command line. Only the calendar date matters,
// so it is taken in the local zone, where the day files are dated.
func parseDate(flag, value string) (time.Time, error) {
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid --%s date %q - use 2024-10-01, 01.10.2024 (day first) or 10/01/2024 (month first)", flag, value)
}

// isoDate renders a date the way the store keys days, and the zero time as the
// empty string that stands for no limit.
func isoDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(worklog.DateLayout)
}

// historyTotals sums up every week on record.
func historyTotals(weeks []worklog.WeekReport) (days int, worked, breaks time.Duration) {
	for _, report := range weeks {
		days += report.WorkDays()
		worked += report.Worked
		breaks += report.Breaks
	}
	return days, worked, breaks
}

// printHistory writes the history as text: each week broken down into days,
// sessions and breaks, and the totals over all of it. The range it was limited
// to is reported in the closing line, because the weeks it cuts through are
// then listed with fewer days than they really hold.
func printHistory(out io.Writer, dir string, weeks []worklog.WeekReport, since, until, now time.Time) {
	if len(weeks) == 0 {
		if !since.IsZero() || !until.IsZero() {
			fmt.Fprintf(out, "Nothing recorded %s.\n", rangePhrase(since, until))
			return
		}
		fmt.Fprintf(out, "Nothing recorded in %s yet. Run `work start` to begin the day.\n", dir)
		return
	}

	for i, report := range weeks {
		if i > 0 {
			fmt.Fprintln(out)
		}
		printWeek(out, report, now, weekLayout{detail: true, from: since, until: until})
	}

	days, worked, breaks := historyTotals(weeks)

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s · %s · %s · %s worked · %s on breaks\n",
		rangeLabel(since, until), plural(len(weeks), "week"), plural(days, "working day"),
		duration(worked), duration(breaks))
}

// rangeLabel opens the closing line with the stretch of time the totals behind
// it cover.
func rangeLabel(since, until time.Time) string {
	switch {
	case !since.IsZero() && !until.IsZero():
		return fullDateLabel(since) + " - " + fullDateLabel(until)
	case !since.IsZero():
		return "Since " + fullDateLabel(since)
	case !until.IsZero():
		return "Up to " + fullDateLabel(until)
	default:
		return "All time"
	}
}

// rangePhrase says the same thing mid sentence, for the message that nothing
// was recorded in that stretch.
func rangePhrase(since, until time.Time) string {
	switch {
	case !since.IsZero() && !until.IsZero():
		return fmt.Sprintf("between %s and %s", fullDateLabel(since), fullDateLabel(until))
	case !since.IsZero():
		return "since " + fullDateLabel(since)
	default:
		return "up to " + fullDateLabel(until)
	}
}

// marker flags the day that is currently being worked on.
func marker(d *worklog.Day) string {
	switch d.State() {
	case worklog.Working:
		return "   " + iconPlay + " now"
	case worklog.OnBreak:
		return "   " + iconPause + " on break"
	default:
		return ""
	}
}

// span renders the range a week covers, e.g. "Mon 14 - Sun 20 Sep". The month
// is only named once unless the week runs across one.
func span(report worklog.WeekReport) string {
	start, end := report.Week.StartDate(), report.Week.EndDate()
	if start.Month() == end.Month() {
		return start.Format("Mon 02") + " - " + dateLabel(end)
	}
	return dateLabel(start) + " - " + dateLabel(end)
}

// weekdayCount says how far through the five weekdays the week is.
func weekdayCount(report worklog.WeekReport) string {
	return fmt.Sprintf("%d of %d weekdays", report.Weekdays(), worklog.WeekdaysPerWeek)
}

// weekendNote is added only when there are weekend days to account for, which
// the count of weekdays would otherwise leave unmentioned.
func weekendNote(report worklog.WeekReport) string {
	if n := report.WeekendDays(); n > 0 {
		return " + " + plural(n, "weekend day")
	}
	return ""
}

func dateLabel(t time.Time) string { return t.Format("Mon 02 Jan") }

// fullDateLabel carries the year too, for dates that can lie years back.
func fullDateLabel(t time.Time) string { return t.Format("Mon 02 Jan 2006") }

// plural renders a count with its unit, e.g. "1 day" or "3 days".
func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// weekLine adds the running week total under the daily status.
func weekLine(store *worklog.Store, now time.Time, out io.Writer) error {
	report, err := store.CurrentWeek(now)
	if err != nil {
		return err
	}
	if report.WorkDays() == 0 {
		return nil
	}
	fmt.Fprintf(out, "  This week: %s over %s%s (%s)\n",
		duration(report.Worked), weekdayCount(report), weekendNote(report), span(report))
	return nil
}

// hint turns a state transition error into a message that says what the timer
// is actually doing, so the user knows what to run instead.
func hint(err error, day *worklog.Day, at, now time.Time) error {
	switch {
	case errors.Is(err, worklog.ErrOutOfOrder) && day.Started():
		return fmt.Errorf("%w - %s is before the last entry of %s at %s%s",
			err, stamp(at, now), dateLabel(day.CalendarDate()), clock(day.LastActivity()), rolledBack(at, now))
	case errors.Is(err, worklog.ErrOutOfOrder):
		return fmt.Errorf("%w - %s is before %s, the day being recorded%s",
			err, stamp(at, now), dateLabel(day.CalendarDate()), rolledBack(at, now))
	case errors.Is(err, worklog.ErrAlreadyStarted):
		return fmt.Errorf("%w (since %s, %s worked so far)",
			err, clock(day.FirstStart()), duration(day.Worked(now)))
	case errors.Is(err, worklog.ErrNotStarted) && day.Started():
		return fmt.Errorf("work was already stopped at %s (%s worked) - run `work start` to pick it back up",
			clock(day.LastActivity()), duration(day.Worked(now)))
	case errors.Is(err, worklog.ErrNotStarted):
		return fmt.Errorf("%w - run `work start`", err)
	case errors.Is(err, worklog.ErrAlreadyPaused):
		b, _ := day.CurrentBreak()
		return fmt.Errorf("%w (since %s) - run `work resume`", err, clock(b.Start))
	case errors.Is(err, worklog.ErrNotPaused):
		return fmt.Errorf("%w - run `work pause` to start one", err)
	default:
		return err
	}
}

// Media control symbols used by both output formats. All three default to text
// presentation, so they stay single width in a status bar.
const (
	iconPlay  = "▶"
	iconPause = "⏸"
	iconStop  = "⏹"
)

func reportStart(out io.Writer, day *worklog.Day, at, now time.Time) {
	if len(day.Sessions) > 1 {
		fmt.Fprintf(out, iconPlay+" Back at work at %s.\n", stamp(at, now))
		fmt.Fprintf(out, "  Worked today: %s\n", duration(day.Worked(now)))
		return
	}
	fmt.Fprintf(out, iconPlay+" Work started at %s. Have a good one.\n", stamp(at, now))
}

func reportPause(out io.Writer, day *worklog.Day, at, now time.Time) {
	fmt.Fprintf(out, iconPause+" Break started at %s.\n", stamp(at, now))
	fmt.Fprintf(out, "  Worked today: %s\n", duration(day.Worked(now)))
	if b := day.BreakTime(now); b > 0 {
		fmt.Fprintf(out, "  Breaks so far: %s\n", duration(b))
	}
}

func reportResume(out io.Writer, day *worklog.Day, at, now time.Time) {
	fmt.Fprintf(out, iconPlay+" Back to work at %s.\n", stamp(at, now))
	if b, ok := day.LastBreak(); ok {
		fmt.Fprintf(out, "  Break lasted: %s\n", duration(b.Duration(now)))
	}
	fmt.Fprintf(out, "  Worked today: %s\n", duration(day.Worked(now)))
}

func reportStop(out io.Writer, day *worklog.Day, at, now time.Time) {
	fmt.Fprintf(out, iconStop+" Work stopped at %s.\n", stamp(at, now))
	fmt.Fprintf(out, "  Worked today: %s\n", duration(day.Worked(now)))
	fmt.Fprintf(out, "  Breaks:       %s\n", duration(day.BreakTime(now)))
	fmt.Fprintf(out, "  From %s to %s (%s elapsed)\n",
		clock(day.FirstStart()), clock(at), duration(day.Span(now)))
}

func status(store *worklog.Store, now time.Time, out io.Writer, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(out)
	format := fs.String("format", "text", "output format: text or tmux")
	fs.StringVar(format, "f", "text", "shorthand for --format")
	if err := fs.Parse(args); err != nil {
		return err
	}

	day, err := store.Current(now)
	if err != nil {
		return err
	}

	switch *format {
	case "tmux":
		fmt.Fprintln(out, tmuxStatus(day, now))
		return nil
	case "text":
		fmt.Fprintln(out, textStatus(day, now))
		return weekLine(store, now, out)
	default:
		return fmt.Errorf("unknown format %q (use text or tmux)", *format)
	}
}

func textStatus(day *worklog.Day, now time.Time) string {
	if !day.Started() {
		return iconStop + " Not started today."
	}
	worked := duration(day.Worked(now))
	switch day.State() {
	case worklog.Working:
		return fmt.Sprintf(iconPlay+" Working since %s · %s worked · %s on breaks",
			clock(day.FirstStart()), worked, duration(day.BreakTime(now)))
	case worklog.OnBreak:
		b, _ := day.CurrentBreak()
		return fmt.Sprintf(iconPause+" On break since %s (%s) · %s worked today",
			clock(b.Start), duration(b.Duration(now)), worked)
	default:
		return fmt.Sprintf(iconStop+" Done at %s · %s worked · %s on breaks",
			clock(day.LastActivity()), worked, duration(day.BreakTime(now)))
	}
}

// tmuxStatus is a compact line for the tmux status bar.
func tmuxStatus(day *worklog.Day, now time.Time) string {
	if !day.Started() {
		return ""
	}
	switch day.State() {
	case worklog.Working:
		return iconPlay + " " + compact(day.Worked(now))
	case worklog.OnBreak:
		b, _ := day.CurrentBreak()
		return iconPause + " " + compact(b.Duration(now))
	default:
		return iconStop + " " + compact(day.Worked(now))
	}
}

func clock(t time.Time) string { return t.Format("15:04") }

// stamp renders a recorded time, adding the date when it is not today's, so an
// --at value that rolled back over midnight is never ambiguous.
func stamp(t, now time.Time) string {
	if sameDay(t, now) {
		return clock(t)
	}
	return dateLabel(t) + " " + clock(t)
}

// rolledBack explains an --at value that landed on yesterday because the clock
// time it names is still ahead today, which is otherwise a puzzling error.
func rolledBack(at, now time.Time) string {
	if sameDay(at, now) {
		return ""
	}
	return fmt.Sprintf(" (%s has not come round yet today, so it was read as yesterday)", clock(at))
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// duration renders a duration as "7h 32m", rounded down to whole minutes.
func duration(d time.Duration) string {
	h, m := hm(d)
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %02dm", h, m)
}

// compact renders a duration as "7h32m", without spaces, for the status bar.
func compact(d time.Duration) string {
	h, m := hm(d)
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}

func hm(d time.Duration) (hours, minutes int) {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Minute)
	return int(d / time.Hour), int(d % time.Hour / time.Minute)
}
