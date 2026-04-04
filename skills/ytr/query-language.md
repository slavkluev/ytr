# Yandex Tracker Query Language Reference

Use with `ytr issue list --query '...'`.

Note: Query language uses Title Case field names (`Author`, `Created`). These differ from `--filter` keys which use camelCase API names (`createdBy`, `createdAt`).

## Fields

| Field | Example |
|-------|---------|
| Queue | `Queue: PROJ` |
| Status | `Status: open` |
| Assignee | `Assignee: login@` |
| Author | `Author: login@` |
| Priority | `Priority: critical` |
| Type | `Type: bug` |
| Summary | `Summary: "login page"` |
| Description | `Description: "error handling"` |
| Created | `Created: >2024-01-01` |
| Updated | `Updated: >2024-06-01` |
| Resolved | `Resolved: >2024-01-01` |
| Deadline | `Deadline: <2024-12-31` |
| Components | `Components: backend` |
| Tags | `Tags: urgent` |
| Sprint | `Sprint: "Sprint 5"` |
| Story Points | `Story Points: >3` |
| Epic | `Epic: PROJ-100` |
| Resolution | `Resolution: unresolved()` |

Queue-local fields use `QUEUE.fieldName` syntax: `PROJ."Custom Field": value`.

## User Values

- Username (most reliable): `login@`
- Full name (quoted): `"Ivan Ivanov"`
- Current user: `me()`

## Operators

| Operator | Meaning | Example |
|----------|---------|---------|
| `AND` | Both conditions (can be omitted) | `Queue: PROJ AND Status: open` |
| `OR` | Either condition | `Priority: critical OR Priority: blocker` |
| `()` | Grouping | `Queue: PROJ AND (Status: open OR Status: inProgress)` |
| `!` | Negation / exclude text | `!Assignee: empty()` |
| `#` | Exact match | `Summary: #"Version 2.0"` |
| `~` | Does not exactly match (Summary only) | `Summary: ~"old title"` |
| `>` `<` `>=` `<=` | Comparison | `Created: >2024-01-01` |
| `..` | Range | `Story Points: 1..5` |

Note: `AND` can be omitted — space between conditions implies `AND`.

## Functions

| Function | Meaning | Example |
|----------|---------|---------|
| `me()` | Current user | `Assignee: me()` |
| `empty()` | Field is empty | `Assignee: empty()` |
| `notEmpty()` | Field has value | `Deadline: notEmpty()` |
| `now()` | Current datetime | `Deadline: <now()` |
| `today()` | Current date | `Created: >=today()` |
| `week()` | Current week | `Created: >=week()` |
| `month()` | Current month | `Created: >=month()` |
| `quarter()` | Current quarter | `Created: >=quarter()` |
| `year()` | Current year | `Created: >=year()` |
| `unresolved()` | Open issues | `Resolution: unresolved()` |
| `group()` | User group | `Assignee: group(value: "Dev Team")` |
| `changed()` | Field change history | `Status: changed(to: "In Progress" date: 01.09.2024 .. 30.09.2024)` |

Time arithmetic with `now()`: `Created: >now()-12h`, `Updated: >now()-2d`.

## Sorting

Append `"Sort By":` at the end of the query:

```
Queue: PROJ AND Status: open "Sort By": Updated DESC
Queue: PROJ "Sort By": Priority ASC, Created DESC
```

## Date Formats

- `YYYY-MM-DD` (recommended)
- `DD.MM.YYYY`
- `DD-MM-YYYY`
- `MM/DD/YYYY`
- With time: `"YYYY-MM-DD XXh:XXm:XXs"`
- Intervals: `"2M 1w 3d 5h 32m"` (M=months, w=weeks, d=days, h=hours, m=minutes, s=seconds)
