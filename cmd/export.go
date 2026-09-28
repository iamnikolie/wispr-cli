package cmd

import (
	"fmt"

	"github.com/iamnikolie/wispr-cli/internal/store"
	"github.com/spf13/cobra"
)

var (
	eSince, eUntil, eOut string
	eTranscript          bool
	eSkip                []string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export meetings, dictations and notes as Markdown into --out",
	Long: "Writes --out/meetings/*.md (one per meeting), --out/dictations/<day>.md and\n" +
		"--out/notes/*.md. Idempotent: unchanged files are left alone, so it is safe to\n" +
		"run from a cron job or before every agent session.",
	Example: `  wispr export --out ~/notes/wispr
  wispr export --since 7d --transcript --out ./vault
  wispr export --skip dictations --out ./vault`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := parseRange(eSince, eUntil)
		if err != nil {
			return err
		}
		skip := map[string]bool{}
		for _, s := range eSkip {
			skip[s] = true
		}
		mOut, dOut, nOut = eOut, eOut, eOut
		mTranscript = eTranscript
		if !skip["meetings"] {
			c, err := exportMeetings(cmd, r, "", 0)
			if err != nil {
				return err
			}
			fmt.Fprintf(stderr, "meetings: %d written, %d unchanged\n", c.written, c.unchanged)
		}
		if !skip["dictations"] {
			c, err := exportDictations(cmd, store.DictationFilter{Range: r})
			if err != nil {
				return err
			}
			fmt.Fprintf(stderr, "dictations: %d files written, %d unchanged\n", c.written, c.unchanged)
		}
		if !skip["notes"] {
			c, err := exportNotes(cmd, r, "", 0)
			if err != nil {
				return err
			}
			fmt.Fprintf(stderr, "notes: %d written, %d unchanged\n", c.written, c.unchanged)
		}
		return nil
	},
}

func init() {
	rangeFlags(exportCmd, &eSince, &eUntil)
	exportCmd.Flags().StringVarP(&eOut, "out", "o", ".", "output directory")
	exportCmd.Flags().BoolVar(&eTranscript, "transcript", false, "include meeting transcripts")
	exportCmd.Flags().StringSliceVar(&eSkip, "skip", nil, "sections to skip: meetings, dictations, notes")
	rootCmd.AddCommand(exportCmd)
}
