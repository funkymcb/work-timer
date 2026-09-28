// Command work is a small CLI to track working hours: start the day, take
// breaks, stop, and see how much you have worked so far.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

  work hours        show the contracted hours per week
  work hours 37.5   set the contracted hours per week (default 40)
  work overtime     show the overtime balance and what it is made of

  work history      list every record, down to single sessions and breaks
  work where        print the path of the work log directory

Flags:
  work start|pause|resume|stop [--at HH:MM]   record the entry at that time
                                              instead of now (default now)
  work status [--format text|tmux]            output format (default text)
  work overtime [--reset]                     set the balance back to zero
  work overtime [--use-up HOURS]              take hours out of the balance,
                                              e.g. --use-up 8 for a day off
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

Overtime is the hours worked against a contracted week for every finished
week that has anything on record. The week that is running is left out until
it is over; work status reports it on its own line. Weeks with nothing
logged at all - holidays, sick leave, the time before you started - are left
out for good. --reset counts again from the Monday of this week, dropping the
withdrawals along with the balance they came out of.

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
	case "hours":
		return contract(store, out, rest)
	case "overtime":
		return overtime(store, now, out, rest)
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
	fmt.Fprintln(out, tableRow(detail, "Day", "Entry", "Worked", "Breaks"))
	fmt.Fprintln(out, tableRule(detail))
	for _, d := range report.Days {
		printDay(out, d, now, detail)
	}
	fmt.Fprintln(out, tableRule(detail))
	fmt.Fprintln(out, tableRow(detail, "Total", "", duration(report.Worked), duration(report.Breaks)))
	fmt.Fprintln(out, tableRow(detail, "Average", "", duration(report.Average()), ""))
}

// Column widths of the day table. The entry column carries an icon and a time
// range, e.g. "▶ 08:12 - 17:24+1d", and is only shown in the detailed view.
const (
	colDay    = 12
	colEntry  = 18
	colWorked = 8
	colBreaks = 7
)

// tableRow lays out one line of the day table. Cells that do not apply to a
// row are passed empty and left blank, so the entries of a day sit under the
// day they belong to without restating its figures. Separators are kept on
// those blank cells, which holds the shape of the table down the entry rows,
// and only the padding past the last one is dropped, so no line carries
// trailing blanks.
func tableRow(detail bool, day, entry, worked, breaks string) string {
	cells := []string{padRight(day, colDay)}
	if detail {
		cells = append(cells, padRight(entry, colEntry))
	}
	cells = append(cells, padLeft(worked, colWorked), padLeft(breaks, colBreaks))
	return strings.TrimRight("  "+strings.Join(cells, " │ "), " ")
}

// tableRule is the horizontal rule that separates the header and the totals
// from the days between them.
func tableRule(detail bool) string {
	cells := []string{strings.Repeat("─", colDay)}
	if detail {
		cells = append(cells, strings.Repeat("─", colEntry))
	}
	cells = append(cells, strings.Repeat("─", colWorked), strings.Repeat("─", colBreaks))
	return "  " + strings.Join(cells, "─┼─")
}

// padRight and padLeft pad a cell to its column width. They count runes rather
// than bytes, because the icons of an entry cell are multi byte but single
// width, and fmt would pad them by byte and pull the column out of line.
func padRight(s string, width int) string { return s + strings.Repeat(" ", gap(s, width)) }

func padLeft(s string, width int) string { return strings.Repeat(" ", gap(s, width)) + s }

func gap(s string, width int) int {
	if n := width - utf8.RuneCountInString(s); n > 0 {
		return n
	}
	return 0
}

// printDay renders the totals of one day and, with detail, the entries behind
// them. The first entry shares the row of its day, so a day of one session
// states its times and its total on a single line rather than repeating the
// same figures underneath themselves.
func printDay(out io.Writer, d *worklog.Day, now time.Time, detail bool) {
	label := dateLabel(d.CalendarDate())
	worked, breaks := duration(d.Worked(now)), duration(d.BreakTime(now))

	entries := dayEntries(d)
	if !detail || len(entries) == 0 {
		fmt.Fprintln(out, tableRow(detail, label, "", worked, breaks)+marker(d))
		return
	}

	fmt.Fprintln(out, tableRow(detail, label, entries[0], worked, breaks)+marker(d))
	for _, entry := range entries[1:] {
		fmt.Fprintln(out, tableRow(detail, "", entry, "", ""))
	}
}

