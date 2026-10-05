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
  version: "28.0"
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

# Only who is signed in, and where
ytr auth status --json status,user,org_id
```

Authentication precedence:

1. Flags: `--token`, `--org-id`, `--org-type`
2. Environment variables: `YTR_TOKEN`, `YTR_ORG_ID`, `YTR_ORG_TYPE`
3. Config file saved by `ytr auth login`

Notes:

- For regular commands, flag-based and env-based auth require all three values together.
- `ytr auth login` is the exception: it can detect the organization type when `--org-type` is omitted.
- `auth status`, `auth login` and `auth logout` take `--json` fields and `--jq` like any other
  command. `--json=`, which names no field, and an unknown field both exit 1 before any request
  or config change; the unknown-field error lists the command's fields in `validFields`.
- Config is stored in `~/.config/ytr/config.yaml`.

## Command Reference

### Issue Tracking

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `ytr issue create` | Create an issue | `--queue`, `--summary`, `--description`, `--type`, `--priority`, `--assignee`, `--parent`, `--from-json` |
| `ytr issue list` | List issues | `--query`, `--filter`, `--order-by`, `--order-asc`, `--limit`, `--all`, `--cursor` |
| `ytr issue view ISSUE-KEY` | View issue details | |
| `ytr issue update ISSUE-KEY` | Update an issue | `--summary`, `--description`, `--type`, `--priority`, `--assignee`, `--parent`, `--from-json` |
| `ytr issue transition ISSUE-KEY` | Transition issue status | `--to`, `--from-json` |
| `ytr issue changelog ISSUE-KEY` | Show issue change history | `--field`, `--type`, `--limit`, `--cursor`, `--all` |
| `ytr comment create ISSUE-KEY` | Add comment to issue | `--body`, `--from-json` |
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
| `ytr help [command]` | Show help for any command | |

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

Special value: `me()` for current user (e.g., `--filter 'assignee=me()'`).
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
ytr issue list --filter queue=PROJ --filter status=open --filter 'assignee=me()'

# Sort by field (descending by default)
ytr issue list --filter queue=PROJ --order-by updatedAt

# Sort ascending
ytr issue list --filter queue=PROJ --order-by createdAt --order-asc
```

### Issue Lifecycle

```bash
# Create an issue and print its key
ytr issue create --from-json '{"queue":"PROJ","summary":"Implement login"}' --json key --jq '.key'

# List issues in a queue
ytr issue list --filter queue=PROJ --json key,summary,status

# Transition to in-progress
ytr issue transition PROJ-123 --from-json '{"to":"inProgress"}'

# Check how the issue moved through statuses
ytr issue changelog PROJ-123 --field status --json date,type,fields

# Add a comment
ytr comment create PROJ-123 --from-json '{"text":"Started implementation"}'

# Add a comment whose body is in a file
ytr comment create PROJ-123 --from-json @comment.json

# Log time spent
ytr worklog create PROJ-123 --from-json '{"start":"2026-03-30T10:00:00Z","duration":"PT2H","comment":"Backend work"}'

# Add a checklist item
ytr checklist create PROJ-123 --from-json '{"text":"Write unit tests"}'
```

### Bulk Operations

```bash
# Move multiple issues to another queue
ytr bulk move --from-json '{"queue":"TARGET","issues":["PROJ-1","PROJ-2","PROJ-3"]}'

# Bulk update with a body built from a search and piped in
ytr issue list --filter queue=PROJ --all --jq '{issues:[.items[].key],values:{priority:"critical"}}' | ytr bulk update --from-json -

# Bulk transition with a shorter wait; keep --timeout well under your tool's own timeout
ytr bulk transition --from-json '{"transition":"close","issues":["PROJ-1","PROJ-2"]}' --timeout 30s

# Check bulk operation status
ytr bulk status 6543210abcdef
```

