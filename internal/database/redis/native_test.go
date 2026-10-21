package redis

import (
	"slices"
	"testing"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// TestSplitArgs follows redis-cli's own argument rules (sdssplitargs), so
// a command pasted from redis-cli or the Redis docs means the same here.
func TestSplitArgs(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"GET user:1", []string{"GET", "user:1"}},
		{"  set   k   v  ", []string{"set", "k", "v"}},
		{`SET k "hello world"`, []string{"SET", "k", "hello world"}},
		{`SET k "a\"b\n\t"`, []string{"SET", "k", "a\"b\n\t"}},
		{`SET k "\xff\x00z"`, []string{"SET", "k", "\xff\x00z"}},
		{`SET k "\xzz"`, []string{"SET", "k", "xzz"}},
		{`SET k 'it\'s'`, []string{"SET", "k", "it's"}},
		{`SET k 'no \n escape'`, []string{"SET", "k", `no \n escape`}},
		{`SET k ""`, []string{"SET", "k", ""}},
		{`SET a"b c" x`, []string{"SET", "ab c", "x"}},
		{"", nil},
	} {
		got, err := splitArgs(c.in)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("splitArgs(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{`SET k "open`, `SET k 'open`, `SET k "a"b`, `SET k 'a'b`} {
		if got, err := splitArgs(bad); err == nil {
			t.Errorf("splitArgs(%q) = %q, want an error", bad, got)
		}
	}
}

func TestShapeReply(t *testing.T) {
	rs := shapeReply("hgetall", []any{"name", "Ada", "plan", "pro"}, 0)
	if len(rs.Rows) != 2 || rs.Rows[0]["field"] != "name" || rs.Rows[1]["value"] != "pro" {
		t.Fatalf("RESP2 HGETALL rows = %v", rs.Rows)
	}
	rs = shapeReply("hgetall", map[any]any{"plan": "pro", "name": "Ada"}, 0)
	if len(rs.Rows) != 2 || rs.Rows[0]["field"] != "name" {
		t.Fatalf("RESP3 map rows = %v, want sorted by field", rs.Rows)
	}
	rs = shapeReply("lrange", []any{"a", "b", "c"}, 2)
	if len(rs.Rows) != 2 || !rs.Truncated || rs.Rows[1]["#"] != 2 {
		t.Fatalf("capped array = %+v", rs)
	}
	rs = shapeReply("info", "# Server\r\nredis_version:7.2.0\r\n\r\nrole:master\r\n", 0)
	if len(rs.Rows) != 2 || rs.Rows[0]["value"] != "7.2.0" {
		t.Fatalf("INFO rows = %v", rs.Rows)
	}
	rs = shapeReply("get", "\xff", 0)
	if b, ok := rs.Rows[0]["reply"].(model.BlobValue); !ok || b.Base64 != "/w==" {
		t.Fatalf("a non-UTF-8 reply = %#v, want a BlobValue of its exact bytes", rs.Rows[0]["reply"])
	}
}