// dayEntries flattens the sessions of a day, and the breaks nested in them,
// into the lines of the entry column, in the order they were recorded. Only
// the spans are listed: their durations are what the day totals add up, and
// stating both next to each other is what made the listing hard to read.
func dayEntries(d *worklog.Day) []string {
	var entries []string
	for _, s := range d.Sessions {
		entries = append(entries, iconPlay+" "+timeRange(s.Start, s.End))
		for _, b := range s.Breaks {
			entries = append(entries, iconPause+" "+timeRange(b.Start, b.End))
		}
	}
	return entries
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

// weekLine adds the running week, measured against the contracted hours, and
// the overtime balance under the daily status. Either line is left out when it
// has nothing to say: a week that has not been worked yet, or a balance with
// no finished week and no withdrawal behind it. The week that is running is on
// the first line and not in the balance, so neither line counts it twice.
func weekLine(store *worklog.Store, config worklog.Config, now time.Time, out io.Writer) error {
	week, err := store.CurrentWeek(now)
	if err != nil {
		return err
	}
	if week.WorkDays() > 0 {
		fmt.Fprintf(out, "  This week: %s / %s over %s%s (%s)\n",
			duration(week.Worked), target(config.WeeklyTarget()),
			weekdayCount(week), weekendNote(week), span(week))
	}

	report, err := store.Overtime(config, now)
	if err != nil {
		return err
	}
	if report.Empty() {
		return nil
	}
	fmt.Fprintf(out, "  Overtime:  %s over %s%s\n",
		balance(report.Balance()), plural(report.Weeks, "week"), countedSince(report))
	return nil
}

// countedSince names the reset the balance is counted from, and is empty while
// it still covers everything on record.
func countedSince(report worklog.OvertimeReport) string {
	if report.Since == "" {
		return ""
	}
	return " (since " + dateLabel(report.SinceDate()) + ")"
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
	case errors.Is(err, worklog.ErrAlreadyStarted) && day.State() == worklog.OnBreak:
		b, _ := day.CurrentBreak()
		return fmt.Errorf("%w (since %s, %s worked so far) and you are on a break since %s - run `work resume`",
			err, clock(day.FirstStart()), duration(day.Worked(now)), clock(b.Start))
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
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q for `work status`", fs.Arg(0))
	}

	config, err := store.Config()
	if err != nil {
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
		return weekLine(store, config, now, out)
	default:
		return fmt.Errorf("unknown format %q (use text or tmux)", *format)
	}
}

// overtime reports the balance, and first changes it when asked to: --reset
// sets it back to zero, --use-up takes hours out of it.
func overtime(store *worklog.Store, now time.Time, out io.Writer, args []string) error {
	fs := flag.NewFlagSet("overtime", flag.ContinueOnError)
	fs.SetOutput(out)
	reset := fs.Bool("reset", false, "set the balance back to zero and count again from this week")
	use := fs.String("use-up", "", "take these hours out of the balance, e.g. 8 for a day off")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q for `work overtime`", fs.Arg(0))
	}
	if *reset && *use != "" {
		return fmt.Errorf("--reset and --use-up cannot be given together - a reset already zeroes the balance")
	}

	config, err := store.Config()
	if err != nil {
		return err
	}
	switch {
	case *reset:
		if config, err = resetOvertime(store, config, now, out); err != nil {
			return err
		}
	case *use != "":
		if config, err = useUpOvertime(store, config, *use, now, out); err != nil {
			return err
		}
	}

	report, err := store.Overtime(config, now)
	if err != nil {
		return err
	}
	printOvertime(out, report, config)
	return nil
}

// resetOvertime starts the balance over from the Monday of the week that is
// running, and returns the settings it saved. The cut is made on a week
// boundary because whole weeks are what the balance is built out of.
func resetOvertime(store *worklog.Store, config worklog.Config, now time.Time, out io.Writer) (worklog.Config, error) {
	week := config.ResetOvertime(now)
	if err := store.SaveConfig(config); err != nil {
		return config, err
	}
	fmt.Fprintf(out, "Overtime reset. Counting again from %s.\n", dateLabel(week.StartDate()))
	return config, nil
}

// useUpOvertime takes hours out of the balance, which is what a day off paid
// for out of built up overtime does to it.
func useUpOvertime(store *worklog.Store, config worklog.Config, value string, now time.Time, out io.Writer) (worklog.Config, error) {
	used, err := parseUsedHours(value)
	if err != nil {
		return config, err
	}
	config.UseUp(used, now)
	if err := store.SaveConfig(config); err != nil {
		return config, err
	}

	if used < 0 {
		fmt.Fprintf(out, "Put %s back into the balance.\n", duration(hours(-used)))
	} else {
		fmt.Fprintf(out, "Used up %s of overtime.\n", duration(hours(used)))
	}
	return config, nil
}

