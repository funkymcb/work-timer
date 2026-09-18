package worklog

import (
	"errors"
	"testing"
	"time"
)

// at returns a time on a fixed reference day, so tests read like a clock.
func at(hour, min int) time.Time {
	return time.Date(2026, 3, 10, hour, min, 0, 0, time.UTC)
}

func TestDayLifecycle(t *testing.T) {
	d := NewDay(at(8, 0))

	if got := d.State(); got != Idle {
		t.Fatalf("fresh day state = %v, want idle", got)
	}
	if err := d.Start(at(8, 0)); err != nil {
		t.Fatalf("start: %v", err)
	}
	if got := d.State(); got != Working {
		t.Fatalf("state after start = %v, want working", got)
	}
	if got, want := d.Worked(at(10, 0)), 2*time.Hour; got != want {
		t.Errorf("worked at 10:00 = %v, want %v", got, want)
	}

	if err := d.Pause(at(12, 0)); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if got := d.State(); got != OnBreak {
		t.Fatalf("state after pause = %v, want on break", got)
	}
	// Time on a break must not increase the worked total.
	if got, want := d.Worked(at(12, 30)), 4*time.Hour; got != want {
		t.Errorf("worked during break = %v, want %v", got, want)
	}
	if got, want := d.BreakTime(at(12, 30)), 30*time.Minute; got != want {
		t.Errorf("break time during break = %v, want %v", got, want)
	}

	if err := d.Resume(at(12, 45)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if got := d.State(); got != Working {
		t.Fatalf("state after resume = %v, want working", got)
	}

	if err := d.Stop(at(17, 15)); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if got := d.State(); got != Idle {
		t.Fatalf("state after stop = %v, want idle", got)
	}
	// 08:00-17:15 is 9h15m elapsed, minus a 45m break.
	if got, want := d.Worked(at(20, 0)), 8*time.Hour+30*time.Minute; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	if got, want := d.BreakTime(at(20, 0)), 45*time.Minute; got != want {
		t.Errorf("break time = %v, want %v", got, want)
	}
	// The span of a finished day must not keep growing after the stop.
	if got, want := d.Span(at(20, 0)), 9*time.Hour+15*time.Minute; got != want {
		t.Errorf("span = %v, want %v", got, want)
	}
}

func TestStopWhileOnBreakClosesTheBreak(t *testing.T) {
	d := NewDay(at(9, 0))
	mustDo(t, d.Start(at(9, 0)))
	mustDo(t, d.Pause(at(11, 0)))
	mustDo(t, d.Stop(at(11, 30)))

	if b, ok := d.CurrentBreak(); ok {
		t.Fatalf("break still open after stop: %+v", b)
	}
	if got, want := d.Worked(at(18, 0)), 2*time.Hour; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	if got, want := d.BreakTime(at(18, 0)), 30*time.Minute; got != want {
		t.Errorf("break time = %v, want %v", got, want)
	}
}

func TestRestartAfterStopAddsSession(t *testing.T) {
	d := NewDay(at(8, 0))
	mustDo(t, d.Start(at(8, 0)))
	mustDo(t, d.Stop(at(12, 0)))
	mustDo(t, d.Start(at(14, 0)))

	if len(d.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(d.Sessions))
	}
	// The gap between stop and start is not a break, it is simply not worked.
	if got, want := d.Worked(at(16, 0)), 6*time.Hour; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
	if got, want := d.BreakTime(at(16, 0)), time.Duration(0); got != want {
		t.Errorf("break time = %v, want %v", got, want)
	}
	if got, want := d.Span(at(16, 0)), 8*time.Hour; got != want {
		t.Errorf("span = %v, want %v", got, want)
	}
}

func TestInvalidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Day)
		do    func(*Day) error
		want  error
	}{
		{"start twice", func(d *Day) { d.Start(at(8, 0)) }, func(d *Day) error { return d.Start(at(9, 0)) }, ErrAlreadyStarted},
		{"pause before start", nil, func(d *Day) error { return d.Pause(at(9, 0)) }, ErrNotStarted},
		{"resume before start", nil, func(d *Day) error { return d.Resume(at(9, 0)) }, ErrNotStarted},
		{"stop before start", nil, func(d *Day) error { return d.Stop(at(9, 0)) }, ErrNotStarted},
		{
			"pause twice",
			func(d *Day) { d.Start(at(8, 0)); d.Pause(at(9, 0)) },
			func(d *Day) error { return d.Pause(at(9, 30)) },
			ErrAlreadyPaused,
		},
		{
			"resume while working",
			func(d *Day) { d.Start(at(8, 0)) },
			func(d *Day) error { return d.Resume(at(9, 0)) },
			ErrNotPaused,
		},
		{
			"start after stop is allowed",
			func(d *Day) { d.Start(at(8, 0)); d.Stop(at(12, 0)) },
			func(d *Day) error { return d.Start(at(13, 0)) },
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDay(at(8, 0))
			if tc.setup != nil {
				tc.setup(d)
			}
			if err := tc.do(d); !errors.Is(err, tc.want) {
				t.Fatalf("got error %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBackdatedEntriesAreRecorded(t *testing.T) {
	// The break was only remembered at 13:30, but it ran 12:00 to 12:45.
	d := NewDay(at(8, 0))
	mustDo(t, d.Start(at(8, 0)))
	mustDo(t, d.Pause(at(12, 0)))
	mustDo(t, d.Resume(at(12, 45)))

	if got, want := d.BreakTime(at(13, 30)), 45*time.Minute; got != want {
		t.Errorf("break time = %v, want %v", got, want)
	}
	if got, want := d.Worked(at(13, 30)), 4*time.Hour+45*time.Minute; got != want {
		t.Errorf("worked = %v, want %v", got, want)
	}
}

func TestEntriesOutOfOrderAreRejected(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Day)
		do    func(*Day) error
	}{
		{
			"pause before the session started",
			func(d *Day) { d.Start(at(9, 0)) },
			func(d *Day) error { return d.Pause(at(8, 30)) },
		},
		{
			"resume before the break started",
			func(d *Day) { d.Start(at(8, 0)); d.Pause(at(12, 0)) },
			func(d *Day) error { return d.Resume(at(11, 30)) },
		},
		{
			"stop before the last break ended",
			func(d *Day) { d.Start(at(8, 0)); d.Pause(at(12, 0)); d.Resume(at(12, 45)) },
			func(d *Day) error { return d.Stop(at(12, 30)) },
		},
		{
			"second session before the first one ended",
			func(d *Day) { d.Start(at(8, 0)); d.Stop(at(12, 0)) },
			func(d *Day) error { return d.Start(at(11, 0)) },
		},
		{
			"first session before the day itself",
			nil,
			func(d *Day) error { return d.Start(at(8, 0).AddDate(0, 0, -1)) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDay(at(8, 0))
			if tc.setup != nil {
				tc.setup(d)
			}
			before := len(d.Sessions)
			if err := tc.do(d); !errors.Is(err, ErrOutOfOrder) {
				t.Fatalf("got error %v, want %v", err, ErrOutOfOrder)
			}
			if len(d.Sessions) != before {
				t.Errorf("sessions = %d, want %d - the rejected entry was written anyway", len(d.Sessions), before)
			}
		})
	}
}

func TestDurationsClampOnBackwardsClock(t *testing.T) {
	d := NewDay(at(10, 0))
	mustDo(t, d.Start(at(10, 0)))

	if got := d.Worked(at(9, 0)); got != 0 {
		t.Errorf("worked with earlier now = %v, want 0", got)
	}
	if got := d.Span(at(9, 0)); got != 0 {
		t.Errorf("span with earlier now = %v, want 0", got)
	}
}

func mustDo(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
