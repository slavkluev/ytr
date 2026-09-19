---
name: ytr
description: >-
  Yandex Tracker CLI for managing issues, comments, worklogs, and more.
  Use when the user needs to interact with Yandex Tracker: search/filter
  issues, create or update tasks, manage worklogs, bulk operations, etc.
license: MIT
compatibility: Requires ytr binary in PATH
metadata:
  author: slavkluev
  version: "6.0"
---

# ytr -- Yandex Tracker CLI

## Auth

```bash
# Environment variables (recommended for agents)
export YTR_TOKEN="your-oauth-token"
export YTR_ORG_ID="your-org-id"
export YTR_ORG_TYPE="360"   # or "cloud"

# Or interactive login (humans)
ytr auth login

# Or explicit login flags
ytr auth login --token TOKEN --org-id ORG --org-type 360

# Verify authentication
ytr auth status
```

Authentication precedence:

1. Flags: `--token`, `--org-id`, `--org-type`
2. Environment variables: `YTR_TOKEN`, `YTR_ORG_ID`, `YTR_ORG_TYPE`
3. Config file saved by `ytr auth login`

Notes:

- For regular commands, flag-based and env-based auth require all three values together.
- `ytr auth login` is the exception: it can detect the organization type when `--org-type` is omitted.
- Config is stored in `~/.config/ytr/config.yaml`.

## Command Reference

### Issue Tracking

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `ytr issue create` | Create an issue | `--queue`, `--summary`, `--description`, `--from-json` |
| `ytr issue list` | List issues | `--query`, `--filter`, `--order-by`, `--order-asc`, `--limit`, `--all`, `--cursor` |
| `ytr issue view ISSUE-KEY` | View issue details | |
| `ytr issue update ISSUE-KEY` | Update an issue | `--summary`, `--description`, `--type`, `--priority`, `--assignee`, `--parent`, `--from-json` |
| `ytr issue transition ISSUE-KEY` | Transition issue status | `--to` |
| `ytr issue changelog ISSUE-KEY` | Show issue change history | `--field`, `--type`, `--limit`, `--cursor`, `--all` |
| `ytr comment create ISSUE-KEY` | Add comment to issue | `--body` |
| `ytr comment edit ISSUE-KEY COMMENT-ID` | Edit a comment | `--body`, `--from-json` |
| `ytr comment delete ISSUE-KEY COMMENT-ID` | Delete a comment | |
| `ytr comment list ISSUE-KEY` | List comments on an issue | |
| `ytr link create ISSUE-KEY` | Create a link to another issue | `--type`, `--issue`, `--from-json` |
| `ytr link delete ISSUE-KEY LINK-ID` | Delete a link | |
| `ytr link list ISSUE-KEY` | List links on an issue | |
| `ytr worklog create ISSUE-KEY` | Create a worklog | `--duration`, `--start`, `--comment`, `--from-json` |
| `ytr worklog edit ISSUE-KEY WORKLOG-ID` | Edit a worklog | `--duration`, `--start`, `--comment`, `--from-json` |
| `ytr worklog delete ISSUE-KEY WORKLOG-ID` | Delete a worklog | |
| `ytr worklog list ISSUE-KEY` | List worklogs on an issue | |
| `ytr checklist create ISSUE-KEY` | Add checklist item to issue | `--text`, `--assignee`, `--from-json` |
| `ytr checklist edit ISSUE-KEY ITEM-ID` | Edit a checklist item | `--text`, `--checked`, `--assignee`, `--from-json` |
| `ytr checklist delete ISSUE-KEY ITEM-ID` | Delete a checklist item | |
| `ytr checklist list ISSUE-KEY` | List checklist items on an issue | |
| `ytr bulk move [ISSUE-KEY...]` | Move issues to another queue | `--queue`, `--field`, `--from-json`, `--timeout` |
| `ytr bulk update [ISSUE-KEY...]` | Update fields on multiple issues | `--field`, `--from-json`, `--timeout` |
| `ytr bulk transition [ISSUE-KEY...]` | Transition multiple issues | `--transition`, `--field`, `--from-json`, `--timeout` |
| `ytr bulk status OPERATION-ID` | Show bulk operation status | |

