package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iamnikolie/wispr-cli/internal/export"
	"github.com/iamnikolie/wispr-cli/internal/render"
	"github.com/iamnikolie/wispr-cli/internal/store"
	"github.com/spf13/cobra"
)

func format() string {
	if jsonOutput {
		return "json"
	}
	if outputFormat == "" {
		return "md"
	}
	return outputFormat
}

// fmtTime renders a stored timestamp for display: RFC3339 without fractional
// seconds, in local time unless --utc.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if utcFlag {
		return t.UTC().Format("2006-01-02T15:04:05Z")
	}
	return t.Local().Format("2006-01-02T15:04:05-07:00")
}

func fmtDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if utcFlag {
		return t.UTC().Format("2006-01-02")
	}
	return t.Local().Format("2006-01-02")
}

func fmtClock(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if utcFlag {
		return t.UTC().Format("15:04")
	}
	return t.Local().Format("15:04")
}

func fmtDuration(d time.Duration) string { return export.Duration(d) }

// writeJSON encodes v as compact UTF-8 JSON without HTML escaping.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func activeFields(defaults []string) []string {
	if len(fieldsFlag) > 0 {
		return fieldsFlag
	}
	return defaults
}

func project(row map[string]any, fields []string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if v, ok := row[f]; ok {
			out[f] = v
		}
	}
	return out
}

func projectAll(rows []map[string]any, cols []string) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = project(r, cols)
	}
	return out
}

// emitRows prints a list in the active format. JSON projects each row only
// when --fields is set; md and table both render a Markdown table.
func emitRows(rows []map[string]any, defaults []string) error {
	cols := activeFields(defaults)
	switch format() {
	case "json":
		if len(fieldsFlag) == 0 {
			if rows == nil {
				rows = []map[string]any{}
			}
			return writeJSON(stdout, rows)
		}
		return writeJSON(stdout, projectAll(rows, fieldsFlag))
	case "csv":
		render.Separated(stdout, projectAll(rows, cols), cols, ",")
	case "tsv":
		render.Separated(stdout, projectAll(rows, cols), cols, "\t")
	default:
		render.Table(stdout, projectAll(rows, cols), cols)
	}
	return nil
}

// toRow converts a struct with json tags into a generic row via JSON, so
// --fields can address any tagged field by its JSON name.
func toRow(v any) map[string]any {
	b, _ := json.Marshal(v)
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	_ = dec.Decode(&m)
	return m
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// rangeFlags adds --since/--until to a listing command.
func rangeFlags(cmd *cobra.Command, since, until *string) {
	cmd.Flags().StringVar(since, "since", "", "only entries after this: 7d, 24h, 2w, 3mo or 2026-09-01[T10:00]")
	cmd.Flags().StringVar(until, "until", "", "only entries before this (same forms as --since)")
}

func parseRange(since, until string) (store.Range, error) {
	var r store.Range
	var err error
	n := now()
	if r.Since, err = store.ParseSince(since, n); err != nil {
		return r, fmt.Errorf("--since: %w", err)
	}
	if r.Until, err = store.ParseSince(until, n); err != nil {
		return r, fmt.Errorf("--until: %w", err)
	}
	return r, nil
}
