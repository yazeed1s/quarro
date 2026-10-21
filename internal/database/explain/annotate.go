// Package explain computes exclusive (self) metrics, diagnostic badges, and
// plan-wide stats over an already-parsed query plan tree. It is the shared
// algorithm layer for provider.Explainer, the same role internal/database/
// dialect plays for SQL generation/execution: each engine package parses
// its own EXPLAIN output into a raw model.ExplainPlan (operation names,
// relations, per-node inclusive time/cost/rows exactly as the engine
// reported them), and Annotate does the one, non-duplicated pass of smart
// work over the resulting tree. It has no knowledge of any engine's EXPLAIN
// syntax or output format, only of the normalized model.ExplainNode shape.
package explain

import (
	"fmt"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/model"
)

const (
	estimateGapWarn   = 4.0
	estimateGapSevere = 10.0
	hotspotShare      = 0.25
	filterWasteRatio  = 3.0
)

// Annotate fills in ExclusiveTimeMs/ExclusiveCost, Badges, and Stats on
// plan, returning the same value with those fields populated. Every SQL
// provider's Explain implementation calls this before returning, so the
// frontend never computes a badge or an exclusive metric itself.
func Annotate(plan model.ExplainPlan) model.ExplainPlan {
	if plan.Root == nil {
		return plan
	}
	computeExclusive(plan.Root)
	stats := model.ExplainStats{OperationCounts: map[string]int{}}
	accumulate(plan.Root, &stats)
	plan.Stats = stats
	addBadges(plan.Root, stats)
	return plan
}

// computeExclusive subtracts each node's children's inclusive time/cost
// from its own inclusive value, post-order, so a node's Exclusive* fields
// represent work attributable to that node alone. Postgres/MySQL/MariaDB
// all report Total Cost and Actual Total Time as cumulative over a node's
// whole subtree, so plain subtraction is correct for both. Floating-point
// noise can push a result slightly negative; floored at 0 rather than
// shown as a fabricated negative cost.
func computeExclusive(n *model.ExplainNode) {
	for _, c := range n.Children {
		computeExclusive(c)
	}
	if n.InclusiveTimeMs != nil {
		total := *n.InclusiveTimeMs * float64(max(n.Loops, 1))
		for _, c := range n.Children {
			if c.InclusiveTimeMs != nil {
				total -= *c.InclusiveTimeMs * float64(max(c.Loops, 1))
			}
		}
		if total < 0 {
			total = 0
		}
		n.ExclusiveTimeMs = &total
	}
	if n.EstimatedCost != nil {
		total := *n.EstimatedCost
		for _, c := range n.Children {
			if c.EstimatedCost != nil {
				total -= *c.EstimatedCost
			}
		}
		if total < 0 {
			total = 0
		}
		n.ExclusiveCost = &total
	}
}

// accumulate walks the tree once, building the plan-wide Stats: operation
// counts, distinct relations/indexes touched, node count, and totals taken
// from the root (the root's inclusive value already covers its whole
// subtree, so it doubles as the plan total).
func accumulate(n *model.ExplainNode, stats *model.ExplainStats) {
	stats.NodeCount++
	if n.Operation != "" {
		stats.OperationCounts[n.Operation]++
	}
	if n.Relation != "" {
		addUnique(&stats.RelationsScanned, n.Relation)
	}
	if name, ok := n.Extra["indexName"].(string); ok && name != "" {
		addUnique(&stats.IndexesUsed, name)
	}
	for _, c := range n.Children {
		accumulate(c, stats)
	}
	if n.ParentID == "" {
		if n.EstimatedCost != nil {
			cost := *n.EstimatedCost
			stats.TotalCost = &cost
		}
		if n.InclusiveTimeMs != nil {
			total := *n.InclusiveTimeMs * float64(max(n.Loops, 1))
			stats.TotalTimeMs = &total
		}
	}
}

func addUnique(list *[]string, v string) {
	for _, existing := range *list {
		if existing == v {
			return
		}
	}
	*list = append(*list, v)
}

