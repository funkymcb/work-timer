# work-timer
CLI-Tool to track working hours and directly sync them to common Boards (Jira etc.)

# Overview
This CLI-Tool is designed to help tracking working hours in agile Workflows.

## Commands
- Start working on a ticket:
    - `work start <Ticket-Name>`
- Pause working on a ticket
    - `work pause <Ticket-Name>`
- Resume working on a ticket
    - `work resume <Ticket-Name>`
- End working on a ticket
    - `work end <Ticket-Name>`
- Close a ticket
    - `work close <Ticket-Name>`
- Show Current Status of work
    - `work status`
- Show history
    - `work history (--from (20d, dates, timestamps) --to x)???`
- Show all available tickets
    - `work list --mine --sprint=current --project???`
- Create Alias for work-items
    - `work alias daily <Ticket-Name>`

# ToDo
- [ ] implement cli handling (cobra)
- [ ] implement jira-api connection
- [ ] evaluate (logfiles or else) for storing ticket data

# Roadmap
- [ ] work-item status for p10k and tmux
