package redis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// nativeReq is the envelope ExecuteNative's req carries: one command line,
// exactly as the user typed it, redis-cli style.
type nativeReq struct {
	Command string `json:"command"`
}

// ExecuteNative runs one redis-cli style command line and shapes the reply
// into rows, so the grid, export, and visualize paths work on it unchanged.
// A refused command (see refusal) never reaches the server.
func (Provider) ExecuteNative(
	ctx context.Context,
	conn any,
	req string,
	maxRows int,
) model.ResultSet {
	c := conn.(*redis.Client)
	line, args, err := parseNative(req)
	if err != nil {
		return model.ResultSet{Kind: "error", Statement: line, Error: err.Error()}
	}
	msg, err := refusal(ctx, c, args)
	if err != nil {
		return model.ResultSet{Kind: "error", Statement: line, Error: err.Error()}
	}
	if msg != "" {
		return model.ResultSet{Kind: "error", Statement: line, Error: msg}
	}
	cmd := make([]any, len(args))
	for i, a := range args {
		cmd[i] = a
	}
	v, err := c.Do(ctx, cmd...).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		// An error reply (WRONGTYPE, a bad argument) is a failed statement,
		// the same as a SQL error; a lost connection is reported the same
		// way since this surface has no separate channel for it.
		return model.ResultSet{Kind: "error", Statement: line, Error: err.Error()}
	}
	rs := shapeReply(strings.ToLower(args[0]), v, maxRows)
	rs.Statement = line
	return rs
}

// readSafe are server reads Redis does not flag readonly (they touch no
// keys), allowed on a read-only profile anyway.
var readSafe = map[string]bool{
	"ping": true, "echo": true, "info": true, "time": true,
	"lastsave": true, "role": true, "command": true,
}

// NativeQueryWrites reports whether req's command may modify anything,
// for a read-only profile to refuse it. Only commands Redis itself flags
// readonly (plus readSafe) count as reads. Excluding "write" alone is not
// enough: EVAL, FCALL, PUBLISH, SCRIPT FLUSH, and CONFIG SET are not
// flagged write but can change data or server state. This errs toward
// refusing some reads on a read-only profile (CONFIG GET, CLIENT LIST),
// which is the profile's point. An unknown command counts as a read: the
// server rejects it anyway.
func (Provider) NativeQueryWrites(
	ctx context.Context,
	conn any,
	req string,
) (bool, error) {
	c := conn.(*redis.Client)
	_, args, err := parseNative(req)
	if err != nil {
		return false, err
	}
	m, err := commandMeta(ctx, c, args)
	if err != nil {
		return false, err
	}
	if m.flags == nil || readSafe[strings.ToLower(args[0])] {
		return false, nil
	}
	return !m.has("readonly"), nil
}

func parseNative(req string) (string, []string, error) {
	var r nativeReq
	if err := json.Unmarshal([]byte(req), &r); err != nil {
		return "", nil, fmt.Errorf("invalid command request: %w", err)
	}
	line := strings.TrimSpace(r.Command)
	args, err := splitArgs(line)
	if err != nil {
		return line, nil, err
	}
	if len(args) == 0 {
		return line, nil, fmt.Errorf("nothing to run: type a command first")
	}
	return line, args, nil
}

// splitArgs splits a command line the way redis-cli does (its
// sdssplitargs): whitespace separates arguments; double quotes allow
// \n \r \t \b \a \\ \" and \xHH escapes; single quotes allow only \'. A
// closing quote must be followed by whitespace or the end of the line.
func splitArgs(line string) ([]string, error) {
	var out []string
	p, n := 0, len(line)
	isSpace := func(b byte) bool { return b == ' ' || b == '\n' || b == '\r' || b == '\t' }
	for {
		for p < n && isSpace(line[p]) {
			p++
		}
		if p == n {
			return out, nil
		}
		var cur []byte
		inDq, inSq := false, false
		for done := false; !done; p++ {
			switch {
			case inDq:
				if p == n {
					return nil, fmt.Errorf("unbalanced double quote")
				}
				ch := line[p]
				if ch == '\\' && p+3 < n && line[p+1] == 'x' && isHex(line[p+2]) && isHex(line[p+3]) {
					cur = append(cur, hexVal(line[p+2])<<4|hexVal(line[p+3]))
					p += 3
				} else if ch == '\\' && p+1 < n {
					p++
					switch line[p] {
					case 'n':
						cur = append(cur, '\n')
					case 'r':
						cur = append(cur, '\r')
					case 't':
						cur = append(cur, '\t')
					case 'b':
						cur = append(cur, '\b')
					case 'a':
						cur = append(cur, '\a')
					default:
						cur = append(cur, line[p])
					}
				} else if ch == '"' {
					if p+1 < n && !isSpace(line[p+1]) {
						return nil, fmt.Errorf("closing quote must be followed by a space")
					}
					done = true
				} else {
					cur = append(cur, ch)
				}
			case inSq:
				if p == n {
					return nil, fmt.Errorf("unbalanced single quote")
				}
				ch := line[p]
				if ch == '\\' && p+1 < n && line[p+1] == '\'' {
					cur = append(cur, '\'')
					p++
				} else if ch == '\'' {
					if p+1 < n && !isSpace(line[p+1]) {
						return nil, fmt.Errorf("closing quote must be followed by a space")
					}
					done = true
				} else {
					cur = append(cur, ch)
				}
			default:
				if p == n || isSpace(line[p]) {
					done = true
				} else if line[p] == '"' {
					inDq = true
				} else if line[p] == '\'' {
					inSq = true
				} else {
					cur = append(cur, line[p])
				}
			}
		}
		out = append(out, string(cur))
		// The loop's p++ ran once past the terminator; step back so a
		// token ending exactly at the end of the line is not skipped over.
		if p > n {
			p = n
		}
	}
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func hexVal(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	}
	return b - 'A' + 10
}

