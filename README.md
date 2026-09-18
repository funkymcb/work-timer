# work-timer

A small, dependency-free CLI to track working hours.

```console
$ work start
▶ Work started at 08:12. Have a good one.

$ work pause
⏸ Break started at 12:30.
  Worked today: 4h 18m

$ work resume
▶ Back to work at 13:12.
  Break lasted: 42m
  Worked today: 4h 18m

$ work stop
⏹ Work stopped at 17:30.
  Worked today: 8h 36m
  Breaks:       42m
  From 08:12 to 17:30 (9h 18m elapsed)
```

## Commands

| Command           | Meaning                                  |
| ----------------- | ---------------------------------------- |
| `work start`      | begin the working day                    |
| `work pause`      | begin a break                            |
| `work resume`     | end the break and continue working       |
| `work stop`       | end the working day                      |
| `work status`     | show what the timer is doing right now   |
| `work week`       | show the days and hours of this week     |
| `work history`    | list every record down to single entries |
| `work where`      | print the path of the work log directory |

`work start`, `pause`, `resume` and `stop` take `--at HH:MM` to record the
entry at a time that has already passed, e.g. `work resume --at 13:07`.
`work history` takes `--since DATE`, `--until DATE` and `--out text|json|yaml`.

`work status` takes `--format text` (default) or `--format tmux` for a compact
line.

Running `work start` again after a `work stop` opens a second session on the
same day, so an evening shift is added to the day's total. The gap between the
stop and the next start is not counted as a break.

## Recording a time you missed

`start`, `pause`, `resume` and `stop` all take `--at HH:MM`, for when you only
remember the timer once the moment has passed:

```console
$ work resume --at 13:07
▶ Back to work at 13:07.
  Break lasted: 37m
  Worked today: 5h 12m
```

The entry is written at 13:07, while the totals underneath it are still the
totals right now — so the worked time above already includes everything since
13:07.

Three rules keep the log honest:

- **`--at` only moves an entry backwards.** A clock time is read as the most
  recent one that has passed, so `--at 23:50` typed at 00:30 means last night,
  and typed at 15:00 it means yesterday (and is then almost certainly refused
  by the next rule).
- **Entries stay in order.** A time before the previous entry of the day is
  refused rather than recorded, because it would otherwise silently shorten a
  session or a break.
- **Only the day being recorded.** `--at` moves the time within that day; it
  cannot log hours onto an earlier date. For that, edit the JSON file.

Week boundaries are whole calendar days, so `work week` takes no `--at`.

## Work weeks

Weeks run Monday to Sunday and are not recorded anywhere: they are simply the
calendar weeks the day files fall in. There is no week to open, to close, or to
forget, and `work start` never refuses.

```console
$ work week
▶ Week Mon 14 - Sun 20 Sep · 3 of 5 weekdays

  Mon 14 Sep   8h 30m   breaks     0m
  Tue 15 Sep   8h 50m   breaks     0m
  Fri 18 Sep   8h 01m   breaks     0m   ▶ now

  Total       25h 21m   breaks     0m
  Average      8h 27m
```

The count is of the five weekdays. A Saturday or Sunday still counts into the
totals and is called out separately (`· 2 of 5 weekdays + 1 weekend day`),
because it would otherwise go unmentioned.

`work status` prints the running week total underneath the day:

```console
$ work status
▶ Working since 08:12 · 4h 18m worked · 0m on breaks
  This week: 25h 21m over 3 of 5 weekdays (Mon 14 - Sun 20 Sep)
```

### Days you forgot to log

Because the week is known without being declared, the days missing from it are
too. Starting a day names the weekdays since your last record that have nothing
on them:

```console
$ work start
▶ Work started at 08:05. Have a good one.
  Nothing logged on Wed 16 Sep and Thu 17 Sep.
```

