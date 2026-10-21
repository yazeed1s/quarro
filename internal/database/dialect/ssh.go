package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/sshtunnel"
)

// ValidateSSHProfile checks a profile's SSH tunnel fields. It is a no-op
// when SSHEnabled is false, so engines that never set the field (SQLite,
// MongoDB, Redis) are unaffected. Credentials themselves (SSH password,
// key passphrase) are resolved and checked at connect time, the same way
// the database password is; this only checks the fields a saved profile
// carries.
func ValidateSSHProfile(p connections.Profile) error {
	if !p.SSHEnabled {
		return nil
	}
	if p.Engine != connections.EnginePostgres && p.Engine != connections.EngineMySQL {
		return fmt.Errorf(
			"SSH tunneling is only supported for PostgreSQL and MySQL connections",
		)
	}
	if p.Socket != "" {
		return fmt.Errorf(
			"SSH tunneling cannot be combined with a Unix socket connection",
		)
	}
	if p.SSHHost == "" {
		return fmt.Errorf("SSH host is required")
	}
	if p.SSHUsername == "" {
		return fmt.Errorf("SSH username is required")
	}
	switch p.SSHAuthMethod {
	case "password":
	case "key":
		if p.SSHPrivateKeyPath == "" {
			return fmt.Errorf("SSH private key path is required")
		}
	default:
		return fmt.Errorf(`SSH auth method must be "password" or "key"`)
	}
	return nil
}

// OpenTunnel opens an SSH tunnel for a profile with SSHEnabled set, or
// returns a nil tunnel and nil error when it is not. postgres.Open/
// mysql.Open assign the returned tunnel's DialContext to the driver's own
// dial-override hook (pgconn.Config.DialFunc, mysql.Config.DialFunc) and
// must call RegisterTunnel once the pool itself is constructed, or Close it
// directly if construction fails first.
func OpenTunnel(ctx context.Context, i connections.Input) (*sshtunnel.Tunnel, error) {
	if !i.SSHEnabled {
		return nil, nil
	}
	if e := ValidateSSHProfile(i.Profile); e != nil {
		return nil, e
	}
	var key []byte
	if i.SSHAuthMethod == "key" {
		data, e := os.ReadFile(i.SSHPrivateKeyPath)
		if e != nil {
			return nil, fmt.Errorf("SSH private key: %w", e)
		}
		key = data
	}
	return sshtunnel.Open(ctx, sshtunnel.Config{
		Host:            i.SSHHost,
		Port:            i.SSHPort,
		Username:        i.SSHUsername,
		AuthMethod:      i.SSHAuthMethod,
		Password:        i.SSHPassword,
		PrivateKeyPEM:   key,
		Passphrase:      i.SSHPassphrase,
		HostFingerprint: i.SSHHostFingerprint,
	})
}

// tunnels associates a live *sql.DB pool with the SSH tunnel backing it, so
// CloseHandle can tear the tunnel down exactly when the pool itself closes.
// Keyed by the pool's own pointer identity: safe because a *sql.DB returned
// by an engine's Open is never duplicated, only passed around as a handle.
var (
	tunnelsMu sync.Mutex
	tunnels   = map[*sql.DB]*sshtunnel.Tunnel{}
)

// RegisterTunnel records that t backs db, for CloseHandle to close later.
// A nil t is a no-op, so callers can pass OpenTunnel's result unconditionally.
func RegisterTunnel(db *sql.DB, t *sshtunnel.Tunnel) {
	if t == nil {
		return
	}
	tunnelsMu.Lock()
	tunnels[db] = t
	tunnelsMu.Unlock()
}

// TunnelFingerprint returns the SSH host-key fingerprint actually seen for
// h's tunnel, if RegisterTunnel recorded one. ConnectionsService uses this
// right after a successful Open to surface a just-connected host's
// fingerprint back to the caller for trust-on-first-use confirmation.
func TunnelFingerprint(h any) (string, bool) {
	db, ok := h.(*sql.DB)
	if !ok {
		return "", false
	}
	tunnelsMu.Lock()
	t, ok := tunnels[db]
	tunnelsMu.Unlock()
	if !ok {
		return "", false
	}
	return t.Fingerprint(), true
}
