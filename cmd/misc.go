package cmd

import (
	"github.com/spf13/cobra"
)

var dictionaryCmd = &cobra.Command{
	Use:   "dictionary",
	Short: "Personal dictionary: learned words, manual entries and snippets",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		es, err := st.Dictionary(ctxOf(cmd))
		if err != nil {
			return err
		}
		rows := make([]map[string]any, 0, len(es))
		for _, e := range es {
			row := toRow(e)
			row["last_used"] = fmtDay(e.LastUsed)
			rows = append(rows, row)
		}
		return emitRows(rows, []string{"phrase", "replacement", "snippet", "source", "used"})
	},
}

var todosCmd = &cobra.Command{
	Use:   "todos",
	Short: "Action items captured from meetings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ts, err := st.Todos(ctxOf(cmd))
		if err != nil {
			return err
		}
		rows := make([]map[string]any, 0, len(ts))
		for _, t := range ts {
			row := toRow(t)
			row["created"] = fmtTime(t.Created)
			row["meeting_id"] = shortID(t.MeetingID)
			rows = append(rows, row)
		}
		return emitRows(rows, []string{"created", "status", "title", "meeting_id"})
	},
}

func init() {
	rootCmd.AddCommand(dictionaryCmd, todosCmd)
}
