// Package dialect_test exercises dialect's tunnel registry (RegisterTunnel/
// TunnelFingerprint/CloseHandle) as an external test package: it needs a
// real *sql.DB (modernc.org/sqlite, pure Go, no cgo) to exercise the
// registry against, and that driver package itself imports dialect, so an
// internal test file (package dialect) would create an import cycle through
// it. The positive case, a real tunnel actually registered and its
// fingerprint read back, is covered by postgres/mysql's own
// TestSSHTunnelContract against a real SSH server instead of duplicated
// here with another in-process server.
package dialect_test

import (
	"database/sql"
	"testing"

	"github.com/yazeed1s/quarro/internal/database/dialect"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTunnelFingerprintNotAHandle(t *testing.T) {
	if _, ok := dialect.TunnelFingerprint("not a handle"); ok {
		t.Fatal("TunnelFingerprint() on a non-*sql.DB value should report ok=false")
	}
}

func TestTunnelFingerprintUnregistered(t *testing.T) {
	db := openTestDB(t)
	if _, ok := dialect.TunnelFingerprint(db); ok {
		t.Fatal("TunnelFingerprint() on a db with no registered tunnel should report ok=false")
	}
}

func TestRegisterTunnelNilIsNoop(t *testing.T) {
	db := openTestDB(t)
	dialect.RegisterTunnel(db, nil)
	if _, ok := dialect.TunnelFingerprint(db); ok {
		t.Fatal("RegisterTunnel(db, nil) should not register anything")
	}
}

func TestCloseHandleWithNoTunnelStillCloses(t *testing.T) {
	db := openTestDB(t)
	if err := dialect.CloseHandle(db); err != nil {
		t.Fatalf("CloseHandle: %v", err)
	}
}
