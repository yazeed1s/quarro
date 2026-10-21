package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// editPlan is one KeyEdit as a conflict check plus a write. check runs
// under WATCH and returns a conflict message, or "" when the key is still
// what the user saw; write queues the commands for MULTI/EXEC.
type editPlan struct {
	check func(ctx context.Context, tx *redis.Tx) (string, error)
	write func(ctx context.Context, p redis.Pipeliner)
}

// ApplyKeyEdit applies one edit with optimistic locking: WATCH the key,
// compare it against what the user saw, then write in MULTI/EXEC. go-redis
// runs Watch on its own dedicated connection, which matters because the
// client is a pool and a WATCH on some other pooled connection would
// guard nothing.
func (Provider) ApplyKeyEdit(
	ctx context.Context,
	conn any,
	e model.KeyEdit,
) (model.KeyEditResult, error) {
	c := conn.(*redis.Client)
	pl, err := planEdit(e, true)
	if err != nil {
		return model.KeyEditResult{}, err
	}
	res, err := watchAndApply(ctx, c, e.Key, pl)
	if e.Op == "set" && err != nil && isOldSetErr(err) {
		// SET ... KEEPTTL needs Redis 6.0. An older server rejects the
		// queued command, so EXEC ran nothing; redo it the long way.
		pl, _ = planEdit(e, false)
		res, err = watchAndApply(ctx, c, e.Key, pl)
	}
	return res, err
}

// watchAndApply runs pl.check under WATCH key and, when it passes,
// pl.write in MULTI/EXEC. EXISTS is queued after the write so the result
// can say whether the key survived it.
func watchAndApply(
	ctx context.Context,
	c *redis.Client,
	key string,
	pl editPlan,
) (model.KeyEditResult, error) {
	var res model.KeyEditResult
	err := c.Watch(ctx, func(tx *redis.Tx) error {
		msg, err := pl.check(ctx, tx)
		if err != nil {
			return err
		}
		if msg != "" {
			res = model.KeyEditResult{Conflict: true, Message: msg}
			return nil
		}
		var ex *redis.IntCmd
		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			pl.write(ctx, p)
			ex = p.Exists(ctx, key)
			return nil
		})
		if err != nil {
			return err
		}
		res = model.KeyEditResult{Applied: true, KeyRemoved: ex.Val() == 0}
		return nil
	}, key)
	if errors.Is(err, redis.TxFailedErr) {
		// EXEC returned nil: the watched key changed between the check and
		// the write. Nothing was written.
		return model.KeyEditResult{
			Conflict: true,
			Message:  "This key changed while the edit was being applied. Reload it and try again.",
		}, nil
	}
	if err != nil {
		return model.KeyEditResult{}, err
	}
	return res, nil
}

// isOldSetErr reports a pre-6.0 server refusing SET's KEEPTTL option.
func isOldSetErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "syntax error") || strings.Contains(s, "EXECABORT")
}

