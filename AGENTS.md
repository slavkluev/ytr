<!-- bmad:context -->
<!-- Verified 2026-10-04 against ac1eac5. Managed by bmad-project-context; edits inside this block are replaced on refresh. Keep anything you want preserved outside the markers. -->

## ytr

Yandex Tracker CLI for LLM agents and humans: Go 1.26, cobra, built on `github.com/slavkluev/go-yandex-tracker` (sibling checkout `../go-yandex-tracker`). `skills/ytr/SKILL.md` and `skills/ytr/query-language.md` are the shipped guide for agents that use ytr, so they are part of the CLI's contract. Tracker API reference: https://yandex.ru/support/tracker/en/api-ref/about-api

## Policy

- Commit freely; `git push` and any tag only after the user confirms — a `v*` tag publishes a GitHub release and the Homebrew formula. Same rule in `../go-yandex-tracker`.
- The real Tracker is read-only for agents: run only `list`, `view`, `get`, `changelog`, `myself`, `bulk status`, `auth status`. Never run create, update, edit, delete, transition, move, or `auth login`/`logout` — a fake token still sends the request to the live API.
- When the Tracker API allows something go-yandex-tracker does not, change the library in `../go-yandex-tracker`; never work around it in ytr.
- No planning references in code, tests, or commit messages: no decision or requirement ids (`D-06`, `OUT-05`), review finding labels (`Finding: Medium 3`, `Info #4`), or task ids. State the reason itself.

## Where things are

- New command: copy the declaration of its shape under `internal/cmd/` — `worklog/list.go` (`runner.List`; `status/list.go` when the endpoint pages, through `runner.Collect`), `queue/list.go` (`runner.Pages`, for the `{items, pagination}` envelope with `--limit`, `--cursor` and `--all`), `component/get.go` (`runner.Get`), `worklog/create.go` (`runner.Write` with `Required`), `worklog/edit.go` (`runner.Write` with `Update`), `worklog/delete.go` (`runner.Delete`); `FromJSON` adds `--from-json`. A group file such as `worklog/worklog.go` holds only the group command.
- Procedural, because no shape fits them: `auth`, `bulk`, `completion`, `comment list`, `issue changelog`, `issue create`/`update`/`transition`, `queue context`, `version`. Copy one only for a command no shape fits. All but `auth` and `completion` take shared pieces from `internal/cmd/runner/runner.go` (`Client`, `SetFields`, `ItemFields`).

## Running and verifying

- Done means `make check` passes (golangci-lint plus race tests, about 35 s cold), not `go test` or `go vet` alone — lint drift has needed separate cleanup commits.
- Lint needs golangci-lint v2.11 (the CI pin) built with Go 1.26; if it cannot load the code, install that version — never substitute `go vet`.
- Until a library change is tagged, build against it with `go work init . ../go-yandex-tracker` (`go.work` is gitignored); never add `replace` to `go.mod` — `gomoddirectives` fails lint. After the tag, `go get github.com/slavkluev/go-yandex-tracker@vX.Y.Z`, then commit here — CI has no `go.work`.

## Conventions that differ from defaults

- A command's `--json` fields are the json tags of its flat item struct, in order; a runner declaration derives the `JSON FIELDS` help, the `--json=` hint, validation and completion from them, so never hand-write them. A procedural command gives `runner.SetFields(cmd, …)` an `XxxFields` slice built with `runner.ItemFields[item]()` or kept equal to the tags by hand — no check compares a hand-kept slice with the tags, and a mismatch silently drops the field.
- Render JSON from that flat struct with value types, never from an SDK struct; read SDK pointers with `api.Deref*`, falling back to `""` in JSON and `"-"` in tables. Every user-valued field `x` gets a sibling `xId` from `User.IDOr("")`, without `omitempty`.
- Return SDK errors as `api.MapAPIError(err)` and other failures as `errors.NewUserError`/`NewAuthError`/`NewNotFoundError` (`internal/errors`) with a Suggestion; never `os.Exit` outside `cmd/ytr/main.go`.
- Declare a runner command's positional args as `runner.IssueKey`, `runner.StringID(label)` or `runner.NumericID(label)`: the runner checks them before the field hint and auth and hands `Call` the parsed values. Outside the runner, validate them before auth with `validate.ValidateIssueKey`, `ValidateStringID` or `ValidateNumericID`.
- Outside `runner.Write`, check a write's flags with `validate.Body.CheckFlags` and decode `--from-json` through `Body.Decode`, which rejects unknown keys and applies `Required` and `Update` — never `json.Unmarshal`. Set a request field only when `cmd.Flags().Changed(name)`, since requests are partial PATCHes.
- When an output shape or exit code changes, update `skills/ytr/SKILL.md` in the same commit and bump its `metadata.version` major; a wording change bumps nothing.
- User-visible changes are `feat` or `fix` commits — goreleaser drops `docs`, `test`, and `chore` from release notes. Commit and branch format: `CONTRIBUTING.md`.
- Test every command through `runCLI` (`internal/cmd/harness_test.go`) as `leafRow` table rows (`internal/cmd/leaf_test.go`) against inline Tracker exchanges that carry only the fields ytr reads, never a mock, SDK interface or factory var. Tests are white-box and stdlib `testing` only. Give a row its terminal facts (`term: output.Options{TTY: true, Colors: true}`), `stdin`, extra `env` and starting `config`, or run it `signedOut`, and read what it wrote under `cliResult.ConfigDir` — never through `t.Setenv` or a package global, which the parallel `internal/cmd` tests cannot share.
- Comment only a why the code cannot say — never restate a name, label a step, or narrate a past bug; pin the bug with a named regression test instead. `internal/cmd/comments_test.go` catches only doc comments of the form `newX|runX|renderX creates|executes|handles|renders|returns…` and testability phrases, not step labels or bug history.

## Known pitfalls

- Never do less than asked silently: reject unknown or conflicting input with a user error, paginate to the end instead of capping, pass the server's error text through. This class has been fixed a dozen times (`--from-json` dropping keys, `--all` ignoring `--cursor`, the 50-comment cap).
- Do not encode assumed API behavior in a test's Tracker exchanges: check the API reference or make a read-only call to the real Tracker first. Wrong assumptions "verified" by mocks shipped eight times (default page size, where 422 details live, what a field's `type` means).
- The exit code must say whether the change happened: non-zero on failure, and 0 after a successful non-idempotent write even if a follow-up step fails — a false failure makes agents retry and create duplicates.
- Sub-resource lists (comment, link, worklog, checklist) return bare JSON arrays; only `issue list`, `queue list`, `user list`, and `issue changelog` use the `{items, pagination}` envelope. Changing a shape or flag semantics is a breaking change: its own commit with `!` and a `BREAKING CHANGE:` footer.
- Pass Tracker identifiers through unchanged (field ids like `storyPoints`, `<queueId>--<key>`); use `EqualFold` only for ytr's own field names — case-folding once silently returned zero results.

<!-- /bmad:context -->
