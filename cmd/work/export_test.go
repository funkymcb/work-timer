package main

import (
	"strings"
	"testing"
	"time"

	"github.com/funkymcb/work-timer/internal/worklog"
)

// exportFixture records a running week with one day of two sessions - the
// second still open - plus a day from before weeks were kept, and returns the
// store together with the time the history is taken at.
func exportFixture(t *testing.T) (*worklog.Store, time.Time) {
	t.Helper()
	store := worklog.NewStore(t.TempDir())

	loose := worklog.NewDay(at(10, 0).AddDate(0, 0, -2))
	mustDo(t, loose.Start(at(10, 0).AddDate(0, 0, -2)))
	mustDo(t, loose.Stop(at(12, 10).AddDate(0, 0, -2)))
	mustDo(t, store.Save(loose))

	day := worklog.NewDay(at(8, 0))
	mustDo(t, day.Start(at(8, 0)))
	mustDo(t, day.Pause(at(9, 30)))
	mustDo(t, day.Resume(at(9, 45)))
	mustDo(t, day.Stop(at(12, 0)))
	mustDo(t, day.Start(at(13, 0)))
	mustDo(t, store.Save(day))

	mustDo(t, store.SaveWeek(worklog.NewWeek(at(8, 0))))

	return store, at(14, 0)
}

func TestHistoryAsJSON(t *testing.T) {
	store, now := exportFixture(t)

	var out strings.Builder
	if err := history(store, now, &out, []string{"--out", "json"}); err != nil {
		t.Fatalf("history: %v", err)
	}

	want := strings.Join([]string{
		`{`,
		`  "generated_at": "2026-03-10T14:00:00Z",`,
		`  "weeks": [`,
		`    {`,
		`      "start": "2026-03-10",`,
		`      "active": true,`,
		`      "worked_minutes": 285,`,
		`      "break_minutes": 15,`,
		`      "days": [`,
		`        {`,
		`          "date": "2026-03-10",`,
		`          "state": "working",`,
		`          "worked_minutes": 285,`,
		`          "break_minutes": 15,`,
		`          "sessions": [`,
		`            {`,
		`              "start": "2026-03-10T08:00:00Z",`,
		`              "end": "2026-03-10T12:00:00Z",`,
		`              "worked_minutes": 225,`,
		`              "break_minutes": 15,`,
		`              "breaks": [`,
		`                {`,
		`                  "start": "2026-03-10T09:30:00Z",`,
		`                  "end": "2026-03-10T09:45:00Z",`,
		`                  "minutes": 15`,
		`                }`,
		`              ]`,
		`            },`,
		`            {`,
		`              "start": "2026-03-10T13:00:00Z",`,
		`              "worked_minutes": 60,`,
		`              "break_minutes": 0,`,
		`              "breaks": []`,
		`            }`,
		`          ]`,
		`        }`,
		`      ]`,
		`    }`,
		`  ],`,
		`  "days_outside_weeks": [`,
		`    {`,
		`      "date": "2026-03-08",`,
		`      "state": "idle",`,
		`      "worked_minutes": 130,`,
		`      "break_minutes": 0,`,
		`      "sessions": [`,
		`        {`,
		`          "start": "2026-03-08T10:00:00Z",`,
		`          "end": "2026-03-08T12:10:00Z",`,
		`          "worked_minutes": 130,`,
		`          "break_minutes": 0,`,
		`          "breaks": []`,
		`        }`,
		`      ]`,
		`    }`,
		`  ],`,
		`  "totals": {`,
		`    "weeks": 1,`,
		`    "working_days": 2,`,
		`    "worked_minutes": 415,`,
		`    "break_minutes": 15`,
		`  }`,
		`}`,
		``,
	}, "\n")

	if got := out.String(); got != want {
		t.Errorf("json output:\n%s\nwant:\n%s", got, want)
	}
}