### Reference Data

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `ytr status list` | List workflow statuses | |
| `ytr priority list` | List priorities | |
| `ytr resolution list` | List resolutions | |
| `ytr issuetype list` | List issue types | |
| `ytr field list` | List available fields | `--queue` |
| `ytr field get FIELD-KEY` | Show field details | `--queue` |

### Organization

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `ytr queue list` | List queues | `--limit`, `--all`, `--cursor` |
| `ytr queue view QUEUE-KEY` | View queue details | |
| `ytr queue context QUEUE-KEY` | Everything needed to create and move issues in a queue, as one JSON document | `--json` selects parts |
| `ytr component create` | Create a component | `--name`, `--queue`, `--description`, `--lead`, `--assign-auto`, `--from-json` |
| `ytr component edit COMPONENT-ID` | Edit a component | `--name`, `--queue`, `--description`, `--lead`, `--assign-auto`, `--from-json` |
| `ytr component delete COMPONENT-ID` | Delete a component | |
| `ytr component get COMPONENT-ID` | Show component details | |
| `ytr component list` | List components | |

### Account

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `ytr user myself` | Show current user | |
| `ytr user get UID` | Show user details | |
| `ytr user list` | List organization users | `--limit`, `--all`, `--cursor` |
| `ytr auth login` | Authenticate with Yandex Tracker | `--token`, `--org-id`, `--org-type` |
| `ytr auth logout` | Remove stored credentials | |
| `ytr auth status` | Show authentication status | |

### System

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `ytr version` | Show ytr version information | `--json`, `--jq` |
| `ytr completion bash` | Generate bash completion script | |
| `ytr completion zsh` | Generate zsh completion script | |
| `ytr completion fish` | Generate fish completion script | |

## Workflow Examples

### Issue Search

Two modes (mutually exclusive):
- `--filter key=value` — structured filters, repeatable, best for programmatic use
- `--query '...'` — Tracker query language, for complex boolean/date logic

**Query language reference:** See [query-language.md](query-language.md) for fields, operators, functions, and sort syntax.

Common `--filter` keys (camelCase, from API JSON field names):
`queue`, `status`, `assignee`, `priority`, `type`, `createdBy`,
`createdAt`, `updatedAt`, `resolvedAt`, `deadline`, `components`,
`tags`, `sprint`, `storyPoints`, `parent`, `summary`

Note: `--filter` uses camelCase API field names (`createdBy`, `createdAt`), while `--query` uses Title Case display names (`Author`, `Created`). See [query-language.md](query-language.md) for query syntax.

Special value: `me()` for current user (e.g., `--filter assignee=me()`).
Discover all fields: `ytr field list --queue QUEUE --json id,key,name,schema,options`.
`id` is the full field id (`<queueId>--<key>` for local fields); `options` lists the
allowed values for enum fields in the JSON type Tracker sent, so numeric options
stay numbers (`possibleSpam` has `options: [0, 1]`). When Tracker sets the allowed
values per queue, `options` is omitted: `queueOptions` maps each queue key to its
list, and `defaultOptions` holds Tracker's `defaults` list. `field get` has the
same three fields.
A local field is filtered by its full id, `--filter <queueId>--size=L`; the short
`--filter size=L` gets a bare 400. `ytr queue context QUEUE --json localFields`
lists a queue's local fields with their full ids and allowed values.

```bash
# Search with Tracker query language (complex boolean/date queries)
ytr issue list --query 'Queue: PROJ AND Status: open "Sort By": Updated DESC'

# Filter by key=value fields (repeatable)
ytr issue list --filter queue=PROJ --filter priority=critical

# Multiple filters
ytr issue list --filter queue=PROJ --filter status=open --filter assignee=me()

# Sort by field (descending by default)
ytr issue list --filter queue=PROJ --order-by updatedAt

# Sort ascending
ytr issue list --filter queue=PROJ --order-by createdAt --order-asc
```

