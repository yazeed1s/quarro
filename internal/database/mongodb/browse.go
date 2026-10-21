package mongodb

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/yazeed1s/quarro/internal/database/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Browse finds documents in a collection with a real Mongo filter (id is a
// bare collection name, since Mongo collections aren't schema-qualified the way
// PostgreSQL/MySQL tables are), sorted and paged with skip/limit. filter, if
// non-empty, is MongoDB Extended JSON for a query document (e.g.
// {"status":"active"}); an empty filter matches every document. Documents
// are presented as rows in the same shape a SQL table's rows take:
// map[string]any per document, with the column list built from every key
// seen on the page, since Mongo documents in one collection can have
// different shapes. filterColumn is accepted only to satisfy the shared
// Browser interface; Mongo has no SQL-shaped foreign key concept for a
// single-column equality shortcut to apply to, so it is ignored here.
func (Provider) Browse(
	ctx context.Context,
	conn any,
	id, filter, filterColumn, sort string,
	desc bool,
	offset, n int,
) model.ResultSet {
	_ = filterColumn
	hd := conn.(*handle)
	if offset < 0 || offset > 1000000 {
		return model.ResultSet{
			Kind:  "error",
			Error: "page offset out of range",
		}
	}
	if n < 1 || n > 10000 {
		n = 500
	}
	query := bson.D{}
	if filter != "" {
		if err := bson.UnmarshalExtJSON([]byte(filter), false, &query); err != nil {
			return model.ResultSet{
				Kind:  "error",
				Error: fmt.Sprintf("invalid filter: %s", err),
			}
		}
	}
	// _id is always a unique index, so appending it as a tie-breaker makes
	// offset pagination stable the same way a SQL primary-key tie-breaker
	// does, see internal/database/postgres/schema.go's Browse for the
	// equivalent.
	findSort := bson.D{}
	if sort != "" {
		dir := 1
		if desc {
			dir = -1
		}
		findSort = append(findSort, bson.E{Key: sort, Value: dir})
	}
	findSort = append(findSort, bson.E{Key: "_id", Value: 1})
	opts := options.Find().
		SetSkip(int64(offset)).
		SetLimit(int64(n + 1)).
		SetSort(findSort)
	coll := hd.db.Collection(id)
	cur, err := coll.Find(ctx, query, opts)
	if err != nil {
		return model.ResultSet{Kind: "error", Error: err.Error()}
	}
	defer cur.Close(ctx)
	rows := []map[string]any{}
	seen := map[string]bool{}
	order := []string{}
	for cur.Next(ctx) {
		var raw bson.M
		if err := cur.Decode(&raw); err != nil {
			return model.ResultSet{Kind: "error", Error: err.Error()}
		}
		row := map[string]any{}
		for k, v := range raw {
			if !seen[k] {
				seen[k] = true
				order = append(order, k)
			}
			row[k] = sanitize(v)
		}
		rows = append(rows, row)
	}
	if err := cur.Err(); err != nil {
		return model.ResultSet{Kind: "error", Error: err.Error()}
	}
	truncated := len(rows) > n
	if truncated {
		rows = rows[:n]
	}
	columns := make([]model.ColumnMeta, len(order))
	for i, name := range order {
		columns[i] = model.ColumnMeta{
			Name:         name,
			DeclaredType: "mixed",
		}
	}
	return model.ResultSet{
		Kind:        "rows",
		Columns:     columns,
		Rows:        rows,
		Truncated:   truncated,
		StableOrder: true,
	}
}

// sanitize converts a decoded BSON value into the same JSON-safe shape
// dialect.Safe produces for SQL driver values: an ObjectID becomes its hex
// string, a datetime becomes RFC3339Nano text, binary data becomes the same
// {blob,size,base64} placeholder BLOB cells use, and nested documents/arrays
// are sanitized recursively.
func sanitize(v any) any {
	switch x := v.(type) {
	case bson.ObjectID:
		return x.Hex()
	case bson.DateTime:
		return x.Time().UTC().Format(time.RFC3339Nano)
	case bson.Binary:
		return model.BlobValue{Blob: true, Size: len(x.Data), Base64: base64.StdEncoding.EncodeToString(x.Data)}
	case bson.M:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = sanitize(e)
		}
		return out
	case bson.D:
		out := make(map[string]any, len(x))
		for _, e := range x {
			out[e.Key] = sanitize(e.Value)
		}
		return out
	case bson.A:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = sanitize(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = sanitize(e)
		}
		return out
	default:
		return v
	}
}
