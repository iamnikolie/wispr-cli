package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamnikolie/wispr-cli/internal/export"
	"github.com/iamnikolie/wispr-cli/internal/store"
	"github.com/spf13/cobra"
)

var dictationsCmd = &cobra.Command{
	Use:     "dictations",
	Aliases: []string{"history"},
	Short:   "Dictation history: everything you said to Flow, per app",
}

var (
	dSince, dUntil, dApp, dSearch string
	dStatus                       []string
	dLimit, dWidth                int
	dArchived, dDesc, dFull       bool
	dOut                          string
)

func dictationRow(d store.Dictation) map[string]any {
	row := toRow(d)
	row["time"] = fmtTime(d.Time)
	row["day"] = fmtDay(d.Time)
	row["app_label"] = export.AppLabel(d.App)
	return row
}

func dictationFilter() (store.DictationFilter, error) {
	r, err := parseRange(dSince, dUntil)
	if err != nil {
		return store.DictationFilter{}, err
	}
	return store.DictationFilter{Range: r, App: dApp, Search: dSearch, Statuses: dStatus, Archived: dArchived, Limit: dLimit, Desc: dDesc}, nil
}

var dictationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List dictations (oldest first; --since 1d for today)",
	Example: `  wispr dictations list --since 1d
  wispr dictations list --since 7d --app cursor --format text
  wispr dictations list --search "deploy" --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := dictationFilter()
		if err != nil {
			return err
		}
		ds, err := st.Dictations(ctxOf(cmd), f)
		if err != nil {
			return err
		}
		switch format() {
		case "json":
			rows := make([]map[string]any, 0, len(ds))
			for _, d := range ds {
				rows = append(rows, dictationRow(d))
			}
			if len(fieldsFlag) > 0 {
				return writeJSON(stdout, projectAll(rows, fieldsFlag))
			}
			return writeJSON(stdout, rows)
		case "text":
			for _, d := range ds {
				text := strings.TrimSpace(d.Text)
				if !dFull {
					text = strings.Join(strings.Fields(text), " ")
				}
				fmt.Fprintf(stdout, "%s %s [%s] %s\n", fmtDay(d.Time), fmtClock(d.Time), export.AppLabel(d.App), text)
			}
			return nil
		default:
			rows := make([]map[string]any, 0, len(ds))
			for _, d := range ds {
				row := dictationRow(d)
				row["app"] = export.AppLabel(d.App)
				if !dFull {
					row["text"] = truncate(d.Text, dWidth)
				}
				rows = append(rows, row)
			}
			return emitRows(rows, []string{"time", "app", "words", "text"})
		}
	},
}

var dictationsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write one Markdown file per day into --out/dictations",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := dictationFilter()
		if err != nil {
			return err
		}
		n, err := exportDictations(cmd, f)
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "dictations: %d files written, %d unchanged\n", n.written, n.unchanged)
		return nil
	},
}

func exportDictations(cmd *cobra.Command, f store.DictationFilter) (exportCount, error) {
	var n exportCount
	ds, err := st.Dictations(ctxOf(cmd), f)
	if err != nil {
		return n, err
	}
	files := export.DictationsByDay(clock(), ds)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	dir := filepath.Join(dictOutDir(), "dictations")
	for _, name := range names {
		path, changed, err := export.Write(dir, name, files[name])
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

func dictOutDir() string {
	if dOut != "" {
		return dOut
	}
	return "."
}

func init() {
	for _, c := range []*cobra.Command{dictationsListCmd, dictationsExportCmd} {
		rangeFlags(c, &dSince, &dUntil)
		c.Flags().StringVar(&dApp, "app", "", "substring match on the app bundle id (e.g. cursor, slack)")
		c.Flags().StringVar(&dSearch, "search", "", "substring match on the dictated text")
		c.Flags().StringSliceVar(&dStatus, "status", nil, "statuses to include (default formatted,raw_transcript; 'all' for every row)")
		c.Flags().IntVarP(&dLimit, "limit", "n", 0, "max entries (0 = all)")
		c.Flags().BoolVar(&dArchived, "archived", false, "include archived entries")
	}
	dictationsListCmd.Flags().BoolVar(&dDesc, "desc", false, "newest first")
	dictationsListCmd.Flags().BoolVar(&dFull, "full", false, "do not truncate text")
	dictationsListCmd.Flags().IntVar(&dWidth, "width", 120, "truncate text to this many characters in table output")
	dictationsExportCmd.Flags().StringVarP(&dOut, "out", "o", ".", "output directory")
	dictationsCmd.AddCommand(dictationsListCmd, dictationsExportCmd)
	rootCmd.AddCommand(dictationsCmd)
}