### Issue Lifecycle

```bash
# Create an issue and print its key
ytr issue create --queue PROJ --summary "Implement login" --json key --jq '.key'

# List issues in a queue
ytr issue list --filter queue=PROJ --json key,summary,status

# Transition to in-progress
ytr issue transition PROJ-123 --to "In Progress"

# Check how the issue moved through statuses
ytr issue changelog PROJ-123 --field status --json date,type,fields

# Add a comment
ytr comment create PROJ-123 --body "Started implementation"

# Log time spent
ytr worklog create PROJ-123 --duration PT2H --start 2026-03-30T10:00:00Z --comment "Backend work"

# Add a checklist item
ytr checklist create PROJ-123 --text "Write unit tests"
```

### Bulk Operations

```bash
# Move multiple issues to another queue
ytr bulk move PROJ-1 PROJ-2 PROJ-3 --queue TARGET

# Bulk update via stdin pipe
echo "PROJ-1\nPROJ-2" | ytr bulk update --field priority=critical

# Bulk transition with timeout
ytr bulk transition PROJ-1 PROJ-2 --transition close --timeout 10m

# Check bulk operation status
ytr bulk status 6543210abcdef
```

### Issue History

```bash
# Show all changes for an issue
ytr issue changelog PROJ-123

# Filter to only status transitions
ytr issue changelog PROJ-123 --field status

# Filter by change type (e.g., workflow transitions only)
ytr issue changelog PROJ-123 --type IssueWorkflow

# JSON output with specific fields (per-entry structure)
ytr issue changelog PROJ-123 --json date,author,type,fields,comments,links

# Extract status transitions using jq
ytr issue changelog PROJ-123 --json date,type,fields --jq '.items[] | select(.type=="IssueWorkflow")'

# Fetch all pages automatically
ytr issue changelog PROJ-123 --all
```

### Sub-resources

```bash
# Create a link between issues
ytr link create PROJ-123 --type "relates" --issue PROJ-456

# List links on an issue
ytr link list PROJ-123 --json id,type,issue

# Add and manage checklist items
ytr checklist create PROJ-123 --text "Review PR"
ytr checklist edit PROJ-123 42 --checked
```

### Discovery

Before creating or moving issues in a queue, run `ytr queue context QUEUE` once.
It prints one JSON document whose top-level keys are its parts:

- `key`, `name`: the queue.
- `defaultType`, `defaultPriority`: keys, the form `issue create --type` and `--priority` take.
- `issueTypes`: `key`, `name`, and the `workflow` id each type follows.
- `statuses`: every status of the queue's workflows, once, as `key` and `name`.
- `workflows`: `id`, `initialStatus`, and `transitions`, which maps each status key
  to the status keys an issue can move to from it (`[]` for a status with no outgoing transitions).
  `issue transition --to` takes such a key.
- `components`: `id` and `name`.
- `requiredFields`: `summary`, then every field the queue marks required. On `type`
  and `priority`, `default` is the queue's default, which fills the field when the
  issue does not set it. Tracker's list of queue fields is often empty; then
  `incomplete` says so, because other fields may still be required.
- `localFields`: `id` is the full `<queueId>--<key>` that `--filter` needs;
  `options` are the allowed values in the JSON type Tracker sent.
- `globalFields`: the editable global fields, `key` and `name` only. Run
  `ytr field get KEY` for a field's schema and allowed values.
- `incomplete`: `{"part", "reason"}` for each part that is missing or may be missing entries.

