package dialect

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"github.com/yazeed1s/quarro/internal/connections"
)

// TLSConfig builds the *tls.Config a PostgreSQL or MySQL connection should
// dial with, from a connection profile's TLS fields. It returns a nil
// config (plaintext) only when the profile explicitly disables TLS.
func TLSConfig(p connections.Profile) (*tls.Config, error) {
	if p.TLSMode == "disable" {
		return nil, nil
	}
	name := p.TLSServerName
	if name == "" {
		name = p.Host
	}
	if name == "" {
		return nil, fmt.Errorf(
			"TLS server name is required for a Unix socket",
		)
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: name}
	if p.TLSCA != "" {
		data, e := os.ReadFile(p.TLSCA)
		if e != nil {
			return nil, fmt.Errorf("read CA certificate: %w", e)
		}
		roots, e := x509.SystemCertPool()
		if e != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("CA file contains no certificates")
		}
		cfg.RootCAs = roots
	}
	if p.TLSCert != "" {
		cert, e := tls.LoadX509KeyPair(p.TLSCert, p.TLSKey)
		if e != nil {
			return nil, fmt.Errorf("load client certificate: %w", e)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

// Redact removes any occurrence of password from s, for safely surfacing a
// driver error message that may otherwise echo the connection password back.
func Redact(s, password string) string {
	if password != "" {
		s = strings.ReplaceAll(s, password, "[redacted]")
	}
	return s
}
