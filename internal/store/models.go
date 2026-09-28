package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Dictation is one entry of the History table: a single Flow dictation.
type Dictation struct {
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	App       string    `json:"app"`
	URL       string    `json:"url,omitempty"`
	Status    string    `json:"status"`
	Language  string    `json:"language,omitempty"`
	Words     int64     `json:"words"`
	Duration  float64   `json:"duration_s"`
	Text      string    `json:"text"`
	ASR       string    `json:"asr,omitempty"`
	Edited    string    `json:"edited,omitempty"`
	Platform  string    `json:"platform,omitempty"`
	Command   string    `json:"command,omitempty"`
	MicDevice string    `json:"mic,omitempty"`
}

// DictationFilter narrows a History listing.
type DictationFilter struct {
	Range    Range
	App      string // substring match on the bundle id / app name
	Search   string // substring match on text
	Statuses []string
	Archived bool // include archived rows
	Limit    int
	Desc     bool
}

// DefaultStatuses are the rows with real text: what Flow pasted (formatted)
// and what it captured without formatting (raw_transcript). Dismissed, empty
// and in-flight rows are noise for an export.
var DefaultStatuses = []string{"formatted", "raw_transcript"}

// Dictations lists History rows matching f, oldest first unless f.Desc.
func (s *Store) Dictations(ctx context.Context, f DictationFilter) ([]Dictation, error) {
	q := `SELECT transcriptEntityId, COALESCE(timestamp,''), COALESCE(app,''), COALESCE(url,''),
	  COALESCE(status,''), COALESCE(detectedLanguage, language, ''), COALESCE(numWords,0), COALESCE(duration,0),
	  COALESCE(formattedText,''), COALESCE(asrText,''), COALESCE(editedText,''),
	  COALESCE(platform,''), COALESCE(transcriptCommand,''), COALESCE(micDevice,'')
	FROM History WHERE 1=1`
	var args []any
	if !f.Archived {
		q += " AND COALESCE(isArchived,0) = 0"
	}
	w, a := f.Range.where("timestamp")
	q += w
	args = append(args, a...)
	statuses := f.Statuses
	if len(statuses) == 0 {
		statuses = DefaultStatuses
	}
	if !(len(statuses) == 1 && statuses[0] == "all") {
		ph := strings.TrimRight(strings.Repeat("?,", len(statuses)), ",")
		q += " AND COALESCE(status,'') IN (" + ph + ")"
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	if f.App != "" {
		q += " AND app LIKE ?"
		args = append(args, "%"+f.App+"%")
	}
	if f.Search != "" {
		q += " AND (formattedText LIKE ? OR asrText LIKE ? OR editedText LIKE ?)"
		p := "%" + f.Search + "%"
		args = append(args, p, p, p)
	}
	q += " ORDER BY timestamp"
	if f.Desc {
		q += " DESC"
	}
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query History: %w", err)
	}
	defer rows.Close()
	var out []Dictation
	for rows.Next() {
		var d Dictation
		var ts string
		if err := rows.Scan(&d.ID, &ts, &d.App, &d.URL, &d.Status, &d.Language, &d.Words, &d.Duration,
			&d.Text, &d.ASR, &d.Edited, &d.Platform, &d.Command, &d.MicDevice); err != nil {
			return nil, err
		}
		d.Time = ParseTime(ts)
		if strings.TrimSpace(d.Text) == "" {
			d.Text = d.ASR
		}
		d.Text = strings.TrimSpace(d.Text)
		out = append(out, d)
	}
	return out, rows.Err()
}

// DictationStats summarises the History table for `status`.
type DictationStats struct {
	Count int64
	Words int64
	First time.Time
	Last  time.Time
}

