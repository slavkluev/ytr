# Contributing

Thank you for contributing to ytr! This guide covers the development workflow.

## Development Setup

```bash
git clone https://github.com/slavkluev/ytr.git
cd ytr
```

Requirements: Go 1.26 or later.

## Running Tests

```bash
make lint      # run golangci-lint
make test      # run tests with race detector
make check     # run lint + test (recommended before pushing)
```

Additional commands:

```bash
make build     # compile the ytr binary
make fmt       # format code with gofmt
make coverage  # generate coverage profile
```

## Submitting Changes

1. Fork the repository and branch off `main`
2. Make your changes — keep each branch to a single logical change
3. Run `make check` to verify lint and tests pass
4. Commit using the Conventional Commits format (see below)
5. Push to your fork and open a Pull Request

### Branch naming

Name branches `type/short-description`, where `type` matches a commit type below
and the description is kebab-case and meaningful on its own:

```
feat/issue-search
fix/bulk-exit-code
docs/contribution-guidelines
```

Avoid internal references (ticket IDs, private notes): the name should make sense
to someone seeing the repository for the first time.

### Commit messages

This project follows [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): subject

[optional body explaining what changed and why]
```

Common types are `feat`, `fix`, `docs`, `refactor`, `test`, and `chore`. The
scope is the affected area (e.g. `bulk`, `checklist`, `output`). Examples:

```
feat(issue): add --query filter to issue list
fix(checklist): match created item by text, not list position
chore(deps): bump gojq
```

### Pull requests

- Keep each PR focused on one logical change — a reviewer should be able to say
  "this PR does X".
- Write a self-contained title and description: a reader should understand the
  what and why without access to private notes or external trackers.
- Make sure CI is green before requesting review.

## Code Style

This project uses golangci-lint for code quality. Run `make lint` to check your code. The linter configuration is in `.golangci.yml`.
