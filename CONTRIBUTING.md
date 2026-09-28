# Contributing

Thanks for taking a look. This is a small, focused CLI — bug reports and pull
requests are welcome, and so is a plain question in an issue.

## Reporting a bug

Include the output of `wispr version` and `wispr status` (redact the path if
you like), the exact command you ran, and what you expected instead. If the
problem is an unknown column or table after a Wispr Flow update, paste the
relevant `wispr schema <table> --full` output and the app version — the schema
belongs to the app and drifts between releases.

**Never paste dictation text, meeting summaries or transcripts into a public
issue** unless you are sure they contain nothing personal.

## Pull requests

Before opening one:

```bash
make fmt           # gofmt -w .
make vet           # go vet ./...
make test          # go test ./...
```

CI runs the same three (plus `-race`) on Linux and macOS, so a green local run
usually means a green PR.

House rules:

- **Read-only, always.** The store opens the database with `mode=ro` and
  `PRAGMA query_only`. Nothing in this tool may write to `flow.sqlite` or the
  `meetings/` folder — exports go to the user's `--out` directory only.
- **One concern per PR.** A bug fix and a refactor in the same diff take three
  times as long to review.
- **Tests against a fixture.** The suite builds a throwaway SQLite database in
  a temp dir; new columns or tables the tool reads must be added to the
  fixtures in `internal/store/store_test.go` and `cmd/cmd_test.go`.
- **Keep the output token-lean.** The default rendering exists so an agent can
  read it without burning context. New columns belong behind `--fields` or
  `--json`, not in the default set.
- **Data goes to stdout.** Only errors, hints and counters go to stderr, so
  `x=$(wispr ...)` always works.
- **Update the docs in the same commit.** Any change to the CLI surface must
  also update `cmd/skill.md` (embedded in the binary, printed by
  `wispr skill`, and the single source of truth for command UX) and `README.md`.
- **Conventional commit subjects** — `feat:`, `fix:`, `docs:`, `refactor:`,
  `test:`, `chore:`. Release notes are generated from them.

## Releases

Maintainer-only. Tag and push:

```bash
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

GoReleaser builds archives for linux/darwin/windows on amd64 and arm64 and
publishes the GitHub release with a generated changelog.
