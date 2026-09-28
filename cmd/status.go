package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Where the database is and what it holds",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := ctxOf(cmd)
		info, _ := os.Stat(st.Path)
		var size int64
		if info != nil {
			size = info.Size()
		}
		ds, err := st.DictationStats(ctx)
		if err != nil {
			return err
		}
		meetingDirs := 0
		if entries, err := os.ReadDir(st.MeetingsDir()); err == nil {
			for _, e := range entries {
				if e.IsDir() && e.Name()[0] != '.' {
					meetingDirs++
				}
			}
		}
		var appVersion string
		_ = st.DB.QueryRowContext(ctx, "SELECT COALESCE(max(appVersion),'') FROM History").Scan(&appVersion)
		row := map[string]any{
			"db":               st.Path,
			"db_mb":            fmt.Sprintf("%.1f", float64(size)/1024/1024),
			"app_version":      appVersion,
			"dictations":       ds.Count,
			"dictation_words":  ds.Words,
			"dictations_first": fmtDay(ds.First),
			"dictations_last":  fmtDay(ds.Last),
			"meetings":         st.Count(ctx, "Meetings"),
			"meeting_folders":  meetingDirs,
			"meetings_dir":     filepath.Clean(st.MeetingsDir()),
			"notes":            st.Count(ctx, "Notes"),
			"dictionary":       st.Count(ctx, "Dictionary"),
			"todos":            st.Count(ctx, "Todos"),
		}
		if format() == "json" {
			return writeJSON(stdout, row)
		}
		cols := []string{"db", "db_mb", "app_version", "dictations", "dictation_words", "dictations_first", "dictations_last",
			"meetings", "meeting_folders", "meetings_dir", "notes", "dictionary", "todos"}
		for _, c := range cols {
			fmt.Fprintf(stdout, "%s: %v\n", c, row[c])
		}
		return nil
	},
}

var schemaCmd = &cobra.Command{
	Use:   "schema [table]",
	Short: "Print CREATE TABLE statements (the schema drifts between app versions)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		tables, err := st.Schema(ctxOf(cmd), name)
		if err != nil {
			return err
		}
		if name != "" && len(tables) == 0 {
			return fmt.Errorf("no table %q (run `wispr schema` to list them)", name)
		}
		if format() == "json" {
			rows := make([]map[string]any, 0, len(tables))
			for _, t := range tables {
				rows = append(rows, map[string]any{"name": t.Name, "sql": t.SQL})
			}
			return writeJSON(stdout, rows)
		}
		for _, t := range tables {
			if name == "" && !schemaFull {
				fmt.Fprintln(stdout, t.Name)
				continue
			}
			fmt.Fprintf(stdout, "%s;\n\n", t.SQL)
		}
		return nil
	},
}

var schemaFull bool

var sqlCmd = &cobra.Command{
	Use:   "sql <query>",
	Short: "Run a read-only SQL query (writes are rejected by SQLite)",
	Example: `  wispr sql "SELECT app, count(*) n FROM History GROUP BY app ORDER BY n DESC LIMIT 5"
  wispr sql "SELECT title FROM Meetings ORDER BY createdAt DESC" --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cols, rows, err := st.Query(ctxOf(cmd), args[0])
		if err != nil {
			return err
		}
		if format() == "json" {
			if rows == nil {
				rows = []map[string]any{}
			}
			return writeJSON(stdout, rows)
		}
		return emitRows(rows, cols)
	},
}

func init() {
	schemaCmd.Flags().BoolVar(&schemaFull, "full", false, "print every table's CREATE statement, not just names")
	rootCmd.AddCommand(statusCmd, schemaCmd, sqlCmd)
}
