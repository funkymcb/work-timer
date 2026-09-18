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
| `work week start` | open a work week                         |
| `work week`       | show the days and hours of the open week |
| `work week end`   | close the week and print its total       |
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

Week boundaries are whole calendar days, so `work week start` and
`work week end` take no `--at`.

## Work weeks

Weeks are opened and closed by hand, because not every week runs Monday to
Friday:

```console
$ work week start
▶ Work week opened on Mon 14 Sep.
  Room for 5 working days. Run `work start` to begin the day.

$ work week
▶ Work week since Mon 14 Sep · 3 of 5 working days used

  Mon 14 Sep    8h 42m   breaks    30m
  Tue 15 Sep    8h 15m   breaks    30m
  Wed 16 Sep    3h 05m   breaks     0m   ▶ now

  Total        20h 02m   breaks  1h 00m
  Average       6h 40m

$ work week end
⏹ Work week Mon 14 Sep - Fri 18 Sep · 5 working days
  ...
```

Two rules are enforced:

- **`work start` needs an open week.** Without one it refuses, so no hours can
  end up outside a week.
- **A week holds at most 5 working days.** Starting work on a sixth *calendar
  day* is refused; a second session on a day that already counts is always
  fine, so `stop` and `start` again in the evening still works.

A week ends on its **last working day**, not on the day you happen to run
`work week end`. Closing a Mon–Fri week on Sunday evening still records it as
Mon–Fri, which leaves the following days free for the next week. The only time
a new week cannot be opened is when today's hours already belong to the week
you just closed — then the next week starts tomorrow.

`work status` also prints the running week total underneath the day:

```console
$ work status
▶ Working since 08:12 · 4h 18m worked · 0m on breaks
  This week: 20h 02m over 3 of 5 days (since Mon 14 Sep)
```

## History

`work history` prints everything on record, oldest first: every week, every day
inside it, and every session and break inside those days.

```console
$ work history
⏹ Work week Mon 07 Sep - Fri 11 Sep · 5 working days

  Mon 07 Sep   8h 42m   breaks    30m
      ▶ 08:12 - 17:24     8h 42m
        ⏸ 12:30 - 13:00      30m
  Tue 08 Sep   8h 15m   breaks    30m
      ▶ 08:00 - 16:45     8h 15m
        ⏸ 12:00 - 12:30      30m
  ...

  Total       39h 47m   breaks 2h 15m
  Average      7h 57m

▶ Work week since Thu 17 Sep · 2 of 5 working days used

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

Days that no week covers — worked before weeks were kept, or left over from
editing the files by hand — are listed at the end under *Outside any work
week*, so the history stays a complete account of what is on disk. The output
is a plain list; pipe it through `less` once there is a lot of it.

### Only part of it

`--since DATE` and `--until DATE` limit the history to a stretch of days, both
dates included. Either can stand on its own:

```console
$ work history --since 15.09.2026
⏹ Work week Mon 14 Sep - Wed 16 Sep · 2 working days · from Tue 15 Sep on

  Tue 15 Sep   7h 55m   breaks    30m
      ▶ 07:55 - 16:20     7h 55m
        ⏸ 11:50 - 12:20      30m
  Wed 16 Sep   7h 40m   breaks    45m
      ▶ 08:40 - 17:05     7h 40m
        ⏸ 12:30 - 13:15      45m

  Total       15h 35m   breaks 1h 15m
  Average      7h 47m

Since Tue 15 Sep 2026 · 1 week · 2 working days · 15h 35m worked · 1h 15m on breaks
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
    end: "2026-09-11"
    active: false
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
      # ... the rest of the days, and the two weeks after this one
days_outside_weeks: []
totals:
  weeks: 3
  working_days: 11
  worked_minutes: 4764
  break_minutes: 359
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

```console
$ work history --out json | jq '[.weeks[] | select(.active | not) | .worked_minutes] | add / 60'
63.78
```

## Install

```sh
go install github.com/funkymcb/work-timer/cmd/work@latest
```

Or from a checkout: `make install` (installs into `$(go env GOPATH)/bin`).

## Storage

One JSON file per calendar day plus one per work week, written atomically with
`0600` permissions:

```
2026-09-15.json        a day, with its sessions and breaks
week-2026-09-14.json   a week, just the start and end dates
```

Day files are the only record of hours; a week file merely marks a range, so
week totals are always derived from the days inside it.

The directory follows `$WORK_TIMER_DIR`, then `$XDG_DATA_HOME/work-timer`, and
otherwise falls back to the default in `DefaultDir` (`internal/worklog/store.go`).
Run `work where` to see which one is in use. The files are plain JSON and safe
to edit by hand if you forget to stop the timer.

A session that crosses midnight keeps counting: if today has no entries yet and
yesterday's session is still open, the commands keep operating on yesterday.

## tmux status bar

`work status --format tmux` prints a single short line, and nothing at all when
the day has not been started:

```
▶ 4h18m     working, 4h18m worked so far
⏸ 12m       on a break that started 12 minutes ago
⏹ 8h36m     done for the day
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
