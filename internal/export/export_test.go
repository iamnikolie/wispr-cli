package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamnikolie/wispr-cli/internal/store"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Creating Single Source of Truth": "creating-single-source-of-truth",
		"  Разбор: долги / цены!  ":       "разбор-долги-цены",
		"": "untitled",
		"A very long title that keeps going and going": "a-very-long-title",
	}
	for in, want := range cases {
		max := 60
		if strings.HasPrefix(in, "A very") {
			max = 18
		}
		if got := Slug(in, max); got != want {
			t.Fatalf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDuration(t *testing.T) {
	if Duration(0) != "" || Duration(45*time.Second) != "45s" || Duration(12*time.Minute+30*time.Second) != "12m30s" || Duration(time.Hour+5*time.Minute) != "1h05m" {
		t.Fatal("duration formats")
	}
}

func TestMeetingMarkdown(t *testing.T) {
	c := Clock{UTC: true}
	m := store.Meeting{
		ID: "abcdefgh-1234", Title: "Sync: plan", Created: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC),
		RecordedMs: 90000, Participants: []string{"Ann", "Bob"}, Summary: "Decided.", Notes: "my note",
	}
	tr := []store.Utterance{{Timestamp: "00:00", Speaker: "Ann", Text: "Hi"}, {Text: "no meta"}}
	md := MeetingMarkdown(c, m, tr, "/x/upload.ogg")
	for _, want := range []string{
		"id: abcdefgh-1234\n", `title: "Sync: plan"`, "created: 2026-09-22T10:00:00Z\n", "duration: 1m30s\n",
		"participants: [Ann, Bob]\n", "audio: /x/upload.ogg\n", "# Sync: plan\n", "## Summary\n\nDecided.\n",
		"## Notes\n\nmy note\n", "## Transcript\n\n[00:00] Ann: Hi\nno meta\n",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in:\n%s", want, md)
		}
	}
	if MeetingFile(c, m) != "2026-09-22-sync-plan-abcdefgh.md" {
		t.Fatalf("file name: %s", MeetingFile(c, m))
	}
	plain := MeetingMarkdown(c, store.Meeting{ID: "x", Title: "T"}, nil, "")
	if strings.Contains(plain, "## Summary") || strings.Contains(plain, "## Transcript") || strings.Contains(plain, "audio:") {
		t.Fatalf("empty sections must be omitted:\n%s", plain)
	}
}

func TestDictationsByDay(t *testing.T) {
	c := Clock{UTC: true}
	ds := []store.Dictation{
		{Time: time.Date(2026, 9, 22, 10, 5, 0, 0, time.UTC), App: "com.cmuxterm.app", Words: 2, Text: "second\nline two"},
		{Time: time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC), App: "com.google.Chrome", Words: 1, Text: "first"},
		{Time: time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC), App: "Slack", Words: 1, Text: "next day"},
	}
	files := DictationsByDay(c, ds)
	if len(files) != 2 {
		t.Fatalf("files: %v", files)
	}
	day := files["2026-09-22.md"]
	if !strings.Contains(day, "dictations: 2\nwords: 3\n") {
		t.Fatalf("front matter:\n%s", day)
	}
	if i, j := strings.Index(day, "**09:00** `Chrome` first"), strings.Index(day, "**10:05** `cmuxterm` second\n  line two"); i < 0 || j < 0 || i > j {
		t.Fatalf("ordering or formatting wrong:\n%s", day)
	}
	if !strings.Contains(files["2026-09-23.md"], "`Slack` next day") {
		t.Fatalf("plain app name:\n%s", files["2026-09-23.md"])
	}
}

func TestNoteMarkdownAndWrite(t *testing.T) {
	c := Clock{UTC: true}
	n := store.Note{ID: "n1", Title: "Shopping", Content: "milk", Pinned: true, Created: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	md := NoteMarkdown(c, n)
	if !strings.Contains(md, "pinned: true\n") || !strings.HasSuffix(md, "# Shopping\n\nmilk\n") {
		t.Fatalf("note md:\n%s", md)
	}
	dir := filepath.Join(t.TempDir(), "out")
	p, changed, err := Write(dir, NoteFile(c, n), md)
	if err != nil || !changed || filepath.Base(p) != "2026-09-01-shopping-n1.md" {
		t.Fatalf("write: %v %v %s", err, changed, p)
	}
	if _, changed, _ = Write(dir, NoteFile(c, n), md); changed {
		t.Fatal("identical content must report unchanged")
	}
	if _, changed, _ = Write(dir, NoteFile(c, n), md+"x"); !changed {
		t.Fatal("new content must report changed")
	}
	if b, _ := os.ReadFile(p); string(b) != md+"x" {
		t.Fatal("file not overwritten")
	}
}
