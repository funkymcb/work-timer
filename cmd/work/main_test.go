package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/funkymcb/work-timer/internal/worklog"
)

func at(hour, min int) time.Time {
	return time.Date(2026, 3, 10, hour, min, 0, 0, time.UTC)
}

func TestTmuxStatus(t *testing.T) {
	tests := []struct {
		name  string
		build func() *worklog.Day
		now   time.Time
		want  string
	}{
		{
			name:  "not started is empty so the segment disappears",
			build: func() *worklog.Day { return worklog.NewDay(at(8, 0)) },
			now:   at(9, 0),
			want:  "",
		},
		{
			name: "working shows play and net worked time",
			build: func() *worklog.Day {
				d := worklog.NewDay(at(8, 0))
				d.Start(at(8, 12))
				return d
			},
			now:  at(12, 30),
			want: "▶ 4h18m",
		},
		{
			name: "on break shows pause and the length of this break",
			build: func() *worklog.Day {
				d := worklog.NewDay(at(8, 0))
				d.Start(at(8, 12))
				d.Pause(at(12, 30))
				return d
			},
			now:  at(12, 42),
			want: "⏸ 12m",
		},
		{
			name: "stopped shows stop and the day total",
			build: func() *worklog.Day {
				d := worklog.NewDay(at(8, 0))
				d.Start(at(8, 12))
				d.Pause(at(12, 30))
				d.Resume(at(13, 12))
				d.Stop(at(17, 30))
				return d
			},
			now:  at(19, 0),
			want: "⏹ 8h36m",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tmuxStatus(tc.build(), tc.now); got != tc.want {
				t.Errorf("tmuxStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseAt(t *testing.T) {
	now := at(14, 30)
	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantErr bool
	}{
		{name: "earlier today", value: "13:07", want: at(13, 7)},
		{name: "seconds are allowed", value: "13:07:45", want: at(13, 7).Add(45 * time.Second)},
		{name: "now itself", value: "14:30", want: now},
		// A time that has not come round yet today can only mean yesterday,
		// which is what a shift across midnight needs.
		{name: "later today means yesterday", value: "23:50", want: at(23, 50).AddDate(0, 0, -1)},
		{name: "not a time", value: "lunch", wantErr: true},
		{name: "no colon", value: "1307", wantErr: true},
		{name: "not on the clock", value: "25:00", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAt(tc.value, now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseAt(%q) = %v, want an error", tc.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAt(%q): %v", tc.value, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("parseAt(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestParseAtFlag(t *testing.T) {
	now := at(14, 30)

	got, err := parseAtFlag("resume", nil, now, io.Discard)
	if err != nil {
		t.Fatalf("without --at: %v", err)
	}
	if !got.Equal(now) {
		t.Errorf("without --at = %v, want now %v", got, now)
	}

	got, err = parseAtFlag("resume", []string{"--at", "13:07"}, now, io.Discard)
	if err != nil {
		t.Fatalf("with --at: %v", err)
	}
	if want := at(13, 7); !got.Equal(want) {
		t.Errorf("with --at = %v, want %v", got, want)
	}

	if _, err := parseAtFlag("resume", []string{"13:07"}, now, io.Discard); err == nil {
		t.Error("a bare time argument was accepted, want an error pointing at --at")
	}
}

func TestTimeRange(t *testing.T) {
	end := at(17, 24)
	nextDay := at(1, 30).AddDate(0, 0, 1)

	tests := []struct {
		name  string
		start time.Time
		end   *time.Time
		want  string
	}{
		{"finished", at(8, 12), &end, "08:12 - 17:24"},
		{"still running", at(8, 12), nil, "08:12 - open"},
		{"over midnight", at(21, 0), &nextDay, "21:00 - 01:30+1d"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := timeRange(tc.start, tc.end); got != tc.want {
				t.Errorf("timeRange() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	oct1 := time.Date(2024, 10, 1, 0, 0, 0, 0, time.Local)

	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantErr bool
	}{
		{name: "iso", value: "2024-10-01", want: oct1},
		{name: "iso without padding", value: "2024-10-1", want: oct1},
		{name: "day first with dots", value: "01.10.2024", want: oct1},
		{name: "day first, short year", value: "01.10.24", want: oct1},
		{name: "day first without padding", value: "1.10.2024", want: oct1},
		{name: "month first with slashes", value: "10/01/2024", want: oct1},
		{name: "month first, short year", value: "10/1/24", want: oct1},
		{name: "year first with slashes", value: "2024/10/01", want: oct1},
		// A four digit year must never be read as a day or a month.
		{name: "year first is not day first", value: "2024/10/1", want: oct1},
		{name: "no date at all", value: "october", wantErr: true},
		{name: "not a day of that month", value: "31.02.2024", wantErr: true},
		{name: "month out of range", value: "01.13.2024", wantErr: true},
		{name: "empty", value: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDate("since", tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDate(%q) = %v, want an error", tc.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDate(%q): %v", tc.value, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("parseDate(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestHistoryListsEverythingOnRecord(t *testing.T) {
	store := worklog.NewStore(t.TempDir())
	// It is Wednesday evening; work is still running from the afternoon.
	now := at(19, 30).AddDate(0, 0, 1)

	// A day worked before any week was opened.
	loose := worklog.NewDay(at(10, 0).AddDate(0, 0, -2))
	mustDo(t, loose.Start(at(10, 0).AddDate(0, 0, -2)))
	mustDo(t, loose.Stop(at(12, 10).AddDate(0, 0, -2)))
	mustDo(t, store.Save(loose))

	// Tuesday: one session with a break in it.
	tue := worklog.NewDay(at(8, 0))
	mustDo(t, tue.Start(at(8, 0)))
	mustDo(t, tue.Pause(at(12, 0)))
	mustDo(t, tue.Resume(at(12, 30)))
	mustDo(t, tue.Stop(at(17, 0)))
	mustDo(t, store.Save(tue))

	// Wednesday: a morning session, then an evening one that is still open.
	wed := worklog.NewDay(at(8, 0).AddDate(0, 0, 1))
	mustDo(t, wed.Start(at(8, 0).AddDate(0, 0, 1)))
	mustDo(t, wed.Stop(at(12, 0).AddDate(0, 0, 1)))
	mustDo(t, wed.Start(at(18, 0).AddDate(0, 0, 1)))
	mustDo(t, store.Save(wed))

	mustDo(t, store.SaveWeek(worklog.NewWeek(at(8, 0))))

	var out strings.Builder
	if err := history(store, now, &out, nil); err != nil {
		t.Fatalf("history: %v", err)
	}

	want := strings.Join([]string{
		"▶ Work week since Tue 10 Mar · 2 of 5 working days used",
		"",
		"  Tue 10 Mar   8h 30m   breaks    30m",
		"      ▶ 08:00 - 17:00     8h 30m",
		"        ⏸ 12:00 - 12:30      30m",
		"  Wed 11 Mar   5h 30m   breaks     0m   ▶ now",
		"      ▶ 08:00 - 12:00     4h 00m",
		"      ▶ 18:00 - open      1h 30m",
		"",
		"  Total       14h 00m   breaks    30m",
		"  Average      7h 00m",
		"",
		"⏹ Outside any work week · 1 day",
		"",
		"  Sun 08 Mar   2h 10m   breaks     0m",
		"      ▶ 10:00 - 12:10     2h 10m",
		"",
		"All time · 1 week · 3 working days · 16h 10m worked · 30m on breaks",
		"",
	}, "\n")

	if got := out.String(); got != want {
		t.Errorf("history output:\n%s\nwant:\n%s", got, want)
	}
}

func TestHistorySinceMarksAWeekItCutsThrough(t *testing.T) {
	store := worklog.NewStore(t.TempDir())
	now := at(19, 0).AddDate(0, 0, 1)

	for _, offset := range []int{0, 1} {
		d := worklog.NewDay(at(8, 0).AddDate(0, 0, offset))
		mustDo(t, d.Start(at(8, 0).AddDate(0, 0, offset)))
		mustDo(t, d.Stop(at(16, 0).AddDate(0, 0, offset)))
		mustDo(t, store.Save(d))
	}
	week := worklog.NewWeek(at(8, 0))
	mustDo(t, week.Close(at(16, 0).AddDate(0, 0, 1)))
	mustDo(t, store.SaveWeek(week))

	// Cutting into the week hides its first day, so the header has to say
	// that the days and totals below cover only part of it.
	var cut strings.Builder
	if err := history(store, now, &cut, []string{"--since", "11.03.2026"}); err != nil {
		t.Fatalf("history: %v", err)
	}
	want := "⏹ Work week Tue 10 Mar - Wed 11 Mar · 1 working day · from Wed 11 Mar on\n"
	if !strings.HasPrefix(cut.String(), want) {
		t.Errorf("history starts with:\n%s\nwant it to start with:\n%s", cut.String(), want)
	}

	// A cutoff the week lies entirely after leaves it whole, and unmarked.
	var whole strings.Builder
	if err := history(store, now, &whole, []string{"--since", "10.03.2026"}); err != nil {
		t.Fatalf("history: %v", err)
	}
	if strings.Contains(whole.String(), " on\n") {
		t.Errorf("a week that is shown in full is marked as cut:\n%s", whole.String())
	}
}

func mustDo(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompactAndDuration(t *testing.T) {
	tests := []struct {
		in           time.Duration
		wantDuration string
		wantCompact  string
	}{
		{0, "0m", "0m"},
		{45 * time.Second, "0m", "0m"},
		{12 * time.Minute, "12m", "12m"},
		{time.Hour + 5*time.Minute, "1h 05m", "1h05m"},
		{8*time.Hour + 36*time.Minute + 59*time.Second, "8h 36m", "8h36m"},
		{-time.Hour, "0m", "0m"},
	}

	for _, tc := range tests {
		if got := duration(tc.in); got != tc.wantDuration {
			t.Errorf("duration(%v) = %q, want %q", tc.in, got, tc.wantDuration)
		}
		if got := compact(tc.in); got != tc.wantCompact {
			t.Errorf("compact(%v) = %q, want %q", tc.in, got, tc.wantCompact)
		}
	}
}
