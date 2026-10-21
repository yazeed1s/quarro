// Package redis implements Quarro's second non-SQL provider: Redis, a
// key-value store. Where MongoDB (internal/database/mongodb) is close
// enough to a SQL table that it could honestly implement provider.Browser,
// Redis cannot: there is no filter+sort+skip/limit operation over a
// keyspace, only a key lookup and a cursor-based prefix scan, which is why
// provider.KeyValueStore exists as its own interface.
//
// This package implements only provider.Connector, provider.Discoverer,
// provider.KeyValueStore, and provider.KeyValueEditor, never
// provider.SQLExecutor, provider.Browser, provider.Editor,
// provider.Transactor, provider.QueryCompiler, or
// provider.IdentifierFormatter. Writes are one key at a time, each
// conflict-checked under WATCH (see edit.go); streams are read-only.
package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
)

// Provider implements the subset of provider.Provider that Redis can
// honestly support. It holds no state; every method takes the connection
// resource it needs as an argument.
type Provider struct{}

// New returns a Redis Provider.
func New() Provider { return Provider{} }

// ValidateProfile checks a Redis connection profile. Like MongoDB's, and
// unlike every SQL engine's, a username is never required. Redis
// authenticates with a password alone (or, commonly for local/dev
// instances, no auth at all). Database, when set, is Redis's numbered
// database index as a string, not a name.
func ValidateProfile(p connections.Profile) error {
	if p.Host == "" {
		return fmt.Errorf("host is required")
	}
	if p.Port != "" {
		if n, err := strconv.Atoi(p.Port); err != nil || n < 1 ||
			n > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	}
	if p.Database != "" {
		if n, err := strconv.Atoi(p.Database); err != nil || n < 0 {
			return fmt.Errorf(
				"database must be a non-negative index, e.g. 0",
			)
		}
	}
	return nil
}

func (Provider) Open(i connections.Input) (any, error) {
	if err := ValidateProfile(i.Profile); err != nil {
		return nil, err
	}
	port := i.Port
	if port == "" {
		port = "6379"
	}
	dbIndex := 0
	if i.Database != "" {
		dbIndex, _ = strconv.Atoi(
			i.Database,
		) // already validated above
	}
	opts := &redis.Options{
		Addr:     i.Host + ":" + port,
		Password: i.Password,
		DB:       dbIndex,
	}
	if i.TLSMode != "" && i.TLSMode != "disable" {
		tc, err := dialect.TLSConfig(i.Profile)
		if err != nil {
			return nil, err
		}
		opts.TLSConfig = tc
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	return client, nil
}

func (Provider) OpenConnection(
	_ context.Context,
	h any,
) (any, error) {
	return h.(*redis.Client), nil
}

// DiscardConnection is a no-op: like MongoDB's client, *redis.Client is
// itself a connection pool with no separate per-statement resource to
// poison.
func (Provider) DiscardConnection(any) error { return nil }

func (Provider) Close(
	h any,
) error {
	c := h.(*redis.Client)
	metaCache.Delete(c)
	return c.Close()
}
