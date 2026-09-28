package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iamnikolie/wispr-cli/internal/export"
	"github.com/iamnikolie/wispr-cli/internal/store"
	"github.com/spf13/cobra"
)

var notesCmd = &cobra.Command{
	Use:   "notes",
	Short: "Flow notes (the Notes tab)",
}

var (
	nSince, nUntil, nSearch string
	nLimit                  int
	nDeleted                bool
	nOut                    string
)

func noteRow(n store.Note) map[string]any {
	row := toRow(n)
	row["id_short"] = shortID(n.ID)
	row["created"] = fmtTime(n.Created)
	row["modified"] = fmtTime(n.Modified)
	row["chars"] = len([]rune(n.Content))
	return row
}

var notesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List notes (most recently modified first)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := parseRange(nSince, nUntil)
		if err != nil {
			return err
		}
		ns, err := st.Notes(ctxOf(cmd), store.NoteFilter{Range: r, Search: nSearch, Deleted: nDeleted, Limit: nLimit, Desc: true})
		if err != nil {
			return err
		}
		rows := make([]map[string]any, 0, len(ns))
		for _, n := range ns {
			row := noteRow(n)
			if format() != "json" {
				row["id"] = shortID(n.ID)
			}
			delete(row, "content") // use `notes show` for the body
			rows = append(rows, row)
		}
		return emitRows(rows, []string{"id", "modified", "title", "chars"})
	},
}

var notesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Print one note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		n, err := st.Note(ctxOf(cmd), args[0])
		if err != nil {
			return err
		}
		switch format() {
		case "json":
			return writeJSON(stdout, noteRow(n))
		case "text":
			fmt.Fprintln(stdout, strings.TrimSpace(n.Content))
			return nil
		default:
			fmt.Fprint(stdout, export.NoteMarkdown(clock(), n))
			return nil
		}
	},
}

var notesExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write each note as a Markdown file into --out/notes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := parseRange(nSince, nUntil)
		if err != nil {
			return err
		}
		c, err := exportNotes(cmd, r, nSearch, nLimit)
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "notes: %d written, %d unchanged\n", c.written, c.unchanged)
		return nil
	},
}

func exportNotes(cmd *cobra.Command, r store.Range, search string, limit int) (exportCount, error) {
	var c exportCount
	ns, err := st.Notes(ctxOf(cmd), store.NoteFilter{Range: r, Search: search, Deleted: nDeleted, Limit: limit, Desc: true})
	if err != nil {
		return c, err
	}
	dir := filepath.Join(noteOutDir(), "notes")
	for _, n := range ns {
		path, changed, err := export.Write(dir, export.NoteFile(clock(), n), export.NoteMarkdown(clock(), n))
		if err != nil {
			return c, err
		}
		if changed {
			c.written++
			fmt.Fprintln(stdout, path)
		} else {
			c.unchanged++
		}
	}
	return c, nil
}

func noteOutDir() string {
	if nOut != "" {
		return nOut
	}
	return "."
}

func init() {
	for _, c := range []*cobra.Command{notesListCmd, notesExportCmd} {
		rangeFlags(c, &nSince, &nUntil)
		c.Flags().StringVar(&nSearch, "search", "", "substring match on title or content")
		c.Flags().IntVarP(&nLimit, "limit", "n", 0, "max notes (0 = all)")
		c.Flags().BoolVar(&nDeleted, "deleted", false, "include deleted notes")
	}
	notesExportCmd.Flags().StringVarP(&nOut, "out", "o", ".", "output directory")
	notesCmd.AddCommand(notesListCmd, notesShowCmd, notesExportCmd)
	rootCmd.AddCommand(notesCmd)
}
