package sqlite

import (
	"context"
	"testing"

	"github.com/yazeed1s/quarro/internal/database/model"
)

func TestExplainRejectsAnalyze(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cn, e := db.Conn(ctx)
	if e != nil {
		t.Fatalf("conn: %v", e)
	}
	defer cn.Close()

	if _, e := (Provider{}).Explain(ctx, cn, "SELECT 1", true, 100); e == nil {
		t.Fatal("expected EXPLAIN ANALYZE to be rejected on SQLite")
	}
}

func TestExplainBuildsTreeFromFlatRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, s := range []string{
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, status TEXT, user_id INTEGER)`,
		`CREATE INDEX idx_orders_user ON orders(user_id)`,
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
	} {
		if _, e := db.ExecContext(ctx, s); e != nil {
			t.Fatalf("exec %q: %v", s, e)
		}
	}
	cn, e := db.Conn(ctx)
	if e != nil {
		t.Fatalf("conn: %v", e)
	}
	defer cn.Close()

	q := `SELECT * FROM orders o JOIN users u ON u.id = o.user_id WHERE o.status = 'x'`
	plan, e := (Provider{}).Explain(ctx, cn, q, false, 100)
	if e != nil {
		t.Fatalf("Explain: %v", e)
	}
	if plan.Analyzed {
		t.Fatal("plain EXPLAIN must not report Analyzed=true")
	}
	if plan.Root == nil || plan.Root.Operation != "QUERY PLAN" {
		t.Fatalf(
			"root = %+v, want synthetic QUERY PLAN root",
			plan.Root,
		)
	}
	if len(plan.Root.Children) == 0 {
		t.Fatalf(
			"expected at least one child under the root, got %+v",
			plan.Root,
		)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal(
			"expected a warning that SQLite reports no execution metrics",
		)
	}
	if plan.Stats.TotalCost != nil || plan.Stats.TotalTimeMs != nil {
		t.Fatalf(
			"expected no fabricated totals, got stats=%+v",
			plan.Stats,
		)
	}

	// Walk every node and confirm nothing carries a cost/timing/actual-rows
	// value SQLite's own EXPLAIN QUERY PLAN never reports.
	var walk func(n *model.ExplainNode)
	found := false
	walk = func(n *model.ExplainNode) {
		if n.EstimatedCost != nil || n.ActualRows != nil ||
			n.InclusiveTimeMs != nil {
			t.Fatalf(
				"node %+v should carry no cost/row/time metrics",
				n,
			)
		}
		if n.Operation == "SCAN" {
			found = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(plan.Root)
	if !found {
		t.Fatal("expected a SCAN node scanning orders or users")
	}
}
