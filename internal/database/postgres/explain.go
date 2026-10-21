package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/yazeed1s/quarro/internal/database/explain"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// pgPlanDoc is the top-level shape of `EXPLAIN (FORMAT JSON) ...`'s single
// output row: a one-element JSON array of {"Plan": {...}, "Planning Time":
// ..., "Execution Time": ...}. Plan is decoded generically (map[string]any)
// rather than into a fixed struct because Postgres's own plan-node shape
// varies by node type (buffers, workers, sort method, ...) and this parser
// only needs to recognize the keys it actually maps in convertNode below.
type pgPlanDoc struct {
	Plan          map[string]any `json:"Plan"`
	PlanningTime  *float64       `json:"Planning Time"`
	ExecutionTime *float64       `json:"Execution Time"`
}

// pgExtraKeys maps a raw Postgres JSON key to the normalized model.ExplainNode
// Extra key internal/database/explain's badge detectors look for. Any other
// key on a plan node is still carried into Extra, under its own raw Postgres
// name, for the node detail panel; this map exists only for the handful of
// keys Annotate's badges need a stable name for.
var pgExtraKeys = map[string]string{
	"Sort Method":            "sortMethod",
	"Index Name":             "indexName",
	"Heap Fetches":           "heapFetches",
	"Workers Planned":        "workersPlanned",
	"Workers Launched":       "workersLaunched",
	"Rows Removed by Filter": "rowsRemovedByFilter",
}

// pgCoreKeys are consumed directly into named model.ExplainNode fields (or
// are structural, like "Plans") and are never also copied into Extra.
var pgCoreKeys = map[string]bool{
	"Node Type":         true,
	"Relation Name":     true,
	"Alias":             true,
	"Filter":            true,
	"Index Cond":        true,
	"Plan Rows":         true,
	"Actual Rows":       true,
	"Actual Loops":      true,
	"Total Cost":        true,
	"Actual Total Time": true,
	"Plans":             true,
}

// Explain runs EXPLAIN (FORMAT JSON, ...) against sqlText and returns the
// engine-neutral, already-annotated plan (internal/database/explain).
// analyze also requests ANALYZE/BUFFERS/TIMING, which actually executes
// sqlText. Callers (internal/app.QueriesService.Explain) are responsible
// for the same job-registry/cancellation/timeout handling ordinary
// execution gets, since this can run for as long as the query itself does.
func (Provider) Explain(
	ctx context.Context,
	c any,
	sqlText string,
	analyze bool,
	maxRows int,
) (model.ExplainPlan, error) {
	conn := c.(*sql.Conn)
	opts := "FORMAT JSON"
	if analyze {
		opts += ", ANALYZE, BUFFERS, TIMING"
	}
	var raw string
	if e := conn.QueryRowContext(ctx, fmt.Sprintf("EXPLAIN (%s) %s", opts, sqlText)).Scan(&raw); e != nil {
		return model.ExplainPlan{}, e
	}

	var docs []pgPlanDoc
	if e := json.Unmarshal([]byte(raw), &docs); e != nil ||
		len(docs) == 0 ||
		docs[0].Plan == nil {
		return model.ExplainPlan{}, fmt.Errorf(
			"parsing EXPLAIN output: %w",
			e,
		)
	}
	doc := docs[0]

	counter := 0
	nextID := func() string {
		counter++
		return fmt.Sprintf("%d", counter)
	}
	plan := model.ExplainPlan{
		Engine:          "postgres",
		Analyzed:        analyze,
		Root:            convertNode(doc.Plan, nextID),
		RawOutput:       raw,
		RawFormat:       "json",
		PlanningTimeMs:  doc.PlanningTime,
		ExecutionTimeMs: doc.ExecutionTime,
	}
	return explain.Annotate(plan), nil
}

func convertNode(
	raw map[string]any,
	nextID func() string,
) *model.ExplainNode {
	operation := str(raw["Node Type"])
	node := &model.ExplainNode{
		ID:             nextID(),
		Operation:      operation,
		Relation:       str(raw["Relation Name"]),
		Alias:          str(raw["Alias"]),
		Filter:         str(raw["Filter"]),
		IndexCondition: str(raw["Index Cond"]),
		Loops:          1,
		IsSeqScan:      operation == "Seq Scan",
	}
	if v, ok := num(raw["Plan Rows"]); ok {
		n := int64(v)
		node.EstimatedRows = &n
	}
	if v, ok := num(raw["Actual Rows"]); ok {
		n := int64(v)
		node.ActualRows = &n
	}
	if v, ok := num(raw["Actual Loops"]); ok {
		node.Loops = int(v)
	}
	if v, ok := num(raw["Total Cost"]); ok {
		node.EstimatedCost = &v
	}
	if v, ok := num(raw["Actual Total Time"]); ok {
		node.InclusiveTimeMs = &v
	}

	extra := map[string]any{}
	for key, val := range raw {
		if pgCoreKeys[key] {
			continue
		}
		if norm, ok := pgExtraKeys[key]; ok {
			extra[norm] = val
		} else {
			extra[key] = val
		}
	}
	if len(extra) > 0 {
		node.Extra = extra
	}

	if children, ok := raw["Plans"].([]any); ok {
		for _, c := range children {
			if childRaw, ok := c.(map[string]any); ok {
				child := convertNode(childRaw, nextID)
				child.ParentID = node.ID
				node.Children = append(node.Children, child)
			}
		}
	}
	return node
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}
