package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
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
		port = "3306"
	}
	cfg := mysqldriver.NewConfig()
	cfg.User = i.Username
	cfg.Passwd = i.Password
	cfg.DBName = i.Database
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(i.Host, port)
	if i.Socket != "" {
		cfg.Net = "unix"
		cfg.Addr = i.Socket
	}
	cfg.TLS = tc
	cfg.Timeout = 10 * time.Second
	cfg.ReadTimeout = 310 * time.Second
	cfg.WriteTimeout = 10 * time.Second
	cfg.ParseTime = false
	cfg.MultiStatements = false
	if i.ReadOnly {
		// go-sql-driver applies DSN params as system variables whenever it
		// establishes a physical connection. This covers every connection in
		// the pool, including catalog reads that do not use a document handle.
		cfg.Params = map[string]string{"transaction_read_only": "1"}
	}

	// A tunnel dial has its own timeout, separate from the pool's own
	// PingContext timeout below: authenticating against the jump host and
	// authenticating against MySQL are two distinct failures a user needs
	// to tell apart. Unlike pgx (internal/database/postgres/open.go), this
	// driver has no separate hostname-resolution hook: DialFunc receives
	// cfg.Addr exactly as set above, so no LookupFunc-style override is
	// needed here.
	var tun *sshtunnel.Tunnel
	if i.SSHEnabled {
		oc, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		tun, e = dialect.OpenTunnel(oc, i)
		cancel()
		if e != nil {
			return nil, e
		}
		cfg.DialFunc = tun.DialContext
	}

	connector, e := mysqldriver.NewConnector(cfg)
	if e != nil {
		if tun != nil {
			tun.Close()
		}
		return nil, e
	}
	db := sql.OpenDB(connector)
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
