# Security Policy

## Supported versions

The latest release is the supported one. Fixes land on `main` and go out in the
next tag.

## Reporting a vulnerability

Please do **not** open a public issue for a security problem.

Use GitHub's private vulnerability reporting instead:
[Security → Report a vulnerability](https://github.com/iamnikolie/wispr-cli/security/advisories/new).
That opens a private advisory visible only to the maintainers.

Include what you did, what happened, and the impact you think it has. Expect a
first response within a week — this is a spare-time project, not a product with
an on-call rotation.

## Scope notes

Some things are known and by design rather than vulnerabilities:

- **It reads your private data.** Dictation history, meeting transcripts and
  notes are personal. `wispr` only reads what is already on your disk and
  never sends anything anywhere, but whatever you pipe its output into (an
  LLM, a chat, an issue) receives that data. Treat the output as sensitive.
- **The database is opened read-only** (`mode=ro`, `PRAGMA query_only`). If
  you find a way to make `wispr` modify `flow.sqlite` or the `meetings/`
  folder, that is a bug worth reporting privately.
- **`wispr sql` runs the SQL you give it** against the same read-only
  connection. It is a convenience for the database owner, not a sandbox for
  untrusted queries.
- **Exports overwrite files** in the `--out` directory that match its own
  naming scheme (`meetings/*.md`, `dictations/*.md`, `notes/*.md`). Point it
  at a dedicated folder.
