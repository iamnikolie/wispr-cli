package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture builds a small flow.sqlite look-alike with the columns the CLI
// reads, plus a meetings folder with a refined transcript.
func fixture(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		"CREATE TABLE History (transcriptEntityId TEXT PRIMARY KEY, asrText TEXT, formattedText TEXT, editedText TEXT, timestamp DATETIME, status TEXT, app TEXT, url TEXT, numWords INTEGER, duration FLOAT, isArchived TINYINT DEFAULT 0, language TEXT, detectedLanguage TEXT, platform TEXT, transcriptCommand TEXT, micDevice TEXT, appVersion TEXT, audio BLOB)",
		"CREATE TABLE Meetings (id TEXT PRIMARY KEY, title TEXT, createdAt DATETIME, modifiedAt DATETIME, endedAt INTEGER, recordedMs INTEGER, participantNames TEXT, summary TEXT, notes TEXT, speakerMap TEXT, refineStatus TEXT, importSource TEXT, shareSlug TEXT, isDeleted TINYINT DEFAULT 0, isTourDemo TINYINT DEFAULT 0)",
		"CREATE TABLE Notes (id TEXT PRIMARY KEY, title TEXT, content TEXT, createdAt DATETIME, modifiedAt DATETIME, pinned TINYINT DEFAULT 0, isDeleted TINYINT DEFAULT 0)",
		"CREATE TABLE Dictionary (id TEXT PRIMARY KEY, phrase TEXT, replacement TEXT, isSnippet TINYINT DEFAULT 0, source TEXT, frequencyUsed INTEGER DEFAULT 0, remoteFrequencyUsed INTEGER DEFAULT 0, lastUsed DATETIME, isDeleted TINYINT DEFAULT 0)",
		"CREATE TABLE Todos (id TEXT PRIMARY KEY, meetingId TEXT, title TEXT, status TEXT, isDeleted TINYINT DEFAULT 0, createdAt DATETIME)",
		`INSERT INTO History VALUES
		  ('d1','hello world raw','Hello, world.','', '2026-09-20 08:00:00.000 +00:00','formatted','com.example.terminal','',2,1.5,0,NULL,'en','darwin','','Mic','1.6.0',NULL),
		  ('d2','raw only','', '', '2026-09-21 09:30:00.000 +00:00','raw_transcript','com.example.editor','',2,1.0,0,NULL,'ru','darwin','','Mic','1.6.0',NULL),
		  ('d3','','dismissed text','', '2026-09-21 10:00:00.000 +00:00','dismissed','com.google.Chrome','',2,1.0,0,NULL,'en','darwin','','Mic','1.6.0',NULL),
		  ('d4','','archived text','', '2026-09-22 10:00:00.000 +00:00','formatted','com.google.Chrome','',2,1.0,1,NULL,'en','darwin','','Mic','1.6.0',NULL),
		  ('d5','','Deploy it now.   ','', '2026-09-23 11:00:00.000 +00:00','formatted','ru.keepcoder.Telegram','',3,2.0,0,NULL,'en','darwin','','Mic','1.6.0',X'0102')`,
		`INSERT INTO Meetings VALUES
		  ('aaaa1111-0000-0000-0000-000000000001','Planning sync','2026-09-22 10:00:00.000 +00:00','2026-09-22 11:00:00.000 +00:00',1790000000000,3600000,'','Decided X. <@speaker:2> owns Y.','', '{"people":{"p1":{"name":"Mykola K","origin":"self"}},"assignments":{"2":{"consensus":"p1"}}}','complete','','',0,0),
		  ('aaaa2222-0000-0000-0000-000000000002','Deleted one','2026-09-10 10:00:00.000 +00:00','2026-09-10 10:00:00.000 +00:00',0,0,'','gone','','','complete','','',1,0),
		  ('aaaa3333-0000-0000-0000-000000000003','Tour demo','2026-09-01 10:00:00.000 +00:00','2026-09-01 10:00:00.000 +00:00',0,0,'','demo','','','complete','','',0,1)`,
		`INSERT INTO Notes VALUES ('nnnn1111-0000-0000-0000-000000000001','Shopping','milk' || char(10) || 'eggs','2026-09-01 10:00:00.000 +00:00','2026-09-02 10:00:00.000 +00:00',1,0)`,
		`INSERT INTO Dictionary VALUES ('k1','btw','by the way',0,'default',3,0,'2026-09-01 10:00:00.000 +00:00',0), ('k2','Zerg','',0,'manual',0,5,'',0), ('k3','old','',0,'manual',9,0,'',1)`,
		`INSERT INTO Todos VALUES ('t1','aaaa1111-0000-0000-0000-000000000001','Ship it','open',0,'2026-09-22 11:00:00.000 +00:00')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	db.Close()
	mdir := filepath.Join(dir, "meetings", "aaaa1111-0000-0000-0000-000000000001")
	if err := os.MkdirAll(mdir, 0o755); err != nil {
		t.Fatal(err)
	}
	refined := `{"id":"u1","timestamp":"00:00","text":"Hi all.","speaker":{"id":2,"source":"refined","name":null}}
{"id":"u2","timestamp":"00:05","text":"Hello.","speaker":{"id":3,"source":"refined","name":null}}
`
	if err := os.WriteFile(filepath.Join(mdir, "refined.ndjson"), []byte(refined), 0o644); err != nil {
		t.Fatal(err)
	}
	live := `{"meta":{"v":3}}
{"id":"l1","timestamp":"00:00","text":"Hi all live.","speaker":"2"}
`
	if err := os.WriteFile(filepath.Join(mdir, "live.ndjson"), []byte(live), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mdir, "upload.ogg"), []byte("ogg"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenMissing(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "nope.sqlite"))
	if err == nil || !strings.Contains(err.Error(), "--db") {
		t.Fatalf("want helpful missing-file error, got %v", err)
	}
}

func TestReadOnly(t *testing.T) {
	s := fixture(t)
	if _, err := s.DB.Exec("DELETE FROM Todos"); err == nil {
		t.Fatal("write succeeded on a read-only store")
	}
}

func TestDictationsDefaults(t *testing.T) {
	s := fixture(t)
	ds, err := s.Dictations(context.Background(), DictationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, d := range ds {
		ids = append(ids, d.ID)
	}
	if got := strings.Join(ids, ","); got != "d1,d2,d5" {
		t.Fatalf("default listing = %s, want d1,d2,d5 (no dismissed, no archived, oldest first)", got)
	}
	if ds[1].Text != "raw only" {
		t.Fatalf("raw_transcript row should fall back to asrText, got %q", ds[1].Text)
	}
	if ds[2].Text != "Deploy it now." {
		t.Fatalf("text not trimmed: %q", ds[2].Text)
	}
	if !ds[0].Time.Equal(time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("time parse: %v", ds[0].Time)
	}
}

func TestDictationsFilters(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	since := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	ds, _ := s.Dictations(ctx, DictationFilter{Range: Range{Since: since}})
	if len(ds) != 2 {
		t.Fatalf("since filter: got %d rows", len(ds))
	}
	ds, _ = s.Dictations(ctx, DictationFilter{App: "telegram"})
	if len(ds) != 1 || ds[0].ID != "d5" {
		t.Fatalf("app filter: %+v", ds)
	}
	ds, _ = s.Dictations(ctx, DictationFilter{Search: "deploy"})
	if len(ds) != 1 {
		t.Fatalf("search filter: %+v", ds)
	}
	ds, _ = s.Dictations(ctx, DictationFilter{Statuses: []string{"all"}, Archived: true})
	if len(ds) != 5 {
		t.Fatalf("status all + archived: got %d rows", len(ds))
	}
	ds, _ = s.Dictations(ctx, DictationFilter{Desc: true, Limit: 1})
	if len(ds) != 1 || ds[0].ID != "d5" {
		t.Fatalf("desc+limit: %+v", ds)
	}
}

func TestDictationStats(t *testing.T) {
	s := fixture(t)
	st, err := s.DictationStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Count != 3 || st.Words != 7 || st.First.Day() != 20 || st.Last.Day() != 23 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestMeetings(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	ms, err := s.Meetings(ctx, MeetingFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || ms[0].Title != "Planning sync" {
		t.Fatalf("deleted and tour-demo meetings must be hidden: %+v", ms)
	}
	if ms[0].Summary != "Decided X. Mykola K owns Y." {
		t.Fatalf("speaker ref not resolved: %q", ms[0].Summary)
	}
	if ms[0].Duration() != time.Hour {
		t.Fatalf("duration: %v", ms[0].Duration())
	}
	ms, _ = s.Meetings(ctx, MeetingFilter{Deleted: true})
	if len(ms) != 2 {
		t.Fatalf("with deleted: %d", len(ms))
	}
	m, err := s.Meeting(ctx, "aaaa1")
	if err != nil || m.ID != ms[1].ID && m.ID != ms[0].ID {
		t.Fatalf("prefix lookup: %v %+v", err, m)
	}
	if _, err := s.Meeting(ctx, "aaaa"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("want ambiguous error, got %v", err)
	}
	if _, err := s.Meeting(ctx, "zzz"); err == nil {
		t.Fatal("want not-found error")
	}
}

func TestTranscript(t *testing.T) {
	s := fixture(t)
	m, _ := s.Meeting(context.Background(), "aaaa1111")
	tr, err := s.Transcript(m, Refined)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr) != 2 || tr[0].Speaker != "Mykola K" || tr[1].Speaker != "Speaker 3" || tr[1].Timestamp != "00:05" {
		t.Fatalf("refined: %+v", tr)
	}
	live, err := s.Transcript(m, Live)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 || live[0].Speaker != "Mykola K" || live[0].Text != "Hi all live." {
		t.Fatalf("live (meta line skipped, string speaker resolved): %+v", live)
	}
	if s.AudioPath(m.ID) == "" {
		t.Fatal("audio path not found")
	}
	m2 := Meeting{ID: "missing"}
	if _, err := s.Transcript(m2, Refined); err == nil || !strings.Contains(err.Error(), "no transcript") {
		t.Fatalf("want ErrNoTranscript, got %v", err)
	}
}

func TestNotesDictionaryTodos(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	ns, err := s.Notes(ctx, NoteFilter{})
	if err != nil || len(ns) != 1 || !ns[0].Pinned || ns[0].Content != "milk\neggs" {
		t.Fatalf("notes: %v %+v", err, ns)
	}
	n, err := s.Note(ctx, "nnnn")
	if err != nil || n.Title != "Shopping" {
		t.Fatalf("note prefix: %v %+v", err, n)
	}
	es, err := s.Dictionary(ctx)
	if err != nil || len(es) != 2 || es[0].Phrase != "Zerg" || es[0].Used != 5 || es[1].Replacement != "by the way" {
		t.Fatalf("dictionary (deleted hidden, most used first): %v %+v", err, es)
	}
	ts, err := s.Todos(ctx)
	if err != nil || len(ts) != 1 || ts[0].Status != "open" {
		t.Fatalf("todos: %v %+v", err, ts)
	}
}

func TestSchemaAndQuery(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	tables, err := s.Schema(ctx, "")
	if err != nil || len(tables) != 5 {
		t.Fatalf("schema: %v %d", err, len(tables))
	}
	one, _ := s.Schema(ctx, "history")
	if len(one) != 1 || one[0].Name != "History" {
		t.Fatalf("case-insensitive single table: %+v", one)
	}
	cols, rows, err := s.Query(ctx, "SELECT app, audio FROM History WHERE transcriptEntityId='d5'")
	if err != nil || len(cols) != 2 || len(rows) != 1 {
		t.Fatalf("query: %v %v %v", err, cols, rows)
	}
	if rows[0]["audio"] != "<blob 2 bytes>" {
		t.Fatalf("blob should be summarised, got %v", rows[0]["audio"])
	}
	if s.Count(ctx, "Nope") != -1 || s.Count(ctx, "Todos") != 1 {
		t.Fatal("count")
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"":                          {},
		"24h":                       now.Add(-24 * time.Hour),
		"7d":                        now.Add(-7 * 24 * time.Hour),
		"2w":                        now.Add(-14 * 24 * time.Hour),
		"1mo":                       now.Add(-30 * 24 * time.Hour),
		"30m":                       now.Add(-30 * time.Minute),
		"2026-09-01":                time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		"2026-09-01T10:30":          time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC),
		"2026-09-01T10:30:00+02:00": time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC),
	}
	for in, want := range cases {
		got, err := ParseSince(in, now)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if !got.Equal(want) {
			t.Fatalf("%q: got %v want %v", in, got, want)
		}
	}
	for _, bad := range []string{"yesterday", "-3d", "3x"} {
		if _, err := ParseSince(bad, now); err == nil {
			t.Fatalf("%q should fail", bad)
		}
	}
}

func TestParseTimeFormats(t *testing.T) {
	for _, in := range []string{"2026-06-05 07:34:51.566 +00:00", "2026-06-05 07:34:51 +00:00", "2026-06-05T07:34:51Z", "2026-06-05 07:34:51"} {
		if got := ParseTime(in); got.Year() != 2026 || got.Hour() != 7 {
			t.Fatalf("%q parsed as %v", in, got)
		}
	}
	if !ParseTime("garbage").IsZero() {
		t.Fatal("garbage should be zero")
	}
	if FormatTime(time.Date(2026, 6, 5, 7, 34, 51, 566e6, time.UTC)) != "2026-06-05 07:34:51.566 +00:00" {
		t.Fatal("FormatTime must round-trip the stored layout")
	}
}

func TestSpeakerHelpers(t *testing.T) {
	m := Meeting{SpeakerMap: `{"people":{"p1":{"name":"Ann"}},"assignments":{"1":{"user":"p1"},"2":{"mic":"nobody"}}}`}
	if m.SpeakerName("1") != "Ann" || m.SpeakerName("2") != "Speaker 2" || m.SpeakerName("x") != "x" {
		t.Fatal("speaker names")
	}
	if got := m.ResolveSpeakerRefs("<@speaker:1> and <@speaker:7>"); got != "Ann and Speaker 7" {
		t.Fatalf("refs: %q", got)
	}
	if splitNames(`["A","B"]`)[1] != "B" || splitNames("A, B")[1] != "B" || splitNames("") != nil {
		t.Fatal("splitNames")
	}
}