// refusedCmds are refused whatever the server says about them. The client
// is a connection pool, so connection-state commands would change one
// arbitrary pooled connection and silently affect later commands that
// happen to land on it; subscribe-style commands and MONITOR never return
// to request/reply.
var refusedCmds = map[string]string{
	"select":       "SELECT would switch the database of one pooled connection only. Use the session's database selector instead.",
	"auth":         "AUTH changes one pooled connection's identity. Reconnect with different credentials instead.",
	"hello":        "HELLO changes one pooled connection's protocol and identity, which the app relies on.",
	"reset":        "RESET clears one pooled connection's state, which the app relies on.",
	"quit":         "QUIT closes one pooled connection. Disconnect the session instead.",
	"multi":        "Transactions span several commands on one connection, and each run here may use a different pooled connection.",
	"exec":         "Transactions span several commands on one connection, and each run here may use a different pooled connection.",
	"discard":      "Transactions span several commands on one connection, and each run here may use a different pooled connection.",
	"watch":        "WATCH only guards later commands on the same connection, and each run here may use a different pooled connection.",
	"unwatch":      "WATCH only guards later commands on the same connection, and each run here may use a different pooled connection.",
	"shutdown":     "SHUTDOWN stops the server; it is not run from here.",
	"debug":        "DEBUG can crash or stall the server; it is not run from here.",
	"monitor":      "MONITOR streams every command and never returns a reply.",
	"subscribe":    "Subscribing turns the connection into a message stream that never returns a reply.",
	"psubscribe":   "Subscribing turns the connection into a message stream that never returns a reply.",
	"ssubscribe":   "Subscribing turns the connection into a message stream that never returns a reply.",
	"unsubscribe":  "There is no subscription to leave: subscribing is not run from here.",
	"punsubscribe": "There is no subscription to leave: subscribing is not run from here.",
	"sunsubscribe": "There is no subscription to leave: subscribing is not run from here.",
	"sync":         "SYNC starts a replication stream that never returns a reply.",
	"psync":        "PSYNC starts a replication stream that never returns a reply.",
}

// blockingCmds backs up the "blocking" flag, which Redis only reports from
// 7.0 on.
var blockingCmds = map[string]bool{
	"blpop": true, "brpop": true, "brpoplpush": true, "blmove": true,
	"blmpop": true, "bzpopmin": true, "bzpopmax": true, "bzmpop": true,
	"wait": true, "waitaof": true,
}

// refusal returns why args must not run here, or "" when it may.
func refusal(
	ctx context.Context,
	c *redis.Client,
	args []string,
) (string, error) {
	name := strings.ToLower(args[0])
	if msg, ok := refusedCmds[name]; ok {
		return msg, nil
	}
	if name == "client" && len(args) > 1 {
		switch strings.ToLower(args[1]) {
		case "setname", "reply", "tracking":
			return "CLIENT " + strings.ToUpper(args[1]) + " changes one pooled connection's state, which later commands may silently inherit.", nil
		}
	}
	block := "Blocking commands hold a pooled connection until they time out. Use the non-blocking form instead (e.g. LPOP for BLPOP)."
	if name == "xread" || name == "xreadgroup" {
		// XREAD is flagged as possibly blocking, but only blocks with BLOCK.
		if slices.ContainsFunc(args[1:], func(a string) bool { return strings.EqualFold(a, "block") }) {
			return block, nil
		}
		return "", nil
	}
	if blockingCmds[name] {
		return block, nil
	}
	m, err := commandMeta(ctx, c, args)
	if err != nil {
		return "", err
	}
	if m.has("blocking") {
		return block, nil
	}
	return "", nil
}

// cmdMeta is what COMMAND INFO says about one command or subcommand.
type cmdMeta struct {
	flags []string
	subs  bool // a container command (CONFIG, CLIENT) with subcommands
}

func (m cmdMeta) has(f string) bool { return slices.Contains(m.flags, f) }

// metaCache holds COMMAND INFO results per client (a *sync.Map of command
// name to cmdMeta), so each command is looked up once per session. Close
// drops a client's entry.
var metaCache sync.Map

