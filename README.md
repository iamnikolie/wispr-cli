# wispr-cli (`wispr`)

[![CI](https://github.com/iamnikolie/wispr-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/iamnikolie/wispr-cli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iamnikolie/wispr-cli.svg)](https://pkg.go.dev/github.com/iamnikolie/wispr-cli)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Read your [Wispr Flow](https://wisprflow.ai) data from the terminal: dictation
history, Notetaker meeting summaries and transcripts, Flow notes, your
personal dictionary. Built for agents (Claude Code, Cursor, scripts) that want
to pull "what did I say today" or "last week's meetings" into context without
an API.

Wispr Flow has no public API (the developer API is invite-only), but the
desktop app keeps everything in a local SQLite file. `wispr` opens that file
**read-only** and prints token-lean Markdown, JSON or plain text. One static
binary, no runtime, no network.

Sibling of [`exa-cli`](https://github.com/iamnikolie/exa-cli),
[`fibery-cli`](https://github.com/iamnikolie/fibery-cli),
[`gitlab-cli`](https://github.com/iamnikolie/gitlab-cli),
[`slack-cli`](https://github.com/iamnikolie/slack-cli) and
[`svidoq`](https://github.com/iamnikolie/svidoq): same doctrine — plain text
on stdout, nothing costs context until it is called.

> Unofficial, community-built tool. Not affiliated with, endorsed by, or
> supported by Wispr Inc. It reads the app's private database, whose schema
> can change with any release.

## What you get

```text
$ wispr status
db: /Users/me/Library/Application Support/Wispr Flow/flow.sqlite
db_mb: 120.4
app_version: 1.6.957
dictations: 812
dictation_words: 15930
dictations_first: 2026-06-01
dictations_last: 2026-09-20
meetings: 5
...

$ wispr dictations list --since 1d --format text
2026-09-25 17:54 [Cursor] Add a retry with backoff around the upload call and cover it with a test.
2026-09-25 17:55 [Telegram] Running late, start without me.

$ wispr meetings list --since 7d
| id | created | duration | title | summary_chars |
| --- | --- | --- | --- | --- |
| 3f9a1c2e | 2026-09-21T10:00:12+02:00 | 58m10s | Weekly planning | 3120 |
| b71e04d9 | 2026-09-18T15:30:05+02:00 | 41m22s | Release review | 2410 |

$ wispr meetings show 3f9a --transcript      # Markdown: summary + transcript
$ wispr export --transcript -o ~/vault/wispr # meetings/, dictations/, notes/ as .md
```

## Install

**Homebrew:**

```bash
brew trust iamnikolie/tap   # Homebrew 6 refuses untrusted third-party taps
brew tap iamnikolie/tap
brew install iamnikolie/tap/wispr-cli
```

**Prebuilt binary** — download the archive for your platform from
[Releases](https://github.com/iamnikolie/wispr-cli/releases), then:

```bash
tar xzf wispr-cli_*_darwin_arm64.tar.gz
sudo mv wispr /usr/local/bin/
```

**With Go** (1.25+) — `go install` names the binary after the module, so
rename it:

```bash
go install github.com/iamnikolie/wispr-cli@latest
mv "$(go env GOPATH)/bin/wispr-cli" "$(go env GOPATH)/bin/wispr"
```

**From source** — `make install` symlinks the binary, so a later `make build`
updates the installed CLI:

```bash
git clone https://github.com/iamnikolie/wispr-cli.git
cd wispr-cli
make install        # symlink → ~/.local/bin/wispr
```

## Setup

None on macOS: the default database path is
`~/Library/Application Support/Wispr Flow/flow.sqlite`. On Windows it is
`%APPDATA%\Wispr Flow\flow.sqlite`. Point elsewhere with `--db <path>` or
`WISPR_DB=<path>` (a copy from `backups/`, a synced file, a test fixture).

Wispr Flow can keep running. The database is opened with `mode=ro` and
`PRAGMA query_only`, so `wispr` cannot modify it — `wispr sql "DELETE ..."`
fails inside SQLite.

## Commands

| Command | What it prints |
|---|---|
| `wispr status` | db path/size, app version, counts, dictation date range |
| `wispr dictations list` | dictation history: time, app, words, text. `--since 1d`, `--app cursor`, `--search q`, `--format text`, `--json` |
| `wispr dictations export -o DIR` | `DIR/dictations/<day>.md`, one bullet per dictation |
| `wispr meetings list` | Notetaker meetings, newest first |
| `wispr meetings show <id> [--transcript]` | Markdown: front matter, AI summary, notes, transcript |
| `wispr meetings export -o DIR [--transcript]` | `DIR/meetings/<date>-<slug>-<id>.md` |
| `wispr notes list \| show <id> \| export` | Flow notes (Notes tab) |
| `wispr dictionary` | personal dictionary and snippets |
| `wispr todos` | Notetaker action items |
| `wispr export -o DIR` | all of the above as Markdown; idempotent |
| `wispr schema [table]` | table names / CREATE statement |
| `wispr sql "<select>"` | any read-only query |
| `wispr skill` | the embedded agent reference |

Global flags: `--format md|table|json|csv|tsv|text`, `--json`,
`--fields a,b`, `--utc`, `--db`. Time bounds `--since`/`--until` take `24h`,
`7d`, `2w`, `3mo` or `2026-09-01[T10:00]`.

Ids are UUIDs; any unique prefix works (`wispr meetings show 3f9a`).

## Using it from an agent

`wispr skill` prints a compact reference designed to be dropped into a
system prompt or a Claude Code skill. A minimal skill file:

```markdown
---
name: wispr
description: Read the user's Wispr Flow dictations, meeting summaries and notes via the `wispr` CLI.
---
Run `wispr skill` for the command reference. Typical calls:
`wispr dictations list --since 1d --format text`,
`wispr meetings list --since 7d`, `wispr meetings show <id> --format text`.
Dictations are personal data — quote them only when the task needs it.
```

For a notes vault (Obsidian, Logseq, plain folders) run the export on a
schedule; unchanged files are left untouched, so it is safe to run often:

```bash
wispr export --transcript -o ~/vault/wispr
```

Each exported file carries YAML front matter (`id`, `title`, `created`,
`duration`, `participants`, `audio`, `source: wispr-flow`) so other tools can
index it.

## Where the data comes from

| `wispr` | Wispr Flow storage |
|---|---|
| dictations | `History` table (`formattedText`, falling back to `asrText`) |
| meetings | `Meetings` table (`summary`, `notes`, `speakerMap`) |
| transcripts | `meetings/<id>/refined.ndjson` (or `live.ndjson`) |
| audio path | `meetings/<id>/upload.ogg` |
| notes | `Notes` table |
| dictionary | `Dictionary` table |
| todos | `Todos` table |

Only these columns are read. Audio blobs and screenshots stored in `History`
are never loaded (they are most of the file's size).

The schema is the app's own and changes between versions (it is managed by
Sequelize migrations). If a listing fails with an unknown-column error after
an update, `wispr schema <table> --full` shows what is there now and
`wispr sql` still works; please open an issue with the app version.

## Development

```bash
make build      # ./wispr
make test       # go test ./...
make vet
make fmt
```

The tests build a throwaway SQLite fixture; nothing touches your real
database. Pure-Go SQLite ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)),
so cross-compiling needs no C toolchain.

## License

MIT — see [LICENSE](LICENSE).
