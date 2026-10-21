package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/sshtunnel"
)

func (Provider) Open(i connections.Input) (any, error) {
	if e := dialect.ValidateProfile(i.Profile); e != nil {
		return nil, e
	}
	tc, e := dialect.TLSConfig(i.Profile)
	if e != nil {
		return nil, e
	}
	port := i.Port
	if port == "" {
		port = "5432"
	}
	// Parse a constant config, then override ambient PG* settings explicitly.
	cfg, e := pgx.ParseConfig(
		"postgres://localhost/postgres?sslmode=disable",
	)
	if e != nil {
		return nil, e
	}
	cfg.Host = i.Host
	if i.Socket != "" {
		cfg.Host = i.Socket
	}
	p, _ := strconv.Atoi(port)
	cfg.Port = uint16(p)
	cfg.User = i.Username
	cfg.Password = i.Password
	cfg.Database = i.Database
	cfg.TLSConfig = tc
	cfg.Fallbacks = nil
	cfg.ConnectTimeout = 10 * time.Second
	if i.Socket != "" && tc != nil {
		return nil, fmt.Errorf(
			"PostgreSQL Unix sockets do not use TLS; explicitly select Disabled",
		)
	}

	// A tunnel dial has its own timeout, separate from the pool's own
	// PingContext timeout below: authenticating against the jump host and
	// authenticating against PostgreSQL are two distinct failures a user
	// needs to tell apart.
	var tun *sshtunnel.Tunnel
	if i.SSHEnabled {
		oc, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		tun, e = dialect.OpenTunnel(oc, i)
		cancel()
		if e != nil {
			return nil, e
		}
		cfg.DialFunc = tun.DialContext
		// pgx resolves the hostname itself before ever calling DialFunc,
		// using the local resolver (to support its own multi-address
		// fallback). A host reachable only through the tunnel, the usual
		// real reason to use one, is not locally resolvable at all, so
		// skip that lookup and let the SSH server's own network resolve
		// the name when DialFunc actually dials it.
		cfg.LookupFunc = func(ctx context.Context, host string) ([]string, error) {
			return []string{host}, nil
		}
	}

	cfg.RuntimeParams = map[string]string{
		"application_name": "Quarro",
	}
	if i.ReadOnly {
		// Applied by PostgreSQL while each physical pooled connection starts,
		// so even handle-level catalog work and newly-created connections begin
		// with read-only transactions by default.
		cfg.RuntimeParams["default_transaction_read_only"] = "on"
	}
	if i.Schema != "" {
		cfg.RuntimeParams["search_path"] = quote(i.Schema)
	}
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(18)
	db.SetMaxIdleConns(2)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()
	if e := db.PingContext(ctx); e != nil {
		db.Close()
		if tun != nil {
			tun.Close()
		}
		return nil, fmt.Errorf(
			"connection failed: %s",
			dialect.Redact(e.Error(), i.Password),
		)
	}
	dialect.RegisterTunnel(db, tun)
	return db, nil
}
