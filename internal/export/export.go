// Package export renders store records as Markdown files for a notes vault.
package export

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/iamnikolie/wispr-cli/internal/store"
)

// Clock formats times for file names and headers; the CLI injects local/UTC.
type Clock struct {
	UTC bool
}

func (c Clock) in(t time.Time) time.Time {
	if c.UTC {
		return t.UTC()
	}
	return t.Local()
}

// Day is the calendar day used for file names.
func (c Clock) Day(t time.Time) string {
	if t.IsZero() {
		return "undated"
	}
	return c.in(t).Format("2006-01-02")
}

// Stamp is a full timestamp for front matter.
func (c Clock) Stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if c.UTC {
		return t.UTC().Format("2006-01-02T15:04:05Z")
	}
	return t.Local().Format("2006-01-02T15:04:05-07:00")
}

// Slug makes a file-name-safe fragment from a title. Unicode letters are
// kept (Cyrillic titles stay readable); punctuation and spaces become "-".
func Slug(s string, max int) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if max > 0 {
		r := []rune(out)
		if len(r) > max {
			out = strings.Trim(string(r[:max]), "-")
		}
	}
	if out == "" {
		return "untitled"
	}
	return out
}

func yamlStr(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#\"'\n[]{},&*!|>%@`") || strings.TrimSpace(s) != s {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
	}
	return s
}

// MeetingMarkdown renders one meeting: front matter, summary, notes and an
// optional transcript.
func MeetingMarkdown(c Clock, m store.Meeting, transcript []store.Utterance, audio string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", m.ID)
	fmt.Fprintf(&b, "title: %s\n", yamlStr(m.Title))
	fmt.Fprintf(&b, "created: %s\n", c.Stamp(m.Created))
	if d := m.Duration(); d > 0 {
		fmt.Fprintf(&b, "duration: %s\n", Duration(d))
	}
	if len(m.Participants) > 0 {
		parts := make([]string, len(m.Participants))
		for i, p := range m.Participants {
			parts[i] = yamlStr(p)
		}
		fmt.Fprintf(&b, "participants: [%s]\n", strings.Join(parts, ", "))
	}
	if audio != "" {
		fmt.Fprintf(&b, "audio: %s\n", yamlStr(audio))
	}
	b.WriteString("source: wispr-flow\n")
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", strings.TrimSpace(m.Title))
	if s := strings.TrimSpace(m.Summary); s != "" {
		b.WriteString("## Summary\n\n")
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	if n := strings.TrimSpace(m.Notes); n != "" {
		b.WriteString("## Notes\n\n")
		b.WriteString(n)
		b.WriteString("\n\n")
	}
	if len(transcript) > 0 {
		b.WriteString("## Transcript\n\n")
		b.WriteString(TranscriptText(transcript))
		b.WriteString("\n")
	}
	return b.String()
}

// Duration renders a length as 1h05m, 12m30s or 45s.
func Duration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// TranscriptText renders utterances as "[mm:ss] Speaker: text" lines.
func TranscriptText(u []store.Utterance) string {
	var b strings.Builder
	for _, x := range u {
		if x.Timestamp != "" {
			fmt.Fprintf(&b, "[%s] ", x.Timestamp)
		}
		if x.Speaker != "" {
			fmt.Fprintf(&b, "%s: ", x.Speaker)
		}
		b.WriteString(strings.TrimSpace(x.Text))
		b.WriteByte('\n')
	}
	return b.String()
}

// MeetingFile is the file name for a meeting inside the meetings folder.
func MeetingFile(c Clock, m store.Meeting) string {
	return fmt.Sprintf("%s-%s-%s.md", c.Day(m.Created), Slug(m.Title, 60), shortID(m.ID))
}

// NoteMarkdown renders one Flow note.
func NoteMarkdown(c Clock, n store.Note) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", n.ID)
	fmt.Fprintf(&b, "title: %s\n", yamlStr(n.Title))
	fmt.Fprintf(&b, "created: %s\n", c.Stamp(n.Created))
	fmt.Fprintf(&b, "modified: %s\n", c.Stamp(n.Modified))
	if n.Pinned {
		b.WriteString("pinned: true\n")
	}
	b.WriteString("source: wispr-flow\n")
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", strings.TrimSpace(n.Title))
	b.WriteString(strings.TrimSpace(n.Content))
	b.WriteString("\n")
	return b.String()
}

// NoteFile is the file name for a note inside the notes folder.
func NoteFile(c Clock, n store.Note) string {
	return fmt.Sprintf("%s-%s-%s.md", c.Day(n.Created), Slug(n.Title, 60), shortID(n.ID))
}

// DictationsByDay groups dictations into one Markdown document per day,
// keyed by the file name.
func DictationsByDay(c Clock, ds []store.Dictation) map[string]string {
	byDay := map[string][]store.Dictation{}
	for _, d := range ds {
		day := c.Day(d.Time)
		byDay[day] = append(byDay[day], d)
	}
	out := make(map[string]string, len(byDay))
	for day, list := range byDay {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Time.Before(list[j].Time) })
		out[day+".md"] = DictationsMarkdown(c, day, list)
	}
	return out
}

// DictationsMarkdown renders one day's dictations as a bullet list.
func DictationsMarkdown(c Clock, day string, ds []store.Dictation) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "date: %s\n", day)
	fmt.Fprintf(&b, "dictations: %d\n", len(ds))
	var words int64
	for _, d := range ds {
		words += d.Words
	}
	fmt.Fprintf(&b, "words: %d\n", words)
	b.WriteString("source: wispr-flow\n")
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# Dictations %s\n\n", day)
	for _, d := range ds {
		t := c.in(d.Time).Format("15:04")
		text := strings.TrimSpace(d.Text)
		lines := strings.Split(text, "\n")
		fmt.Fprintf(&b, "- **%s** `%s` %s\n", t, appLabel(d.App), strings.TrimSpace(lines[0]))
		for _, l := range lines[1:] {
			fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(l))
		}
	}
	return b.String()
}

// appLabel shortens a bundle id ("com.example.terminal" → "terminal") for the
// inline tag; the full id stays available in list/json output.
func appLabel(app string) string {
	if app == "" {
		return "?"
	}
	parts := strings.Split(app, ".")
	if len(parts) < 2 {
		return app
	}
	last := parts[len(parts)-1]
	if strings.EqualFold(last, "app") && len(parts) >= 3 {
		return parts[len(parts)-2]
	}
	return last
}

// AppLabel is exported for the CLI's text listing.
func AppLabel(app string) string { return appLabel(app) }

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Write stores content at dir/name, creating dir. It reports whether the
// file changed so callers can count real updates.
func Write(dir, name, content string) (string, bool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	path := filepath.Join(dir, name)
	if old, err := os.ReadFile(path); err == nil && string(old) == content {
		return path, false, nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", false, err
	}
	return path, true, nil
}