`bulk move`, `bulk update` and `bulk transition` take each issue as a key such
as `PROJ-1` or a 24-character hexadecimal issue ID such as
`4ff3e8dae4b0e2ac00000001`, as arguments, one per line on stdin, or in the
`"issues"` array of `--from-json`. Anything else exits 1 with
`invalid issue key or ID "bad"` and sends nothing, as does a `--from-json`
body whose `"issues"` is missing or empty (`no issue keys provided`). Keys
given as arguments or on stdin, and `--field` values, are checked before auth
and before `--json=` is refused. Under `--from-json` the body's `"issues"`
is the only source of keys: key arguments beside it exit 1 with
`cannot combine --from-json with issue keys`, and stdin is not read for keys
(with `--from-json -` it carries the body itself).

`bulk move`, `bulk update` and `bulk transition` wait up to `--timeout`
(default 1m) for the operation they start. Exit 0 means Tracker accepted the
change; the document's `status` says whether it has finished:

- `COMPLETED`: the change is done, and `suggestion` is empty.
- Any other status, such as `CREATED`: the wait ended before Tracker reported
  the operation finished. Do not run the command again, which would start a
  second operation; run its `suggestion`, `ytr bulk status <id>`, to check it
  later.

```bash
ytr bulk move --from-json '{"queue":"TARGET","issues":["PROJ-1","PROJ-2"]}' --json id,status,suggestion
# {"id":"6543210abcdef","status":"CREATED","suggestion":"ytr bulk status 6543210abcdef"}
```

They exit 1 when the operation ends `FAILED`, writing nothing to stdout. The
error document on stderr carries `operationId`, `statusText`, `totalIssues`
and `totalCompletedIssues`, so it still says how much of the change landed.
Its `suggestion` is empty: the operation is final, and `bulk status` would
only report the same failure:

```bash
ytr bulk move --from-json '{"queue":"TARGET","issues":["PROJ-1","PROJ-2"]}' --json id,status
# stdout: (empty)
# stderr: {"code":"bulk_failed","message":"bulk operation 6543210abcdef failed: ... (1 of 2 issues completed)",
#          "operationId":"6543210abcdef","statusText":"...","totalIssues":2,"totalCompletedIssues":1,
#          "suggestion":""}
```

`bulk status` reports a `FAILED` operation the same way: exit 1, nothing on
stdout, and the same `bulk_failed` document on stderr. Any other status prints
its document at exit 0; its `suggestion` is the same command while the
operation is unfinished, and empty once it is `COMPLETED`.

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
# Create a link between issues; "issue" takes the other issue's key or its
# 24-character ID, such as 4ff3e8dae4b0e2ac00000001
ytr link create PROJ-123 --from-json '{"relationship":"relates","issue":"PROJ-456"}'

# List links on an issue
ytr link list PROJ-123 --json id,type,issue

# Add and manage checklist items
ytr checklist create PROJ-123 --from-json '{"text":"Review PR"}'
ytr checklist edit PROJ-123 42 --from-json '{"checked":true}'
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
need; `incomplete` is always included.

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
```

## Output Shape

Every command except help (`ytr help`, `--help`) prints its result as one
line of compact JSON on stdout, the same whether stdout is a terminal, a pipe
or a file.

- Without `--json`, the result holds every field the command's `JSON FIELDS`
  lists; `--json a,b` keeps only those, and `--jq` filters the result.
- A write prints the item Tracker answers with, a delete prints
  `{"deleted":true,"id":"..."}`, and an empty list prints `[]` or its
  envelope with `"items":[]`.
- Times are RFC 3339 with the offset the server sent, such as
  `2026-09-19T14:22:31+03:00`.
- A field Tracker sent no value for is left out of the item, or else prints
  as `""` for a string, `0` for a number and `false` for a boolean.

```bash
# Every field of an issue
ytr issue view PROJ-123

