package explain

import (
	"testing"

	"github.com/yazeed1s/quarro/internal/database/model"
)

func f(v float64) *float64 { return &v }
func i(v int64) *int64     { return &v }

// buildPlan wires up a two-level tree (one root, two children) with
// explicit inclusive time/cost, the shape Annotate is meant to subtract
// over: a Nested Loop over a Seq Scan and an Index Scan.
func buildPlan(analyzed bool) model.ExplainPlan {
	seqScan := &model.ExplainNode{
		ID: "2", ParentID: "1", Operation: "Seq Scan", Relation: "orders",
		Loops: 1, EstimatedRows: i(1000), ActualRows: i(950),
		EstimatedCost: f(
			100,
		), InclusiveTimeMs: f(8.0), IsSeqScan: true,
	}
	indexScan := &model.ExplainNode{
		ID: "3", ParentID: "1", Operation: "Index Scan", Relation: "users",
		Loops: 950, EstimatedRows: i(1), ActualRows: i(1),
		EstimatedCost: f(20), InclusiveTimeMs: f(0.001),
	}
	root := &model.ExplainNode{
		ID: "1", Operation: "Nested Loop", Loops: 1,
		EstimatedRows: i(1000), ActualRows: i(950),
		EstimatedCost: f(140), InclusiveTimeMs: f(10.0),
		Children: []*model.ExplainNode{seqScan, indexScan},
	}
	return model.ExplainPlan{
		Engine:   "postgres",
		Analyzed: analyzed,
		Root:     root,
	}
}

func TestAnnotateNilRoot(t *testing.T) {
	plan := Annotate(model.ExplainPlan{})
	if plan.Stats.OperationCounts != nil {
		t.Fatalf(
			"expected no stats computed for a nil root, got %+v",
			plan.Stats,
		)
	}
}

func TestAnnotateExclusiveMetrics(t *testing.T) {
	plan := Annotate(buildPlan(true))
	root := plan.Root

	// root inclusive 10.0ms, minus children's 8.0 + (0.001*950=0.95) = 8.95 -> exclusive ~1.05
	if root.ExclusiveTimeMs == nil {
		t.Fatal("expected root ExclusiveTimeMs to be set")
	}
	if got, want := *root.ExclusiveTimeMs, 1.05; !within(
		got,
		want,
		0.001,
	) {
		t.Fatalf("root exclusive time = %v, want ~%v", got, want)
	}
	// root exclusive cost: 140 - 100 - 20 = 20
	if got := *root.ExclusiveCost; got != 20 {
		t.Fatalf("root exclusive cost = %v, want 20", got)
	}
	// leaf with no children: exclusive == inclusive
	seqScan := root.Children[0]
	if *seqScan.ExclusiveTimeMs != *seqScan.InclusiveTimeMs {
		t.Fatalf(
			"leaf exclusive time %v should equal inclusive %v",
			*seqScan.ExclusiveTimeMs,
			*seqScan.InclusiveTimeMs,
		)
	}
}

func TestAnnotateExclusiveNeverNegative(t *testing.T) {
	// A child reporting more inclusive time than its parent (floating-point
	// noise from the engine) must floor at 0, not go negative.
	child := &model.ExplainNode{
		ID:              "2",
		ParentID:        "1",
		InclusiveTimeMs: f(5.0),
		Loops:           1,
	}
	root := &model.ExplainNode{
		ID:              "1",
		InclusiveTimeMs: f(3.0),
		Loops:           1,
		Children:        []*model.ExplainNode{child},
	}
	plan := Annotate(model.ExplainPlan{Root: root})
	if *plan.Root.ExclusiveTimeMs != 0 {
		t.Fatalf(
			"exclusive time = %v, want 0 (floored)",
			*plan.Root.ExclusiveTimeMs,
		)
	}
}

func TestAnnotateStats(t *testing.T) {
	plan := Annotate(buildPlan(true))
	stats := plan.Stats
	if stats.NodeCount != 3 {
		t.Fatalf("NodeCount = %d, want 3", stats.NodeCount)
	}
	if stats.OperationCounts["Seq Scan"] != 1 ||
		stats.OperationCounts["Nested Loop"] != 1 {
		t.Fatalf("OperationCounts = %+v", stats.OperationCounts)
	}
	if len(stats.RelationsScanned) != 2 {
		t.Fatalf(
			"RelationsScanned = %v, want 2 distinct relations",
			stats.RelationsScanned,
		)
	}
	if stats.TotalCost == nil || *stats.TotalCost != 140 {
		t.Fatalf("TotalCost = %v, want 140", stats.TotalCost)
	}
	if stats.TotalTimeMs == nil || *stats.TotalTimeMs != 10.0 {
		t.Fatalf("TotalTimeMs = %v, want 10.0", stats.TotalTimeMs)
	}
}