// DictationStats returns totals over non-archived rows with text.
func (s *Store) DictationStats(ctx context.Context) (DictationStats, error) {
	var st DictationStats
	var first, last sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT count(*), COALESCE(sum(numWords),0), min(timestamp), max(timestamp)
		FROM History WHERE COALESCE(isArchived,0)=0 AND status IN ('formatted','raw_transcript')`).
		Scan(&st.Count, &st.Words, &first, &last)
	if err != nil {
		return st, err
	}
	st.First = ParseTime(first.String)
	st.Last = ParseTime(last.String)
	return st, nil
}

// Meeting is a Notetaker meeting: AI summary, user notes, transcript files.
type Meeting struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Created      time.Time `json:"created"`
	Modified     time.Time `json:"modified"`
	Ended        time.Time `json:"ended,omitempty"`
	RecordedMs   int64     `json:"recorded_ms"`
	Participants []string  `json:"participants,omitempty"`
	Summary      string    `json:"summary"`
	Notes        string    `json:"notes,omitempty"`
	SpeakerMap   string    `json:"-"`
	RefineStatus string    `json:"refine_status,omitempty"`
	ImportSource string    `json:"import_source,omitempty"`
	ShareSlug    string    `json:"share_slug,omitempty"`
	Deleted      bool      `json:"deleted,omitempty"`
}

// Duration is the recorded length.
func (m Meeting) Duration() time.Duration {
	return time.Duration(m.RecordedMs) * time.Millisecond
}

// MeetingFilter narrows a Meetings listing.
type MeetingFilter struct {
	Range   Range
	Search  string // substring on title, summary or notes
	Deleted bool   // include deleted rows
	Limit   int
	Desc    bool
}

const meetingCols = `id, COALESCE(title,''), COALESCE(createdAt,''), COALESCE(modifiedAt,''), COALESCE(endedAt,0),
	COALESCE(recordedMs,0), COALESCE(participantNames,''), COALESCE(summary,''), COALESCE(notes,''),
	COALESCE(speakerMap,''), COALESCE(refineStatus,''), COALESCE(importSource,''), COALESCE(shareSlug,''), COALESCE(isDeleted,0)`

func scanMeeting(sc interface{ Scan(...any) error }) (Meeting, error) {
	var m Meeting
	var created, modified, participants string
	var ended int64
	var deleted int
	if err := sc.Scan(&m.ID, &m.Title, &created, &modified, &ended, &m.RecordedMs, &participants,
		&m.Summary, &m.Notes, &m.SpeakerMap, &m.RefineStatus, &m.ImportSource, &m.ShareSlug, &deleted); err != nil {
		return m, err
	}
	m.Created = ParseTime(created)
	m.Modified = ParseTime(modified)
	if ended > 0 {
		m.Ended = time.UnixMilli(ended).UTC()
	}
	m.Participants = splitNames(participants)
	m.Deleted = deleted != 0
	m.Summary = m.ResolveSpeakerRefs(m.Summary)
	m.Notes = m.ResolveSpeakerRefs(m.Notes)
	return m, nil
}

func splitNames(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "[") {
		return parseJSONStrings(s)
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Meetings lists Notetaker meetings, oldest first unless f.Desc.
func (s *Store) Meetings(ctx context.Context, f MeetingFilter) ([]Meeting, error) {
	q := "SELECT " + meetingCols + " FROM Meetings WHERE COALESCE(isTourDemo,0)=0"
	var args []any
	if !f.Deleted {
		q += " AND COALESCE(isDeleted,0)=0"
	}
	w, a := f.Range.where("createdAt")
	q += w
	args = append(args, a...)
	if f.Search != "" {
		q += " AND (title LIKE ? OR summary LIKE ? OR notes LIKE ?)"
		p := "%" + f.Search + "%"
		args = append(args, p, p, p)
	}
	q += " ORDER BY createdAt"
	if f.Desc {
		q += " DESC"
	}
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query Meetings: %w", err)
	}
	defer rows.Close()
	var out []Meeting
	for rows.Next() {
		m, err := scanMeeting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Meeting resolves an id or unique id prefix.
func (s *Store) Meeting(ctx context.Context, idPrefix string) (Meeting, error) {
	q := "SELECT " + meetingCols + " FROM Meetings WHERE id LIKE ? ORDER BY createdAt DESC LIMIT 2"
	rows, err := s.DB.QueryContext(ctx, q, idPrefix+"%")
	if err != nil {
		return Meeting{}, fmt.Errorf("query Meetings: %w", err)
	}
	defer rows.Close()
	var found []Meeting
	for rows.Next() {
		m, err := scanMeeting(rows)
		if err != nil {
			return Meeting{}, err
		}
		found = append(found, m)
	}
	if err := rows.Err(); err != nil {
		return Meeting{}, err
	}
	switch len(found) {
	case 0:
		return Meeting{}, fmt.Errorf("no meeting with id starting %q (see `wispr meetings list`)", idPrefix)
	case 1:
		return found[0], nil
	default:
		return Meeting{}, fmt.Errorf("id prefix %q is ambiguous; give more characters", idPrefix)
	}
}

// Note is a Flow note (the Notes tab of the app).
type Note struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
	Pinned   bool      `json:"pinned,omitempty"`
	Deleted  bool      `json:"deleted,omitempty"`
	Content  string    `json:"content"`
}

// NoteFilter narrows a Notes listing.
type NoteFilter struct {
	Range   Range
	Search  string
	Deleted bool
	Limit   int
	Desc    bool
}

const noteCols = `id, COALESCE(title,''), COALESCE(createdAt,''), COALESCE(modifiedAt,''), COALESCE(pinned,0), COALESCE(isDeleted,0), COALESCE(content,'')`

func scanNote(sc interface{ Scan(...any) error }) (Note, error) {
	var n Note
	var created, modified string
	var pinned, deleted int
	if err := sc.Scan(&n.ID, &n.Title, &created, &modified, &pinned, &deleted, &n.Content); err != nil {
		return n, err
	}
	n.Created = ParseTime(created)
	n.Modified = ParseTime(modified)
	n.Pinned = pinned != 0
	n.Deleted = deleted != 0
	return n, nil
}

// Notes lists Flow notes, oldest first unless f.Desc.
func (s *Store) Notes(ctx context.Context, f NoteFilter) ([]Note, error) {
	q := "SELECT " + noteCols + " FROM Notes WHERE 1=1"
	var args []any
	if !f.Deleted {
		q += " AND COALESCE(isDeleted,0)=0"
	}
	w, a := f.Range.where("modifiedAt")
	q += w
	args = append(args, a...)
	if f.Search != "" {
		q += " AND (title LIKE ? OR content LIKE ?)"
		p := "%" + f.Search + "%"
		args = append(args, p, p)
	}
	q += " ORDER BY modifiedAt"
	if f.Desc {
		q += " DESC"
	}
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query Notes: %w", err)
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Note resolves an id or unique id prefix.
func (s *Store) Note(ctx context.Context, idPrefix string) (Note, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+noteCols+" FROM Notes WHERE id LIKE ? LIMIT 2", idPrefix+"%")
	if err != nil {
		return Note{}, fmt.Errorf("query Notes: %w", err)
	}
	defer rows.Close()
	var found []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return Note{}, err
		}
		found = append(found, n)
	}
	if err := rows.Err(); err != nil {
		return Note{}, err
	}
	switch len(found) {
	case 0:
		return Note{}, fmt.Errorf("no note with id starting %q (see `wispr notes list`)", idPrefix)
	case 1:
		return found[0], nil
	default:
		return Note{}, fmt.Errorf("id prefix %q is ambiguous; give more characters", idPrefix)
	}
}

// DictionaryEntry is a personal-dictionary word or snippet.
type DictionaryEntry struct {
	Phrase      string    `json:"phrase"`
	Replacement string    `json:"replacement,omitempty"`
	Snippet     bool      `json:"snippet,omitempty"`
	Source      string    `json:"source,omitempty"`
	Used        int64     `json:"used"`
	LastUsed    time.Time `json:"last_used,omitempty"`
}

// Dictionary lists non-deleted dictionary entries, most used first.
func (s *Store) Dictionary(ctx context.Context) ([]DictionaryEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT COALESCE(phrase,''), COALESCE(replacement,''), COALESCE(isSnippet,0),
		COALESCE(source,''), COALESCE(frequencyUsed,0)+COALESCE(remoteFrequencyUsed,0), COALESCE(lastUsed,'')
		FROM Dictionary WHERE COALESCE(isDeleted,0)=0 ORDER BY 5 DESC, phrase`)
	if err != nil {
		return nil, fmt.Errorf("query Dictionary: %w", err)
	}
	defer rows.Close()
	var out []DictionaryEntry
	for rows.Next() {
		var e DictionaryEntry
		var snippet int
		var last string
		if err := rows.Scan(&e.Phrase, &e.Replacement, &snippet, &e.Source, &e.Used, &last); err != nil {
			return nil, err
		}
		e.Snippet = snippet != 0
		e.LastUsed = ParseTime(last)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Todo is a Notetaker action item.
type Todo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	MeetingID string    `json:"meeting_id,omitempty"`
	Created   time.Time `json:"created"`
}

// Todos lists open and done action items, newest first.
func (s *Store) Todos(ctx context.Context) ([]Todo, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, COALESCE(title,''), COALESCE(status,''), COALESCE(meetingId,''), COALESCE(createdAt,'')
		FROM Todos WHERE COALESCE(isDeleted,0)=0 ORDER BY createdAt DESC`)
	if err != nil {
		return nil, fmt.Errorf("query Todos: %w", err)
	}
	defer rows.Close()
	var out []Todo
	for rows.Next() {
		var t Todo
		var created string
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.MeetingID, &created); err != nil {
			return nil, err
		}
		t.Created = ParseTime(created)
		out = append(out, t)
	}
	return out, rows.Err()
}
