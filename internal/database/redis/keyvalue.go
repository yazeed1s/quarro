package redis

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
	"github.com/yazeed1s/quarro/internal/database/model"
)

const (
	// maxStringBytes bounds one string read; a longer value comes back
	// truncated (KeyDetail.Length > Value.Size) rather than loaded whole.
	maxStringBytes = 256 * 1024
	defaultPage    = 100
	maxPage        = 1000
)

// Scan lists key metadata (type, length, TTL) for keys matching prefix,
// up to limit keys. It never reads values: listing a keyspace of large
// hashes should not load every hash. See capability.KeyValueStore for
// what group means.
func (Provider) Scan(
	ctx context.Context,
	conn any,
	prefix string,
	group bool,
	limit int,
) model.ResultSet {
	c := conn.(*redis.Client)
	if limit < 1 || limit > 10000 {
		limit = 500
	}
	keys, more, err := scanKeys(ctx, c, prefix, group, limit)
	if err != nil {
		return model.ResultSet{Kind: "error", Error: err.Error()}
	}
	ents, err := keyMeta(ctx, c, keys)
	if err != nil {
		return model.ResultSet{Kind: "error", Error: err.Error()}
	}
	return model.ResultSet{
		Kind:      "keyvalue",
		Entries:   ents,
		Truncated: more,
	}
}

// scanKeys collects up to limit distinct keys. SCAN may return a key more
// than once, so keys are deduped. more reports that at least one further
// key matched.
func scanKeys(
	ctx context.Context,
	c *redis.Client,
	prefix string,
	group bool,
	limit int,
) ([]string, bool, error) {
	pat := "*"
	if prefix != "" {
		pat = escapeGlob(prefix) + "*"
	}
	keys := []string{}
	seen := map[string]bool{}
	if group && prefix != "" {
		// A colon-less key is its own Discover group, so the bare key
		// belongs to the group too.
		pat = escapeGlob(prefix) + ":*"
		n, err := c.Exists(ctx, prefix).Result()
		if err != nil {
			return nil, false, err
		}
		if n == 1 {
			keys = append(keys, prefix)
		}
		seen[prefix] = true
	}
	var cur uint64
	for {
		got, next, err := c.Scan(ctx, cur, pat, int64(limit)).Result()
		if err != nil {
			return nil, false, err
		}
		for _, k := range got {
			if seen[k] {
				continue
			}
			if len(keys) == limit {
				return keys, true, nil
			}
			seen[k] = true
			keys = append(keys, k)
		}
		cur = next
		if cur == 0 {
			return keys, false, nil
		}
	}
}

