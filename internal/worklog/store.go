package worklog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store persists one JSON file per calendar day in a data directory. Weeks are
// not stored: they are the calendar weeks the recorded days fall in.
type Store struct {
	dir string
}

// DefaultDir returns the directory the work log is stored in. It honours
// WORK_TIMER_DIR and XDG_DATA_HOME, defaulting to ~/.local/share/work-timer.
func DefaultDir() (string, error) {
	if dir := os.Getenv("WORK_TIMER_DIR"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "work-timer"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, "kycnow", "Documents", "Workhours"), nil
}

// NewStore returns a store rooted at dir.
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Dir returns the directory the store writes to.
func (s *Store) Dir() string { return s.dir }

func (s *Store) path(date string) string {
	return filepath.Join(s.dir, date+".json")
}

// Load reads the day for the calendar date of t. A day that was never written
// is returned empty, not as an error.
func (s *Store) Load(t time.Time) (*Day, error) {
	date := t.Format(DateLayout)
	data, err := os.ReadFile(s.path(date))
	if errors.Is(err, fs.ErrNotExist) {
		return NewDay(t), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read work log for %s: %w", date, err)
	}
	var day Day
	if err := json.Unmarshal(data, &day); err != nil {
		return nil, fmt.Errorf("parse work log for %s: %w", date, err)
	}
	if day.Date == "" {
		day.Date = date
	}
	return &day, nil
}

// Current returns the day the commands should act on. That is normally today,
// but if today is idle and yesterday's session is still running (a shift that
// crossed midnight), yesterday's day is returned instead.
func (s *Store) Current(now time.Time) (*Day, error) {
	today, err := s.Load(now)
	if err != nil {
		return nil, err
	}
	if today.Started() {
		return today, nil
	}
	yesterday, err := s.Load(now.AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}
	if yesterday.State() != Idle {
		return yesterday, nil
	}
	return today, nil
}

// Save writes the day atomically, creating the data directory if needed.
func (s *Store) Save(day *Day) error {
	return s.write(s.path(day.Date), day)
}

// Days returns every started day whose date lies between from and to
// inclusive, in chronological order. An empty to means "up to the last day on
// record". Dates are compared as strings, which works because they are ISO
// formatted.
func (s *Store) Days(from, to string) ([]*Day, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read data directory %s: %w", s.dir, err)
	}

	var days []*Day
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		date, ok := dayFileDate(entry.Name())
		if !ok || date < from || (to != "" && date > to) {
			continue
		}
		day, err := s.Load(parseDate(date))
		if err != nil {
			return nil, err
		}
		if day.Started() {
			days = append(days, day)
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	return days, nil
}

// dayFileDate returns the date encoded in a day file name. Anything else in
// the directory, including the week files older versions wrote, is rejected.
func dayFileDate(name string) (string, bool) {
	base, ok := strings.CutSuffix(name, ".json")
	if !ok || parseDate(base).IsZero() {
		return "", false
	}
	return base, true
}

// Report loads the days of a calendar week and totals them up.
func (s *Store) Report(week Week, now time.Time) (WeekReport, error) {
	days, err := s.Days(week.Start, week.End)
	if err != nil {
		return WeekReport{}, err
	}
	return NewWeekReport(week, days, now), nil
}

// CurrentWeek reports the calendar week t falls in.
func (s *Store) CurrentWeek(now time.Time) (WeekReport, error) {
	return s.Report(WeekOf(now), now)
}

// History returns a report per calendar week that has days on record, oldest
// first. The ISO since and until dates limit it to the days between them, both
// included; either may be empty for no limit on that end. A week reaching past
// either one is reported with the days that are left of it.
func (s *Store) History(now time.Time, since, until string) ([]WeekReport, error) {
	days, err := s.Days(since, until)
	if err != nil {
		return nil, err
	}

	// Days arrive in chronological order, so a day either belongs to the week
	// the one before it did or opens the next one.
	var reports []WeekReport
	for _, day := range days {
		week := WeekOf(day.CalendarDate())
		if n := len(reports); n > 0 && reports[n-1].Week == week {
			reports[n-1].Days = append(reports[n-1].Days, day)
			continue
		}
		reports = append(reports, WeekReport{Week: week, Days: []*Day{day}})
	}
	for i, r := range reports {
		reports[i] = NewWeekReport(r.Week, r.Days, now)
	}
	return reports, nil
}

// gapLookback is how far back GapBefore looks for the last day on record.
// Coming back from a longer absence than this is not worth being reminded of.
const gapLookback = 31

// Gap is the stretch between the last day on record and the day being started.
type Gap struct {
	// Since is the last day that has anything recorded on it.
	Since time.Time
	// Weekdays are the days in between, weekends left out, that have nothing
	// recorded on them: the ones that were presumably worked but never logged.
	Weekdays []time.Time
}

// Empty reports whether nothing was skipped, which is also what a first ever
// day looks like.
func (g Gap) Empty() bool { return len(g.Weekdays) == 0 }

// GapBefore returns the weekdays between the last day on record and t, both
// excluded, that have nothing recorded on them. Anything already recorded on
// t itself is the day being started, not a gap.
func (s *Store) GapBefore(t time.Time) (Gap, error) {
	today := t.Format(DateLayout)
	days, err := s.Days(t.AddDate(0, 0, -gapLookback).Format(DateLayout), today)
	if err != nil {
		return Gap{}, err
	}
	for len(days) > 0 && days[len(days)-1].Date >= today {
		days = days[:len(days)-1]
	}
	if len(days) == 0 {
		return Gap{}, nil
	}

	gap := Gap{Since: days[len(days)-1].CalendarDate()}
	for day := gap.Since.AddDate(0, 0, 1); day.Format(DateLayout) < today; day = day.AddDate(0, 0, 1) {
		if !isWeekend(day) {
			gap.Weekdays = append(gap.Weekdays, day)
		}
	}
	return gap, nil
}

// write serialises v as JSON and replaces path with it atomically.
func (s *Store) write(path string, v any) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create data directory %s: %w", s.dir, err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("set permissions on %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("save %s: %w", path, err)
	}
	return nil
}