It is a remark, not a refusal — nothing is blocked and nothing is invented. The
rule is one line: *weekdays between the last day on record and today*. So
coming back on a Tuesday names the Monday, coming back on a Monday names the
Friday before, and a Friday-to-Monday stretch names nothing, because a weekend
is not a gap. Past three days it counts them off instead of listing them
(`Nothing logged on the 6 weekdays since Fri 06 Mar.`), and a second session on
a day that already has one stays quiet.

Deliberate days off will be named too — the tool cannot tell a holiday from an
oversight, which is exactly why it remarks rather than decides.

## History

`work history` prints everything on record, oldest first: every week, every day
inside it, and every session and break inside those days.

```console
$ work history
⏹ Week Mon 07 - Sun 13 Sep · 5 of 5 weekdays

  Mon 07 Sep   8h 42m   breaks    30m
      ▶ 08:12 - 17:24     8h 42m
        ⏸ 12:30 - 13:00      30m
  Tue 08 Sep   8h 15m   breaks    30m
      ▶ 08:00 - 16:45     8h 15m
        ⏸ 12:00 - 12:30      30m
  ...

  Total       39h 47m   breaks 2h 15m
  Average      7h 57m

▶ Week Mon 14 - Sun 20 Sep · 2 of 5 weekdays

  Thu 17 Sep   8h 17m   breaks    40m
      ▶ 08:15 - 17:12     8h 17m
        ⏸ 12:00 - 12:40      40m
  Fri 18 Sep   5h 10m   breaks    54m   ⏸ on break
      ▶ 08:05 - 12:00     3h 40m
        ⏸ 09:30 - 09:45      15m
      ▶ 13:00 - open      1h 30m
        ⏸ 14:30 - open       39m

  Total       13h 27m   breaks 1h 34m
  Average      6h 43m

All time · 2 weeks · 7 working days · 53h 14m worked · 3h 49m on breaks
```

Each `▶` line is one session with its net worked time, and the `⏸` lines under
it are the breaks taken during that session. An entry still running reads
`- open`, and one that ended on a later date carries the offset
(`21:00 - 01:30+1d`), so a shift over midnight is never mistaken for a session
that ran backwards.

Every day belongs to exactly one calendar week, so nothing falls outside the
listing. The output is a plain list; pipe it through `less` once there is a lot
of it.

### Only part of it

`--since DATE` and `--until DATE` limit the history to a stretch of days, both
dates included. Either can stand on its own:

```console
$ work history --since 15.09.2026
▶ Week Mon 14 - Sun 20 Sep · 2 of 5 weekdays · from Tue 15 Sep on

  Thu 17 Sep   8h 17m   breaks    40m
      ▶ 08:15 - 17:12     8h 17m
        ⏸ 12:00 - 12:40      40m
  Fri 18 Sep   5h 10m   breaks    54m   ⏸ on break
      ▶ 08:05 - 12:00     3h 40m
        ⏸ 09:30 - 09:45      15m
      ▶ 13:00 - open      1h 30m
        ⏸ 14:30 - open       39m

  Total       13h 27m   breaks 1h 34m
  Average      6h 43m

Since Tue 15 Sep 2026 · 1 week · 2 working days · 13h 27m worked · 1h 34m on breaks
```

Three date orders are accepted, told apart by the separator:

| Written           | Read as                | Means      |
| ----------------- | ---------------------- | ---------- |
| `01.10.2024`      | day first, German      | 1 Oct 2024 |
| `01.10.24`, `1.10.2024` | same, short year or unpadded | 1 Oct 2024 |
| `10/01/2024`      | month first, US        | 1 Oct 2024 |
| `10/1/24`         | same, short year       | 1 Oct 2024 |
| `2024-10-01`, `2024/10/01` | year first, ISO | 1 Oct 2024 |

A week that reaches past either end of the range is reported with the days that
are left of it, and its header says which end was cut:

```
· from Tue 15 Sep on             the days before the range are missing
· up to Wed 16 Sep               the days after it are
· from Tue 15 Sep to Wed 16 Sep   both ends were cut
```

