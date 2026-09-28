package worklog

import (
	"testing"
	"time"
)

func TestWeeklyTarget(t *testing.T) {
	tests := []struct {
		name  string
		hours float64
		want  time.Duration
	}{
		{"the default full time week", DefaultWeeklyHours, 40 * time.Hour},
		{"half hours round to the minute", 37.5, 37*time.Hour + 30*time.Minute},
		{"a part time week", 20, 20 * time.Hour},
		{"an unset week falls back on the default", 0, 40 * time.Hour},
		{"so does one edited below zero", -5, 40 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Config{WeeklyHours: tc.hours}).WeeklyTarget(); got != tc.want {
				t.Errorf("WeeklyTarget() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestConfigDefaultsWhenNeverWritten(t *testing.T) {
	config, err := NewStore(t.TempDir()).Config()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if got, want := config.WeeklyHours, float64(DefaultWeeklyHours); got != want {
		t.Errorf("weekly hours = %v, want %v", got, want)
	}
	if config.OvertimeSince != "" {
		t.Errorf("overtime since = %q, want empty - nothing has been reset yet", config.OvertimeSince)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())
	mustDo(t, s.SaveConfig(Config{WeeklyHours: 37.5, OvertimeSince: "2026-03-09"}))

	config, err := s.Config()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if got, want := config.WeeklyTarget(), 37*time.Hour+30*time.Minute; got != want {
		t.Errorf("weekly target = %v, want %v", got, want)
	}
	if got, want := config.OvertimeSince, "2026-03-09"; got != want {
		t.Errorf("overtime since = %q, want %q", got, want)
	}
}

func TestConfigFileIsNotReadAsADay(t *testing.T) {
	s := NewStore(t.TempDir())
	mustDo(t, s.SaveConfig(DefaultConfig()))
	mustDo(t, s.Save(workedDay(t, 9, 8)))

	days, err := s.Days("", "")
	if err != nil {
		t.Fatalf("days: %v", err)
	}
	if got, want := len(days), 1; got != want {
		t.Fatalf("days = %d, want %d - the settings file is not a day", got, want)
	}
}

func TestOvertimeSkipsWeeksWithNothingLogged(t *testing.T) {
	s := NewStore(t.TempDir())
	// Two days in the week of the 2nd, one in the week of the 16th. The week
	// of the 9th holds nothing at all and must not count as 40 hours owed.
	for _, day := range []int{5, 6, 16} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}

	report, err := s.Overtime(DefaultConfig(), onDay(23, 20))
	if err != nil {
		t.Fatalf("overtime: %v", err)
	}
	if got, want := report.Weeks, 2; got != want {
		t.Fatalf("weeks = %d, want %d - the empty week is not owed", got, want)
	}
	if got, want := report.Worked, 24*time.Hour; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	if got, want := report.Target, 80*time.Hour; got != want {
		t.Errorf("target = %v, want %v", got, want)
	}
	if got, want := report.Balance(), -56*time.Hour; got != want {
		t.Errorf("balance = %v, want %v", got, want)
	}
	if got, want := report.First.Start, "2026-03-02"; got != want {
		t.Errorf("first week = %q, want %q", got, want)
	}
	if got, want := report.Last.Start, "2026-03-16"; got != want {
		t.Errorf("last week = %q, want %q", got, want)
	}
}

func TestOvertimeLeavesTheRunningWeekOut(t *testing.T) {
	s := NewStore(t.TempDir())
	// Monday of the week that is running, eight hours in.
	mustDo(t, s.Save(workedDay(t, 16, 8)))

	report, err := s.Overtime(DefaultConfig(), onDay(16, 20))
	if err != nil {
		t.Fatalf("overtime: %v", err)
	}
	if !report.Empty() {
		t.Errorf("report = %+v, want nothing counted - the week is still running", report)
	}
	if got, want := report.Balance(), time.Duration(0); got != want {
		t.Errorf("balance = %v, want %v - a part worked week is not a shortfall", got, want)
	}

	// The week after, the same days are a finished week and do count.
	report, err = s.Overtime(DefaultConfig(), onDay(23, 20))
	if err != nil {
		t.Fatalf("overtime: %v", err)
	}
	if got, want := report.Balance(), -32*time.Hour; got != want {
		t.Errorf("balance = %v, want %v once the week is over", got, want)
	}
}

func TestOvertimeStartsOverAtTheResetDate(t *testing.T) {
	s := NewStore(t.TempDir())
	// A short week of 24 hours, then a full one of 40.
	for _, day := range []int{9, 10, 11} {
		mustDo(t, s.Save(workedDay(t, day, 8)))
	}
	for _, day := range []int{16, 17, 18, 19} {
		mustDo(t, s.Save(workedDay(t, day, 10)))
	}

	config := Config{WeeklyHours: DefaultWeeklyHours, OvertimeSince: "2026-03-16"}
	report, err := s.Overtime(config, onDay(23, 20))
	if err != nil {
		t.Fatalf("overtime: %v", err)
	}
	if got, want := report.Weeks, 1; got != want {
		t.Fatalf("weeks = %d, want %d - the week before the reset is gone", got, want)
	}
	if got, want := report.Balance(), time.Duration(0); got != want {
		t.Errorf("balance = %v, want %v", got, want)
	}
	if got, want := report.SinceDate(), onDay(16, 0); !got.Equal(want) {
		t.Errorf("since = %v, want %v", got, want)
	}
}

func TestUsedUpComesOffTheBalance(t *testing.T) {
	s := NewStore(t.TempDir())
	// A week of 44 hours: four hours of overtime built up.
	for _, day := range []int{9, 10, 11, 12} {
		mustDo(t, s.Save(workedDay(t, day, 11)))
	}

	config := DefaultConfig()
	config.UseUp(3, onDay(17, 9))
	report, err := s.Overtime(config, onDay(17, 20))
	if err != nil {
		t.Fatalf("overtime: %v", err)
	}
	if got, want := report.Used, 3*time.Hour; got != want {
		t.Errorf("used = %v, want %v", got, want)
	}
	if got, want := report.Balance(), time.Hour; got != want {
		t.Errorf("balance = %v, want %v - three of the four hours were taken", got, want)
	}

	// A negative withdrawal puts hours back, which is how a slip is undone.
	config.UseUp(-3, onDay(17, 10))
	if got, want := config.Used(), time.Duration(0); got != want {
		t.Errorf("used = %v, want %v after putting the hours back", got, want)
	}
	if got, want := len(config.OvertimeUsed), 2; got != want {
		t.Errorf("entries = %d, want %d - both stay on the ledger", got, want)
	}
}

func TestResetOvertimeDropsTheWithdrawalsWithTheBalance(t *testing.T) {
	config := DefaultConfig()
	config.UseUp(8, onDay(11, 9))

	week := config.ResetOvertime(onDay(17, 9))
	if got, want := week.Start, "2026-03-16"; got != want {
		t.Errorf("reset to %q, want the Monday %q", got, want)
	}
	if got, want := config.OvertimeSince, "2026-03-16"; got != want {
		t.Errorf("overtime since = %q, want %q", got, want)
	}
	if config.OvertimeUsed != nil {
		t.Errorf("withdrawals = %+v, want none - they came out of a balance that is gone", config.OvertimeUsed)
	}
}
