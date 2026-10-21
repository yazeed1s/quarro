package redis

import (
	"context"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// maxKeysScanned bounds Discover's SCAN pass so a database with millions of
// keys degrades to "an incomplete but fast keyspace list" instead of a long
// hang, the same philosophy as every bounded fetch elsewhere in this app.
const maxKeysScanned = 10000

// Databases returns Redis's numbered databases (0..15 by default) as
// strings, the closest equivalent a key-value store has to "database
// names." CONFIG GET can be disabled on managed/hardened Redis; when it is,
// this falls back to the documented default of 16.
func (Provider) Databases(
	ctx context.Context,
	h any,
) ([]string, error) {
	client := h.(*redis.Client)
	count := 16
	if vals, err := client.ConfigGet(ctx, "databases").Result(); err == nil {
		if raw, ok := vals["databases"]; ok {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				count = n
			}
		}
	}
	out := make([]string, count)
	for i := range out {
		out[i] = strconv.Itoa(i)
	}
	return out, nil
}

// Discover groups keys by a "prefix:" namespace convention (e.g. "user:123"
// and "user:456" both belong to "user") the way many Redis GUIs do, since
// Redis has no schema of its own to describe: a key with no colon becomes
// its own single-key group. Each group is reported as a
// provider.Discoverer-shaped model.Table with Kind "keyspace" and no
// columns/indexes/foreign keys: there is nothing SQL-shaped to fill them
// with, and pretending otherwise would be dishonest about what Redis is.
func (Provider) Discover(
	ctx context.Context,
	h any,
) ([]model.Table, error) {
	client := h.(*redis.Client)
	groups := map[string]bool{}
	order := []string{}
	var cursor uint64
	scanned := 0
	for {
		keys, next, err := client.Scan(ctx, cursor, "", 1000).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			prefix := key
			if i := strings.IndexByte(key, ':'); i >= 0 {
				prefix = key[:i]
			}
			if !groups[prefix] {
				groups[prefix] = true
				order = append(order, prefix)
			}
			scanned++
		}
		cursor = next
		if cursor == 0 || scanned >= maxKeysScanned {
			break
		}
	}
	tables := make([]model.Table, len(order))
	for i, prefix := range order {
		tables[i] = model.Table{
			Name:        prefix,
			ObjectName:  prefix,
			Kind:        "keyspace",
			Columns:     []model.Column{},
			Indexes:     []model.Index{},
			ForeignKeys: []model.ForeignKey{},
		}
	}
	return tables, nil
}
