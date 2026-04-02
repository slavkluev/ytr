# ytr

Yandex Tracker CLI for humans and LLM agents.

A command-line client for [Yandex Tracker](https://tracker.yandex.ru/) — Yandex's project management tool, similar to Jira. Designed primarily for LLM agents calling commands programmatically, with a strong secondary focus on human developers in the terminal.

## Features

- Issue management (create, list, view, update, transition)
- Comments, links, worklogs, checklists as sub-resources
- Bulk operations (move, update, transition) with async polling
- Reference data (statuses, priorities, resolutions, issue types)
- Field discovery (global and queue-local fields)
- Components and queue management
- User lookup and organization listing
- Structured JSON output with `--json` field selection and `--jq` filtering
- Shell completions (bash, zsh, fish)
- Designed for LLM agents with predictable output and semantic exit codes

## Installation

### Homebrew (macOS)

```
brew install slavkluev/tap/ytr
```

### Go install

```
go install github.com/slavkluev/ytr@latest
```

### Binary download

Download the latest release from [GitHub Releases](https://github.com/slavkluev/ytr/releases).

## Quick Start

```bash
# Authenticate
export YTR_TOKEN="your-oauth-token"
export YTR_ORG_ID="your-org-id"
export YTR_ORG_TYPE="360"  # or "cloud"

# Issue management
ytr issue create --queue PROJ --summary "New task"
ytr issue list --queue PROJ --json key,summary,status
ytr issue transition PROJ-123 --to "In Progress"

# Comments and worklogs
ytr comment create PROJ-123 --body "Working on this"
ytr worklog create PROJ-123 --duration PT2H --start 2026-03-30T10:00:00Z

# Bulk operations
ytr bulk move PROJ-1 PROJ-2 PROJ-3 --queue TARGET
echo "PROJ-1\nPROJ-2" | ytr bulk update --field priority=critical

# Reference data
ytr status list --json key,name
ytr field list --queue PROJ

# JSON output with jq
ytr issue list --queue PROJ --jq '.items[].key'
```

## Authentication

ytr supports three authentication methods (in order of precedence):

1. **Flags:** `--token`, `--org-id`, and `--org-type` on any command; provide all three together
2. **Environment variables:** `YTR_TOKEN`, `YTR_ORG_ID`, and `YTR_ORG_TYPE`
3. **Config file:** `ytr auth login` saves credentials to `~/.config/ytr/config.yaml`

## Documentation

- Run `ytr --help` for a list of commands
- Run `ytr <command> --help` for command-specific help
- See [SKILL.md](skills/ytr/SKILL.md) for LLM agent integration guide

## License

MIT — see [LICENSE](LICENSE) for details.
