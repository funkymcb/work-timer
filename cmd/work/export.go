package main

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/funkymcb/work-timer/internal/worklog"
)

// The machine readable shape of `work history`. Durations are whole minutes,
// truncated exactly like the text output, so both formats report the same
// numbers. Timestamps are RFC 3339, as they are in the day files. An entry
// that is still running has no end.
type (
	historyView struct {
		// GeneratedAt is the moment the totals were taken. It is what the
		// worked minutes of a running entry were measured against.
		GeneratedAt string `json:"generated_at"`
		// Since and Until are the ends of the range the history was limited
		// to, each absent when that end is open. A week reaching past either
		// one is reported with the days inside it, and totalled over those.
		Since  string     `json:"since,omitempty"`
		Until  string     `json:"until,omitempty"`
		Weeks  []weekView `json:"weeks"`
		Totals totalsView `json:"totals"`
	}

	weekView struct {
		Start string `json:"start"`
		End   string `json:"end"`
		// Current marks the week today falls in, the only one whose totals
		// are still moving.
		Current       bool      `json:"current"`
		WorkedMinutes int       `json:"worked_minutes"`
		BreakMinutes  int       `json:"break_minutes"`
		Days          []dayView `json:"days"`
	}

	dayView struct {
		Date  string `json:"date"`
		State string `json:"state"`
		// Worked and break minutes cover the whole day, sessions included.
		WorkedMinutes int           `json:"worked_minutes"`
		BreakMinutes  int           `json:"break_minutes"`
		Sessions      []sessionView `json:"sessions"`
	}

	sessionView struct {
		Start         string      `json:"start"`
		End           string      `json:"end,omitempty"`
		WorkedMinutes int         `json:"worked_minutes"`
		BreakMinutes  int         `json:"break_minutes"`
		Breaks        []breakView `json:"breaks"`
	}

	breakView struct {
		Start   string `json:"start"`
		End     string `json:"end,omitempty"`
		Minutes int    `json:"minutes"`
	}

	totalsView struct {
		Weeks         int `json:"weeks"`
		WorkingDays   int `json:"working_days"`
		WorkedMinutes int `json:"worked_minutes"`
		BreakMinutes  int `json:"break_minutes"`
	}
)

// newHistoryView turns the history into the structure the json and yaml output
// are written from.
func newHistoryView(weeks []worklog.WeekReport, since, until, now time.Time) historyView {
	days, worked, breaks := historyTotals(weeks)
	view := historyView{
		GeneratedAt: now.Format(time.RFC3339),
		Since:       isoDate(since),
		Until:       isoDate(until),
		Weeks:       make([]weekView, 0, len(weeks)),
		Totals: totalsView{
			Weeks:         len(weeks),
			WorkingDays:   days,
			WorkedMinutes: minutes(worked),
			BreakMinutes:  minutes(breaks),
		},
	}
	for _, report := range weeks {
		view.Weeks = append(view.Weeks, weekView{
			Start:         report.Week.Start,
			End:           report.Week.End,
			Current:       report.Week.Current(now),
			WorkedMinutes: minutes(report.Worked),
			BreakMinutes:  minutes(report.Breaks),
			Days:          newDayViews(report.Days, now),
		})
	}
	return view
}

func newDayViews(days []*worklog.Day, now time.Time) []dayView {
	views := make([]dayView, 0, len(days))
	for _, d := range days {
		view := dayView{
			Date:          d.Date,
			State:         d.State().String(),
			WorkedMinutes: minutes(d.Worked(now)),
			BreakMinutes:  minutes(d.BreakTime(now)),
			Sessions:      make([]sessionView, 0, len(d.Sessions)),
		}
		for _, s := range d.Sessions {
			session := sessionView{
				Start:         timestamp(s.Start),
				End:           optionalTimestamp(s.End),
				WorkedMinutes: minutes(s.Worked(now)),
				BreakMinutes:  minutes(s.BreakTime(now)),
				Breaks:        make([]breakView, 0, len(s.Breaks)),
			}
			for _, b := range s.Breaks {
				session.Breaks = append(session.Breaks, breakView{
					Start:   timestamp(b.Start),
					End:     optionalTimestamp(b.End),
					Minutes: minutes(b.Duration(now)),
				})
			}
			view.Sessions = append(view.Sessions, session)
		}
		views = append(views, view)
	}
	return views
}