// commandMeta looks up args' command, or its subcommand ("config|set")
// when the command is a container, since Redis 7 flags a container's
// subcommands individually and leaves the container itself unflagged.
func commandMeta(
	ctx context.Context,
	c *redis.Client,
	args []string,
) (cmdMeta, error) {
	v, _ := metaCache.LoadOrStore(c, &sync.Map{})
	cache := v.(*sync.Map)
	name := strings.ToLower(args[0])
	m, err := lookupMeta(ctx, c, cache, name)
	if err != nil || !m.subs || len(args) < 2 {
		return m, err
	}
	sub, err := lookupMeta(ctx, c, cache, name+"|"+strings.ToLower(args[1]))
	if err != nil {
		return cmdMeta{}, err
	}
	if sub.flags == nil {
		return m, nil
	}
	return sub, nil
}

func lookupMeta(
	ctx context.Context,
	c *redis.Client,
	cache *sync.Map,
	name string,
) (cmdMeta, error) {
	if v, ok := cache.Load(name); ok {
		return v.(cmdMeta), nil
	}
	v, err := c.Do(ctx, "command", "info", name).Result()
	if err != nil {
		return cmdMeta{}, err
	}
	var m cmdMeta
	// The reply is one entry per name asked for: nil for an unknown
	// command, else [name, arity, flags, first, last, step, acl
	// categories, tips, key specs, subcommands] (fields after step are
	// Redis 7+).
	if list, ok := v.([]any); ok && len(list) == 1 {
		if e, ok := list[0].([]any); ok && len(e) > 2 {
			m.flags = []string{}
			if fs, ok := e[2].([]any); ok {
				for _, f := range fs {
					m.flags = append(m.flags, strings.ToLower(fmt.Sprint(f)))
				}
			}
			if len(e) > 9 {
				subs, _ := e[9].([]any)
				m.subs = len(subs) > 0
			}
		}
	}
	cache.Store(name, m)
	return m, nil
}

// pairReplies return a flat [field, value, field, value, ...] array over
// RESP2 (RESP3 already sends a map), shown as field/value rows.
var pairReplies = map[string]bool{"hgetall": true, "config": true}

// shapeReply turns a reply into rows: a scalar is one "reply" row, a map
// or a known field/value array is field/value rows, INFO's text is split
// into field/value rows, and any other array is one row per element with
// nested values left nested for the grid's JSON tree.
func shapeReply(name string, v any, maxRows int) model.ResultSet {
	cols := func(names ...string) []model.ColumnMeta {
		out := make([]model.ColumnMeta, len(names))
		for i, n := range names {
			out[i] = model.ColumnMeta{Name: n}
		}
		return out
	}
	var rows []map[string]any
	var rs model.ResultSet
	switch t := v.(type) {
	case map[any]any:
		rs.Columns = cols("field", "value")
		keys := make([]string, 0, len(t))
		vals := make(map[string]any, len(t))
		for k, x := range t {
			ks := fmt.Sprint(k)
			keys = append(keys, ks)
			vals[ks] = x
		}
		sort.Strings(keys)
		for _, k := range keys {
			rows = append(rows, map[string]any{"field": cell(k), "value": cell(vals[k])})
		}
	case []any:
		if pairReplies[name] && len(t)%2 == 0 && allScalar(t) {
			rs.Columns = cols("field", "value")
			for i := 0; i+1 < len(t); i += 2 {
				rows = append(rows, map[string]any{"field": cell(t[i]), "value": cell(t[i+1])})
			}
			break
		}
		rs.Columns = cols("#", "value")
		for i, x := range t {
			rows = append(rows, map[string]any{"#": i + 1, "value": cell(x)})
		}
	case string:
		if name == "info" {
			rs.Columns = cols("field", "value")
			for _, ln := range strings.Split(t, "\n") {
				ln = strings.TrimSpace(ln)
				f, val, ok := strings.Cut(ln, ":")
				if ln == "" || strings.HasPrefix(ln, "#") || !ok {
					continue
				}
				rows = append(rows, map[string]any{"field": f, "value": val})
			}
			break
		}
		rs.Columns = cols("reply")
		rows = []map[string]any{{"reply": cell(t)}}
	default:
		rs.Columns = cols("reply")
		rows = []map[string]any{{"reply": cell(t)}}
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
		rs.Truncated = true
	}
	rs.Kind = "rows"
	rs.Rows = rows
	return rs
}

func allScalar(xs []any) bool {
	for _, x := range xs {
		switch x.(type) {
		case []any, map[any]any:
			return false
		}
	}
	return true
}

// cell converts one reply value for the grid. A string that is not valid
// UTF-8 becomes a BlobValue (JSON would replace its bytes), an error
// inside an array reply becomes readable text, and arrays and maps stay
// nested.
func cell(v any) any {
	switch t := v.(type) {
	case string:
		if utf8.ValidString(t) {
			return t
		}
		return model.BlobValue{
			Blob:   true,
			Size:   len(t),
			Base64: base64.StdEncoding.EncodeToString([]byte(t)),
		}
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = cell(x)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[fmt.Sprint(k)] = cell(x)
		}
		return out
	case *big.Int:
		return t.String()
	case error:
		return "(error) " + t.Error()
	case nil, int64, float64, bool:
		return t
	}
	return fmt.Sprint(v)
}
