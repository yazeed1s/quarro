package redis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// TestContract runs the same connect/discover/read shapes as the other
// engines' contract tests (internal/database/provider/provider_test.go's
// TestRemoteContracts, internal/database/mongodb/mongodb_test.go's
// TestContract), against a real local Redis, to prove the provider
// contracts generalize to a key-value store too, not just SQL and
// document engines. Skipped unless QUARRO_TEST_REDIS_URL is set to a
// "host:port" address; see scripts/test-engine.sh's redis mode for a
// disposable container that sets it.
// openContract connects to the disposable Redis named by
// QUARRO_TEST_REDIS_URL, selects database db, and flushes it before and
// after the test. It skips the test when the variable is unset.
func openContract(
	t *testing.T,
	db string,
) (Provider, any, any, *goredis.Client, context.Context) {
	t.Helper()
	raw := os.Getenv("QUARRO_TEST_REDIS_URL")
	if raw == "" {
		t.Skip(
			"set QUARRO_TEST_REDIS_URL to a disposable Redis address",
		)
	}
	host, port := raw, "6379"
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] == ':' {
			host, port = raw[:i], raw[i+1:]
			break
		}
	}

	p := New()
	input := connections.Input{Profile: connections.Profile{
		Engine:   connections.EngineRedis,
		Host:     host,
		Port:     port,
		Database: db,
	}}

	h, err := p.Open(input)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { p.Close(h) })

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	t.Cleanup(cancel)

	conn, err := p.OpenConnection(ctx, h)
	if err != nil {
		t.Fatalf("OpenConnection: %v", err)
	}
	client := conn.(*goredis.Client)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()
		client.FlushDB(cleanupCtx)
	})
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	return p, h, conn, client, ctx
}

