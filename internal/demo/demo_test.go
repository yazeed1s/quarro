package demo

import (
	"os"
	"testing"

	"github.com/yazeed1s/quarro/internal/database/sqlite"
)

func TestEnsureDatabaseCreatesAndSeeds(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path, err := EnsureDatabase()
	if err != nil {
		t.Fatalf("EnsureDatabase: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("demo database file missing: %v", err)
	}

	db, err := sqlite.Open(path, true)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 125 {
		t.Errorf("users count = %d, want 125", count)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&count); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if count != 82 {
		t.Errorf("orders count = %d, want 82", count)
	}

	var email string
	if err := db.QueryRow(`SELECT email FROM users WHERE name = 'Sofia García' LIMIT 1`).Scan(&email); err != nil {
		t.Fatalf("query Sofia García: %v", err)
	}
	if email == "" {
		t.Error("expected a non-empty email for Sofia García")
	}
}

func TestEnsureDatabaseIdempotent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path1, err := EnsureDatabase()
	if err != nil {
		t.Fatalf("EnsureDatabase (1st): %v", err)
	}
	info1, err := os.Stat(path1)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	path2, err := EnsureDatabase()
	if err != nil {
		t.Fatalf("EnsureDatabase (2nd): %v", err)
	}
	if path1 != path2 {
		t.Fatalf("paths differ: %q vs %q", path1, path2)
	}
	info2, err := os.Stat(path2)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info1.ModTime() != info2.ModTime() {
		t.Error(
			"second EnsureDatabase call modified the existing file",
		)
	}
}
