// Package demo provisions a small sample SQLite database so a fresh install
// has something real to explore without needing credentials. Its seed data
// intentionally mirrors what the frontend's old fixture data used to show
// (same names, same row-generation formulas) for continuity.
package demo

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"

	"github.com/yazeed1s/quarro/internal/database/sqlite"
)

var names = []string{
	"Olivia Rhye", "Phoenix Baker", "Lana Steiner", "Demi Wilkinson",
	"Drew Cano", "Natali Craig", "Orlando Diggs", "Andi Lane",
	"Kate Morrison", "Yaz Haddad", "Sofia García", "Amir Saleh",
}

// Path returns where the demo database lives, without creating it.
func Path() (string, error) {
	return xdg.DataFile(filepath.Join("quarro", "demo.db"))
}

// EnsureDatabase creates and seeds the demo database if it doesn't already
// exist. Calling it again once the file exists is a no-op.
func EnsureDatabase() (string, error) {
	return EnsureDatabaseForStage("quarro", "")
}

// EnsureDatabaseForStage is like EnsureDatabase, but stores the database as
// "demo<suffix>.db" within dir so different build stages can share one
// directory without clobbering each other's demo database.
func EnsureDatabaseForStage(dir, suffix string) (string, error) {
	path, err := xdg.DataFile(filepath.Join(dir, "demo"+suffix+".db"))
	if err != nil {
		return "", fmt.Errorf("resolve demo database path: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	db, err := sqlite.Open(path, false)
	if err != nil {
		return "", fmt.Errorf("create demo database: %w", err)
	}
	defer db.Close()

	if err := seed(db); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("seed demo database: %w", err)
	}
	return path, nil
}

func seed(sqlDB *sql.DB) error {
	tx, err := sqlDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	db := tx

	ddl := []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			role TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_login TEXT
		)`,
		`CREATE TABLE orders (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id),
			total TEXT NOT NULL,
			currency TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE products (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			price TEXT NOT NULL,
			stock INTEGER NOT NULL,
			description TEXT
		)`,
		`CREATE TABLE categories (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			parent_id INTEGER REFERENCES categories(id)
		)`,
		`CREATE VIEW active_users AS SELECT * FROM users WHERE status = 'active'`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	for i := 0; i < 125; i++ {
		name := names[i%len(names)]
		email := emailFor(name, i)
		role := "member"
		if i%7 == 0 {
			role = "admin"
		} else if i%3 == 0 {
			role = "editor"
		}
		status := "active"
		if i%5 == 0 {
			status = "invited"
		}
		createdAt := isoDate(2026, 9, 14-(i%14), 9, i%60)
		var lastLogin any
		if i%5 != 0 {
			lastLogin = isoDate(2026, 9, 14, 12, i%60)
		}
		if _, err := db.Exec(
			`INSERT INTO users (id, name, email, role, status, created_at, last_login) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			i+1, name, email, role, status, createdAt, lastLogin,
		); err != nil {
			return err
		}
	}

	for i := 0; i < 82; i++ {
		status := "completed"
		if i%3 == 0 {
			status = "pending"
		}
		total := fmt.Sprintf("%.2f", float64(i)*17.95+29)
		if _, err := db.Exec(
			`INSERT INTO orders (id, user_id, total, currency, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			i+1, (i%125)+1, total, "USD", status, "2026-09-14T10:30:00.000Z",
		); err != nil {
			return err
		}
	}

	productNames := []string{
		"Desk lamp",
		"Notebook",
		"Canvas tote",
		"Coffee cup",
	}
	for i := 0; i < 24; i++ {
		price := fmt.Sprintf("%.2f", 12+float64(i)*2.5)
		var description any = "Everyday essentials, thoughtfully made."
		if i%3 == 0 {
			description = nil
		}
		if _, err := db.Exec(
			`INSERT INTO products (id, name, price, stock, description) VALUES (?, ?, ?, ?, ?)`,
			i+1, productNames[i%len(productNames)], price, 30+i, description,
		); err != nil {
			return err
		}
	}

	categories := []string{"Home", "Stationery", "Accessories"}
	for i, name := range categories {
		if _, err := db.Exec(`INSERT INTO categories (id, name, parent_id) VALUES (?, ?, NULL)`, i+1, name); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func emailFor(name string, i int) string {
	slug := ""
	for _, r := range name {
		switch {
		case r == ' ':
			slug += "."
		default:
			slug += string(toLower(r))
		}
	}
	suffix := ""
	if i > 11 {
		suffix = fmt.Sprintf("%d", i)
	}
	return fmt.Sprintf("%s%s@acme.co", slug, suffix)
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

func isoDate(
	year int,
	month int,
	day int,
	hour int,
	minute int,
) string {
	// Mirrors the frontend's `new Date(Date.UTC(...)).toISOString()` output.
	// time.Date normalizes an out-of-range day (e.g. day 0) into the
	// previous month, matching JS Date.UTC's rollover behavior.
	t := time.Date(
		year,
		time.Month(month),
		day,
		hour,
		minute,
		0,
		0,
		time.UTC,
	)
	return t.Format("2006-01-02T15:04:05.000Z")
}