func TestHistoryAsYAML(t *testing.T) {
	store, now := exportFixture(t)

	var out strings.Builder
	if err := history(store, now, &out, []string{"--out", "yaml"}); err != nil {
		t.Fatalf("history: %v", err)
	}

	want := strings.Join([]string{
		`generated_at: "2026-03-10T14:00:00Z"`,
		`weeks:`,
		`  - start: "2026-03-10"`,
		`    active: true`,
		`    worked_minutes: 285`,
		`    break_minutes: 15`,
		`    days:`,
		`      - date: "2026-03-10"`,
		`        state: "working"`,
		`        worked_minutes: 285`,
		`        break_minutes: 15`,
		`        sessions:`,
		`          - start: "2026-03-10T08:00:00Z"`,
		`            end: "2026-03-10T12:00:00Z"`,
		`            worked_minutes: 225`,
		`            break_minutes: 15`,
		`            breaks:`,
		`              - start: "2026-03-10T09:30:00Z"`,
		`                end: "2026-03-10T09:45:00Z"`,
		`                minutes: 15`,
		`          - start: "2026-03-10T13:00:00Z"`,
		`            worked_minutes: 60`,
		`            break_minutes: 0`,
		`            breaks: []`,
		`days_outside_weeks:`,
		`  - date: "2026-03-08"`,
		`    state: "idle"`,
		`    worked_minutes: 130`,
		`    break_minutes: 0`,
		`    sessions:`,
		`      - start: "2026-03-08T10:00:00Z"`,
		`        end: "2026-03-08T12:10:00Z"`,
		`        worked_minutes: 130`,
		`        break_minutes: 0`,
		`        breaks: []`,
		`totals:`,
		`  weeks: 1`,
		`  working_days: 2`,
		`  worked_minutes: 415`,
		`  break_minutes: 15`,
		``,
	}, "\n")

	if got := out.String(); got != want {
		t.Errorf("yaml output:\n%s\nwant:\n%s", got, want)
	}
}

// The two formats are written from the same structs, so every key of one has
// to appear in the other. A field added without a yaml case shows up here.
func TestJSONAndYAMLCarryTheSameKeys(t *testing.T) {
	store, now := exportFixture(t)

	var jsonOut, yamlOut strings.Builder
	if err := history(store, now, &jsonOut, []string{"--out", "json"}); err != nil {
		t.Fatalf("json: %v", err)
	}
	if err := history(store, now, &yamlOut, []string{"--out", "yaml"}); err != nil {
		t.Fatalf("yaml: %v", err)
	}

	jsonKeys := keysOf(jsonOut.String(), `"`)
	if len(jsonKeys) == 0 {
		t.Fatal("no keys found in the json output, the comparison below would pass on anything")
	}

	for _, key := range jsonKeys {
		if !strings.Contains(yamlOut.String(), key+":") {
			t.Errorf("key %q is in the json output but not in the yaml one", key)
		}
	}
	for _, key := range keysOf(yamlOut.String(), "") {
		if !strings.Contains(jsonOut.String(), `"`+key+`":`) {
			t.Errorf("key %q is in the yaml output but not in the json one", key)
		}
	}
}

// keysOf collects the field names of an encoded document, i.e. what sits in
// front of every colon, stripped of the given quoting and of list markers.
func keysOf(doc, quote string) []string {
	var keys []string
	for _, line := range strings.Split(doc, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		key, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.Trim(key, quote)
		if key != "" && !strings.Contains(key, " ") {
			keys = append(keys, key)
		}
	}
	return keys
}

func TestHistorySinceLimitsEveryOutput(t *testing.T) {
	store, now := exportFixture(t)
	// The fixture holds a loose day on the 8th and a week day on the 10th.
	const since = "10.03.2026"

	var text strings.Builder
	if err := history(store, now, &text, []string{"--since", since}); err != nil {
		t.Fatalf("text: %v", err)
	}
	if strings.Contains(text.String(), "Outside any work week") {
		t.Errorf("the loose day before the cutoff is still listed:\n%s", text.String())
	}
	// The closing line names the cutoff, because the weeks above it can be
	// listed with fewer days than they really hold.
	if want := "Since Tue 10 Mar 2026 · 1 week · 1 working day · 4h 45m worked · 15m on breaks\n"; !strings.HasSuffix(text.String(), want) {
		t.Errorf("history ends with:\n%s\nwant it to end with:\n%s", text.String(), want)
	}

	var data strings.Builder
	if err := history(store, now, &data, []string{"--since", since, "--out", "json"}); err != nil {
		t.Fatalf("json: %v", err)
	}
	if want := `"since": "2026-03-10",`; !strings.Contains(data.String(), want) {
		t.Errorf("json output does not record the cutoff as %s:\n%s", want, data.String())
	}
	if strings.Contains(data.String(), "2026-03-08") {
		t.Errorf("the day before the cutoff is still in the json output:\n%s", data.String())
	}

	// Without the flag the cutoff is left out rather than reported as empty.
	var unfiltered strings.Builder
	if err := history(store, now, &unfiltered, []string{"--out", "yaml"}); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	if strings.Contains(unfiltered.String(), "since:") {
		t.Errorf("an unfiltered history carries a since field:\n%s", unfiltered.String())
	}
}

func TestHistoryRejectsAnUnknownSince(t *testing.T) {
	store, now := exportFixture(t)

	var out strings.Builder
	err := history(store, now, &out, []string{"--since", "last monday"})
	if err == nil {
		t.Fatal("an unparsable date was accepted")
	}
	if !strings.Contains(err.Error(), "01.10.2024") {
		t.Errorf("error %q does not show an accepted date format", err)
	}
}

