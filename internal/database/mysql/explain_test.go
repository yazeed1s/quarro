package mysql

import (
	"testing"
)

// The two samples below are real EXPLAIN ANALYZE output captured from a
// live MySQL 8.4.11 container, not a hand-guessed format.
const analyzeJoin = `-> Nested loop inner join  (cost=0.9 rows=1) (actual time=0.0265..0.0304 rows=1 loops=1)
    -> Filter: ((o.` + "`status`" + ` = 'a') and (o.user_id is not null))  (cost=0.55 rows=1) (actual time=0.0173..0.0209 rows=1 loops=1)
        -> Table scan on o  (cost=0.55 rows=3) (actual time=0.0156..0.0182 rows=3 loops=1)
    -> Single-row index lookup on u using PRIMARY (id=o.user_id)  (cost=0.35 rows=1) (actual time=0.00817..0.00823 rows=1 loops=1)
`

const analyzeSortGroup = `-> Sort: c DESC  (actual time=0.773..0.773 rows=3 loops=1)
    -> Table scan on <temporary>  (actual time=0.754..0.755 rows=3 loops=1)
        -> Aggregate using temporary table  (actual time=0.753..0.753 rows=3 loops=1)
            -> Inner hash join (o.user_id = u.id)  (cost=1 rows=0.667) (actual time=0.685..0.688 rows=3 loops=1)
                -> Filter: (o.` + "`status`" + ` <> 'zz')  (cost=0.225 rows=1) (actual time=0.0104..0.0121 rows=3 loops=1)
                    -> Table scan on o  (cost=0.225 rows=3) (actual time=0.00695..0.00827 rows=3 loops=1)
                -> Hash
                    -> Covering index scan on u using PRIMARY  (cost=0.45 rows=2) (actual time=0.0224..0.024 rows=2 loops=1)
`

func TestParseTextTreeJoin(t *testing.T) {
	n := 0
	rt := parseTextTree(
		analyzeJoin,
		func() string { n++; return string(rune('a' + n)) },
	)
	if rt == nil || rt.Operation != "Nested loop inner join" {
		t.Fatalf("root = %+v", rt)
	}
	if len(rt.Children) != 2 {
		t.Fatalf(
			"root children = %d, want 2: %+v",
			len(rt.Children),
			rt.Children,
		)
	}
	if rt.EstimatedCost == nil || *rt.EstimatedCost != 0.9 {
		t.Fatalf(
			"root EstimatedCost = %v, want 0.9",
			rt.EstimatedCost,
		)
	}
	if rt.ActualRows == nil || *rt.ActualRows != 1 {
		t.Fatalf("root ActualRows = %v, want 1", rt.ActualRows)
	}

	filter := rt.Children[0]
	if filter.Operation != "Filter" || filter.Filter == "" {
		t.Fatalf("filter node = %+v", filter)
	}
	if len(filter.Children) != 1 {
		t.Fatalf("filter children = %d, want 1", len(filter.Children))
	}
	scan := filter.Children[0]
	if scan.Operation != "ALL" || scan.Relation != "o" {
		t.Fatalf("scan node = %+v, want ALL on o", scan)
	}
	if scan.ActualRows == nil || *scan.ActualRows != 3 {
		t.Fatalf("scan ActualRows = %v, want 3", scan.ActualRows)
	}

	lookup := rt.Children[1]
	if lookup.Operation != "eq_ref" || lookup.Relation != "u" {
		t.Fatalf("lookup node = %+v, want eq_ref on u", lookup)
	}
	if lookup.Extra["indexName"] != "PRIMARY" {
		t.Fatalf(
			"lookup index = %v, want PRIMARY",
			lookup.Extra["indexName"],
		)
	}
}