# Only the key and summary
ytr issue view PROJ-123 --json key,summary
```

## JSON Output

Most resource commands support `--json field1,field2` field selection.
Run `ytr <command> --help` and look for the `JSON FIELDS` section to see
the available field names for that command; the error for an unknown field
names them too, in `validFields`. A `--json` that names no field
(`--json=`, even beside `--jq`) or an unknown one exits 1 before any request.

### Streams

stdout carries the command's output and nothing else. A run that fails writes
nothing at all to stdout and one JSON document to stderr, with or without
`--json` or `--jq`: `code`, `message`, and `suggestion` when the error has
one. Read together, the two streams therefore hold exactly one JSON document,
the result on stdout or the failure on stderr, never both -- unless the run
asked for a `--jq` stream or help, which are text:

- Without `--jq`, stdout on success is exactly one JSON document.
- `--jq` is a stream, not a document: one line per result with an implicit
  `-r`, so strings arrive unquoted. A filter that matches nothing writes
  nothing and exits 0. A filter that fails writes nothing and exits 1 -- never
  the results it had already produced -- so a partial stream is not a shape you
  have to handle.
- `--help` and `ytr help <command>` are not command output: they write plain
  text to stdout and exit 0 even under `--json`.

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

List shapes:

- Paginated list commands such as `issue list`, `issue changelog`, `queue list`, and `user list` return an object with `items` and `pagination`.
- Non-paginated sub-resource list commands such as `comment list`, `link list`, `worklog list`, and `checklist list` return arrays.

```bash
# Filter paginated issue-list output
ytr issue list --filter queue=PROJ --json key --jq '.items[].key'

# Filter sub-resource array output
ytr comment list PROJ-123 --json body --jq '.[].body'
```

## Error Recovery

| Exit Code | Meaning | Recovery |
|-----------|---------|----------|
| 0 | Success | -- |
| 1 | User error | Read `message` and `suggestion` in the JSON error on stderr |
| 3 | Auth error | Set `YTR_TOKEN`, `YTR_ORG_ID`, and `YTR_ORG_TYPE`, or run `ytr auth login` |
| 4 | Not found | Verify the resource key exists |
| 5 | Rate limited | Wait and retry |
| 130 | Interrupted | Re-run the command |

`--from-json` fails with exit code 1 when the input carries a key the request
body has no field for. The JSON error lists every offending key in
`invalidFields` and the accepted ones in `validFields`. Local queue fields are
among the rejected keys: the API supports them, `--from-json` does not yet.

Every create, edit, update and transition command, and `bulk move`, words its
flag errors the same way, and each exits 1 before any request:

- `cannot combine --from-json with --summary, --type`: a request flag next to
  `--from-json`, naming the flags you set. Pass the request one way or the other.
  Issue key arguments next to a bulk `--from-json` fail the same way, as
  `cannot combine --from-json with issue keys`.
- `missing --name, --queue`: a create without a required flag, or a
  `--from-json` without the matching key (here `"name"`, `"queue"`); the
  suggestion names the keys. `comment create --body` (key `"text"`),
  `issue transition --to` (key `"to"`), `bulk move --queue`,
  `bulk update --field` (key `"values"`, which needs at least one field) and
  `bulk transition --transition` are required the same way.
- `nothing to update`: an edit or `issue update` with no request flag, or with a
  `--from-json` object that sets no key, such as `'{}'` or `'{"text": null}'`.
- `control character U+0000 at position 1 in summary`: a character below
  U+0020 other than tab, newline and carriage return in `--summary` or
  `--description` of `issue create` and `issue update`, or in `--body` of
  `comment create` and `comment edit`, or in the key the flag sets in
  `--from-json` (`"text"` for `--body`); the error names the flag either way.
  Remove it.
- `invalid issue key or ID "bad"`: a `link create --issue`, or an `"issue"` in
  its `--from-json`, that is neither an issue key such as `PROJ-456` nor a
  24-character hexadecimal issue ID, or such a bulk issue, given as an
  argument, on stdin or in the `"issues"` of a bulk `--from-json`.
- `no issue keys provided`: a bulk `--from-json` body without `"issues"`, or
  with an empty one; `no issue keys provided via stdin` when a bulk command
  gets no key arguments and stdin holds no key, which exits 1 before auth. A
  bulk command without key arguments reads stdin to its end, so give it the keys
  as arguments or pipe them in.

### Bad invocations

Any invocation ytr cannot serve exits 1, leaves stdout empty, and writes one
JSON error document to stderr, whatever flags it carries. This covers a
mistyped subcommand, a group named without a subcommand (`ytr issue`), `ytr`
with no arguments at all, an unknown flag, a stray positional argument, a
malformed one such as `ytr issue view 123` (not an issue key) or an empty one,
a `--json` that names no field (`--json=`) or an unknown one, a `--limit`
outside 1 to 1000, a `--cursor` of `issue list`, `queue list` or `user list`
that is not a page number (`2` is one, `abc` is not), `--all` together with
`--cursor`, an `issue list --filter` without `=`, and an unknown `ytr help`
topic. None of them reach Tracker, and none print help and exit 0. A bad
`--limit`, `--cursor` or `--filter`, `--all` with `--cursor`, a bad bulk issue
key given as an argument or on stdin, or a flag error above that does not come
from a `--from-json` body, exits 1 even without credentials, since ytr checks
it before auth.

Every `suggestion` that names a command can be run as it stands. A suggested
command is complete and never contains a value ytr picked for you, so act on
it literally:

```bash
ytr issue lst
# {"code":"user_error","message":"unknown command \"lst\" for \"ytr issue\"","suggestion":"Did you mean: ytr issue list"}

