# Contributing to ISMS

Thank you for your interest in contributing.

## Process

1. **Open an issue first** to discuss the change you'd like to make.
2. Fork the repository and create a feature branch.
3. Make your changes and ensure tests pass.
4. Submit a pull request referencing the issue.

## Development Setup

- Go 1.22+
- Node.js 20+ (for web UI)
- PostgreSQL 14+

Build the binary:

```bash
go build -o isms ./cmd/isms/
```

## Running Tests

```bash
pytest tests/ -v
```

## Code Style

- **Go** — `go fmt` and `go vet` on all code.
- **Web UI** — Vue 3 single-file components with Tailwind CSS. Follow existing conventions.
- Keep commits focused. One logical change per commit.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
