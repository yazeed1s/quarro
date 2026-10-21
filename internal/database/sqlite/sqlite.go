// Package sqlite opens SQLite database files for Quarro sessions.
package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens the SQLite database file at path. In read-only mode the file
// must already exist.
// SQLite itself cannot create a file it can't write to,
// and silently falling back to read-write would violate the requested mode.
// In read-write mode a missing file is created.
func Open(path string, readOnly bool) (*sql.DB, error) {
	resolved, err := expand(path)
	if err != nil {
		return nil, err
	}
	if readOnly {
		if _, err := os.Stat(resolved); err != nil {
			return nil, fmt.Errorf(
				"database file not found: %s",
				resolved,
			)
		}
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(resolved)}
	if readOnly {
		uri.RawQuery = "mode=ro"
	}
	dsn := uri.String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(18)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func expand(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("database file path is required")
	}
	if path == "~" || len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
