package sqlite

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")
	db, err := Open(path, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpenReadOnlyMissingFileFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := Open(path, true); err == nil {
		t.Fatal(
			"Open(readOnly=true) on a missing file should fail, not create it",
		)
	}
}

func TestOpenReadOnlyExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	rw, err := Open(path, false)
	if err != nil {
		t.Fatalf("Open (rw create): %v", err)
	}
	rw.Close()

	ro, err := Open(path, true)
	if err != nil {
		t.Fatalf("Open (ro existing): %v", err)
	}
	defer ro.Close()
	if err := ro.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpenEmptyPath(t *testing.T) {
	if _, err := Open("", false); err == nil {
		t.Fatal("Open(\"\") should fail")
	}
}
