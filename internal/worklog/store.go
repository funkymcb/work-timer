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

// Store persists one JSON file per calendar day, plus one file per work week,
// in a data directory.
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

// WeekDays returns the started days that belong to the given week, up to today
// while the week is still active.
func (s *Store) WeekDays(week *Week, now time.Time) ([]*Day, error) {
	to := week.End
	if week.Active() {
		to = now.Format(DateLayout)
	}
	return s.Days(week.Start, to)
}

// Report loads the days of a week and totals them up.
func (s *Store) Report(week *Week, now time.Time) (WeekReport, error) {
	days, err := s.WeekDays(week, now)
	if err != nil {
		return WeekReport{}, err
	}
	return NewWeekReport(week, days, now), nil
}

// History returns a report for every recorded week, oldest first, together
// with the started days that no week lays claim to. Those loose days are days
// worked before weeks were kept, or files edited by hand; reporting them keeps
// the history a complete account of what is on disk.
//
// The ISO since and until dates limit the history to the days between them,
// both included; either may be empty for no limit on that end. A week is then
// reported with the days that are left of it, and dropped entirely once
// nothing of it falls inside the range.
func (s *Store) History(now time.Time, since, until string) (weeks []WeekReport, loose []*Day, err error) {
	recorded, err := s.Weeks()
	if err != nil {
		return nil, nil, err
	}
	days, err := s.Days(since, until)
	if err != nil {
		return nil, nil, err
	}

	// Days arrive in chronological order, so each group keeps that order.
	grouped := make([][]*Day, len(recorded))
	for _, day := range days {
		i := weekOf(recorded, day.Date, now)
		if i < 0 {
			loose = append(loose, day)
			continue
		}
		grouped[i] = append(grouped[i], day)
	}

	weeks = make([]WeekReport, 0, len(recorded))
	for i, week := range recorded {
		// A week with no days left is only worth reporting when it is one that
		// starts inside the range and has nothing logged yet, the way a week
		// that was just opened has not.
		if len(grouped[i]) == 0 && (week.Start < since || (until != "" && week.Start > until)) {
			continue
		}
		weeks = append(weeks, NewWeekReport(week, grouped[i], now))
	}
	return weeks, loose, nil
}

// weekOf returns the index of the first week covering the given date, or -1.
func weekOf(weeks []*Week, date string, now time.Time) int {
	for i, w := range weeks {
		if w.Covers(date, now) {
			return i
		}
	}
	return -1
}

// dayFileDate returns the date encoded in a day file name. Week files and
// anything else in the directory are rejected.
func dayFileDate(name string) (string, bool) {
	base, ok := strings.CutSuffix(name, ".json")
	if !ok || parseDate(base).IsZero() {
		return "", false
	}
	return base, true
}

func (s *Store) weekPath(start string) string {
	return filepath.Join(s.dir, weekFilePrefix+start+".json")
}

const weekFilePrefix = "week-"

// Weeks returns every recorded week ordered by start date.
func (s *Store) Weeks() ([]*Week, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read data directory %s: %w", s.dir, err)
	}

	var weeks []*Week
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, weekFilePrefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var week Week
		if err := json.Unmarshal(data, &week); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if err := week.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		weeks = append(weeks, &week)
	}
	sort.Slice(weeks, func(i, j int) bool { return weeks[i].Start < weeks[j].Start })
	return weeks, nil
}

// ActiveWeek returns the running week, or nil when none is active.
func (s *Store) ActiveWeek() (*Week, error) {
	weeks, err := s.Weeks()
	if err != nil {
		return nil, err
	}
	for i := len(weeks) - 1; i >= 0; i-- {
		if weeks[i].Active() {
			return weeks[i], nil
		}
	}
	return nil, nil
}

// LastWeek returns the most recently started week, or nil when there is none.
func (s *Store) LastWeek() (*Week, error) {
	weeks, err := s.Weeks()
	if err != nil || len(weeks) == 0 {
		return nil, err
	}
	return weeks[len(weeks)-1], nil
}

// SaveWeek writes the week atomically.
func (s *Store) SaveWeek(week *Week) error {
	if err := week.Validate(); err != nil {
		return err
	}
	return s.write(s.weekPath(week.Start), week)
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