func TestContract(t *testing.T) {
	p, h, conn, client, ctx := openContract(t, "1")
	if err := client.Set(ctx, "widget:a", "3", 0).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := client.Set(ctx, "widget:b", "1", 2*time.Minute).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := client.HSet(ctx, "profile:1", map[string]any{"name": "Ada", "plan": "pro"}).Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := client.SAdd(ctx, "tags:widget-a", "new", "featured").Err(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	dbs, err := p.Databases(ctx, h)
	if err != nil {
		t.Fatalf("Databases: %v", err)
	}
	if len(dbs) < 2 || dbs[0] != "0" || dbs[1] != "1" {
		t.Fatalf("Databases() = %v, want at least [0 1 ...]", dbs)
	}

	tables, err := p.Discover(ctx, h)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	groups := map[string]bool{}
	for _, table := range tables {
		if table.Kind != "keyspace" {
			t.Fatalf("kind = %q, want keyspace", table.Kind)
		}
		if len(table.Columns) != 0 {
			t.Fatalf(
				"expected no fictitious SQL columns for a keyspace, got %+v",
				table.Columns,
			)
		}
		groups[table.ObjectName] = true
	}
	for _, want := range []string{"widget", "profile", "tags"} {
		if !groups[want] {
			t.Fatalf(
				"Discover() missing group %q, got %v",
				want,
				groups,
			)
		}
	}

	str, err := p.InspectKey(ctx, conn, "widget:a", "", 0)
	if err != nil || !str.Exists || str.Type != "string" ||
		str.Value == nil || str.Value.Text != "3" || str.Length != 1 {
		t.Fatalf("InspectKey widget:a: %+v, %v", str, err)
	}
	if str.TTLMillis != nil {
		t.Fatalf("widget:a has no TTL, got %v", *str.TTLMillis)
	}

	withTTL, err := p.InspectKey(ctx, conn, "widget:b", "", 0)
	if err != nil || withTTL.TTLMillis == nil ||
		*withTTL.TTLMillis <= 60_000 {
		t.Fatalf("InspectKey widget:b (expected a TTL): %+v, %v", withTTL, err)
	}

	missing, err := p.InspectKey(ctx, conn, "does-not-exist", "", 0)
	if err != nil || missing.Exists {
		t.Fatalf("InspectKey missing key: %+v, %v", missing, err)
	}

	hash, err := p.InspectKey(ctx, conn, "profile:1", "", 0)
	if err != nil || hash.Type != "hash" || hash.Length != 2 ||
		len(hash.Members) != 2 || hash.Cursor != "" {
		t.Fatalf("InspectKey profile:1: %+v, %v", hash, err)
	}
	fields := map[string]string{}
	for _, m := range hash.Members {
		fields[m.Field.Text] = m.Value.Text
	}
	if fields["name"] != "Ada" || fields["plan"] != "pro" {
		t.Fatalf("hash fields = %v", fields)
	}

	// Scan lists metadata only, never values.
	scanned := p.Scan(ctx, conn, "widget:", false, 10)
	if scanned.Kind != "keyvalue" || len(scanned.Entries) != 2 {
		t.Fatalf("Scan widget:: %+v", scanned)
	}
	for _, e := range scanned.Entries {
		if e.Value != nil || e.Type != "string" || e.Length != 1 {
			t.Fatalf("Scan entry = %+v, want string metadata only", e)
		}
	}
	tags := p.Scan(ctx, conn, "tags", true, 10)
	if len(tags.Entries) != 1 || tags.Entries[0].Type != "set" ||
		tags.Entries[0].Length != 2 {
		t.Fatalf("Scan tags group: %+v", tags)
	}

	bounded := p.Scan(ctx, conn, "", false, 1)
	if bounded.Kind != "keyvalue" || len(bounded.Entries) != 1 ||
		!bounded.Truncated {
		t.Fatalf("bounded Scan: %+v", bounded)
	}
}

// TestScanMatchesLiterally covers the keyspace group bug: group "user"
// used to scan "user*" and pick up "username" and "users:9", and glob
// characters in a prefix were read as a pattern.
func TestScanMatchesLiterally(t *testing.T) {
	p, _, conn, client, ctx := openContract(t, "2")
	for _, k := range []string{
		"user", "user:1", "user:2", "username", "users:9",
		"a*b:1", "axb:1",
	} {
		if err := client.Set(ctx, k, "v", 0).Err(); err != nil {
			t.Fatalf("seed %s: %v", k, err)
		}
	}
	keys := func(prefix string, group bool) map[string]bool {
		t.Helper()
		rs := p.Scan(ctx, conn, prefix, group, 100)
		if rs.Kind != "keyvalue" {
			t.Fatalf("Scan(%q, %v): %+v", prefix, group, rs)
		}
		out := map[string]bool{}
		for _, e := range rs.Entries {
			out[e.Key] = true
		}
		return out
	}
	got := keys("user", true)
	if len(got) != 3 || !got["user"] || !got["user:1"] || !got["user:2"] {
		t.Fatalf("group user = %v, want user, user:1, user:2", got)
	}
	if got := keys("a*b", true); len(got) != 1 || !got["a*b:1"] {
		t.Fatalf("group a*b = %v, want only a*b:1", got)
	}
	if got := keys("user", false); len(got) != 5 {
		t.Fatalf("prefix user = %v, want all five user* keys", got)
	}
}

// TestInspectKeyPaging covers the reads that used to be silently capped
// (lists and sorted sets at 100) or unbounded (hashes and sets), binary
// values, and streams.
func TestInspectKeyPaging(t *testing.T) {
	p, _, conn, client, ctx := openContract(t, "3")

	list := make([]any, 250)
	for i := range list {
		list[i] = fmt.Sprintf("item-%03d", i)
	}
	if err := client.RPush(ctx, "big:list", list...).Err(); err != nil {
		t.Fatalf("seed list: %v", err)
	}
	var sizes []int
	cur := ""
	for {
		d, err := p.InspectKey(ctx, conn, "big:list", cur, 100)
		if err != nil {
			t.Fatalf("InspectKey list: %v", err)
		}
		if d.Length != 250 {
			t.Fatalf("list length = %d, want 250", d.Length)
		}
		if len(d.Members) > 0 && *d.Members[0].Index != int64(len(sizes)*100) {
			t.Fatalf("page %d starts at index %d", len(sizes), *d.Members[0].Index)
		}
		sizes = append(sizes, len(d.Members))
		if d.Cursor == "" {
			break
		}
		cur = d.Cursor
	}
	if fmt.Sprint(sizes) != "[100 100 50]" {
		t.Fatalf("list pages = %v, want [100 100 50]", sizes)
	}

	// 600 fields is past the listpack limit, so HSCAN really pages (and
	// may repeat a field, which is the caller's to dedupe).
	hash := map[string]any{}
	for i := range 600 {
		hash[fmt.Sprintf("f%03d", i)] = i
	}
	if err := client.HSet(ctx, "big:hash", hash).Err(); err != nil {
		t.Fatalf("seed hash: %v", err)
	}
	seen := map[string]bool{}
	pages := 0
	cur = ""
	for {
		d, err := p.InspectKey(ctx, conn, "big:hash", cur, 100)
		if err != nil {
			t.Fatalf("InspectKey hash: %v", err)
		}
		pages++
		for _, m := range d.Members {
			seen[m.Field.Text] = true
		}
		if d.Cursor == "" {
			break
		}
		if pages > 50 {
			t.Fatal("hash paging never finished")
		}
		cur = d.Cursor
	}
	if len(seen) != 600 || pages < 2 {
		t.Fatalf("hash paging saw %d fields in %d pages, want 600 in 2+", len(seen), pages)
	}

	if err := client.ZAdd(ctx, "big:zset",
		goredis.Z{Member: "b", Score: 2},
		goredis.Z{Member: "a", Score: 1.5},
	).Err(); err != nil {
		t.Fatalf("seed zset: %v", err)
	}
	z, err := p.InspectKey(ctx, conn, "big:zset", "", 1)
	if err != nil || len(z.Members) != 1 || z.Members[0].Value.Text != "a" ||
		*z.Members[0].Score != 1.5 || z.Cursor != "1" {
		t.Fatalf("zset first page: %+v, %v", z, err)
	}

	raw := "\xff\xfe\x00ok"
	if err := client.Set(ctx, "bin", raw, 0).Err(); err != nil {
		t.Fatalf("seed binary: %v", err)
	}
	bin, err := p.InspectKey(ctx, conn, "bin", "", 0)
	if err != nil || bin.Value == nil || !bin.Value.Binary ||
		bin.Value.Text != "" || bin.Value.Size != len(raw) {
		t.Fatalf("binary value: %+v, %v", bin.Value, err)
	}
	if b, _ := base64.StdEncoding.DecodeString(bin.Value.Base64); string(b) != raw {
		t.Fatalf("binary bytes = %q, want %q", b, raw)
	}

	for i := range 3 {
		if err := client.XAdd(ctx, &goredis.XAddArgs{
			Stream: "events",
			ID:     fmt.Sprintf("1-%d", i+1),
			Values: map[string]any{"n": i},
		}).Err(); err != nil {
			t.Fatalf("seed stream: %v", err)
		}
	}
	if err := client.XGroupCreate(ctx, "events", "workers", "0").Err(); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if err := client.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    "workers",
		Consumer: "w1",
		Streams:  []string{"events", ">"},
		Count:    2,
	}).Err(); err != nil {
		t.Fatalf("read group: %v", err)
	}
	st, err := p.InspectKey(ctx, conn, "events", "", 2)
	if err != nil || st.Type != "stream" || st.Length != 3 ||
		len(st.Members) != 2 || st.Cursor != "1-2" || st.Stream == nil {
		t.Fatalf("stream first page: %+v, %v", st, err)
	}
	if st.Stream.FirstID != "1-1" || st.Stream.LastID != "1-3" ||
		len(st.Stream.Groups) != 1 || st.Stream.Groups[0].Pending != 2 {
		t.Fatalf("stream info: %+v", st.Stream)
	}
	if g := st.Stream.Groups[0]; g.Lag == nil || *g.Lag != 1 {
		t.Fatalf("group lag = %v, want 1", g.Lag)
	}
	if st.Members[0].ID != "1-1" || st.Members[0].Fields["n"].Text != "0" {
		t.Fatalf("stream entry: %+v", st.Members[0])
	}
	rest, err := p.InspectKey(ctx, conn, "events", st.Cursor, 2)
	if err != nil || len(rest.Members) != 1 || rest.Members[0].ID != "1-3" ||
		rest.Stream != nil {
		t.Fatalf("stream second page: %+v, %v", rest, err)
	}
}

