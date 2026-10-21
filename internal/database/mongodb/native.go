package mongodb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// nativeQueryReq is the small JSON envelope ExecuteNative's req parameter
// carries — the frontend builds this from a collection selector and the
// pipeline text the user typed, the same way explainReq is built for
// Explain, never something a user hand-types in this envelope's own
// syntax (Pipeline's own contents are exactly what the user wrote,
// unlike the envelope wrapping it).
type nativeQueryReq struct {
	Collection string          `json:"collection"`
	Pipeline   json.RawMessage `json:"pipeline"`
}

// mutatingStages are aggregation stages that write rather than read.
// ExecuteNative is this app's query surface for MongoDB; a pipeline
// disguising a write as a "query" is refused before it runs, the same
// posture read-only-connection enforcement already takes elsewhere in
// this app for a comparable safety affordance.
var mutatingStages = map[string]bool{"$merge": true, "$out": true}

// ExecuteNative runs a free-form aggregation pipeline (the pipeline field
// of req, MongoDB Extended JSON) against one collection (the collection
// field), the user's own native Mongo query — unlike Aggregate, which
// compiles a model.VisualQuery AST rather than taking a pipeline the user
// wrote directly.
func (Provider) ExecuteNative(
	ctx context.Context,
	conn any,
	req string,
	maxRows int,
) model.ResultSet {
	hd := conn.(*handle)
	var parsed nativeQueryReq
	if err := json.Unmarshal([]byte(req), &parsed); err != nil {
		return model.ResultSet{Kind: "error", Error: fmt.Sprintf("invalid query request: %s", err)}
	}
	if parsed.Collection == "" {
		return model.ResultSet{Kind: "error", Error: "a collection is required"}
	}
	var pipeline []bson.D
	if err := bson.UnmarshalExtJSON(parsed.Pipeline, false, &pipeline); err != nil {
		return model.ResultSet{Kind: "error", Error: fmt.Sprintf("invalid pipeline: %s", err)}
	}
	for _, stage := range pipeline {
		for _, e := range stage {
			if mutatingStages[strings.ToLower(e.Key)] {
				return model.ResultSet{Kind: "error", Error: fmt.Sprintf("%s is a write stage; this query surface is read-only", e.Key)}
			}
		}
	}
	if maxRows < 1 || maxRows > 10000 {
		maxRows = 500
	}
	// Fetch one extra document beyond maxRows so truncation can be
	// reported honestly, the same n+1 trick Browse/Aggregate already use.
	pipeline = append(pipeline, bson.D{{Key: "$limit", Value: int64(maxRows) + 1}})

	cur, err := hd.db.Collection(parsed.Collection).Aggregate(ctx, pipeline)
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

	truncated := len(rows) > maxRows
	if truncated {
		rows = rows[:maxRows]
	}
	columns := make([]model.ColumnMeta, len(order))
	for i, name := range order {
		columns[i] = model.ColumnMeta{Name: name, DeclaredType: "mixed"}
	}
	return model.ResultSet{
		Kind:      "rows",
		Columns:   columns,
		Rows:      rows,
		Truncated: truncated,
	}
}
