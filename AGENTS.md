<!-- bmad:context -->
<!-- Verified 2026-09-19 against 9c4730f. Managed by bmad-project-context; edits inside this block are replaced on refresh. Keep anything you want preserved outside the markers. -->

## ytr

Yandex Tracker CLI for LLM agents and humans: Go 1.26, cobra, built on `github.com/slavkluev/go-yandex-tracker` (sibling checkout `../go-yandex-tracker`). `skills/ytr/SKILL.md` and `skills/ytr/query-language.md` are the shipped guide for agents that use ytr, so they are part of the CLI's contract. Tracker API reference: https://yandex.ru/support/tracker/en/api-ref/about-api

## Policy

- Commit freely; `git push` and any tag only after the user confirms — a `v*` tag publishes a GitHub release and the Homebrew formula. Same rule in `../go-yandex-tracker`.
- The real Tracker is read-only for agents: run only `list`, `view`, `get`, `changelog`, `myself`, `bulk status`, `auth status`. Never run create, update, edit, delete, transition, move, or `auth login`/`logout` — a fake token still sends the request to the live API.
- When the Tracker API allows something go-yandex-tracker does not, change the library in `../go-yandex-tracker`; never work around it in ytr.
- No planning references in code, tests, or commit messages: no decision or requirement ids (`D-06`, `OUT-05`), review finding labels (`Finding: Medium 3`, `Info #4`), or task ids. State the reason itself.

## Where things are

- New or changed command: copy `internal/cmd/worklog/` — `worklog.go` (SDK interface plus a replaceable factory var for tests), `list.go` (read and JSON fields), `edit.go` (partial update, `--from-json`).

## Running and verifying

- Done means `make check` passes (golangci-lint plus race tests, about 35 s cold), not `go test` or `go vet` alone — lint drift has needed separate cleanup commits.
- Lint needs golangci-lint v2.11 (the CI pin) built with Go 1.26; if it cannot load the code, install that version — never substitute `go vet`.
- Until a library change is tagged, build against it with `go work init . ../go-yandex-tracker` (`go.work` is gitignored); never add `replace` to `go.mod` — `gomoddirectives` fails lint. After the tag, `go get github.com/slavkluev/go-yandex-tracker@vX.Y.Z`, then commit here — CI has no `go.work`.

## Conventions that differ from defaults

- A command's JSON fields live in six places that must match, and nothing checks them: the `XxxFields` slice, json tags on its flat output struct, the `JSON FIELDS` block in `Long`, `jsonfields.Register("ytr <res> <verb>", …)`, the `PrintFieldHint` name, and `skills/ytr/SKILL.md`. A mismatch fails silently — the field vanishes or completion goes empty.
- Render JSON from that flat struct with value types, never from an SDK struct; read SDK pointers with `api.Deref*`, falling back to `""` in JSON and `"-"` in tables.
- Every user-valued JSON field `x` gets a sibling `xId` from `api.DerefUserID`, without `omitempty`.
- Return SDK errors as `api.MapAPIError(err)` and other failures as `errors.NewUserError`/`NewAuthError`/`NewNotFoundError` (`internal/errors`) with a Suggestion; never print an error and return nil, never `os.Exit` outside `cmd/ytr/main.go`.
- Validate positional args before auth (`validate.ValidateIssueKey`, `ValidateStringID`, `ValidateNumericID`); `issue view`, `queue view`, and `field get` still skip this — do not copy them.
- Decode `--from-json` only with `validate.UnmarshalRequestJSON`, which rejects unknown keys; never `json.Unmarshal`.
- In update and edit commands, set a request field only when `cmd.Flags().Changed(name)` — requests are partial PATCHes.
- When a flag, JSON field, output shape, or exit code changes, update `skills/ytr/SKILL.md` in the same commit and bump its `metadata.version`: major when a documented invocation stops working or its output changes, minor for additions, none for wording.
- User-visible changes are `feat` or `fix` commits — goreleaser drops `docs`, `test`, and `chore` from release notes. Commit and branch format: `CONTRIBUTING.md`.
- Tests are white-box, stdlib `testing` only, never `t.Parallel` — they swap package globals (factory vars, `output.*` flags, the field registry). Call `testutil.ResetOutputFlags(t)` first in the test body, stub the factory var with a `t.Cleanup` restore, and pass auth as `--token`/`--org-id`/`--org-type` flags; tests touching env or config set `t.Setenv("YTR_CONFIG_DIR", t.TempDir())`.
- gocyclo and cyclop (limit 30) apply to `_test.go` too: write standalone test functions rather than one large table-driven test.

## Known pitfalls

- Never do less than asked silently: reject unknown or conflicting input with a user error, paginate to the end instead of capping, pass the server's error text through. This class has been fixed a dozen times (`--from-json` dropping keys, `--all` ignoring `--cursor`, the 50-comment cap).
- Do not encode assumed API behavior in mocks: check the API reference or make a read-only call to the real Tracker first. Wrong assumptions "verified" by mocks shipped eight times (default page size, where 422 details live, what a field's `type` means).
- When fixing one command, grep its siblings (every `jsonfields.Register`, every `from-json` flag, every `--all` list) and fix them all or the shared helper — single-site fixes have left the bug live six times.
- The exit code must say whether the change happened: non-zero on failure, and 0 after a successful non-idempotent write even if a follow-up step fails — a false failure makes agents retry and create duplicates.
- Sub-resource lists (comment, link, worklog, checklist) return bare JSON arrays; only `issue list`, `queue list`, `user list`, and `issue changelog` use the `{items, pagination}` envelope. Changing a shape or flag semantics is a breaking change: its own commit with `!` and a `BREAKING CHANGE:` footer.
- Do not add `NoOptDefVal` to `--json`: it breaks the `--json key,summary` form. Bare `--json` erroring is known; the field hint is reached with `--json=`.
- Pass Tracker identifiers through unchanged (field ids like `storyPoints`, `<queueId>--<key>`); use `EqualFold` only for ytr's own field names — case-folding once silently returned zero results.

<!-- /bmad:context -->
