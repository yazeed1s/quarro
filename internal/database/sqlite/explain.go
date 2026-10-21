package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/explain"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// row is one line of `EXPLAIN QUERY PLAN`'s own flat, four-column output:
// id, parent, notused (a reserved column SQLite always sets to 0), det
// (the human-readable step, e.g. "SCAN orders").
type row struct {
	id, pid int
	det     string
}

// Explain runs `EXPLAIN QUERY PLAN` against s and returns the plan as a
// tree, wrapped under a synthetic root node ("QUERY PLAN", matching the
// sqlite3 CLI's own presentation) since the pragma can report more than
// one top-level row for a single query (a compound SELECT, a co-routine).
// az (analyze) is rejected: SQLite has no EXPLAIN ANALYZE equivalent, so
// silently running the estimate-only plan under that label would be
// misleading rather than actually reporting real execution numbers.
func (Provider) Explain(
	ctx context.Context,
	c any,
	s string,
	az bool,
	mr int,
) (model.ExplainPlan, error) {
	if az {
		return model.ExplainPlan{}, fmt.Errorf(
			"SQLite has no EXPLAIN ANALYZE equivalent",
		)
	}
	cn := c.(*sql.Conn)
	rs, e := cn.QueryContext(ctx, "EXPLAIN QUERY PLAN "+s)
	if e != nil {
		return model.ExplainPlan{}, e
	}
	defer rs.Close()

	var rows []row
	for rs.Next() {
		var r row
		var nu int
		if e := rs.Scan(&r.id, &r.pid, &nu, &r.det); e != nil {
			return model.ExplainPlan{}, e
		}
		rows = append(rows, r)
	}
	if e := rs.Err(); e != nil {
		return model.ExplainPlan{}, e
	}

	rt := &model.ExplainNode{
		ID:        "0",
		Operation: "QUERY PLAN",
		Loops:     1,
	}
	byID := map[int]*model.ExplainNode{0: rt}
	for _, r := range rows {
		n := &model.ExplainNode{
			ID:     strconv.Itoa(r.id),
			Detail: r.det,
			Loops:  1,
		}
		n.Operation, n.Relation = parseDetail(r.det)
		n.IsSeqScan = n.Operation == "SCAN"
		if ix := indexName(r.det); ix != "" {
			n.Extra = map[string]any{"indexName": ix}
		}
		byID[r.id] = n
	}
	for _, r := range rows {
		n := byID[r.id]
		p, ok := byID[r.pid]
		if !ok {
			p = rt
		}
		n.ParentID = p.ID
		p.Children = append(p.Children, n)
	}

	plan := model.ExplainPlan{
		Engine:    "sqlite",
		Analyzed:  false,
		Root:      rt,
		RawOutput: rawText(rows),
		RawFormat: "text",
		Warnings: []string{
			"SQLite reports no cost or execution metrics for a query plan",
		},
	}
	return explain.Annotate(plan), nil
}

// parseDetail splits an EXPLAIN QUERY PLAN detail line into an operation
// and, for the two forms internal/database/explain's badges care about
// (SCAN, SEARCH), the scanned relation. Every other form (USE TEMP B-TREE,
// CO-ROUTINE, COMPOUND QUERY, ...) keeps the full line as its own operation
// label rather than guessing at a relation that may not be there.
func parseDetail(det string) (string, string) {
	f := strings.Fields(det)
	if len(f) == 0 {
		return "", ""
	}
	if (f[0] == "SCAN" || f[0] == "SEARCH") && len(f) > 1 {
		return f[0], f[1]
	}
	return det, ""
}

// indexName pulls the index name out of a "... USING INDEX <name> (...)"
// detail line, for internal/database/explain's IndexesUsed stat. Absent on
// a plain SCAN or a covering rowid lookup, which is a normal, not a missing,
// result.
func indexName(det string) string {
	const m = "USING INDEX "
	i := strings.Index(det, m)
	if i < 0 {
		return ""
	}
	f := strings.Fields(det[i+len(m):])
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func rawText(rows []row) string {
	var b strings.Builder
	b.WriteString("id\tparent\tdetail\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%d\t%d\t%s\n", r.id, r.pid, r.det)
	}
	return b.String()
}
