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

1. Fork the repository
2. Create a feature branch (`git checkout -b my-feature`)
3. Make your changes
4. Run `make check` to verify lint and tests pass
5. Commit your changes with a clear, descriptive message
6. Push to your fork and open a Pull Request

## Code Style

This project uses golangci-lint for code quality. Run `make lint` to check your code. The linter configuration is in `.golangci.yml`.
