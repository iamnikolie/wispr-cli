package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iamnikolie/wispr-cli/internal/export"
	"github.com/iamnikolie/wispr-cli/internal/store"
	"github.com/spf13/cobra"
)

var meetingsCmd = &cobra.Command{
	Use:   "meetings",
	Short: "Notetaker meetings: AI summaries, notes, transcripts",
}

var (
	mSince, mUntil, mSearch string
	mLimit                  int
	mDeleted, mDesc         bool
	mTranscript, mLive      bool
	mOut                    string
)

func meetingRow(m store.Meeting) map[string]any {
	row := toRow(m)
	row["id_short"] = shortID(m.ID)
	row["created"] = fmtTime(m.Created)
	row["modified"] = fmtTime(m.Modified)
	row["ended"] = fmtTime(m.Ended)
	row["duration"] = fmtDuration(m.Duration())
	row["summary_chars"] = len([]rune(m.Summary))
	row["participants"] = strings.Join(m.Participants, ", ")
	return row
}

var meetingsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List meetings (newest first)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := parseRange(mSince, mUntil)
		if err != nil {
			return err
		}
		ms, err := st.Meetings(ctxOf(cmd), store.MeetingFilter{Range: r, Search: mSearch, Deleted: mDeleted, Limit: mLimit, Desc: !mDesc})
		if err != nil {
			return err
		}
		if format() == "json" && len(fieldsFlag) == 0 {
			out := make([]map[string]any, 0, len(ms))
			for _, m := range ms {
				row := meetingRow(m)
				delete(row, "summary")
				delete(row, "notes")
				out = append(out, row)
			}
			return writeJSON(stdout, out)
		}
		rows := make([]map[string]any, 0, len(ms))
		for _, m := range ms {
			row := meetingRow(m)
			row["id"] = shortID(m.ID)
			rows = append(rows, row)
		}
		return emitRows(rows, []string{"id", "created", "duration", "title", "summary_chars"})
	},
}

var meetingsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Print one meeting's summary and notes; --transcript adds the transcript",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := st.Meeting(ctxOf(cmd), args[0])
		if err != nil {
			return err
		}
		var tr []store.Utterance
		if mTranscript {
			tr, err = loadTranscript(m)
			if err != nil {
				return err
			}
		}
		audio := st.AudioPath(m.ID)
		switch format() {
		case "json":
			row := meetingRow(m)
			row["audio"] = audio
			if mTranscript {
				row["transcript"] = tr
			}
			return writeJSON(stdout, row)
		case "text":
			if mTranscript {
				fmt.Fprint(stdout, export.TranscriptText(tr))
				return nil
			}
			fmt.Fprintln(stdout, strings.TrimSpace(m.Summary))
			if n := strings.TrimSpace(m.Notes); n != "" {
				fmt.Fprintln(stdout)
				fmt.Fprintln(stdout, n)
			}
			return nil
		default:
			fmt.Fprint(stdout, export.MeetingMarkdown(clock(), m, tr, audio))
			return nil
		}
	},
}

func loadTranscript(m store.Meeting) ([]store.Utterance, error) {
	src := store.Refined
	if mLive {
		src = store.Live
	}
	tr, err := st.Transcript(m, src)
	if err != nil {
		if errors.Is(err, store.ErrNoTranscript) && src == store.Refined {
			// Refinement may still be pending; fall back to the live file.
			if live, lerr := st.Transcript(m, store.Live); lerr == nil {
				fmt.Fprintln(stderr, "note: refined transcript missing, using live transcript")
				return live, nil
			}
		}
		return nil, err
	}
	return tr, nil
}

var meetingsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write each meeting as a Markdown file into --out/meetings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := parseRange(mSince, mUntil)
		if err != nil {
			return err
		}
		n, err := exportMeetings(cmd, r, mSearch, mLimit)
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "meetings: %d written, %d unchanged\n", n.written, n.unchanged)
		return nil
	},
}

type exportCount struct{ written, unchanged int }

func exportMeetings(cmd *cobra.Command, r store.Range, search string, limit int) (exportCount, error) {
	var n exportCount
	ms, err := st.Meetings(ctxOf(cmd), store.MeetingFilter{Range: r, Search: search, Deleted: mDeleted, Limit: limit, Desc: true})
	if err != nil {
		return n, err
	}
	dir := filepath.Join(outDir(), "meetings")
	for _, m := range ms {
		var tr []store.Utterance
		if mTranscript {
			tr, err = loadTranscript(m)
			if err != nil && !errors.Is(err, store.ErrNoTranscript) {
				return n, err
			}
		}
		content := export.MeetingMarkdown(clock(), m, tr, st.AudioPath(m.ID))
		path, changed, err := export.Write(dir, export.MeetingFile(clock(), m), content)
		if err != nil {
			return n, err
		}
		if changed {
			n.written++
			fmt.Fprintln(stdout, path)
		} else {
			n.unchanged++
		}
	}
	return n, nil
}

func clock() export.Clock { return export.Clock{UTC: utcFlag} }

func outDir() string {
	if mOut != "" {
		return mOut
	}
	return "."
}

func init() {
	for _, c := range []*cobra.Command{meetingsListCmd, meetingsExportCmd} {
		rangeFlags(c, &mSince, &mUntil)
		c.Flags().StringVar(&mSearch, "search", "", "substring match on title, summary or notes")
		c.Flags().IntVarP(&mLimit, "limit", "n", 0, "max meetings (0 = all)")
		c.Flags().BoolVar(&mDeleted, "deleted", false, "include deleted meetings")
	}
	meetingsListCmd.Flags().BoolVar(&mDesc, "asc", false, "oldest first")
	for _, c := range []*cobra.Command{meetingsShowCmd, meetingsExportCmd} {
		c.Flags().BoolVar(&mTranscript, "transcript", false, "include the transcript")
		c.Flags().BoolVar(&mLive, "live", false, "use the live transcript instead of the refined one")
	}
	meetingsExportCmd.Flags().StringVarP(&mOut, "out", "o", ".", "output directory")
	meetingsCmd.AddCommand(meetingsListCmd, meetingsShowCmd, meetingsExportCmd)
	rootCmd.AddCommand(meetingsCmd)
}
