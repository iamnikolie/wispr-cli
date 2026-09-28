# wispr — Wispr Flow local reader (agent reference)

Read-only access to the Wispr Flow desktop app's local database: dictation
history, Notetaker meeting summaries + transcripts, Flow notes, dictionary.
No API, no account, no network — it reads `flow.sqlite` next to the app.
Markdown on stdout, counters/hints on stderr.

## Setup
Nothing to configure on macOS: the default is
`~/Library/Application Support/Wispr Flow/flow.sqlite`. Override with
`--db <path>` or `WISPR_DB`. The app can keep running; the database is opened
`mode=ro` with `PRAGMA query_only`, so nothing here can write.
`wispr status` confirms the path and what it holds.

## Global flags
- `--json` (= `--format json`); `--format md|table|json|csv|tsv|text`.
  `md`/`table` = Markdown table; `text` = plain lines (dictations, transcripts, notes).
- `--fields time,text` projects rows (list commands); with `--json` it emits
  only those keys.
- `--utc` prints times in UTC (default: local time, RFC3339 with offset).
- `--db <path>` database location.

Time bounds (`--since`, `--until`) accept `30m`, `24h`, `7d`, `2w`, `3mo`
(back from now) or `2026-09-01`, `2026-09-01T10:00` (local), RFC3339.

## Commands
- `wispr status` — db path/size, app version, counts, dictation date range,
  meetings folder.
- `wispr dictations list [--since 1d] [--until ..] [--app cursor] [--search q]
  [-n 50] [--desc] [--full] [--width 120] [--status all|formatted,raw_transcript]
  [--archived]` — what you dictated, oldest first. Columns: time, app (short
  label), words, text (truncated to --width unless --full; json is never
  truncated). `history` is an alias. `--format text` → `YYYY-MM-DD HH:MM [app] text`.
  JSON keys: id, time, day, app (bundle id), app_label, status, language,
  words, duration_s, text (what Flow pasted; falls back to raw ASR), asr,
  edited (text after the user changed it in the target app, when captured).
- `wispr dictations export [--since ..] [-o DIR]` — `DIR/dictations/<day>.md`,
  one bullet per dictation.
- `wispr meetings list [--since ..] [--search q] [-n 10] [--asc] [--deleted]` —
  Notetaker meetings, newest first: id (8-char prefix), created, duration,
  title, summary_chars.
- `wispr meetings show <id-prefix> [--transcript] [--live]` — Markdown with
  front matter, `## Summary` (Flow's AI summary; `<@speaker:N>` placeholders
  are resolved to names), `## Notes` (user notes, often empty), and with
  `--transcript` the refined transcript as `[mm:ss] Speaker: text`. `--live`
  reads the raw live transcript instead. `--format text` prints just the
  summary (or just the transcript with `--transcript`). JSON includes `audio`
  (path to the recording) and `transcript` array.
- `wispr meetings export [--since ..] [--transcript] [-o DIR]` —
  `DIR/meetings/<date>-<slug>-<id8>.md`.
- `wispr notes list|show <id-prefix>|export` — Flow notes (Notes tab).
- `wispr dictionary` — personal dictionary/snippets, most used first.
- `wispr todos` — Notetaker action items.
- `wispr export [--since ..] [--transcript] [--skip meetings,dictations,notes]
  [-o DIR]` — all three exports. Idempotent: unchanged files are not
  rewritten; stdout lists written paths, stderr prints per-section counts.
- `wispr schema [table] [--full]` — table names, or a CREATE statement.
- `wispr sql "<select>"` — any read-only query, rendered like a list.
  BLOB columns show as `<blob N bytes>`; do not SELECT `audio`/`screenshot`
  from History unless you mean it (they are the bulk of the file).
- `wispr skill`, `wispr version`.

## Workflows
- "What did I say today?" → `wispr dictations list --since 1d --format text`.
- Prompts dictated into a specific tool → `--app cursor` / `--app slack`
  (substring of the bundle id, case-insensitive).
- Meeting notes for a vault → `wispr export --transcript -o ~/vault/wispr`
  on a schedule; only changed files are touched.
- Digest of last week's meetings → `wispr meetings list --since 7d --json`
  then `wispr meetings show <id> --format text` per row.
- Schema drifted after an app update (a column error) → `wispr schema
  History --full`, then `wispr sql` with the columns that exist.

## Gotchas
- Dictations default to statuses `formatted` + `raw_transcript` and exclude
  archived rows; `--status all --archived` shows everything, including
  `dismissed`/`empty` noise.
- `text` may contain HTML (`<ul><li>…`) when Flow pasted rich text into a
  rich editor; that is what the app produced.
- Meetings without a `refined.ndjson` fall back to `live.ndjson` with a note
  on stderr; a meeting with neither errors on `--transcript`.
- Times in the database are UTC; output is local unless `--utc`. File names
  in exports use the same clock, so pick one and stick with it.
- The database is the app's private store; the schema changes between
  releases (Sequelize migrations). Fixed columns this tool reads may
  disappear — `schema` + `sql` are the escape hatch.
- Dictation content is personal. Treat it as sensitive when pasting into
  prompts or issues.
