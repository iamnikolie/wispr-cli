package cmd

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetFlags restores every flag to its default: cobra keeps parsed values in
// package variables between Execute calls.
func resetFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			sv.Replace(nil)
		} else {
			f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	c.Flags().VisitAll(reset)
	c.PersistentFlags().VisitAll(reset)
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

// fixtureDB writes a minimal flow.sqlite plus one meeting folder and returns
// the database path.
func fixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"CREATE TABLE History (transcriptEntityId TEXT PRIMARY KEY, asrText TEXT, formattedText TEXT, editedText TEXT, timestamp DATETIME, status TEXT, app TEXT, url TEXT, numWords INTEGER, duration FLOAT, isArchived TINYINT DEFAULT 0, language TEXT, detectedLanguage TEXT, platform TEXT, transcriptCommand TEXT, micDevice TEXT, appVersion TEXT)",
		"CREATE TABLE Meetings (id TEXT PRIMARY KEY, title TEXT, createdAt DATETIME, modifiedAt DATETIME, endedAt INTEGER, recordedMs INTEGER, participantNames TEXT, summary TEXT, notes TEXT, speakerMap TEXT, refineStatus TEXT, importSource TEXT, shareSlug TEXT, isDeleted TINYINT DEFAULT 0, isTourDemo TINYINT DEFAULT 0)",
		"CREATE TABLE Notes (id TEXT PRIMARY KEY, title TEXT, content TEXT, createdAt DATETIME, modifiedAt DATETIME, pinned TINYINT DEFAULT 0, isDeleted TINYINT DEFAULT 0)",
		"CREATE TABLE Dictionary (id TEXT PRIMARY KEY, phrase TEXT, replacement TEXT, isSnippet TINYINT DEFAULT 0, source TEXT, frequencyUsed INTEGER DEFAULT 0, remoteFrequencyUsed INTEGER DEFAULT 0, lastUsed DATETIME, isDeleted TINYINT DEFAULT 0)",
		"CREATE TABLE Todos (id TEXT PRIMARY KEY, meetingId TEXT, title TEXT, status TEXT, isDeleted TINYINT DEFAULT 0, createdAt DATETIME)",
		`INSERT INTO History VALUES
		  ('d1','','Hello, world.','', '2026-09-20 08:00:00.000 +00:00','formatted','com.cmuxterm.app','',2,1.5,0,NULL,'en','darwin','','Mic','1.6.0'),
		  ('d2','','Deploy it now.','', '2026-09-27 11:00:00.000 +00:00','formatted','ru.keepcoder.Telegram','',3,2.0,0,NULL,'en','darwin','','Mic','1.6.0')`,
		`INSERT INTO Meetings VALUES ('aaaa1111-0000-0000-0000-000000000001','Planning sync','2026-09-22 10:00:00.000 +00:00','2026-09-22 11:00:00.000 +00:00',0,3600000,'','Decided X.','', '','complete','','',0,0)`,
		`INSERT INTO Notes VALUES ('nnnn1111-0000-0000-0000-000000000001','Shopping','milk','2026-09-01 10:00:00.000 +00:00','2026-09-02 10:00:00.000 +00:00',0,0)`,
		`INSERT INTO Dictionary VALUES ('k1','btw','by the way',0,'default',3,0,'',0)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	db.Close()
	mdir := filepath.Join(dir, "meetings", "aaaa1111-0000-0000-0000-000000000001")
	if err := os.MkdirAll(mdir, 0o755); err != nil {
		t.Fatal(err)
	}
	refined := "{\"id\":\"u1\",\"timestamp\":\"00:00\",\"text\":\"Hi all.\",\"speaker\":{\"id\":1}}\n"
	if err := os.WriteFile(filepath.Join(mdir, "refined.ndjson"), []byte(refined), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func run(t *testing.T, db string, args ...string) (string, string, error) {
	t.Helper()
	resetFlags(rootCmd)
	var out, errb bytes.Buffer
	stdout, stderr = &out, &errb
	t.Cleanup(func() { stdout, stderr = os.Stdout, os.Stderr })
	now = func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) }
	rootCmd.SetArgs(append([]string{"--db", db, "--utc"}, args...))
	err := rootCmd.Execute()
	return out.String(), errb.String(), err
}

func TestStatus(t *testing.T) {
	db := fixtureDB(t)
	out, _, err := run(t, db, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dictations: 2\n", "meetings: 1\n", "meeting_folders: 1\n", "notes: 1\n", "dictations_last: 2026-09-27\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	out, _, _ = run(t, db, "status", "--json")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil || m["db"] != db {
		t.Fatalf("json status: %v %s", err, out)
	}
}

func TestDictationsList(t *testing.T) {
	db := fixtureDB(t)
	out, _, err := run(t, db, "dictations", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "| time | app | words | text |") || !strings.Contains(out, "| 2026-09-20T08:00:00Z | cmuxterm | 2 | Hello, world. |") {
		t.Fatalf("table:\n%s", out)
	}
	out, _, _ = run(t, db, "dictations", "list", "--since", "3d", "--format", "text")
	if strings.TrimSpace(out) != "2026-09-27 11:00 [Telegram] Deploy it now." {
		t.Fatalf("text since 3d: %q", out)
	}
	out, _, _ = run(t, db, "dictations", "list", "--json", "--fields", "time,text")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 2 || rows[0]["text"] != "Hello, world." || rows[0]["app"] != nil {
		t.Fatalf("json fields: %v %s", err, out)
	}
	if _, _, err := run(t, db, "dictations", "list", "--since", "yesterday"); err == nil {
		t.Fatal("bad --since must error")
	}
	out, _, _ = run(t, db, "history", "list", "--search", "deploy", "--format", "csv")
	if !strings.HasPrefix(out, "time,app,words,text\n2026-09-27T11:00:00Z,Telegram,3,Deploy it now.") {
		t.Fatalf("alias + csv:\n%s", out)
	}
}

func TestMeetings(t *testing.T) {
	db := fixtureDB(t)
	out, _, err := run(t, db, "meetings", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "| aaaa1111 | 2026-09-22T10:00:00Z | 1h00m | Planning sync | 10 |") {
		t.Fatalf("list:\n%s", out)
	}
	out, _, err = run(t, db, "meetings", "show", "aaaa", "--transcript")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"title: Planning sync\n", "## Summary\n\nDecided X.\n", "## Transcript\n\n[00:00] Speaker 1: Hi all.\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	out, _, _ = run(t, db, "meetings", "show", "aaaa", "--transcript", "--format", "text")
	if strings.TrimSpace(out) != "[00:00] Speaker 1: Hi all." {
		t.Fatalf("text transcript: %q", out)
	}
	out, _, _ = run(t, db, "meetings", "show", "aaaa", "--json", "--transcript")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil || m["title"] != "Planning sync" || len(m["transcript"].([]any)) != 1 {
		t.Fatalf("json show: %v %s", err, out)
	}
	if _, _, err := run(t, db, "meetings", "show", "zzz"); err == nil {
		t.Fatal("unknown id must error")
	}
}

func TestExportIdempotent(t *testing.T) {
	db := fixtureDB(t)
	out := filepath.Join(t.TempDir(), "vault")
	paths, errs, err := run(t, db, "export", "--out", out, "--transcript")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errs, "meetings: 1 written, 0 unchanged\n") || !strings.Contains(errs, "dictations: 2 files written, 0 unchanged\n") || !strings.Contains(errs, "notes: 1 written, 0 unchanged\n") {
		t.Fatalf("stderr summary:\n%s", errs)
	}
	if strings.Count(paths, "\n") != 4 {
		t.Fatalf("stdout should list 4 paths:\n%s", paths)
	}
	mfile := filepath.Join(out, "meetings", "2026-09-22-planning-sync-aaaa1111.md")
	b, err := os.ReadFile(mfile)
	if err != nil || !strings.Contains(string(b), "[00:00] Speaker 1: Hi all.") {
		t.Fatalf("meeting file: %v\n%s", err, b)
	}
	if b, err := os.ReadFile(filepath.Join(out, "dictations", "2026-09-27.md")); err != nil || !strings.Contains(string(b), "- **11:00** `Telegram` Deploy it now.") {
		t.Fatalf("dictation file: %v\n%s", err, b)
	}
	if _, err := os.Stat(filepath.Join(out, "notes", "2026-09-01-shopping-nnnn1111.md")); err != nil {
		t.Fatal(err)
	}
	paths, errs, err = run(t, db, "export", "--out", out, "--transcript")
	if err != nil || paths != "" || !strings.Contains(errs, "meetings: 0 written, 1 unchanged") {
		t.Fatalf("second run must be a no-op: %v\n%s\n%s", err, paths, errs)
	}
	_, errs, _ = run(t, db, "export", "--out", out, "--skip", "dictations,notes", "--since", "1d")
	if strings.Contains(errs, "dictations:") || strings.Contains(errs, "notes:") || !strings.Contains(errs, "meetings: 0 written, 0 unchanged") {
		t.Fatalf("skip/since:\n%s", errs)
	}
}

func TestNotesDictionarySQLSchema(t *testing.T) {
	db := fixtureDB(t)
	out, _, _ := run(t, db, "notes", "list")
	if !strings.Contains(out, "| nnnn1111 | 2026-09-02T10:00:00Z | Shopping | 4 |") {
		t.Fatalf("notes list:\n%s", out)
	}
	out, _, _ = run(t, db, "notes", "show", "nnnn", "--format", "text")
	if strings.TrimSpace(out) != "milk" {
		t.Fatalf("note text: %q", out)
	}
	out, _, _ = run(t, db, "dictionary")
	if !strings.Contains(out, "| btw | by the way |") {
		t.Fatalf("dictionary:\n%s", out)
	}
	out, _, _ = run(t, db, "todos")
	if !strings.Contains(out, "_No results._") {
		t.Fatalf("todos:\n%s", out)
	}
	out, _, err := run(t, db, "sql", "SELECT count(*) AS n FROM History")
	if err != nil || !strings.Contains(out, "| n |\n| --- |\n| 2 |") {
		t.Fatalf("sql: %v\n%s", err, out)
	}
	if _, _, err := run(t, db, "sql", "DELETE FROM History"); err == nil {
		t.Fatal("writes must be rejected")
	}
	out, _, _ = run(t, db, "schema")
	if strings.TrimSpace(out) != "Dictionary\nHistory\nMeetings\nNotes\nTodos" {
		t.Fatalf("schema names:\n%s", out)
	}
	out, _, _ = run(t, db, "schema", "notes")
	if !strings.HasPrefix(out, "CREATE TABLE Notes") {
		t.Fatalf("schema one:\n%s", out)
	}
}

func TestNoDBAndSkill(t *testing.T) {
	if _, _, err := run(t, filepath.Join(t.TempDir(), "missing.sqlite"), "status"); err == nil {
		t.Fatal("missing db must error")
	}
	out, _, err := run(t, filepath.Join(t.TempDir(), "missing.sqlite"), "skill")
	if err != nil || !strings.HasPrefix(out, "# wispr") {
		t.Fatalf("skill must not need the db: %v %q", err, out)
	}
	if _, _, err := run(t, "x", "--format", "xml", "skill"); err == nil {
		t.Fatal("bad format must error")
	}
}