`key`, `name`, the defaults and `issueTypes` come from the queue request; `statuses`
and `workflows` share the workflow requests; each other part has a request of its own.
A part whose request failed is `null`, and `incomplete` names it with the server's
error text; a part that was fetched but is empty is `[]`. The command exits 0 once the
queue itself is found, so check `incomplete` before relying on a part. An unknown
queue exits 4. `--json a,b` returns only those parts and makes only the requests they
need; `incomplete` is always included. The document is always JSON, and `--quiet` is
an error. As for every command, an error is a JSON document on stderr only under
`--json` or `--jq`.

```bash
# Everything needed to work in a queue
ytr queue context PROJ

# Only issue types and their workflows
ytr queue context PROJ --json issueTypes,workflows

# Statuses an issue in "open" can move to
ytr queue context PROJ --json workflows --jq '.workflows[].transitions.open'

# Full local field ids and allowed values, for --filter
ytr queue context PROJ --json localFields --jq '.localFields[] | {id, options}'

# List available fields for a queue
ytr field list --queue PROJ --json id,key,name,schema,options

# Look up workflow statuses
ytr status list --json key,name

# List issue types
ytr issuetype list --json key,name

# Get field details
ytr field get assignee --json id,key,name,schema,options
```

### JSON Pipeline

```bash
# Paginated list commands return {"items":[...],"pagination":{...}}
ytr issue list --filter queue=PROJ --json key,summary,status
ytr issue list --filter queue=PROJ --jq '.items[].key'

# Changelog returns per-entry structured items: {date, author, type, fields, comments, links, ...}
ytr issue changelog PROJ-123 --json date,author,type,fields,comments,links
ytr issue changelog PROJ-123 --json date,type,fields --jq '.items[] | select(.type=="IssueWorkflow")'

# Non-paginated sub-resource lists return arrays
ytr comment list PROJ-123 --json body --jq '.[].body'

# Quiet mode: minimal text, one item per line
ytr issue list --filter queue=PROJ --quiet

# Quiet mode for changelog outputs "category: from -> to" per line
# Categories: field names (status, assignee), comment, link, attachment, worklog, reaction, relatedResolution
ytr issue changelog PROJ-123 --quiet --field status
```

## Output Shape

ytr checks whether stdout is a terminal and prints accordingly. There is no
flag or environment variable for this: an agent that pipes, captures or
redirects ytr always gets the lean shape below.

Off a terminal:

- Tables print one header row and one line per record, columns joined by a
  single tab. Nothing is padded, truncated or given an ellipsis, so a long
  summary or comment body arrives whole.
- Any value's tab, newline and carriage return are escaped as `\t`, `\n` and
  `\r`, and a literal backslash as `\\`, so one record is always one line and
  the escaping can be reversed.
- Detail views (`issue view`, `queue view`, `field get`, `component get`,
  `user myself`, `user get`, and the `issue create`/`issue update` result)
  print `Label<TAB>value` with no trailing colon. A description still follows
  as its own block after a blank line.
- Times in text output are RFC 3339 with the offset the server sent, the same
  string the JSON fields carry: `2026-09-19T14:22:31+03:00`. `issue changelog`
  already printed RFC 3339 and reads the same in both modes.
- `--json` prints one line of JSON, and so does `queue context`, which prints
  its document without being asked for JSON.
- There is no color.

`--jq` is unchanged: its output was always compact, and a filter that yields
several results still prints one line per result in both modes.

On a terminal the output is for human eyes: padded columns fitted to the
terminal width, relative times such as `3h ago` (except `issue changelog`),
color, and indented `--json`.

`NO_COLOR`, `CLICOLOR_FORCE` and `CLICOLOR` keep their usual meaning, so
`CLICOLOR_FORCE=1` colors the tab-separated output too.

```bash
# One line per issue, columns split on a tab
ytr issue list --filter queue=PROJ | cut -f1,4

# One line of JSON
ytr issue view PROJ-123 --json key,summary
```

## JSON Output

Most resource commands support `--json field1,field2` field selection.
Run `ytr <command> --help` and look for the `JSON FIELDS` section to see
the available field names for that command.