// planEdit validates e and builds its check and write. keep selects SET
// KEEPTTL for a string write; without it the TTL is read under WATCH and
// restored with PEXPIRE in the same transaction.
func planEdit(e model.KeyEdit, keep bool) (editPlan, error) {
	k := e.Key
	if k == "" {
		return editPlan{}, fmt.Errorf("key is required")
	}
	need := func(ok bool, what string) error {
		if !ok {
			return fmt.Errorf("%s requires %s", e.Op, what)
		}
		return nil
	}
	switch e.Op {
	case "set":
		if err := need(e.OldValue != nil, "the value it replaces"); err != nil {
			return editPlan{}, err
		}
		var ttl time.Duration
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				if msg, err := wantType(ctx, tx, k, "string"); msg != "" || err != nil {
					return msg, err
				}
				cur, err := tx.Get(ctx, k).Result()
				if msg, err := compare(cur, err, e.OldValue, "This value"); msg != "" || err != nil {
					return msg, err
				}
				if !keep {
					ttl, err = tx.PTTL(ctx, k).Result()
				}
				return "", err
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				if keep {
					p.SetArgs(ctx, k, e.Value, redis.SetArgs{KeepTTL: true})
					return
				}
				p.Set(ctx, k, e.Value, 0)
				if ttl > 0 {
					p.PExpire(ctx, k, ttl)
				}
			},
		}, nil

	case "hset":
		if err := need(e.Field != "", "a field name"); err != nil {
			return editPlan{}, err
		}
		if err := need(e.OldField == nil || e.OldValue != nil, "the value it replaces"); err != nil {
			return editPlan{}, err
		}
		renamed := e.OldField != nil && *e.OldField != e.Field
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				if msg, err := wantType(ctx, tx, k, "hash"); msg != "" || err != nil {
					return msg, err
				}
				if e.OldField != nil {
					cur, err := tx.HGet(ctx, k, *e.OldField).Result()
					what := fmt.Sprintf("Field %q", *e.OldField)
					if msg, err := compare(cur, err, e.OldValue, what); msg != "" || err != nil {
						return msg, err
					}
				}
				if e.OldField == nil || renamed {
					ok, err := tx.HExists(ctx, k, e.Field).Result()
					if err != nil || ok {
						return taken(ok, "Field %q already exists.", e.Field), err
					}
				}
				return "", nil
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				if renamed {
					p.HDel(ctx, k, *e.OldField)
				}
				p.HSet(ctx, k, e.Field, e.Value)
			},
		}, nil

	case "hdel":
		if err := need(e.Field != "" && e.OldValue != nil, "a field and its value"); err != nil {
			return editPlan{}, err
		}
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				if msg, err := wantType(ctx, tx, k, "hash"); msg != "" || err != nil {
					return msg, err
				}
				cur, err := tx.HGet(ctx, k, e.Field).Result()
				return compare(cur, err, e.OldValue, fmt.Sprintf("Field %q", e.Field))
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				p.HDel(ctx, k, e.Field)
			},
		}, nil

	case "lset", "lrem":
		if err := need(e.Index != nil && e.OldValue != nil, "an index and its value"); err != nil {
			return editPlan{}, err
		}
		i := *e.Index
		// Lists have no delete-by-index. Overwrite the element with a value
		// nothing else holds, then remove that value once; both run in the
		// same MULTI, after the check that index i still holds what the
		// user saw, so a concurrent push that shifted indexes is a conflict
		// rather than a wrong-element delete.
		mark, err := sentinel()
		if err != nil {
			return editPlan{}, err
		}
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				if msg, err := wantType(ctx, tx, k, "list"); msg != "" || err != nil {
					return msg, err
				}
				cur, err := tx.LIndex(ctx, k, i).Result()
				return compare(cur, err, e.OldValue, fmt.Sprintf("Element %d", i))
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				if e.Op == "lset" {
					p.LSet(ctx, k, i, e.Value)
					return
				}
				p.LSet(ctx, k, i, mark)
				p.LRem(ctx, k, 1, mark)
			},
		}, nil

	case "lpush", "rpush":
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				return wantType(ctx, tx, k, "list")
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				if e.Op == "lpush" {
					p.LPush(ctx, k, e.Value)
					return
				}
				p.RPush(ctx, k, e.Value)
			},
		}, nil

	case "sadd":
		renamed := e.OldValue != nil && *e.OldValue != e.Value
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				if msg, err := wantType(ctx, tx, k, "set"); msg != "" || err != nil {
					return msg, err
				}
				if e.OldValue != nil {
					ok, err := tx.SIsMember(ctx, k, *e.OldValue).Result()
					if err != nil || !ok {
						return taken(!ok, "Member %q is no longer in the set.", *e.OldValue), err
					}
				}
				if e.OldValue == nil || renamed {
					ok, err := tx.SIsMember(ctx, k, e.Value).Result()
					if err != nil || ok {
						return taken(ok, "Member %q is already in the set.", e.Value), err
					}
				}
				return "", nil
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				if renamed {
					p.SRem(ctx, k, *e.OldValue)
				}
				p.SAdd(ctx, k, e.Value)
			},
		}, nil

	case "srem":
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				if msg, err := wantType(ctx, tx, k, "set"); msg != "" || err != nil {
					return msg, err
				}
				ok, err := tx.SIsMember(ctx, k, e.Value).Result()
				return taken(err == nil && !ok, "Member %q is no longer in the set.", e.Value), err
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				p.SRem(ctx, k, e.Value)
			},
		}, nil

	case "zadd", "zrem":
		if err := need(e.Op == "zrem" || e.Score != nil, "a score"); err != nil {
			return editPlan{}, err
		}
		// For zadd, OldValue is the member being edited (nil for a new one);
		// for zrem, Value is the member being removed.
		old := e.OldValue
		if e.Op == "zrem" {
			old = &e.Value
		}
		renamed := e.Op == "zadd" && old != nil && *old != e.Value
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				if msg, err := wantType(ctx, tx, k, "zset"); msg != "" || err != nil {
					return msg, err
				}
				if old != nil {
					sc, err := tx.ZScore(ctx, k, *old).Result()
					if errors.Is(err, redis.Nil) {
						return fmt.Sprintf("Member %q is no longer in the sorted set.", *old), nil
					}
					if err != nil {
						return "", err
					}
					if e.OldScore != nil && sc != *e.OldScore {
						return fmt.Sprintf("The score of %q changed since it was loaded.", *old), nil
					}
				}
				if e.Op == "zadd" && (old == nil || renamed) {
					_, err := tx.ZScore(ctx, k, e.Value).Result()
					if err == nil {
						return fmt.Sprintf("Member %q is already in the sorted set.", e.Value), nil
					}
					if !errors.Is(err, redis.Nil) {
						return "", err
					}
				}
				return "", nil
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				if e.Op == "zrem" {
					p.ZRem(ctx, k, e.Value)
					return
				}
				if renamed {
					p.ZRem(ctx, k, *old)
				}
				p.ZAdd(ctx, k, redis.Z{Score: *e.Score, Member: e.Value})
			},
		}, nil

	case "create":
		write, err := createWrite(e)
		if err != nil {
			return editPlan{}, err
		}
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				n, err := tx.Exists(ctx, k).Result()
				return taken(err == nil && n > 0, "A key named %q already exists.", k), err
			},
			write: write,
		}, nil

	case "del":
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				n, err := tx.Exists(ctx, k).Result()
				return taken(err == nil && n == 0, "Key %q no longer exists.", k), err
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				p.Unlink(ctx, k)
			},
		}, nil

	case "expire", "persist":
		// PEXPIRE with zero or a negative TTL deletes the key at once: a
		// delete disguised as a TTL edit. Deleting stays the explicit del.
		if e.Op == "expire" && (e.TTLMillis == nil || *e.TTLMillis <= 0) {
			return editPlan{}, fmt.Errorf("expire requires a TTL greater than zero")
		}
		// A TTL belongs to the key, not its content, so only a deleted key
		// is a conflict; a concurrent value change is not.
		return editPlan{
			check: func(ctx context.Context, tx *redis.Tx) (string, error) {
				n, err := tx.Exists(ctx, k).Result()
				return taken(err == nil && n == 0, "Key %q no longer exists.", k), err
			},
			write: func(ctx context.Context, p redis.Pipeliner) {
				if e.Op == "persist" {
					p.Persist(ctx, k)
					return
				}
				p.PExpire(ctx, k, time.Duration(*e.TTLMillis)*time.Millisecond)
			},
		}, nil
	}
	return editPlan{}, fmt.Errorf("unsupported key edit %q", e.Op)
}

