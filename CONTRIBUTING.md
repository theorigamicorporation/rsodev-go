# Contributing

Thank you for helping. A few things keep changes easy to review.

## Before you open a pull request

- Run `just check` (or `make check`): formatting, `go vet`, the linter and
  the tests with the race detector.
- Add or update a test for what you changed. Every typed operation is
  called in `everyOperation` (`platform/schema_test.go`), which checks its
  GraphQL document and variables against the schema snapshot in
  `platform/testdata`. A new operation needs a line there.
- If the platform API gained what you use, refresh the snapshot:
  `just schema <introspection.json>`.

## Conventions

- One package, `platform`. No dependencies outside the standard library in
  non-test code.
- Every operation takes a `context.Context` first.
- Names follow the existing ones: `ListX`, `GetX` (an error when missing),
  `FindX` (nil when missing), `CreateX`, `UpdateX`, `DeleteX`, `WaitXReady`.
- GraphQL documents and field lists are raw string literals; the snapshot
  tool reads them.
- Callers name a tenant and optionally a region. Nothing in the API of this
  module names where the platform runs things.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/).

## Reporting bugs

Open an issue with what you called, what you expected and what came back.
Leave secrets and tokens out. For security problems see `SECURITY.md`.
