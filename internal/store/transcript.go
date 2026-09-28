package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Utterance is one line of a meeting transcript.
type Utterance struct {
	ID        string `json:"id,omitempty"`
	Timestamp string `json:"timestamp"`
	Speaker   string `json:"speaker"`
	Text      string `json:"text"`
}

// TranscriptSource picks which ndjson file to read.
type TranscriptSource string

const (
	// Refined is the server-refined transcript (speaker turns, cleaned text).
	Refined TranscriptSource = "refined"
	// Live is the raw live transcript written during the recording.
	Live TranscriptSource = "live"
)

// TranscriptPath returns the ndjson file for a meeting and source.
func (s *Store) TranscriptPath(meetingID string, src TranscriptSource) string {
	return filepath.Join(s.MeetingsDir(), meetingID, string(src)+".ndjson")
}

// AudioPath returns the recording file if present, else "".
func (s *Store) AudioPath(meetingID string) string {
	dir := filepath.Join(s.MeetingsDir(), meetingID)
	for _, name := range []string{"upload.ogg", "upload.opus", "upload.m4a", "upload.wav"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ErrNoTranscript is returned when the meeting folder has no such file.
var ErrNoTranscript = errors.New("no transcript file")

// Transcript reads a meeting's ndjson transcript and resolves speaker names
// through the meeting's speakerMap.
func (s *Store) Transcript(m Meeting, src TranscriptSource) ([]Utterance, error) {
	f, err := os.Open(s.TranscriptPath(m.ID, src))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNoTranscript, s.TranscriptPath(m.ID, src))
		}
		return nil, err
	}
	defer f.Close()
	names := speakerNames(m.SpeakerMap)
	return ParseTranscript(f, names)
}

// ParseTranscript decodes ndjson utterances. Lines without a "text" field
// (the live file's leading meta line, for instance) are skipped.
func ParseTranscript(r interface{ Read([]byte) (int, error) }, names map[string]string) ([]Utterance, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var out []Utterance
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var raw struct {
			ID        string          `json:"id"`
			Timestamp json.RawMessage `json:"timestamp"`
			Text      string          `json:"text"`
			Speaker   json.RawMessage `json:"speaker"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		if raw.Text == "" {
			continue
		}
		out = append(out, Utterance{
			ID:        raw.ID,
			Timestamp: rawString(raw.Timestamp),
			Speaker:   resolveSpeaker(raw.Speaker, names),
			Text:      raw.Text,
		})
	}
	return out, sc.Err()
}

func rawString(b json.RawMessage) string {
	if len(b) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		return s
	}
	return strings.Trim(string(b), `"`)
}

// resolveSpeaker turns the speaker field — a string, a number or an object
// {id, name} — into a display name via the speakerMap assignments.
func resolveSpeaker(b json.RawMessage, names map[string]string) string {
	if len(b) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if n, ok := names[s]; ok {
			return n
		}
		return s
	}
	var num json.Number
	if err := json.Unmarshal(b, &num); err == nil {
		return speakerLabel(num.String(), names)
	}
	var obj struct {
		ID   json.RawMessage `json:"id"`
		Name string          `json:"name"`
	}
	if err := json.Unmarshal(b, &obj); err == nil {
		if obj.Name != "" {
			return obj.Name
		}
		return speakerLabel(strings.Trim(string(obj.ID), `"`), names)
	}
	return strings.Trim(string(b), `"`)
}

// SpeakerName resolves a transcript speaker id through the meeting's
// speakerMap, falling back to "Speaker N".
func (m Meeting) SpeakerName(id string) string {
	return speakerLabel(id, speakerNames(m.SpeakerMap))
}

var speakerRef = regexp.MustCompile(`<@speaker:([^>]+)>`)

// ResolveSpeakerRefs replaces the "<@speaker:N>" placeholders Flow leaves in
// summaries and notes with the resolved speaker name.
func (m Meeting) ResolveSpeakerRefs(text string) string {
	if !strings.Contains(text, "<@speaker:") {
		return text
	}
	names := speakerNames(m.SpeakerMap)
	return speakerRef.ReplaceAllStringFunc(text, func(ref string) string {
		id := speakerRef.FindStringSubmatch(ref)[1]
		return speakerLabel(id, names)
	})
}

func speakerLabel(id string, names map[string]string) string {
	if n, ok := names[id]; ok {
		return n
	}
	if _, err := strconv.Atoi(id); err == nil {
		return "Speaker " + id
	}
	return id
}

// speakerNames flattens Meetings.speakerMap
// ({"people":{pid:{"name":..}},"assignments":{sid:{"consensus":pid,...}}})
// into speaker-id → name.
func speakerNames(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var sm struct {
		People map[string]struct {
			Name string `json:"name"`
		} `json:"people"`
		Assignments map[string]struct {
			Consensus string `json:"consensus"`
			User      string `json:"user"`
			Mic       string `json:"mic"`
		} `json:"assignments"`
	}
	if err := json.Unmarshal([]byte(raw), &sm); err != nil {
		return out
	}
	for sid, a := range sm.Assignments {
		for _, pid := range []string{a.User, a.Consensus, a.Mic} {
			if pid == "" {
				continue
			}
			if p, ok := sm.People[pid]; ok && p.Name != "" {
				out[sid] = p.Name
				break
			}
		}
	}
	return out
}

func parseJSONStrings(s string) []string {
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return arr
	}
	var objs []map[string]any
	if err := json.Unmarshal([]byte(s), &objs); err == nil {
		var out []string
		for _, o := range objs {
			if n, ok := o["name"].(string); ok && n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}