Its totals and day count then cover only the part being shown, not the whole
week. Weeks lying entirely outside the range are left out, and so are the
totals of their days: the closing line counts exactly what is above it, which
is why it names the range instead of reading *All time*.

```console
$ work history --since 10.09.2026 --until 15.09.2026 | tail -1
Thu 10 Sep 2026 - Tue 15 Sep 2026 · 2 weeks · 4 working days · 30h 50m worked · 1h 45m on breaks
```

### Machine readable output

`work history --out json` and `--out yaml` print the same history as data, for
a timesheet or a `jq` one-liner (`--out text` is the default):

```console
$ work history --out yaml
generated_at: "2026-09-18T15:21:05+02:00"
weeks:
  - start: "2026-09-07"
    end: "2026-09-13"
    current: false
    worked_minutes: 2387
    break_minutes: 135
    days:
      - date: "2026-09-07"
        state: "idle"
        worked_minutes: 522
        break_minutes: 30
        sessions:
          - start: "2026-09-07T08:12:00+02:00"
            end: "2026-09-07T17:24:00+02:00"
            worked_minutes: 522
            break_minutes: 30
            breaks:
              - start: "2026-09-07T12:30:00+02:00"
                end: "2026-09-07T13:00:00+02:00"
                minutes: 30
      # ... the rest of the days, and the week after this one
totals:
  weeks: 2
  working_days: 7
  worked_minutes: 3194
  break_minutes: 286
```

Both formats carry exactly the same fields:

- **Durations are whole minutes**, truncated the same way the text output is,
  so all three outputs report the same numbers.
- **Timestamps are RFC 3339**, as they are in the day files. An entry that is
  still running simply has no `end`, and its minutes are measured against
  `generated_at`.
- **Lists are always present**, empty rather than missing, so
  `.weeks[].days[].sessions[]` never trips over a null.
- **`--since` and `--until` show up as `since` and `until` fields**, each left
  out when that end is open, so a filtered export says on its face that it is
  filtered.
- **`current` marks the week today falls in**, the only one whose totals are
  still moving.

```console
$ work history --out json | jq '[.weeks[] | select(.current | not) | .worked_minutes] | add / 60'
39.78
```

## Install

```sh
go install github.com/funkymcb/work-timer/cmd/work@latest
```

Or from a checkout: `make install` (installs into `$(go env GOPATH)/bin`).

## Storage

One JSON file per calendar day, written atomically with `0600` permissions:

```
2026-09-15.json   a day, with its sessions and breaks
```

Day files are the only record there is; week totals are derived from the dates
of the days themselves. `week-*.json` files written by earlier versions are
ignored and can be deleted.

The directory follows `$WORK_TIMER_DIR`, then `$XDG_DATA_HOME/work-timer`, and
otherwise falls back to the default in `DefaultDir` (`internal/worklog/store.go`).
Run `work where` to see which one is in use. The files are plain JSON and safe
to edit by hand if you forget to stop the timer.

A session that crosses midnight keeps counting: if today has no entries yet and
yesterday's session is still open, the commands keep operating on yesterday.

## tmux status bar

`work status --format tmux` prints a single short line:

```
▶ 4h18m       working, 4h18m worked so far
⏸ 12m         on a break that started 12 minutes ago
⏹ 8h36m       done for the day
⏹ no record   the day has not been started
```

The symbols are `iconPlay`/`iconPause`/`iconStop` in `cmd/work/main.go`; swap
them for Nerd Font glyphs (`󰐊 󰏤 󰓛`) if you prefer.

Add it to the right status bar in `~/.tmux.conf`:

```tmux
set -g status-interval 60
set -g status-right "#(work status -f tmux) %H:%M "
```

The tmux server does not always inherit your login `PATH`, so use the absolute
path (`#(/Users/you/go/bin/work status -f tmux)`) if the segment stays empty.

## Development

```sh
go test ./...
```