func TestAnnotateHotspotBadge(t *testing.T) {
	plan := Annotate(buildPlan(true))
	seqScan := findNode(plan.Root, "2")
	if !hasBadge(seqScan.Badges, "hotspot") {
		t.Fatalf(
			"expected Seq Scan (8ms of 10ms total) to carry a hotspot badge, got %+v",
			seqScan.Badges,
		)
	}
	indexScan := findNode(plan.Root, "3")
	if hasBadge(indexScan.Badges, "hotspot") {
		t.Fatalf(
			"did not expect Index Scan (~0.001ms) to carry a hotspot badge, got %+v",
			indexScan.Badges,
		)
	}
}

func TestAnnotateSeqScanBadge(t *testing.T) {
	plan := Annotate(buildPlan(true))
	seqScan := findNode(plan.Root, "2")
	if !hasBadge(seqScan.Badges, "seqScan") {
		t.Fatalf(
			"expected a seqScan badge on the Seq Scan node, got %+v",
			seqScan.Badges,
		)
	}
	indexScan := findNode(plan.Root, "3")
	if hasBadge(indexScan.Badges, "seqScan") {
		t.Fatalf(
			"did not expect a seqScan badge on an Index Scan node, got %+v",
			indexScan.Badges,
		)
	}
}

func TestAnnotateEstimateGapBadge(t *testing.T) {
	node := &model.ExplainNode{
		ID:            "1",
		Operation:     "Seq Scan",
		Loops:         1,
		EstimatedRows: i(10),
		ActualRows:    i(500),
		IsSeqScan:     true,
	}
	plan := Annotate(model.ExplainPlan{Root: node})
	if !hasBadge(plan.Root.Badges, "estimateGap") {
		t.Fatalf(
			"expected an estimateGap badge for a 50x estimate/actual mismatch, got %+v",
			plan.Root.Badges,
		)
	}

	close := &model.ExplainNode{
		ID:            "1",
		Loops:         1,
		EstimatedRows: i(100),
		ActualRows:    i(110),
	}
	plan = Annotate(model.ExplainPlan{Root: close})
	if hasBadge(plan.Root.Badges, "estimateGap") {
		t.Fatalf(
			"did not expect an estimateGap badge for a close estimate, got %+v",
			plan.Root.Badges,
		)
	}
}

func TestAnnotateDiskSpillBadge(t *testing.T) {
	node := &model.ExplainNode{
		ID:    "1",
		Loops: 1,
		Extra: map[string]any{"sortMethod": "external merge"},
	}
	plan := Annotate(model.ExplainPlan{Root: node})
	if !hasBadge(plan.Root.Badges, "diskSpill") {
		t.Fatalf(
			"expected a diskSpill badge for an external sort, got %+v",
			plan.Root.Badges,
		)
	}

	inMemory := &model.ExplainNode{
		ID:    "1",
		Loops: 1,
		Extra: map[string]any{"sortMethod": "quicksort"},
	}
	plan = Annotate(model.ExplainPlan{Root: inMemory})
	if hasBadge(plan.Root.Badges, "diskSpill") {
		t.Fatalf(
			"did not expect a diskSpill badge for an in-memory sort, got %+v",
			plan.Root.Badges,
		)
	}
}

func TestAnnotateNoBadgesWhenMetricsAbsent(t *testing.T) {
	// The SQLite shape: no cost, no timing, no actual rows at all.
	node := &model.ExplainNode{
		ID:        "1",
		Operation: "SCAN",
		Relation:  "orders",
		Loops:     1,
		IsSeqScan: true,
	}
	plan := Annotate(model.ExplainPlan{Root: node})
	if len(plan.Root.Badges) != 1 ||
		plan.Root.Badges[0].Kind != "seqScan" {
		t.Fatalf(
			"expected only the seqScan badge with no metrics present, got %+v",
			plan.Root.Badges,
		)
	}
	if plan.Stats.TotalCost != nil || plan.Stats.TotalTimeMs != nil {
		t.Fatalf("expected no fabricated totals, got %+v", plan.Stats)
	}
}

func findNode(n *model.ExplainNode, id string) *model.ExplainNode {
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if found := findNode(c, id); found != nil {
			return found
		}
	}
	return nil
}

func hasBadge(badges []model.ExplainBadge, kind string) bool {
	for _, b := range badges {
		if b.Kind == kind {
			return true
		}
	}
	return false
}

func within(got, want, tolerance float64) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tolerance
}