// TestApplyKeyEdit covers every edit op against a real Redis, and for each
// kind of check the conflict path: the edit is refused and nothing is
// written when the key no longer matches what the user saw.
func TestApplyKeyEdit(t *testing.T) {
	p, _, conn, client, ctx := openContract(t, "4")
	str := func(s string) *string { return &s }
	num := func(n float64) *float64 { return &n }
	idx := func(n int64) *int64 { return &n }
	apply := func(e model.KeyEdit) model.KeyEditResult {
		t.Helper()
		res, err := p.ApplyKeyEdit(ctx, conn, e)
		if err != nil {
			t.Fatalf("ApplyKeyEdit %+v: %v", e, err)
		}
		return res
	}
	applied := func(e model.KeyEdit) model.KeyEditResult {
		t.Helper()
		res := apply(e)
		if !res.Applied || res.Conflict {
			t.Fatalf("ApplyKeyEdit %+v = %+v, want applied", e, res)
		}
		return res
	}
	conflict := func(e model.KeyEdit, want string) {
		t.Helper()
		res := apply(e)
		if res.Applied || !res.Conflict || !strings.Contains(res.Message, want) {
			t.Fatalf("ApplyKeyEdit %+v = %+v, want conflict containing %q", e, res, want)
		}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// String: SET must keep the TTL (plain SET clears it), and a stale
	// value is a conflict.
	must(client.Set(ctx, "s", "a", 10*time.Minute).Err())
	applied(model.KeyEdit{Key: "s", Op: "set", Value: "b", OldValue: str("a")})
	if v := client.Get(ctx, "s").Val(); v != "b" {
		t.Fatalf("s = %q, want b", v)
	}
	if ttl := client.PTTL(ctx, "s").Val(); ttl < 9*time.Minute {
		t.Fatalf("set dropped the TTL: %v", ttl)
	}
	conflict(model.KeyEdit{Key: "s", Op: "set", Value: "c", OldValue: str("a")}, "changed since it was loaded")
	if v := client.Get(ctx, "s").Val(); v != "b" {
		t.Fatalf("conflicting set wrote %q", v)
	}
	// The pre-6.0 path (no KEEPTTL) must keep the TTL too.
	pl, err := planEdit(model.KeyEdit{Key: "s", Op: "set", Value: "d", OldValue: str("b")}, false)
	must(err)
	if res, err := watchAndApply(ctx, client, "s", pl); err != nil || !res.Applied {
		t.Fatalf("set without KEEPTTL: %+v, %v", res, err)
	}
	if ttl := client.PTTL(ctx, "s").Val(); client.Get(ctx, "s").Val() != "d" || ttl < 9*time.Minute {
		t.Fatalf("set without KEEPTTL: value %q ttl %v", client.Get(ctx, "s").Val(), ttl)
	}
	// A binary value is never overwritten with text.
	must(client.Set(ctx, "bin", "\xff\xfe", 0).Err())
	conflict(model.KeyEdit{Key: "bin", Op: "set", Value: "x", OldValue: str("��")}, "binary")

	// WATCH really guards the write: a change made after the check but
	// before EXEC aborts the transaction.
	pl, err = planEdit(model.KeyEdit{Key: "s", Op: "set", Value: "e", OldValue: str("d")}, true)
	must(err)
	check := pl.check
	pl.check = func(ctx context.Context, tx *goredis.Tx) (string, error) {
		msg, err := check(ctx, tx)
		must(client.Set(ctx, "s", "concurrent", goredis.KeepTTL).Err())
		return msg, err
	}
	if res, err := watchAndApply(ctx, client, "s", pl); err != nil || !res.Conflict {
		t.Fatalf("racing write: %+v, %v, want conflict", res, err)
	}
	if v := client.Get(ctx, "s").Val(); v != "concurrent" {
		t.Fatalf("racing write was overwritten: %q", v)
	}

	// Hash: edit, rename, a rename onto an existing field, and removing
	// the last field (which deletes the key).
	must(client.HSet(ctx, "h", "a", "1", "b", "2").Err())
	applied(model.KeyEdit{Key: "h", Op: "hset", Field: "a", OldField: str("a"), Value: "10", OldValue: str("1")})
	applied(model.KeyEdit{Key: "h", Op: "hset", Field: "c", OldField: str("a"), Value: "10", OldValue: str("10")})
	if got := client.HGetAll(ctx, "h").Val(); len(got) != 2 || got["c"] != "10" || got["b"] != "2" {
		t.Fatalf("hash after rename = %v", got)
	}
	conflict(model.KeyEdit{Key: "h", Op: "hset", Field: "b", OldField: str("c"), Value: "x", OldValue: str("10")}, "already exists")
	// Adding a new field, then adding it again.
	applied(model.KeyEdit{Key: "h", Op: "hset", Field: "n", Value: "x"})
	conflict(model.KeyEdit{Key: "h", Op: "hset", Field: "n", Value: "y"}, "already exists")
	applied(model.KeyEdit{Key: "h", Op: "hdel", Field: "n", OldValue: str("x")})
	conflict(model.KeyEdit{Key: "h", Op: "hdel", Field: "b", OldValue: str("stale")}, "changed since it was loaded")
	applied(model.KeyEdit{Key: "h", Op: "hdel", Field: "b", OldValue: str("2")})
	if res := applied(model.KeyEdit{Key: "h", Op: "hdel", Field: "c", OldValue: str("10")}); !res.KeyRemoved {
		t.Fatalf("removing the last field: %+v, want KeyRemoved", res)
	}
	if client.Exists(ctx, "h").Val() != 0 {
		t.Fatal("hash still exists after its last field was removed")
	}

	// List: a concurrent LPUSH shifts indexes, so a delete by the old
	// index is a conflict, not a wrong-element delete.
	must(client.RPush(ctx, "l", "a", "b", "c").Err())
	must(client.LPush(ctx, "l", "x").Err())
	conflict(model.KeyEdit{Key: "l", Op: "lrem", Index: idx(1), OldValue: str("b")}, "Element 1 changed")
	if got := client.LRange(ctx, "l", 0, -1).Val(); fmt.Sprint(got) != "[x a b c]" {
		t.Fatalf("conflicting lrem changed the list: %v", got)
	}
	applied(model.KeyEdit{Key: "l", Op: "lrem", Index: idx(2), OldValue: str("b")})
	applied(model.KeyEdit{Key: "l", Op: "lset", Index: idx(0), Value: "X", OldValue: str("x")})
	applied(model.KeyEdit{Key: "l", Op: "rpush", Value: "z"})
	applied(model.KeyEdit{Key: "l", Op: "lpush", Value: "w"})
	if got := client.LRange(ctx, "l", 0, -1).Val(); fmt.Sprint(got) != "[w X a c z]" {
		t.Fatalf("list after edits = %v, want [w X a c z]", got)
	}

	// Set: rename a member, refuse a rename onto an existing member,
	// remove one.
	must(client.SAdd(ctx, "set", "a", "b").Err())
	applied(model.KeyEdit{Key: "set", Op: "sadd", Value: "c", OldValue: str("a")})
	conflict(model.KeyEdit{Key: "set", Op: "sadd", Value: "b", OldValue: str("c")}, "already in the set")
	conflict(model.KeyEdit{Key: "set", Op: "srem", Value: "a"}, "no longer in the set")
	applied(model.KeyEdit{Key: "set", Op: "srem", Value: "b"})
	if got := client.SMembers(ctx, "set").Val(); fmt.Sprint(got) != "[c]" {
		t.Fatalf("set after edits = %v, want [c]", got)
	}

	// Sorted set: a score that changed since it was loaded is a conflict.
	must(client.ZAdd(ctx, "z", goredis.Z{Member: "a", Score: 1}).Err())
	conflict(model.KeyEdit{Key: "z", Op: "zadd", Value: "a", Score: num(5), OldValue: str("a"), OldScore: num(2)}, "score")
	applied(model.KeyEdit{Key: "z", Op: "zadd", Value: "a", Score: num(5), OldValue: str("a"), OldScore: num(1)})
	applied(model.KeyEdit{Key: "z", Op: "zadd", Value: "b", Score: num(2.5)})
	conflict(model.KeyEdit{Key: "z", Op: "zadd", Value: "a", Score: num(1)}, "already in the sorted set")
	applied(model.KeyEdit{Key: "z", Op: "zrem", Value: "a", OldScore: num(5)})
	if got := client.ZRangeWithScores(ctx, "z", 0, -1).Val(); len(got) != 1 || got[0].Member != "b" || got[0].Score != 2.5 {
		t.Fatalf("zset after edits = %v", got)
	}

	// Create refuses an existing key; delete refuses a missing one; an
	// add to a deleted key does not silently recreate it.
	conflict(model.KeyEdit{Key: "z", Op: "create", Type: "string", Value: "v"}, "already exists")
	applied(model.KeyEdit{Key: "new:h", Op: "create", Type: "hash", Field: "f", Value: "v"})
	if client.HGet(ctx, "new:h", "f").Val() != "v" {
		t.Fatal("create hash did not write its first field")
	}
	if res := applied(model.KeyEdit{Key: "new:h", Op: "del"}); !res.KeyRemoved {
		t.Fatalf("del: %+v, want KeyRemoved", res)
	}
	conflict(model.KeyEdit{Key: "new:h", Op: "del"}, "no longer exists")
	conflict(model.KeyEdit{Key: "new:h", Op: "rpush", Value: "v"}, "no longer exists")
	if client.Exists(ctx, "new:h").Val() != 0 {
		t.Fatal("a push to a deleted key recreated it")
	}
	conflict(model.KeyEdit{Key: "s", Op: "hset", Field: "f", Value: "v"}, "is now a string")

	if _, err := p.ApplyKeyEdit(ctx, conn, model.KeyEdit{Key: "s", Op: "set", Value: "v"}); err == nil {
		t.Fatal("set without OldValue was accepted")
	}
}

// TestKeyTTLEdit covers TTL editing: set, change, and remove an expiry; a
// missing key is a conflict; and a zero TTL (which PEXPIRE would turn
// into an immediate delete) is refused without touching the key.
func TestKeyTTLEdit(t *testing.T) {
	p, _, conn, client, ctx := openContract(t, "5")
	ms := func(n int64) *int64 { return &n }
	if err := client.Set(ctx, "k", "v", 0).Err(); err != nil {
		t.Fatal(err)
	}
	ttl := func() time.Duration {
		t.Helper()
		return client.PTTL(ctx, "k").Val()
	}

	for _, want := range []int64{3_600_000, 60_000} {
		res, err := p.ApplyKeyEdit(ctx, conn, model.KeyEdit{Key: "k", Op: "expire", TTLMillis: ms(want)})
		if err != nil || !res.Applied {
			t.Fatalf("expire %d: %+v, %v", want, res, err)
		}
		if got := ttl(); got <= 0 || got > time.Duration(want)*time.Millisecond {
			t.Fatalf("after expire %d, PTTL = %v", want, got)
		}
	}
	res, err := p.ApplyKeyEdit(ctx, conn, model.KeyEdit{Key: "k", Op: "persist"})
	if err != nil || !res.Applied {
		t.Fatalf("persist: %+v, %v", res, err)
	}
	if got := ttl(); got != -1 {
		t.Fatalf("after persist, PTTL = %v, want -1 (no expiry)", got)
	}

	for _, bad := range []*int64{nil, ms(0), ms(-5)} {
		if _, err := p.ApplyKeyEdit(ctx, conn, model.KeyEdit{Key: "k", Op: "expire", TTLMillis: bad}); err == nil {
			t.Fatalf("expire with TTL %v was accepted", bad)
		}
	}
	if client.Exists(ctx, "k").Val() != 1 {
		t.Fatal("a refused zero TTL deleted the key")
	}

	for _, op := range []string{"expire", "persist"} {
		res, err := p.ApplyKeyEdit(ctx, conn, model.KeyEdit{Key: "gone", Op: op, TTLMillis: ms(1000)})
		if err != nil || !res.Conflict || !strings.Contains(res.Message, "no longer exists") {
			t.Fatalf("%s on a missing key: %+v, %v, want conflict", op, res, err)
		}
	}
	if client.Exists(ctx, "gone").Val() != 0 {
		t.Fatal("a TTL edit created a missing key")
	}
}

// TestExecuteNative runs real commands through the native command path:
// replies shape into rows, refused commands never run (and a refused
// SELECT does not leave a pooled connection on another database), error
// replies are failed statements, and write detection matches the flags a
// read-only profile relies on.
func TestExecuteNative(t *testing.T) {
	p, _, conn, client, ctx := openContract(t, "6")
	run := func(line string) model.ResultSet {
		t.Helper()
		req, _ := json.Marshal(map[string]string{"command": line})
		return p.ExecuteNative(ctx, conn, string(req), 100)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(client.Set(ctx, "greeting", "hi", 0).Err())
	must(client.HSet(ctx, "profile:1", "name", "Ada", "plan", "pro").Err())
	must(client.RPush(ctx, "l", "a", "b", "c").Err())
	must(client.ZAdd(ctx, "z", goredis.Z{Member: "a", Score: 1}, goredis.Z{Member: "b", Score: 2}).Err())

	if rs := run("GET greeting"); rs.Kind != "rows" || rs.Rows[0]["reply"] != "hi" || rs.Statement != "GET greeting" {
		t.Fatalf("GET: %+v", rs)
	}
	if rs := run("GET missing"); rs.Kind != "rows" || len(rs.Rows) != 1 || rs.Rows[0]["reply"] != nil {
		t.Fatalf("GET missing: %+v", rs)
	}
	rs := run("HGETALL profile:1")
	if rs.Kind != "rows" || len(rs.Rows) != 2 || rs.Rows[0]["field"] != "name" || rs.Rows[0]["value"] != "Ada" {
		t.Fatalf("HGETALL: %+v", rs)
	}
	if rs := run("LRANGE l 0 -1"); len(rs.Rows) != 3 || rs.Rows[2]["value"] != "c" || rs.Rows[2]["#"] != 3 {
		t.Fatalf("LRANGE: %+v", rs)
	}
	rs = run("ZRANGE z 0 -1 WITHSCORES")
	if len(rs.Rows) != 2 {
		t.Fatalf("ZRANGE WITHSCORES: %+v", rs)
	}
	if pair, ok := rs.Rows[1]["value"].([]any); !ok || len(pair) != 2 || pair[0] != "b" || pair[1] != 2.0 {
		t.Fatalf("ZRANGE WITHSCORES pair = %#v, want [b 2]", rs.Rows[1]["value"])
	}
	rs = run("INFO server")
	found := false
	for _, r := range rs.Rows {
		found = found || r["field"] == "redis_version"
	}
	if !found {
		t.Fatalf("INFO rows have no redis_version: %+v", rs.Rows)
	}
	if rs := run(`SET quoted "hello world"`); rs.Kind != "rows" || client.Get(ctx, "quoted").Val() != "hello world" {
		t.Fatalf("SET with a quoted argument: %+v", rs)
	}
	if rs := run("INCR greeting"); rs.Kind != "error" || !strings.Contains(rs.Error, "not an integer") {
		t.Fatalf("INCR on text: %+v, want an error result", rs)
	}
	if rs := run("XREAD COUNT 1 STREAMS nostream 0"); rs.Kind != "rows" {
		t.Fatalf("XREAD without BLOCK should run: %+v", rs)
	}

	for _, line := range []string{
		"SELECT 2", "BLPOP l 0", "SUBSCRIBE c", "MULTI", "MONITOR",
		"XREAD BLOCK 0 STREAMS s $", "CLIENT SETNAME x", "WAIT 1 0",
	} {
		if rs := run(line); rs.Kind != "error" || rs.Error == "" {
			t.Fatalf("%s was not refused: %+v", line, rs)
		}
	}
	// SELECT was refused before it ran, so every pooled connection still
	// reads database 6.
	for range 20 {
		if rs := run("GET greeting"); rs.Rows[0]["reply"] != "hi" {
			t.Fatalf("after a refused SELECT, GET read %v", rs.Rows[0]["reply"])
		}
	}
	if l := client.LLen(ctx, "l").Val(); l != 3 {
		t.Fatalf("a refused BLPOP popped from the list: len %d", l)
	}

	for line, want := range map[string]bool{
		"GET greeting":             false,
		"HGETALL profile:1":        false,
		"SET k v":                  true,
		"DEL k":                    true,
		"FLUSHDB":                  true,
		"CONFIG SET maxmemory 0":   true,
		"EVAL \"return 1\" 0":      true,
		"EVAL_RO \"return 1\" 0":   false,
		"CLIENT LIST":              true,
		"nosuchcommand":            false,
		"OBJECT ENCODING greeting": false,
		"XINFO STREAM nostream":    false,
		"MEMORY USAGE greeting":    false,
		"PUBLISH c hello":          true,
		"SCRIPT FLUSH":             true,
		"FUNCTION LIST":            true,
		"PING":                     false,
		"INFO server":              false,
		"SCAN 0":                   false,
		"TTL greeting":             false,
		"FCALL f 0":                true,
	} {
		req, _ := json.Marshal(map[string]string{"command": line})
		got, err := p.NativeQueryWrites(ctx, conn, string(req))
		if err != nil || got != want {
			t.Errorf("NativeQueryWrites(%q) = %v, %v; want %v", line, got, err, want)
		}
	}

	if rs := run("   "); rs.Kind != "error" {
		t.Fatalf("an empty command ran: %+v", rs)
	}
	if rs := run(`SET k "open`); rs.Kind != "error" || !strings.Contains(rs.Error, "quote") {
		t.Fatalf("an unbalanced quote ran: %+v", rs)
	}
}
