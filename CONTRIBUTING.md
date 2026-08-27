# Contributing to ISMS

Thank you for your interest in contributing.

## Process

1. **Open an issue first** to discuss the change you'd like to make.
2. Fork the repository and create a feature branch.
3. Make your changes and ensure tests pass.
4. Submit a pull request referencing the issue.

## Development Setup

- Go 1.22+
- Node.js 22+ (for web UI)
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

## User-Facing Strings

Every string a user reads in the web UI belongs in a locale file under
`web/src/locales/`, not in a component. The same goes for dates, numbers and
enum labels — those go through `useFormat()` and `useEnumLabel()` rather than
being formatted or de-slugged inline, because neither survives translation.

- [`web/src/locales/README.md`](web/src/locales/README.md) — the key naming
  convention and the rules for writing translatable copy.
- [`docs/i18n.md`](docs/i18n.md) — how locale resolution works and how to add a
  language.

Translations are very welcome, and adding one needs no architectural change:
register the tag and its endonym in `internal/isms/i18n/locale.go` — the server's
supported map is the single source of truth, and a locale missing from it never
reaches the picker — then copy `web/src/locales/en/`, translate the values, and
add one loader line. [`docs/i18n.md`](docs/i18n.md#adding-a-locale) has the full
steps.

One part of a translation is not a matter of fluency: the ISO audit terms and the
information-classification levels. Take those from your language's national
adoption of ISO/IEC 27001 or 9001 rather than translating the English, and record
what you took in
[`web/src/locales/README.md`](web/src/locales/README.md#iso-terminology) — it
carries the procedure, a term checklist keyed to the clauses of the standard, and
the traps that recur across languages. A wrong choice here reads perfectly and
still says the wrong thing, so a reviewer cannot catch it for you.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
