package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/iamnikolie/wispr-cli/internal/store"
	"github.com/spf13/cobra"
)

// version is stamped at link time by the Makefile and by GoReleaser
// (-X github.com/iamnikolie/wispr-cli/cmd.version=…). An unstamped build says
// "dev" rather than claiming a version it does not have.
var version = "dev"

func buildVersion() string {
	v := version
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				rev := s.Value
				if len(rev) > 12 {
					rev = rev[:12]
				}
				v += " (" + rev + ")"
			}
		}
	}
	return v
}

var (
	dbPath       string
	jsonOutput   bool
	outputFormat string
	fieldsFlag   []string
	utcFlag      bool

	st *store.Store
)

var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// now is swapped in tests so relative windows are deterministic.
var now = time.Now

var rootCmd = &cobra.Command{
	Use:   "wispr",
	Short: "Read your Wispr Flow dictations, meeting notes and Flow notes from the local database",
	Long: "wispr — agent-facing reader for the Wispr Flow desktop app's local database.\n\n" +
		"Everything is read-only. Run 'wispr skill' for the full agent reference.",
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		switch outputFormat {
		case "", "md", "table", "json", "csv", "tsv", "text":
		default:
			return fmt.Errorf("--format must be one of md, table, json, csv, tsv, text")
		}
		if dbExempt(cmd) {
			return nil
		}
		var err error
		st, err = store.Open(dbPath)
		return err
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if st != nil {
			st.Close()
			st = nil
		}
	},
}

// Execute runs the CLI and exits non-zero on error.
func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&dbPath, "db", store.DefaultPath(), "path to flow.sqlite (env WISPR_DB)")
	pf.BoolVar(&jsonOutput, "json", false, "JSON output (same as --format json)")
	pf.StringVar(&outputFormat, "format", "", "output format: md (default), table, json, csv, tsv, text")
	pf.StringSliceVar(&fieldsFlag, "fields", nil, "comma-separated output fields, e.g. time,text")
	pf.BoolVar(&utcFlag, "utc", false, "print times in UTC instead of local time")

	rootCmd.Version = buildVersion()
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the wispr version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(stdout, "wispr version %s\n", buildVersion())
		return nil
	},
}

// dbExempt reports whether a command runs without opening the database.
func dbExempt(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "wispr", "skill", "help", "completion", "version", "bash", "zsh", "fish", "powershell":
		return true
	}
	return false
}

func ctxOf(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