func TestHistoryRejectsAnUnknownOutput(t *testing.T) {
	store, now := exportFixture(t)

	var out strings.Builder
	err := history(store, now, &out, []string{"--out", "toml"})
	if err == nil {
		t.Fatal("unknown output was accepted")
	}
	if !strings.Contains(err.Error(), "text, json or yaml") {
		t.Errorf("error %q does not say which outputs there are", err)
	}
}

func TestMinutesTruncateLikeTheTextOutput(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want int
	}{
		{0, 0},
		{59 * time.Second, 0},
		{90 * time.Second, 1},
		{8*time.Hour + 30*time.Minute + 59*time.Second, 510},
		{-time.Hour, 0},
	}

	for _, tc := range tests {
		if got := minutes(tc.in); got != tc.want {
			t.Errorf("minutes(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestHistoryUntilLimitsEveryOutput(t *testing.T) {
	store, now := exportFixture(t)
	// The fixture holds a loose day on the 8th and a week day on the 10th.
	const until = "03/08/2026" // month first: 8 March 2026

	var text strings.Builder
	if err := history(store, now, &text, []string{"--until", until}); err != nil {
		t.Fatalf("text: %v", err)
	}
	if strings.Contains(text.String(), "2026-03-10") || strings.Contains(text.String(), "Tue 10 Mar") {
		t.Errorf("the day after the cutoff is still listed:\n%s", text.String())
	}
	if want := "Up to Sun 08 Mar 2026 · 0 weeks · 1 working day · 2h 10m worked · 0m on breaks\n"; !strings.HasSuffix(text.String(), want) {
		t.Errorf("history ends with:\n%s\nwant it to end with:\n%s", text.String(), want)
	}

	var data strings.Builder
	if err := history(store, now, &data, []string{"--until", until, "--out", "yaml"}); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	if want := `until: "2026-03-08"`; !strings.Contains(data.String(), want) {
		t.Errorf("yaml output does not record the cutoff as %s:\n%s", want, data.String())
	}
	if strings.Contains(data.String(), "since:") {
		t.Errorf("an open start is reported as a since field:\n%s", data.String())
	}
}

func TestHistoryRangeMarksBothEndsOfAWeek(t *testing.T) {
	store := worklog.NewStore(t.TempDir())
	now := at(19, 0).AddDate(0, 0, 3)

	// A closed week of four days, Tuesday through Friday.
	for offset := range 4 {
		d := worklog.NewDay(at(8, 0).AddDate(0, 0, offset))
		mustDo(t, d.Start(at(8, 0).AddDate(0, 0, offset)))
		mustDo(t, d.Stop(at(16, 0).AddDate(0, 0, offset)))
		mustDo(t, store.Save(d))
	}
	week := worklog.NewWeek(at(8, 0))
	mustDo(t, week.Close(at(16, 0).AddDate(0, 0, 3)))
	mustDo(t, store.SaveWeek(week))

	var out strings.Builder
	if err := history(store, now, &out, []string{"--since", "11.03.2026", "--until", "12.03.2026"}); err != nil {
		t.Fatalf("history: %v", err)
	}

	head := "⏹ Work week Tue 10 Mar - Fri 13 Mar · 2 working days · from Wed 11 Mar to Thu 12 Mar\n"
	if !strings.HasPrefix(out.String(), head) {
		t.Errorf("history starts with:\n%s\nwant it to start with:\n%s", out.String(), head)
	}
	tail := "Wed 11 Mar 2026 - Thu 12 Mar 2026 · 1 week · 2 working days · 16h 00m worked · 0m on breaks\n"
	if !strings.HasSuffix(out.String(), tail) {
		t.Errorf("history ends with:\n%s\nwant it to end with:\n%s", out.String(), tail)
	}
}

func TestHistoryRejectsABackwardsRange(t *testing.T) {
	store, now := exportFixture(t)

	var out strings.Builder
	err := history(store, now, &out, []string{"--since", "10.03.2026", "--until", "08.03.2026"})
	if err == nil {
		t.Fatal("a range that ends before it starts was accepted")
	}
	if !strings.Contains(err.Error(), "before") {
		t.Errorf("error %q does not say the range runs backwards", err)
	}
}

func TestHistoryRejectsAnUnknownUntil(t *testing.T) {
	store, now := exportFixture(t)

	var out strings.Builder
	err := history(store, now, &out, []string{"--until", "yesterday"})
	if err == nil {
		t.Fatal("an unparsable date was accepted")
	}
	if !strings.Contains(err.Error(), "--until") {
		t.Errorf("error %q does not name the flag it came from", err)
	}
}