// escapeGlob makes s match literally in a SCAN MATCH pattern.
func escapeGlob(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '*', '?', '[', ']', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// keyMeta reads type, TTL, and length for keys in two pipelined round
// trips (the length command depends on the type). Keys that vanished
// since the scan are dropped.
func keyMeta(
	ctx context.Context,
	c *redis.Client,
	keys []string,
) ([]model.KVEntry, error) {
	ents := []model.KVEntry{}
	if len(keys) == 0 {
		return ents, nil
	}
	typs := make([]*redis.StatusCmd, len(keys))
	ttls := make([]*redis.DurationCmd, len(keys))
	_, err := c.Pipelined(ctx, func(p redis.Pipeliner) error {
		for i, k := range keys {
			typs[i] = p.Type(ctx, k)
			ttls[i] = p.PTTL(ctx, k)
		}
		return nil
	})
	if err != nil && !isReplyErr(err) {
		return nil, err
	}
	lens := make([]*redis.IntCmd, len(keys))
	_, err = c.Pipelined(ctx, func(p redis.Pipeliner) error {
		for i, k := range keys {
			lens[i] = lengthCmd(ctx, p, typs[i].Val(), k)
		}
		return nil
	})
	// A key can change type between the two pipelines; its WRONGTYPE
	// reply only costs that key its length, not the whole listing.
	if err != nil && !isReplyErr(err) {
		return nil, err
	}
	for i, k := range keys {
		t := typs[i].Val()
		if t == "" || t == "none" {
			continue
		}
		e := model.KVEntry{Key: k, Type: t}
		if lens[i] != nil {
			e.Length = lens[i].Val()
		}
		e.TTLSeconds, e.TTLMillis = ttlOf(ttls[i].Val())
		ents = append(ents, e)
	}
	return ents, nil
}

// lengthCmd queues t's length command, or returns nil for a type with no
// length (a module type).
func lengthCmd(
	ctx context.Context,
	p redis.Pipeliner,
	t, k string,
) *redis.IntCmd {
	switch t {
	case "string":
		return p.StrLen(ctx, k)
	case "hash":
		return p.HLen(ctx, k)
	case "list":
		return p.LLen(ctx, k)
	case "set":
		return p.SCard(ctx, k)
	case "zset":
		return p.ZCard(ctx, k)
	case "stream":
		return p.XLen(ctx, k)
	}
	return nil
}

// isReplyErr reports a server error reply (WRONGTYPE, a nil reply), as
// opposed to a connection or context failure.
func isReplyErr(err error) bool {
	var re redis.Error
	return errors.As(err, &re)
}

// ttlOf converts a PTTL reply. go-redis returns -1/-2 (no expiry, no key)
// unscaled, so anything not positive means no TTL.
func ttlOf(d time.Duration) (*int64, *int64) {
	if d <= 0 {
		return nil, nil
	}
	s := int64(d / time.Second)
	ms := int64(d / time.Millisecond)
	return &s, &ms
}

// InspectKey reads one page of key's value. A missing key is not an
// error: it comes back with Exists false, so the caller can tell "deleted
// since the listing" from a failure.
func (Provider) InspectKey(
	ctx context.Context,
	conn any,
	key, cursor string,
	count int,
) (model.KeyDetail, error) {
	c := conn.(*redis.Client)
	if count < 1 || count > maxPage {
		count = defaultPage
	}
	t, err := c.Type(ctx, key).Result()
	if err != nil {
		return model.KeyDetail{}, err
	}
	d := model.KeyDetail{Key: key, Type: t, Exists: t != "none"}
	if !d.Exists {
		d.Type = ""
		return d, nil
	}
	if ttl, err := c.PTTL(ctx, key).Result(); err == nil {
		_, d.TTLMillis = ttlOf(ttl)
	}
	switch t {
	case "string":
		err = readString(ctx, c, &d)
	case "hash":
		err = readHash(ctx, c, &d, cursor, count)
	case "list":
		err = readList(ctx, c, &d, cursor, count)
	case "set":
		err = readSet(ctx, c, &d, cursor, count)
	case "zset":
		err = readZSet(ctx, c, &d, cursor, count)
	case "stream":
		err = readStream(ctx, c, &d, cursor, count)
	}
	// Any other type is a module type (RedisJSON etc.): its name is all
	// there is to show here.
	if err != nil {
		return model.KeyDetail{}, err
	}
	return d, nil
}

func readString(
	ctx context.Context,
	c *redis.Client,
	d *model.KeyDetail,
) error {
	n, err := c.StrLen(ctx, d.Key).Result()
	if err != nil {
		return err
	}
	s, err := c.GetRange(ctx, d.Key, 0, maxStringBytes-1).Result()
	if err != nil {
		return err
	}
	v := kvString(s)
	d.Length = n
	d.Value = &v
	return nil
}

func readHash(
	ctx context.Context,
	c *redis.Client,
	d *model.KeyDetail,
	cursor string,
	count int,
) error {
	n, err := c.HLen(ctx, d.Key).Result()
	if err != nil {
		return err
	}
	d.Length = n
	cur, err := scanCursor(cursor)
	if err != nil {
		return err
	}
	d.Members = []model.KeyMember{}
	for {
		kv, next, err := c.HScan(ctx, d.Key, cur, "", int64(count)).
			Result()
		if err != nil {
			return err
		}
		for i := 0; i+1 < len(kv); i += 2 {
			f := kvString(kv[i])
			d.Members = append(d.Members, model.KeyMember{
				Field: &f,
				Value: kvString(kv[i+1]),
			})
		}
		cur = next
		// HSCAN may return an empty page with more to come; keep going
		// until this page has something or the scan is over.
		if cur == 0 || len(d.Members) >= count {
			break
		}
	}
	d.Cursor = nextScanCursor(cur)
	return nil
}

func readSet(
	ctx context.Context,
	c *redis.Client,
	d *model.KeyDetail,
	cursor string,
	count int,
) error {
	n, err := c.SCard(ctx, d.Key).Result()
	if err != nil {
		return err
	}
	d.Length = n
	cur, err := scanCursor(cursor)
	if err != nil {
		return err
	}
	d.Members = []model.KeyMember{}
	for {
		ms, next, err := c.SScan(ctx, d.Key, cur, "", int64(count)).
			Result()
		if err != nil {
			return err
		}
		for _, m := range ms {
			d.Members = append(
				d.Members,
				model.KeyMember{Value: kvString(m)},
			)
		}
		cur = next
		if cur == 0 || len(d.Members) >= count {
			break
		}
	}
	d.Cursor = nextScanCursor(cur)
	return nil
}

func readList(
	ctx context.Context,
	c *redis.Client,
	d *model.KeyDetail,
	cursor string,
	count int,
) error {
	n, err := c.LLen(ctx, d.Key).Result()
	if err != nil {
		return err
	}
	d.Length = n
	off, err := offsetCursor(cursor)
	if err != nil {
		return err
	}
	vs, err := c.LRange(ctx, d.Key, off, off+int64(count)-1).Result()
	if err != nil {
		return err
	}
	d.Members = make([]model.KeyMember, len(vs))
	for i, v := range vs {
		idx := off + int64(i)
		d.Members[i] = model.KeyMember{Index: &idx, Value: kvString(v)}
	}
	d.Cursor = nextOffset(off, len(vs), n)
	return nil
}

func readZSet(
	ctx context.Context,
	c *redis.Client,
	d *model.KeyDetail,
	cursor string,
	count int,
) error {
	n, err := c.ZCard(ctx, d.Key).Result()
	if err != nil {
		return err
	}
	d.Length = n
	off, err := offsetCursor(cursor)
	if err != nil {
		return err
	}
	zs, err := c.ZRangeWithScores(ctx, d.Key, off, off+int64(count)-1).
		Result()
	if err != nil {
		return err
	}
	d.Members = make([]model.KeyMember, len(zs))
	for i, z := range zs {
		sc := z.Score
		d.Members[i] = model.KeyMember{
			Value: kvString(fmt.Sprint(z.Member)),
			Score: &sc,
		}
	}
	d.Cursor = nextOffset(off, len(zs), n)
	return nil
}

// readStream pages entries by ID. cursor is the last ID already read, so
// the next page starts just after it ("(" is XRANGE's exclusive bound).
// The summary and groups come with the first page only.
func readStream(
	ctx context.Context,
	c *redis.Client,
	d *model.KeyDetail,
	cursor string,
	count int,
) error {
	n, err := c.XLen(ctx, d.Key).Result()
	if err != nil {
		return err
	}
	d.Length = n
	if cursor == "" {
		info, err := streamInfo(ctx, c, d.Key)
		if err != nil {
			return err
		}
		d.Stream = info
	}
	start := "-"
	if cursor != "" {
		start = "(" + cursor
	}
	msgs, err := c.XRangeN(ctx, d.Key, start, "+", int64(count)).Result()
	if err != nil {
		return err
	}
	d.Members = make([]model.KeyMember, len(msgs))
	for i, m := range msgs {
		fs := make(map[string]model.KVString, len(m.Values))
		for f, v := range m.Values {
			fs[f] = kvString(fmt.Sprint(v))
		}
		d.Members[i] = model.KeyMember{ID: m.ID, Fields: fs}
	}
	if len(msgs) == count {
		d.Cursor = msgs[len(msgs)-1].ID
	}
	return nil
}

func streamInfo(
	ctx context.Context,
	c *redis.Client,
	key string,
) (*model.StreamInfo, error) {
	si, err := c.XInfoStream(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	gs, err := c.XInfoGroups(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	info := &model.StreamInfo{
		FirstID: si.FirstEntry.ID,
		LastID:  si.LastEntry.ID,
		Groups:  make([]model.StreamGroup, len(gs)),
	}
	for i, g := range gs {
		sg := model.StreamGroup{
			Name:            g.Name,
			Consumers:       g.Consumers,
			Pending:         g.Pending,
			LastDeliveredID: g.LastDeliveredID,
		}
		// Lag is Redis 7+ and -1 when unknown. go-redis leaves it 0 on an
		// older server, which only EntriesAdded (also 7+) tells apart.
		if g.Lag >= 0 && si.EntriesAdded > 0 {
			lag := g.Lag
			sg.Lag = &lag
		}
		info.Groups[i] = sg
	}
	return info, nil
}

// kvString wraps a stored value for JSON. Non-UTF-8 bytes go out as
// base64: JSON would otherwise replace them and corrupt the value.
func kvString(s string) model.KVString {
	if utf8.ValidString(s) {
		return model.KVString{Text: s, Size: len(s)}
	}
	return model.KVString{
		Binary: true,
		Base64: base64.StdEncoding.EncodeToString([]byte(s)),
		Size:   len(s),
	}
}

func scanCursor(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor %q", s)
	}
	return n, nil
}

func nextScanCursor(cur uint64) string {
	if cur == 0 {
		return ""
	}
	return strconv.FormatUint(cur, 10)
}

func offsetCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid cursor %q", s)
	}
	return n, nil
}

func nextOffset(off int64, got int, total int64) string {
	next := off + int64(got)
	if got == 0 || next >= total {
		return ""
	}
	return strconv.FormatInt(next, 10)
}
