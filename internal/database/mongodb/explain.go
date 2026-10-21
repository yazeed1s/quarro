package mongodb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yazeed1s/quarro/internal/database/explain"
	"github.com/yazeed1s/quarro/internal/database/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// explainReq is the small JSON envelope Explain's s parameter carries.
// provider.Explainer's shared interface has one opaque string slot; a SQL
// engine's collection-equivalent (its table name) lives inside its own SQL
// text, but Mongo needs a collection name and a filter as two separate
// values, the same two Browse already takes as separate parameters. The
// frontend builds and displays this envelope; it is never something a user
// types by hand the way SQL is.
type explainReq struct {
	Collection string          `json:"collection"`
	Filter     json.RawMessage `json:"filter,omitempty"`
}

// mongoTimeMs reads a numeric BSON value (int32/int64/float64, depending on
// how the driver decoded it) as a float64.
func mongoTimeMs(v any) (float64, bool) {
	switch x := v.(type) {
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// asM reads a nested BSON subdocument as bson.M. The driver decodes a
// document explicitly requested as bson.M that way at the top level, but
// any subdocument nested inside it (an interface{}-typed map value) always
// decodes as bson.D instead, to preserve field order; asM normalizes
// either shape into the same key-lookup-friendly map this file's own
// explain-tree walk uses throughout.
func asM(v any) (bson.M, bool) {
	switch x := v.(type) {
	case bson.M:
		return x, true
	case bson.D:
		m := make(bson.M, len(x))
		for _, e := range x {
			m[e.Key] = e.Value
		}
		return m, true
	}
	return nil, false
}

// Explain runs MongoDB's own `explain` command against a find on one
// collection: verbosity "queryPlanner" for an estimate, "executionStats"
// under az, converting the resulting stage tree (winningPlan, or
// executionStats.executionStages once analyzed) into the shared,
// already-annotated model.ExplainPlan tree.
func (Provider) Explain(
	ctx context.Context,
	c any,
	s string,
	az bool,
	mr int,
) (model.ExplainPlan, error) {
	hd := c.(*handle)
	var req explainReq
	if e := json.Unmarshal([]byte(s), &req); e != nil ||
		req.Collection == "" {
		return model.ExplainPlan{}, fmt.Errorf(
			"invalid explain request: expected a collection name",
		)
	}
	flt := bson.M{}
	if len(req.Filter) > 0 {
		if e := bson.UnmarshalExtJSON(req.Filter, false, &flt); e != nil {
			return model.ExplainPlan{}, fmt.Errorf(
				"invalid filter: %w",
				e,
			)
		}
	}

	verbosity := "queryPlanner"
	if az {
		verbosity = "executionStats"
	}
	cmd := bson.D{
		{Key: "explain", Value: bson.D{
			{Key: "find", Value: req.Collection},
			{Key: "filter", Value: flt},
		}},
		{Key: "verbosity", Value: verbosity},
	}
	var res bson.M
	if e := hd.db.RunCommand(ctx, cmd).Decode(&res); e != nil {
		return model.ExplainPlan{}, e
	}

	raw, e := json.MarshalIndent(sanitize(res), "", "  ")
	if e != nil {
		return model.ExplainPlan{}, e
	}

	var stg bson.M
	if az {
		if es, ok := asM(res["executionStats"]); ok {
			stg, _ = asM(es["executionStages"])
		}
	}
	if stg == nil {
		if qp, ok := asM(res["queryPlanner"]); ok {
			stg, _ = asM(qp["winningPlan"])
		}
	}

	n := 0
	id := func() string { n++; return fmt.Sprintf("%d", n) }
	plan := model.ExplainPlan{
		Engine:    "mongodb",
		Analyzed:  az,
		Root:      convertStage(stg, req.Collection, id),
		RawOutput: string(raw),
		RawFormat: "json",
	}
	if az {
		if es, ok := asM(res["executionStats"]); ok {
			if v, ok := mongoTimeMs(es["executionTimeMillis"]); ok {
				plan.ExecutionTimeMs = &v
			}
		}
	}
	return explain.Annotate(plan), nil
}

// stageExtraKeys carries diagnostic fields straight into a node's Extra bag
// under a stable name, when the driver's decoded value is a plain number
// (skipping structured values like keyPattern/indexBounds, which stay in
// RawOutput for the detail panel to show as-is rather than flattened).
var stageExtraKeys = []string{
	"docsExamined",
	"keysExamined",
	"direction",
	"isMultiKey",
}

// convertStage converts one MongoDB explain stage (a winningPlan node or an
// executionStats.executionStages node; both share the same "stage" tree
// shape, the latter just with actual counters added) into the engine-
// neutral tree. collection is stamped onto whichever stage actually reads
// data (COLLSCAN/IXSCAN and friends), the same way a SQL leaf table scan
// carries its own relation name; Mongo's explain output only states the
// collection once, at the top level, not per stage.
func convertStage(
	stg bson.M,
	collection string,
	id func() string,
) *model.ExplainNode {
	if stg == nil {
		return nil
	}
	op, _ := stg["stage"].(string)
	nd := &model.ExplainNode{
		ID:        id(),
		Operation: op,
		Loops:     1,
		IsSeqScan: op == "COLLSCAN",
	}

	switch op {
	case "COLLSCAN",
		"IXSCAN",
		"DISTINCT_SCAN",
		"COUNT_SCAN",
		"IDHACK":
		nd.Relation = collection
	}
	if name, ok := stg["indexName"].(string); ok && name != "" {
		nd.Extra = map[string]any{"indexName": name}
	}
	if f, ok := asM(stg["filter"]); ok && len(f) > 0 {
		if b, e := json.Marshal(sanitize(f)); e == nil {
			nd.Filter = string(b)
		}
	}
	// A disk-spilling sort has no cost-based equivalent to report, but the
	// fact itself maps directly onto the same "sortMethod contains external"
	// signal internal/database/explain's diskSpill badge already checks for
	// Postgres, so a real Mongo disk spill gets the same badge, not a
	// fictitious one invented for this engine.
	if used, ok := stg["usedDisk"].(bool); ok && used {
		if nd.Extra == nil {
			nd.Extra = map[string]any{}
		}
		nd.Extra["sortMethod"] = "external disk sort"
	}
	for _, key := range stageExtraKeys {
		if v, ok := stg[key]; ok {
			if nd.Extra == nil {
				nd.Extra = map[string]any{}
			}
			nd.Extra[key] = sanitize(v)
		}
	}
	if v, ok := mongoTimeMs(stg["nReturned"]); ok {
		r := int64(v)
		nd.ActualRows = &r
	}
	if v, ok := mongoTimeMs(stg["executionTimeMillisEstimate"]); ok {
		nd.InclusiveTimeMs = &v
	}

	if child, ok := asM(stg["inputStage"]); ok {
		if cn := convertStage(child, collection, id); cn != nil {
			cn.ParentID = nd.ID
			nd.Children = append(nd.Children, cn)
		}
	}
	if children, ok := stg["inputStages"].(bson.A); ok {
		for _, raw := range children {
			if cm, ok := asM(raw); ok {
				if cn := convertStage(cm, collection, id); cn != nil {
					cn.ParentID = nd.ID
					nd.Children = append(nd.Children, cn)
				}
			}
		}
	}
	return nd
}