func timestamp(t time.Time) string { return t.Format(time.RFC3339) }

// optionalTimestamp renders the end of an entry, which is empty - and so left
// out of the output - while the entry is still running.
func optionalTimestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return timestamp(*t)
}

// minutes truncates a duration to whole minutes, the granularity this tool
// works in.
func minutes(d time.Duration) int {
	if d < 0 {
		return 0
	}
	return int(d.Truncate(time.Minute) / time.Minute)
}

func writeJSON(out io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	_, err = out.Write(append(data, '\n'))
	return err
}

// writeYAML encodes the history view as YAML. It covers exactly the shapes
// that view is built from - structs, slices and the scalars inside them - and
// takes field names from the json tags, so the two formats cannot drift apart.
// Scalars are written in double quoted style, which keeps a date like
// 2026-09-07 a string rather than a timestamp.
func writeYAML(out io.Writer, v any) error {
	var b strings.Builder
	if err := yamlStruct(&b, reflect.ValueOf(v), 0); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func yamlStruct(b *strings.Builder, v reflect.Value, indent int) error {
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("cannot encode %s", v.Kind())
	}
	t := v.Type()
	for i := range t.NumField() {
		name, omitempty, ok := jsonTag(t.Field(i))
		if !ok {
			continue
		}
		f := v.Field(i)
		if omitempty && isEmptyValue(f) {
			continue
		}

		switch f.Kind() {
		case reflect.Slice:
			if f.Len() == 0 {
				fmt.Fprintf(b, "%s%s: []\n", pad(indent), name)
				continue
			}
			fmt.Fprintf(b, "%s%s:\n", pad(indent), name)
			for j := range f.Len() {
				if err := yamlItem(b, f.Index(j), indent+1); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			}
		case reflect.Struct:
			fmt.Fprintf(b, "%s%s:\n", pad(indent), name)
			if err := yamlStruct(b, f, indent+1); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		default:
			scalar, err := yamlScalar(f)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			fmt.Fprintf(b, "%s%s: %s\n", pad(indent), name, scalar)
		}
	}
	return nil
}

// yamlItem writes one element of a sequence. A struct element is rendered one
// level deeper and then has its leading indentation overwritten with the dash,
// which is what lines its first field up with the rest.
func yamlItem(b *strings.Builder, v reflect.Value, indent int) error {
	if v.Kind() != reflect.Struct {
		scalar, err := yamlScalar(v)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%s- %s\n", pad(indent), scalar)
		return nil
	}

	var item strings.Builder
	if err := yamlStruct(&item, v, indent+1); err != nil {
		return err
	}
	b.WriteString(pad(indent) + "- " + item.String()[len(pad(indent+1)):])
	return nil
}

func yamlScalar(v reflect.Value) (string, error) {
	switch v.Kind() {
	case reflect.String:
		return strconv.Quote(v.String()), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	default:
		return "", fmt.Errorf("cannot encode %s", v.Kind())
	}
}

// jsonTag returns the field name and omitempty flag a field is encoded with,
// and whether it is encoded at all.
func jsonTag(f reflect.StructField) (name string, omitempty, ok bool) {
	if !f.IsExported() {
		return "", false, false
	}
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false, false
	}
	name, opts, _ := strings.Cut(tag, ",")
	if name == "" {
		name = f.Name
	}
	return name, opts == "omitempty", true
}

// isEmptyValue mirrors what encoding/json leaves out for omitempty, so a field
// missing from one format is missing from the other too.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

func pad(indent int) string { return strings.Repeat("  ", indent) }
