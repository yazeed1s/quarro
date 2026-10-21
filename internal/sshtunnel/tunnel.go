// Package sshtunnel dials a database host through an SSH jump host. It has
// no dependency on internal/database or internal/connections: it knows
// nothing about profiles, providers, or secrets storage, only how to
// authenticate against an SSH server and proxy a connection through it. A
// caller hands Tunnel.DialContext to a SQL driver's own dial-override hook
// (pgconn.Config.DialFunc, mysql.Config.DialFunc) instead of opening a local
// forwarded listener: each dialed connection gets its own channel
// multiplexed directly over the one SSH connection.
package sshtunnel

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// Config describes how to reach and authenticate against an SSH jump host.
// AuthMethod is "password" or "key"; the matching credential (Password, or
// PrivateKeyPEM with an optional Passphrase) must be set.
type Config struct {
	Host       string
	Port       string // default "22"
	Username   string
	AuthMethod string // "password" | "key"

	Password string

	PrivateKeyPEM []byte
	Passphrase    string

	// HostFingerprint, when non-empty, is the only accepted host key
	// (ssh.FingerprintSHA256 format: "SHA256:..."). Open rejects any other
	// key outright. When empty, Open accepts whatever key the server
	// presents (trust-on-first-use) and the caller reads the actual
	// fingerprint back from Tunnel.Fingerprint to show the user and
	// persist for later connects.
	HostFingerprint string
}

// Tunnel owns one live SSH connection used to dial a database host/port on
// the far side of it.
type Tunnel struct {
	client      *ssh.Client
	fingerprint string
}

// Open dials and authenticates against cfg.Host. It does not dial a
// database; call DialContext per connection attempt afterward.
func Open(ctx context.Context, cfg Config) (*Tunnel, error) {
	auth, e := authMethod(cfg)
	if e != nil {
		return nil, e
	}
	port := cfg.Port
	if port == "" {
		port = "22"
	}
	addr := net.JoinHostPort(cfg.Host, port)

	// fp is set by the callback below, even on a trust-on-first-use
	// accept, so Open can hand the caller back the key it actually saw.
	var fp string
	cc := &ssh.ClientConfig{
		User:    cfg.Username,
		Auth:    []ssh.AuthMethod{auth},
		Timeout: 10 * time.Second,
		HostKeyCallback: func(h string, r net.Addr, k ssh.PublicKey) error {
			fp = ssh.FingerprintSHA256(k)
			if cfg.HostFingerprint == "" {
				return nil
			}
			if fp != cfg.HostFingerprint {
				return fmt.Errorf(
					"SSH host key does not match the previously trusted key for this connection; the host key may have changed, or this could be an interception attempt",
				)
			}
			return nil
		},
	}

	var d net.Dialer
	c, e := d.DialContext(ctx, "tcp", addr)
	if e != nil {
		return nil, fmt.Errorf("SSH connection failed: %w", e)
	}
	sc, nch, req, e := ssh.NewClientConn(c, addr, cc)
	if e != nil {
		c.Close()
		return nil, fmt.Errorf("SSH connection failed: %w", e)
	}
	return &Tunnel{client: ssh.NewClient(sc, nch, req), fingerprint: fp}, nil
}

// authMethod builds the single ssh.AuthMethod cfg.AuthMethod selects, with a
// distinct, specific error for each way a key can fail to parse: that
// distinction matters to a user deciding whether to re-enter a passphrase
// or pick a different key file, so it is not collapsed into one generic
// "authentication failed".
func authMethod(cfg Config) (ssh.AuthMethod, error) {
	switch cfg.AuthMethod {
	case "password":
		return ssh.Password(cfg.Password), nil
	case "key":
		// sg holds the parsed key; named for what it is (an ssh.Signer),
		// not shortened further since no shorter form stays recognizable.
		var sg ssh.Signer
		var e error
		if cfg.Passphrase != "" {
			sg, e = ssh.ParsePrivateKeyWithPassphrase(cfg.PrivateKeyPEM, []byte(cfg.Passphrase))
		} else {
			sg, e = ssh.ParsePrivateKey(cfg.PrivateKeyPEM)
		}
		if e != nil {
			return nil, fmt.Errorf("SSH private key: %w", e)
		}
		return ssh.PublicKeys(sg), nil
	default:
		return nil, fmt.Errorf("unknown SSH auth method %q", cfg.AuthMethod)
	}
}

// Fingerprint returns the connected host's key fingerprint, regardless of
// whether cfg.HostFingerprint was set or empty.
func (t *Tunnel) Fingerprint() string {
	return t.fingerprint
}

// DialContext dials addr (the real database host:port) through the SSH
// connection. network/addr match pgconn.Config.DialFunc's and
// mysql.Config.DialFunc's own parameter names, since this method is handed
// to either driver unchanged. ssh.Client.Dial has no context parameter of
// its own, so ctx cancellation is applied by racing it against the dial on
// a goroutine.
func (t *Tunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, e := t.client.Dial(network, addr)
		ch <- result{c, e}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		return dialConn{r.conn}, nil
	}
}

// Close tears down the underlying SSH connection. Every net.Conn
// DialContext returned is a channel multiplexed over it and cannot outlive
// it; callers must not use those connections after Close.
func (t *Tunnel) Close() error {
	return t.client.Close()
}

// dialConn wraps the net.Conn an SSH channel returns so a SetDeadline call
// succeeds as a no-op instead of failing: ssh.Client.Dial's own connection
// type rejects every deadline call outright ("ssh: tcpChan: deadline not
// supported"), and some SQL drivers (go-sql-driver/mysql) call SetDeadline
// unconditionally and treat that error as a fatal connection failure.
// Per-operation timeouts are already enforced elsewhere (job/execution
// context deadlines), not through this connection's own deadline.
type dialConn struct {
	net.Conn
}

func (dialConn) SetDeadline(time.Time) error      { return nil }
func (dialConn) SetReadDeadline(time.Time) error  { return nil }
func (dialConn) SetWriteDeadline(time.Time) error { return nil }