Every user-valued field ships with a paired `*Id` holding the Tracker user ID:
`author`/`authorId`, `assignee`/`assigneeId`, `lead`/`leadId`,
`createdBy`/`createdById`. Display names are not unique — two people can share
one — so use the `*Id` field whenever identity matters, and feed it to
`ytr user get` to resolve the person. The `*Id` key is always present, holding
an empty string when the resource has no such user.

```bash
# Tell apart two commenters who share a display name
ytr comment list PROJ-123 --json author,authorId

# Resolve a comment author to a full user record
ytr user get "$(ytr comment list PROJ-123 --json authorId --jq '.[0].authorId')"
```

Exceptions:

- Delete commands return fixed confirmation objects such as `{ "id": "...", "deleted": true }`.
- `queue context` prints its document as JSON even without `--json`: its top-level keys
  are its parts. `--json` selects parts, `incomplete` is always present, and `--quiet`
  is an error. Its errors follow the usual rule: a JSON document on stderr only under
  `--json` or `--jq`.
- Paginated list commands such as `issue list`, `issue changelog`, `queue list`, and `user list` return an object with `items` and `pagination`.
- Non-paginated sub-resource list commands such as `comment list`, `link list`, `worklog list`, and `checklist list` return arrays.

```bash
# Filter paginated issue-list output
ytr issue list --filter queue=PROJ --json key --jq '.items[].key'

# Filter sub-resource array output
ytr comment list PROJ-123 --json body --jq '.[].body'

# Quiet mode: minimal text, one item per line
ytr issue list --filter queue=PROJ --quiet
```

## Error Recovery

| Exit Code | Meaning | Recovery |
|-----------|---------|----------|
| 0 | Success | -- |
| 1 | User error | Read `message` and `suggestion` fields in JSON error |
| 3 | Auth error | Set `YTR_TOKEN`, `YTR_ORG_ID`, and `YTR_ORG_TYPE`, or run `ytr auth login` |
| 4 | Not found | Verify the resource key exists |
| 5 | Rate limited | Wait and retry |
| 130 | Interrupted | Re-run the command |

`--from-json` fails with exit code 1 when the input carries a key the request
body has no field for. The JSON error lists every offending key in
`invalidFields` and the accepted ones in `validFields`. Local queue fields are
among the rejected keys: the API supports them, `--from-json` does not yet.

## Flags Reference

| Flag | Scope | Description |
|------|-------|-------------|
| `--json f1,f2` | Global on most commands | Output JSON with selected fields |
| `--jq expr` | Global | Filter JSON output with a jq expression; implies JSON |
| `--quiet` | Global | Output minimal text, one item per line; mutually exclusive with `--json` and `--jq` |
| `--debug` | Global | Emit sanitized debug diagnostics to stderr |
| `--token` | Global auth override | Override auth token |
| `--org-id` | Global auth override | Override organization ID |
| `--org-type` | Global auth override | Override organization type: `360` or `cloud` |
| `--from-json` | Selected create/edit/bulk commands | Raw JSON input (inline, `@file`, or `-` for stdin); keys the request body has no field for are rejected, not dropped |
| `--query` | `issue list` | Search using Tracker query language; mutually exclusive with `--filter` and `--order-by` |
| `--filter k=v` | `issue list` | Filter by field (repeatable); mutually exclusive with `--query` |
| `--order-by` | `issue list` | Sort by field (descending by default); cannot be used with `--query` |
| `--order-asc` | `issue list` | Sort ascending; requires `--order-by` |
| `--field` | `issue changelog` | Filter changes by field name (case-sensitive) |
| `--type` | `issue changelog` | Filter by change type (e.g., IssueWorkflow, IssueCommentAdded) |
| `--limit N` | Paginated list commands | Results per page (default 50, max 1000) |
| `--all` | Paginated list commands | Fetch all pages automatically |
| `--cursor` | Paginated list commands | Pagination cursor (pass the `pagination.cursor` value from the previous response) |
| `--timeout` | Bulk commands | Max wait time (default 5m) |
