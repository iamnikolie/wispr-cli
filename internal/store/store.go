// Package store reads the Wispr Flow desktop app's local SQLite database.
//
// Everything is read-only: the database is opened with mode=ro and
// PRAGMA query_only, so a running Wispr Flow keeps writing undisturbed.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo
)

// Store is an open, read-only handle on flow.sqlite.
type Store struct {
	DB   *sql.DB
	Path string
}

// DefaultPath returns where the Wispr Flow app keeps flow.sqlite on this OS.
// WISPR_DB overrides it.
func DefaultPath() string {
	if p := os.Getenv("WISPR_DB"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Wispr Flow", "flow.sqlite")
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "Wispr Flow", "flow.sqlite")
	default:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "Wispr Flow", "flow.sqlite")
	}
}

// Open opens the database read-only. A missing file is reported with the
// path so the user can point --db at the right place.
func Open(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("database not found at %s (is Wispr Flow installed? pass --db or set WISPR_DB)", path)
		}
		return nil, err
	}
	dsn := "file:" + url.PathEscape(path) +
		"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &Store{DB: db, Path: path}, nil
}

// Close releases the handle.
func (s *Store) Close() error { return s.DB.Close() }

// MeetingsDir is the folder holding per-meeting transcripts and audio.
func (s *Store) MeetingsDir() string {
	return filepath.Join(filepath.Dir(s.Path), "meetings")
}

// Timestamps in flow.sqlite are Sequelize strings such as
// "2026-06-05 07:34:51.566 +00:00".
const tsLayout = "2006-01-02 15:04:05.000 -07:00"

var tsLayouts = []string{
	tsLayout,
	"2006-01-02 15:04:05 -07:00",
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05",
	time.RFC3339Nano,
	time.RFC3339,
}

// ParseTime decodes a stored timestamp. Unknown formats yield the zero time
// rather than an error, so one odd row never breaks a listing.
func ParseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, l := range tsLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// FormatTime encodes a cutoff in the stored format so string comparison in
// SQL matches the app's own ordering.
func FormatTime(t time.Time) string {
	return t.UTC().Format(tsLayout)
}

// Range bounds a query by stored timestamp; zero values mean unbounded.
type Range struct {
	Since time.Time
	Until time.Time
}

func (r Range) where(col string) (string, []any) {
	var parts []string
	var args []any
	if !r.Since.IsZero() {
		parts = append(parts, col+" >= ?")
		args = append(args, FormatTime(r.Since))
	}
	if !r.Until.IsZero() {
		parts = append(parts, col+" < ?")
		args = append(args, FormatTime(r.Until))
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(parts, " AND "), args
}

// Table is one row of sqlite_master.
type Table struct {
	Name string
	SQL  string
}

// Schema lists tables (or the one named) with their CREATE statements.
func (s *Store) Schema(ctx context.Context, name string) ([]Table, error) {
	q := "SELECT name, sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'"
	var args []any
	if name != "" {
		q += " AND name = ? COLLATE NOCASE"
		args = append(args, name)
	}
	q += " ORDER BY name"
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Table
	for rows.Next() {
		var t Table
		var sqlText sql.NullString
		if err := rows.Scan(&t.Name, &sqlText); err != nil {
			return nil, err
		}
		t.SQL = sqlText.String
		out = append(out, t)
	}
	return out, rows.Err()
}

// Count returns the row count of a table, or -1 when the table is missing.
func (s *Store) Count(ctx context.Context, table string) int64 {
	var n int64
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM "`+table+`"`).Scan(&n); err != nil {
		return -1
	}
	return n
}

// Query runs an arbitrary read-only SQL statement and returns generic rows.
// PRAGMA query_only makes any write fail inside SQLite itself.
func (s *Store) Query(ctx context.Context, q string, args ...any) ([]string, []map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = normalize(vals[i])
		}
		out = append(out, m)
	}
	return cols, out, rows.Err()
}

func normalize(v any) any {
	switch x := v.(type) {
	case []byte:
		return fmt.Sprintf("<blob %d bytes>", len(x))
	case time.Time:
		return x.Format(time.RFC3339)
	default:
		return v
	}
}
