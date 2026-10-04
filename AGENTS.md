<!-- bmad:context -->
<!-- Verified 2026-10-04 against 725d30f. Managed by bmad-project-context; edits inside this block are replaced on refresh. Keep anything you want preserved outside the markers. -->

## ytr

Yandex Tracker CLI for LLM agents and humans: Go 1.26, cobra, built on `github.com/slavkluev/go-yandex-tracker` (sibling checkout `../go-yandex-tracker`). `skills/ytr/SKILL.md` and `skills/ytr/query-language.md` are the shipped guide for agents that use ytr, so they are part of the CLI's contract. Tracker API reference: https://yandex.ru/support/tracker/en/api-ref/about-api

## Policy

- Commit freely; `git push` and any tag only after the user confirms — a `v*` tag publishes a GitHub release and the Homebrew formula. Same rule in `../go-yandex-tracker`.
- The real Tracker is read-only for agents: run only `list`, `view`, `get`, `changelog`, `myself`, `bulk status`, `auth status`. Never run create, update, edit, delete, transition, move, or `auth login`/`logout` — a fake token still sends the request to the live API.
- When the Tracker API allows something go-yandex-tracker does not, change the library in `../go-yandex-tracker`; never work around it in ytr.
- No planning references in code, tests, or commit messages: no decision or requirement ids (`D-06`, `OUT-05`), review finding labels (`Finding: Medium 3`, `Info #4`), or task ids. State the reason itself.

## Where things are

- New or changed command: copy `internal/cmd/worklog/` — `worklog.go` (the group command only), `list.go` (`runner.List`: read and JSON fields), `edit.go` (`runner.Write`: partial update, `--from-json`), `create.go` (`runner.Write` with `Required` keys), `delete.go` (`runner.Delete`).

## Running and verifying

- Done means `make check` passes (golangci-lint plus race tests, about 35 s cold), not `go test` or `go vet` alone — lint drift has needed separate cleanup commits.
- Lint needs golangci-lint v2.11 (the CI pin) built with Go 1.26; if it cannot load the code, install that version — never substitute `go vet`.
- Until a library change is tagged, build against it with `go work init . ../go-yandex-tracker` (`go.work` is gitignored); never add `replace` to `go.mod` — `gomoddirectives` fails lint. After the tag, `go get github.com/slavkluev/go-yandex-tracker@vX.Y.Z`, then commit here — CI has no `go.work`.

## Conventions that differ from defaults

- A command's `--json` fields are the json tags of its flat item struct, in order. For a `runner.List`/`runner.Get` declaration the runner derives the `JSON FIELDS` help, the `--json=` hint, validation and completion from them; never hand-write them. A command outside the runner keeps its `XxxFields` slice, `JSON FIELDS` block in `Long` and `PrintFieldHint` name matching the tags by hand and gives the slice to `runner.SetFields(cmd, …)` for completion; a slice it shares with a runner command is `runner.ItemFields[item]()`. A mismatch fails silently — the field vanishes.
- Render JSON from that flat struct with value types, never from an SDK struct; read SDK pointers with `api.Deref*`, falling back to `""` in JSON and `"-"` in tables.
- Every user-valued JSON field `x` gets a sibling `xId` from `User.IDOr("")`, without `omitempty`.
- Return SDK errors as `api.MapAPIError(err)` and other failures as `errors.NewUserError`/`NewAuthError`/`NewNotFoundError` (`internal/errors`) with a Suggestion; never `os.Exit` outside `cmd/ytr/main.go`.
- Declare a runner command's positional args as `runner.IssueKey`, `runner.StringID(label)` or `runner.NumericID(label)`: the runner checks them before the field hint and auth and hands `Call` the parsed values. Outside the runner, validate them before auth with `validate.ValidateIssueKey`, `ValidateStringID` or `ValidateNumericID`.
- Decode `--from-json` only with `validate.UnmarshalRequestJSON`, which rejects unknown keys; never `json.Unmarshal`.
- In update and edit commands, set a request field only when `cmd.Flags().Changed(name)` — requests are partial PATCHes.
- When an output shape or exit code changes, update `skills/ytr/SKILL.md` in the same commit and bump its `metadata.version` major; a wording change bumps nothing.
- User-visible changes are `feat` or `fix` commits — goreleaser drops `docs`, `test`, and `chore` from release notes. Commit and branch format: `CONTRIBUTING.md`.
- Test every command through `runCLI` (`internal/cmd/harness_test.go`) as `leafRow` table rows (`internal/cmd/leaf_test.go`) against inline Tracker exchanges that carry only the fields ytr reads; there is no mock, SDK interface or factory var.
- Tests are white-box, stdlib `testing` only, and parallel in `internal/cmd` (`paralleltest` and `tparallel` enforce it): give a row its terminal facts (`term: output.Options{TTY: true, Colors: true}`), `stdin`, extra `env` and starting `config`, or run it `signedOut`, through the row, and read what it wrote under `cliResult.ConfigDir` — never through `t.Setenv` or a package global, which a parallel test cannot use.
- Comment only a why the code cannot say — never restate a name, label a step, or narrate a past bug; pin the bug with a named regression test instead. `internal/cmd/comments_test.go` fails on the commonest restating shapes.

## Known pitfalls

- Never do less than asked silently: reject unknown or conflicting input with a user error, paginate to the end instead of capping, pass the server's error text through. This class has been fixed a dozen times (`--from-json` dropping keys, `--all` ignoring `--cursor`, the 50-comment cap).
- Do not encode assumed API behavior in a test's Tracker exchanges: check the API reference or make a read-only call to the real Tracker first. Wrong assumptions "verified" by mocks shipped eight times (default page size, where 422 details live, what a field's `type` means).
- When fixing one command, grep its siblings (every `runner.List`/`runner.Get` declaration and `runner.SetFields` call, every `from-json` flag, every `--all` list) and fix them all or the shared helper — `internal/cmd/runner/runner.go` for anything a declaration hands the runner. Single-site fixes have left the bug live six times.
- The exit code must say whether the change happened: non-zero on failure, and 0 after a successful non-idempotent write even if a follow-up step fails — a false failure makes agents retry and create duplicates.
- Write to stdout only after the last step that can fail: under `--json`/`--jq` a successful run writes exactly one document (the result stream under `--jq`) and nothing else, and a failing run leaves stdout empty and puts its single JSON error document on stderr, `--debug` included. A reader cannot tell a truncated stream from a complete one. Buffer instead — `ApplyJQ` collects the whole jq stream before writing, and a FAILED bulk renders nothing. A warning goes inside the document, never as a separate line on either stream.
- Sub-resource lists (comment, link, worklog, checklist) return bare JSON arrays; only `issue list`, `queue list`, `user list`, and `issue changelog` use the `{items, pagination}` envelope. Changing a shape or flag semantics is a breaking change: its own commit with `!` and a `BREAKING CHANGE:` footer.
- Pass Tracker identifiers through unchanged (field ids like `storyPoints`, `<queueId>--<key>`); use `EqualFold` only for ytr's own field names — case-folding once silently returned zero results.

<!-- /bmad:context -->
