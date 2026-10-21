package dialect

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yazeed1s/quarro/internal/connections"
)

// ValidateProfile checks the connection-profile fields common to every
// engine (name/ID presence, SQLite's file path, or a remote engine's
// host/port/credentials/TLS combination). It does not check that p.Engine
// names a registered engine at all: the caller (provider.Validate, or an
// engine's own Open) already knows that from context.
func ValidateProfile(p connections.Profile) error {
	if p.ID == "" || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("connection name and ID are required")
	}
	if p.Engine == connections.EngineSQLite {
		if p.Path == "" {
			return fmt.Errorf("database file path is required")
		}
		return nil
	}
	if p.Database == "" || p.Username == "" {
		return fmt.Errorf("database and username are required")
	}
	if p.Socket == "" && p.Host == "" {
		return fmt.Errorf("host or Unix socket is required")
	}
	if p.Port != "" {
		n, e := strconv.Atoi(p.Port)
		if e != nil || n < 1 || n > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	}
	switch p.TLSMode {
	case "", "verify-full", "disable":
	default:
		return fmt.Errorf(
			"TLS must be verify-full or disable; insecure fallback is not supported",
		)
	}
	if (p.TLSCert == "") != (p.TLSKey == "") {
		return fmt.Errorf(
			"client certificate and key must be supplied together",
		)
	}
	if p.TLSMode == "disable" &&
		(p.TLSCA != "" || p.TLSCert != "" || p.TLSServerName != "") {
		return fmt.Errorf(
			"TLS certificate options require verified TLS",
		)
	}
	return nil
}