ytr issue
# {"code":"user_error","message":"\"ytr issue\" needs a subcommand: changelog, create, list, transition, update, view","suggestion":"Run \"ytr issue --help\" for details."}

ytr issue list --limitt 5
# {"code":"user_error","message":"unknown flag: --limitt","suggestion":"Did you mean: --limit"}

ytr auth status --json=
# {"code":"invalid_field","message":"no fields specified","validFields":["status","user","org_id","org_type","token_source"],"suggestion":"Valid fields: status, user, org_id, org_type, token_source"}
```

Asking for help is not a bad invocation: `--help` and `ytr help <command>` write
the help text to stdout and exit 0, even on a mistyped command path
(`ytr issue lst --help`).

## Flags Reference

| Flag | Scope | Description |
|------|-------|-------------|
| `--json f1,f2` | Global on most commands | Print only the selected fields; without it, every field |
| `--jq expr` | Global | Filter the JSON output with a jq expression |
| `--token` | Global auth override | Override auth token |
| `--org-id` | Global auth override | Override organization ID |
| `--org-type` | Global auth override | Override organization type: `360` or `cloud` |
| `--from-json` | Every create, edit, update and transition command, and `bulk move` | The request body as one JSON object (inline, `@file`, or `-` for stdin), whose keys the command's request flags are shorthand for; keys the request body has no field for are rejected, not dropped; cannot be combined with the command's request flags, nor with a bulk command's issue key arguments, and stdin is not read for bulk keys; a create or bulk command needs its required keys and an edit at least one key; a bulk body needs a non-empty `"issues"`; a key's value must pass its flag's check |
| `--query` | `issue list` | Search using Tracker query language; mutually exclusive with `--filter` and `--order-by` |
| `--filter k=v` | `issue list` | Filter by field (repeatable); mutually exclusive with `--query` |
| `--order-by` | `issue list` | Sort by field (descending by default); cannot be used with `--query` |
| `--order-asc` | `issue list` | Sort ascending; requires `--order-by` |
| `--field` | `issue changelog` | Filter changes by field name (case-sensitive) |
| `--type` | `issue changelog` | Filter by change type (e.g., IssueWorkflow, IssueCommentAdded) |
| `--limit N` | Paginated list commands | Results per page, 1 to 1000 (default 50); any other value exits 1 |
| `--all` | Paginated list commands | Fetch all pages automatically |
| `--cursor` | Paginated list commands | Pagination cursor (pass the `pagination.cursor` value from the previous response) |
| `--timeout` | Bulk commands | Max wait time (default 1m); an operation not finished by then exits 0 with its status and `suggestion` |