func TestParseTextTreeSortGroupDepth(t *testing.T) {
	n := 0
	rt := parseTextTree(
		analyzeSortGroup,
		func() string { n++; return string(rune('a' + n)) },
	)
	if rt == nil || rt.Operation != "Sort: c DESC" {
		t.Fatalf("root = %+v", rt)
	}
	if rt.EstimatedCost != nil {
		t.Fatalf(
			"Sort line carries no cost in the real sample; got %v",
			rt.EstimatedCost,
		)
	}
	if len(rt.Children) != 1 || rt.Children[0].Operation != "ALL" ||
		rt.Children[0].Relation != "<temporary>" {
		t.Fatalf(
			"expected one ALL scan on <temporary> child (classifyLabel normalizes \"Table scan on X\" the same way MySQL's own JSON access_type does), got %+v",
			rt.Children,
		)
	}
	agg := rt.Children[0].Children[0]
	if agg.Operation != "Aggregate using temporary table" {
		t.Fatalf("agg node = %+v", agg)
	}
	join := agg.Children[0]
	if join.Operation != "Inner hash join (o.user_id = u.id)" {
		t.Fatalf("join node = %+v", join)
	}
	if len(join.Children) != 2 {
		t.Fatalf(
			"join children = %d, want 2 (Filter, Hash): %+v",
			len(join.Children),
			join.Children,
		)
	}
	hashSide := join.Children[1]
	if hashSide.Operation != "Hash" {
		t.Fatalf("second join child = %+v, want Hash", hashSide)
	}
	covScan := hashSide.Children[0]
	if covScan.Operation != "index" || covScan.Relation != "u" ||
		covScan.Extra["indexName"] != "PRIMARY" {
		t.Fatalf("covering index scan = %+v", covScan)
	}
}

func TestClassifyLabelForms(t *testing.T) {
	cases := []struct {
		in           string
		op, rel, idx string
	}{
		{"Filter: (x = 1)", "Filter", "", ""},
		{"Table scan on orders", "ALL", "orders", ""},
		{
			"Covering index scan on u using PRIMARY",
			"index",
			"u",
			"PRIMARY",
		},
		{"Index scan on u using PRIMARY", "index", "u", "PRIMARY"},
		{
			"Single-row index lookup on u using PRIMARY (id=o.user_id)",
			"eq_ref",
			"u",
			"PRIMARY",
		},
		{
			"Index lookup on o using idx_orders_user (user_id=u.id)",
			"ref",
			"o",
			"idx_orders_user",
		},
		{"Nested loop inner join", "Nested loop inner join", "", ""},
	}
	for _, c := range cases {
		op, rel, _, idx := classifyLabel(c.in)
		if op != c.op || rel != c.rel || idx != c.idx {
			t.Errorf(
				"classifyLabel(%q) = (%q,%q,idx=%q), want (%q,%q,idx=%q)",
				c.in,
				op,
				rel,
				idx,
				c.op,
				c.rel,
				c.idx,
			)
		}
	}
}

func TestSupportsAnalyzeVersionGate(t *testing.T) {
	cases := []struct {
		ver string
		ok  bool
	}{
		{"8.0.17", false},
		{"8.0.18", true},
		{"8.0.35", true},
		{"8.4.11", true},
		{"9.0.0", true},
		{"5.7.44", false},
	}
	for _, c := range cases {
		if got := supportsAnalyze(c.ver); got != c.ok {
			t.Errorf(
				"supportsAnalyze(%q) = %v, want %v",
				c.ver,
				got,
				c.ok,
			)
		}
	}
}

func TestAsFloatHandlesBothMySQLAndMariaDBShapes(t *testing.T) {
	if v, ok := asFloat("0.55"); !ok || v != 0.55 {
		t.Fatalf("asFloat(string) = %v,%v", v, ok)
	}
	if v, ok := asFloat(3.0); !ok || v != 3 {
		t.Fatalf("asFloat(float64) = %v,%v", v, ok)
	}
	if _, ok := asFloat(nil); ok {
		t.Fatal("asFloat(nil) should report not-ok")
	}
}
