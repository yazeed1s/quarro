package sessions

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open(
		"sqlite",
		"file:"+filepath.Join(t.TempDir(), "s.db"),
	)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	return db
}

func TestPutAndDisconnect(t *testing.T) {
	r := NewRegistry()
	r.Put("s1", "p1", openTemp(t))
	if !r.Connected("s1") {
		t.Fatal("expected s1 to be connected")
	}
	if err := r.Disconnect("s1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if r.Connected("s1") {
		t.Fatal("expected s1 to be disconnected")
	}
}

func TestIndependentSessionsSameProfile(t *testing.T) {
	r := NewRegistry()
	r.Put("s1", "shared-profile", openTemp(t))
	r.Put("s2", "shared-profile", openTemp(t))
	if err := r.Disconnect("s1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if !r.Connected("s2") {
		t.Fatal(
			"closing s1 should not affect s2, even for the same profile",
		)
	}
}

func TestPutReplacesExistingHandle(t *testing.T) {
	r := NewRegistry()
	first := openTemp(t)
	r.Put("s1", "p1", first)
	r.Put("s1", "p1", openTemp(t))
	if err := first.Ping(); err == nil {
		t.Fatal("expected the replaced handle to have been closed")
	}
}

func TestCloseAll(t *testing.T) {
	r := NewRegistry()
	r.Put("s1", "p1", openTemp(t))
	r.Put("s2", "p2", openTemp(t))
	r.CloseAll()
	if r.Connected("s1") || r.Connected("s2") {
		t.Fatal("CloseAll should disconnect every session")
	}
}

func TestLateConnectCannotResurrectClosedSession(t *testing.T) {
	r := NewRegistry()
	generation := r.BeginConnect("s")
	r.Disconnect("s")
	db := openTemp(t)
	if r.Install("s", "p", generation, db) {
		t.Fatal("late connection resurrected session")
	}
	if err := db.Ping(); err == nil {
		t.Fatal("rejected handle leaked")
	}
}
