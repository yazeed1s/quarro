package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// KeyValueStore reads a key-value store; KeyValueEditor writes to one.
//
// Scan lists key metadata (type, length, TTL, never values) via a
// cursor-based scan; there is no offset/skip, unlike Browser, since a
// keyspace has no stable notion of "page 3". With group set, prefix is a
// keyspace group name and matches "prefix:*" plus the bare "prefix" key;
// otherwise it is a literal prefix. Either way it is matched literally,
// never as a glob.
//
// InspectKey reads one key's value one page at a time. cursor is opaque
// ("" starts at the beginning) and comes back "" once nothing is left.
type KeyValueStore interface {
	Scan(ctx context.Context, h Connection, prefix string, group bool, limit int) model.ResultSet
	InspectKey(ctx context.Context, h Connection, key, cursor string, count int) (model.KeyDetail, error)
}

// KeyValueEditor applies one change to one key. Deliberately not Editor
// or DocumentEditor: a key-value write is a type-specific command against
// one key, not a row edit or a whole-document replace. Each edit carries
// what the user last saw, and the implementation refuses (Conflict) a
// change to a key that moved underneath them rather than overwriting it.
type KeyValueEditor interface {
	ApplyKeyEdit(ctx context.Context, h Connection, e model.KeyEdit) (model.KeyEditResult, error)
}
