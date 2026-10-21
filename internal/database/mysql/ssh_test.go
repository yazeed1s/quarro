package mysql

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
)

// TestSSHTunnelContract mirrors
// internal/database/postgres/ssh_test.go's TestSSHTunnelContract: the
// MySQL target is reachable only from inside the SSH jump host's own
// docker network, never directly, so a successful query here is only
// possible if Open really tunneled through SSH. Run via
// `scripts/test-engine.sh ssh`, which brings up both engines' disposable
// targets behind one shared jump host and sets every
// QUARRO_TEST_SSH_MYSQL_* variable this test reads.
func TestSSHTunnelContract(t *testing.T) {
	host := os.Getenv("QUARRO_TEST_SSH_HOST")
	if host == "" {
		t.Skip("set QUARRO_TEST_SSH_HOST (see scripts/test-engine.sh ssh)")
	}
	base := connections.Input{
		Profile: connections.Profile{
			ID:       "ssh-contract-mysql",
			Name:     "ssh-contract-mysql",
			Engine:   connections.EngineMySQL,
			Host:     os.Getenv("QUARRO_TEST_SSH_MYSQL_TARGET_HOST"),
			Port:     os.Getenv("QUARRO_TEST_SSH_MYSQL_TARGET_PORT"),
			Database: os.Getenv("QUARRO_TEST_SSH_MYSQL_DB_NAME"),
			Username: os.Getenv("QUARRO_TEST_SSH_MYSQL_DB_USER"),
			TLSMode:  "disable",

			SSHEnabled:  true,
			SSHHost:     host,
			SSHPort:     os.Getenv("QUARRO_TEST_SSH_PORT"),
			SSHUsername: os.Getenv("QUARRO_TEST_SSH_USER"),
		},
		Password: os.Getenv("QUARRO_TEST_SSH_MYSQL_DB_PASSWORD"),
	}

	t.Run("password auth end to end", func(t *testing.T) {
		in := base
		in.SSHAuthMethod = "password"
		in.SSHPassword = os.Getenv("QUARRO_TEST_SSH_PASSWORD")
		h, e := (Provider{}).Open(in)
		if e != nil {
			t.Fatalf("Open: %v", e)
		}
		defer (Provider{}).Close(h)
		fp, ok := dialect.TunnelFingerprint(h)
		if !ok || !strings.HasPrefix(fp, "SHA256:") {
			t.Fatalf("TunnelFingerprint() = %q, %v; want a real SHA256: fingerprint", fp, ok)
		}
		var one int
		if e := h.(*sql.DB).QueryRow("SELECT 1").Scan(&one); e != nil {
			t.Fatalf("query through tunnel: %v", e)
		}
		if one != 1 {
			t.Fatalf("SELECT 1 through tunnel = %d, want 1", one)
		}
	})

	t.Run("key auth end to end", func(t *testing.T) {
		keyPath := os.Getenv("QUARRO_TEST_SSH_KEY")
		if keyPath == "" {
			t.Skip("set QUARRO_TEST_SSH_KEY")
		}
		in := base
		in.ID = "ssh-contract-mysql-key"
		in.SSHAuthMethod = "key"
		in.SSHPrivateKeyPath = keyPath
		h, e := (Provider{}).Open(in)
		if e != nil {
			t.Fatalf("Open: %v", e)
		}
		defer (Provider{}).Close(h)
		var one int
		if e := h.(*sql.DB).QueryRow("SELECT 1").Scan(&one); e != nil {
			t.Fatalf("query through tunnel: %v", e)
		}
	})

	t.Run("wrong SSH password rejected", func(t *testing.T) {
		in := base
		in.SSHAuthMethod = "password"
		in.SSHPassword = "definitely-wrong"
		if h, e := (Provider{}).Open(in); e == nil {
			(Provider{}).Close(h)
			t.Fatal("Open succeeded with the wrong SSH password")
		}
	})

	t.Run("host key mismatch rejected", func(t *testing.T) {
		in := base
		in.SSHAuthMethod = "password"
		in.SSHPassword = os.Getenv("QUARRO_TEST_SSH_PASSWORD")
		in.SSHHostFingerprint = "SHA256:not-the-real-fingerprint"
		if h, e := (Provider{}).Open(in); e == nil {
			(Provider{}).Close(h)
			t.Fatal("Open succeeded despite a mismatched trusted host key")
		}
	})
}