// createWrite builds the first write of a new key. Redis has no empty
// hash, list, set, or sorted set, so a new key starts with one element.
func createWrite(
	e model.KeyEdit,
) (func(context.Context, redis.Pipeliner), error) {
	k := e.Key
	switch e.Type {
	case "string":
		return func(ctx context.Context, p redis.Pipeliner) {
			p.Set(ctx, k, e.Value, 0)
		}, nil
	case "hash":
		if e.Field == "" {
			return nil, fmt.Errorf("a new hash needs a first field")
		}
		return func(ctx context.Context, p redis.Pipeliner) {
			p.HSet(ctx, k, e.Field, e.Value)
		}, nil
	case "list":
		return func(ctx context.Context, p redis.Pipeliner) {
			p.RPush(ctx, k, e.Value)
		}, nil
	case "set":
		return func(ctx context.Context, p redis.Pipeliner) {
			p.SAdd(ctx, k, e.Value)
		}, nil
	case "zset":
		if e.Score == nil {
			return nil, fmt.Errorf("a new sorted set needs a first score")
		}
		return func(ctx context.Context, p redis.Pipeliner) {
			p.ZAdd(ctx, k, redis.Z{Score: *e.Score, Member: e.Value})
		}, nil
	}
	return nil, fmt.Errorf("cannot create a %q key here", e.Type)
}

// wantType returns a conflict message unless key still exists with type
// t. Checking this first keeps an edit from resurrecting a key someone
// deleted (a push or add to a missing key would silently recreate it).
func wantType(
	ctx context.Context,
	tx *redis.Tx,
	key, t string,
) (string, error) {
	cur, err := tx.Type(ctx, key).Result()
	if err != nil {
		return "", err
	}
	if cur == "none" {
		return "This key no longer exists.", nil
	}
	if cur != t {
		return fmt.Sprintf("This key is now a %s, not a %s.", cur, t), nil
	}
	return "", nil
}

// compare checks a read (cur, err) against what the user saw. A missing
// value, a binary value, or a different value is a conflict.
func compare(
	cur string,
	err error,
	old *string,
	what string,
) (string, error) {
	if errors.Is(err, redis.Nil) {
		return what + " no longer exists.", nil
	}
	if err != nil {
		return "", err
	}
	// The frontend never offers binary values for editing; this guards a
	// stale or hand-built request from writing over one with mangled text.
	if !utf8.ValidString(cur) {
		return what + " is binary and cannot be edited here.", nil
	}
	if old != nil && cur != *old {
		return what + " changed since it was loaded.", nil
	}
	return "", nil
}

// taken formats msg when bad is true, and returns "" otherwise.
func taken(bad bool, msg string, arg string) string {
	if !bad {
		return ""
	}
	return fmt.Sprintf(msg, arg)
}

// sentinel returns a value no real list element holds, for lrem.
func sentinel() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "quarro-lrem-" + hex.EncodeToString(b), nil
}