// addBadges walks the tree once more (after Stats/exclusive metrics are
// known) and attaches diagnostic badges per node. Each check reads only
// the normalized model.ExplainNode fields (plus a handful of Extra keys
// each engine's parser is responsible for normalizing to a common name,
// "sortMethod", "indexName", "heapFetches", "workersPlanned",
// "workersLaunched", "rowsRemovedByFilter", see each provider's own
// Explain implementation). Add a new badge kind only once a real engine's
// output can actually support detecting it; this is a small, fixed list,
// not a subsystem to keep growing speculatively.
func addBadges(root *model.ExplainNode, stats model.ExplainStats) {
	var planTotal float64
	switch {
	case stats.TotalTimeMs != nil:
		planTotal = *stats.TotalTimeMs
	case stats.TotalCost != nil:
		planTotal = *stats.TotalCost
	}

	var visit func(n *model.ExplainNode)
	visit = func(n *model.ExplainNode) {
		n.Badges = badgesFor(n, planTotal)
		for _, c := range n.Children {
			visit(c)
		}
	}
	visit(root)
}

func badgesFor(
	n *model.ExplainNode,
	planTotal float64,
) []model.ExplainBadge {
	var badges []model.ExplainBadge

	var exclusive float64
	haveExclusive := false
	switch {
	case n.ExclusiveTimeMs != nil:
		exclusive, haveExclusive = *n.ExclusiveTimeMs, true
	case n.ExclusiveCost != nil:
		exclusive, haveExclusive = *n.ExclusiveCost, true
	}
	if haveExclusive && planTotal > 0 &&
		exclusive/planTotal >= hotspotShare {
		badges = append(badges, model.ExplainBadge{
			Kind: "hotspot", Severity: "warning",
			Message: fmt.Sprintf(
				"%.0f%% of the plan's total",
				exclusive/planTotal*100,
			),
		})
	}

	if n.EstimatedRows != nil && n.ActualRows != nil &&
		*n.EstimatedRows > 0 &&
		*n.ActualRows > 0 {
		ratio := float64(*n.ActualRows) / float64(*n.EstimatedRows)
		if ratio < 1 {
			ratio = 1 / ratio
		}
		if ratio >= estimateGapWarn {
			sev, label := "warning", "4x"
			if ratio >= estimateGapSevere {
				label = "10x"
			}
			badges = append(badges, model.ExplainBadge{
				Kind: "estimateGap", Severity: sev,
				Message: fmt.Sprintf(
					"Estimated %d rows, actual %d rows (%s+ gap)",
					*n.EstimatedRows,
					*n.ActualRows,
					label,
				),
			})
		}
	}

	if n.IsSeqScan {
		relation := n.Relation
		if relation == "" {
			relation = "relation"
		}
		badges = append(badges, model.ExplainBadge{
			Kind: "seqScan", Severity: "info",
			Message: fmt.Sprintf("Sequential scan on %s", relation),
		})
	}

	if sortMethod, ok := n.Extra["sortMethod"].(string); ok &&
		strings.Contains(strings.ToLower(sortMethod), "external") {
		badges = append(badges, model.ExplainBadge{
			Kind: "diskSpill", Severity: "warning",
			Message: fmt.Sprintf(
				"Sort spilled to disk (%s)",
				sortMethod,
			),
		})
	}

	if fetches, ok := numeric(n.Extra["heapFetches"]); ok &&
		fetches > 0 {
		badges = append(badges, model.ExplainBadge{
			Kind: "heapFetch", Severity: "info",
			Message: fmt.Sprintf(
				"%d heap fetches (index-only scan degraded)",
				int64(fetches),
			),
		})
	}

	planned, havePlanned := numeric(n.Extra["workersPlanned"])
	launched, haveLaunched := numeric(n.Extra["workersLaunched"])
	if havePlanned && haveLaunched && launched < planned {
		badges = append(badges, model.ExplainBadge{
			Kind: "underutilizedWorker", Severity: "info",
			Message: fmt.Sprintf(
				"%d of %d parallel workers launched",
				int64(launched),
				int64(planned),
			),
		})
	}

	if removed, ok := numeric(n.Extra["rowsRemovedByFilter"]); ok &&
		n.ActualRows != nil {
		kept := float64(*n.ActualRows)
		if removed > 0 && removed > kept*filterWasteRatio {
			badges = append(badges, model.ExplainBadge{
				Kind: "filterInefficiency", Severity: "info",
				Message: fmt.Sprintf(
					"Filter removed %d of %d scanned rows",
					int64(removed),
					int64(removed+kept),
				),
			})
		}
	}

	return badges
}

func numeric(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int64:
		return float64(t), true
	case int:
		return float64(t), true
	}
	return 0, false
}