// printOvertime writes the balance and the figures behind it.
func printOvertime(out io.Writer, report worklog.OvertimeReport, config worklog.Config) {
	fmt.Fprintf(out, "Overtime: %s %s\n", balance(report.Balance()), countedWeeks(report))

	if report.Weeks > 0 {
		fmt.Fprintln(out, overtimeRow("Worked", duration(report.Worked), ""))
		fmt.Fprintln(out, overtimeRow("Contracted", duration(report.Target),
			"at "+target(config.WeeklyTarget())+" per week"))
	}
	switch {
	case report.Used > 0:
		fmt.Fprintln(out, overtimeRow("Used up", duration(report.Used), "taken out of the balance"))
	case report.Used < 0:
		fmt.Fprintln(out, overtimeRow("Put back", duration(-report.Used), "added to the balance"))
	}

	if report.Weeks == 0 {
		fmt.Fprintln(out, "  Nothing counts until the week that is running is over.")
		return
	}
	fmt.Fprintln(out, "  The week that is running does not count yet.")
}

// Column widths of the overtime figures, which are laid out as a label and a
// right aligned duration so the totals line up under each other.
const (
	colLabel  = 11
	colFigure = 9
)

func overtimeRow(label, figure, note string) string {
	row := "  " + padRight(label+":", colLabel) + padLeft(figure, colFigure)
	if note != "" {
		row += "   " + note
	}
	return row
}

// countedWeeks says how much the balance was built out of: the stretch of
// weeks it covers, or that there is none of it yet, in which case the reset it
// is counting from is worth naming because that is what emptied it.
func countedWeeks(report worklog.OvertimeReport) string {
	if report.Weeks == 0 {
		return "over no finished week" + countedSince(report)
	}
	return fmt.Sprintf("over %s (%s - %s)", plural(report.Weeks, "week"),
		dateLabel(report.First.StartDate()), dateLabel(report.Last.EndDate()))
}

// contract reports the contracted week the overtime balance is measured
// against, and sets it when given a figure.
func contract(store *worklog.Store, out io.Writer, args []string) error {
	config, err := store.Config()
	if err != nil {
		return err
	}
	if len(args) > 1 {
		return fmt.Errorf("unexpected argument %q for `work hours`", args[1])
	}
	if len(args) == 0 {
		fmt.Fprintf(out, "Contracted hours: %s per week\n", target(config.WeeklyTarget()))
		return nil
	}

	weekly, err := parseWeeklyHours(args[0])
	if err != nil {
		return err
	}
	config.WeeklyHours = weekly
	if err := store.SaveConfig(config); err != nil {
		return err
	}
	fmt.Fprintf(out, "Contracted hours set to %s per week.\n", target(config.WeeklyTarget()))
	return nil
}

// parseWeeklyHours reads a contracted week off the command line, as a number
// of hours with an optional h, e.g. 40, 37.5 or 35h.
func parseWeeklyHours(value string) (float64, error) {
	weekly, err := parseFloatHours(value)
	if err != nil || weekly <= 0 || weekly > 7*24 {
		return 0, fmt.Errorf("invalid weekly hours %q - use a number of hours, e.g. 40 or 37.5", value)
	}
	return weekly, nil
}

// parseUsedHours reads the hours to take out of the balance. A negative figure
// puts hours back, which is how a withdrawal typed wrong is undone.
func parseUsedHours(value string) (float64, error) {
	used, err := parseFloatHours(value)
	if err != nil || used == 0 || used < -weekHours || used > weekHours {
		return 0, fmt.Errorf("invalid --use-up hours %q - use a number of hours, e.g. 8 or 7.5, or a negative one to put them back", value)
	}
	return used, nil
}

// weekHours bounds a single withdrawal at the hours in a week, which is far
// more than a day off and still catches a figure typed in minutes.
const weekHours = 7 * 24

func parseFloatHours(value string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSuffix(value, "h"), 64)
}

// hours turns a number of hours into a duration, for rendering a figure that
// came off the command line.
func hours(h float64) time.Duration {
	return time.Duration(math.Round(h*60)) * time.Minute
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
		return iconStop + " 󰒲"
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

// target renders contracted hours, dropping the minutes when a week divides
// into whole hours, so the common 40 hour week reads as "40h".
func target(d time.Duration) string {
	if h, m := hm(d); m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return duration(d)
}

// balance renders an overtime figure with its sign, which is the whole point
// of the line it sits on: hours owed read as a shortfall, not as time worked.
func balance(d time.Duration) string {
	switch d = d.Truncate(time.Minute); {
	case d < 0:
		return "-" + duration(-d)
	case d == 0:
		return "0m"
	default:
		return "+" + duration(d)
	}
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
