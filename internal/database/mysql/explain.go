package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/explain"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// Explain runs EXPLAIN FORMAT=JSON (estimates, every supported version) or,
// with az true, an actual-execution EXPLAIN against s: MariaDB gets
// ANALYZE FORMAT=JSON (a real JSON tree with r_* fields), MySQL gets
// EXPLAIN ANALYZE (a text tree, MySQL 8.0.18+ only; MySQL has no
// FORMAT=JSON option for its own ANALYZE). Which server this connection
// actually is (MySQL vs MariaDB, and its version) is detected here rather
// than assumed, since one connections.EngineMySQL profile can point at
// either.
func (Provider) Explain(
	ctx context.Context,
	c any,
	s string,
	az bool,
	mr int,
) (model.ExplainPlan, error) {
	cn := c.(*sql.Conn)
	var ver string
	if e := cn.QueryRowContext(ctx, "SELECT VERSION()").Scan(&ver); e != nil {
		return model.ExplainPlan{}, e
	}
	md := strings.Contains(strings.ToUpper(ver), "MARIADB")

	if !az {
		return runJSON(
			ctx,
			cn,
			"EXPLAIN FORMAT=JSON "+s,
			md,
			ver,
			false,
		)
	}
	if md {
		return runJSON(
			ctx,
			cn,
			"ANALYZE FORMAT=JSON "+s,
			md,
			ver,
			true,
		)
	}
	if !supportsAnalyze(ver) {
		return model.ExplainPlan{}, fmt.Errorf(
			"EXPLAIN ANALYZE requires MySQL 8.0.18 or newer (connected server: %s)",
			ver,
		)
	}
	return runTextAnalyze(ctx, cn, "EXPLAIN ANALYZE "+s, ver)
}

// supportsAnalyze reports whether ver (a MySQL "SELECT VERSION()" string,
// never a MariaDB one) is 8.0.18 or newer, the version EXPLAIN ANALYZE was
// introduced in.
func supportsAnalyze(ver string) bool {
	maj, min, pat := verParts(ver)
	if maj != 8 {
		return maj > 8
	}
	if min != 0 {
		return true
	}
	return pat >= 18
}

func verParts(v string) (int, int, int) {
	core := strings.SplitN(v, "-", 2)[0]
	ps := strings.Split(core, ".")
	get := func(i int) int {
		if i >= len(ps) {
			return 0
		}
		n, _ := strconv.Atoi(ps[i])
		return n
	}
	return get(0), get(1), get(2)
}

// runJSON runs st (an already-built EXPLAIN/ANALYZE FORMAT=JSON statement)
// and converts its single JSON-text result column into the engine-neutral
// tree. az marks whether this was an ANALYZE run (MariaDB only; MySQL's
// FORMAT=JSON path is always estimate-only, see Explain above).
func runJSON(
	ctx context.Context,
	cn *sql.Conn,
	st string,
	md bool,
	ver string,
	az bool,
) (model.ExplainPlan, error) {
	var raw string
	if e := cn.QueryRowContext(ctx, st).Scan(&raw); e != nil {
		return model.ExplainPlan{}, e
	}
	var doc map[string]any
	if e := json.Unmarshal([]byte(raw), &doc); e != nil {
		return model.ExplainPlan{}, fmt.Errorf(
			"parsing EXPLAIN output: %w",
			e,
		)
	}
	qb, _ := doc["query_block"].(map[string]any)
	if qb == nil {
		return model.ExplainPlan{}, fmt.Errorf(
			"EXPLAIN output has no query_block",
		)
	}
	n := 0
	id := func() string { n++; return strconv.Itoa(n) }
	eng := "mysql"
	if md {
		eng = "mariadb"
	}
	plan := model.ExplainPlan{
		Engine: eng, EngineVersion: ver, Analyzed: az,
		Root: convertBlock(
			qb,
			md,
			id,
		), RawOutput: raw, RawFormat: "json",
	}
	if md && az {
		if qo, ok := doc["query_optimization"].(map[string]any); ok {
			if v, ok := asFloat(qo["r_total_time_ms"]); ok {
				plan.ExecutionTimeMs = &v
			}
		}
	}
	return explain.Annotate(plan), nil
}

// convertBlock converts one query_block-shaped map (or the map one of its
// wrapper keys nests) into a node. A table leaf and a joined nested_loop
// are structurally identical between MySQL and MariaDB; the surrounding
// operation wrappers (sort, grouping, dedup) use different key names
// between the two (md picks which set), so those are matched separately.
// An unrecognized shape never silently drops its subtree: it becomes an
// "Unknown Operation" node carrying that block's own raw JSON as Detail,
// so a plan this parser does not fully understand is still complete, not
// missing pieces.
func convertBlock(
	blk map[string]any,
	md bool,
	id func() string,
) *model.ExplainNode {
	if t, ok := blk["table"].(map[string]any); ok {
		return convertTable(t, md, id)
	}
	if nl, ok := blk["nested_loop"].([]any); ok {
		var kids []*model.ExplainNode
		for _, it := range nl {
			if m, ok := it.(map[string]any); ok {
				if ch := convertBlock(m, md, id); ch != nil {
					kids = append(kids, ch)
				}
			}
		}
		if len(kids) == 1 {
			// MariaDB wraps every plain table access in a one-element
			// nested_loop, not only real joins (confirmed against a real
			// MariaDB 11 server; MySQL only uses nested_loop for an actual
			// multi-table join). A one-element nested_loop is not a join,
			// so showing a synthetic "Nested Loop" node here would
			// fabricate an operation that never happened; collapse into
			// the single child instead. stampCost still runs on it, since
			// this block's own cost/time (when present) is this table's
			// true total, not a duplicate of what convertTable already set.
			return stampCost(kids[0], blk, md)
		}
		n := &model.ExplainNode{
			ID:        id(),
			Operation: "Nested Loop",
			Loops:     1,
		}
		for _, ch := range kids {
			ch.ParentID = n.ID
		}
		n.Children = kids
		return stampCost(n, blk, md)
	}

	var key, lbl string
	var extra map[string]any
	if md {
		switch {
		case blk["filesort"] != nil:
			key, lbl, extra = "filesort", "Sort", map[string]any{
				"sortMethod": "filesort",
			}
		case blk["temporary_table"] != nil:
			key, lbl = "temporary_table", "Materialize"
		case blk["duplicates_removal"] != nil:
			key, lbl = "duplicates_removal", "Duplicates Removal"
		}
	} else {
		switch {
		case blk["ordering_operation"] != nil:
			key, lbl = "ordering_operation", "Sort"
		case blk["grouping_operation"] != nil:
			key, lbl = "grouping_operation", "Group By"
		case blk["duplicates_removal"] != nil:
			key, lbl = "duplicates_removal", "Duplicates Removal"
		case blk["windowing"] != nil:
			key, lbl = "windowing", "Window"
		}
	}
	if key == "" {
		raw, _ := json.Marshal(blk)
		return &model.ExplainNode{
			ID:        id(),
			Operation: "Unknown Operation",
			Detail:    string(raw),
			Loops:     1,
		}
	}

	inner, _ := blk[key].(map[string]any)
	n := &model.ExplainNode{
		ID:        id(),
		Operation: lbl,
		Loops:     1,
		Extra:     extra,
	}
	if !md {
		if fs, ok := inner["using_filesort"].(bool); ok && fs {
			if n.Extra == nil {
				n.Extra = map[string]any{}
			}
			n.Extra["sortMethod"] = "filesort"
		}
	}
	if ch := convertBlock(inner, md, id); ch != nil {
		ch.ParentID = n.ID
		n.Children = append(n.Children, ch)
	}
	return stampCost(n, blk, md)
}

// stampCost attaches blk's own overall cost/time (MariaDB's bare "cost"/
// "r_total_time_ms", or MySQL's nested cost_info.query_cost) to n. This
// lives beside whichever key (table/nested_loop/wrapper) blk actually has,
// not nested under it, so every caller of convertBlock funnels through
// here rather than repeating the lookup.
func stampCost(
	n *model.ExplainNode,
	blk map[string]any,
	md bool,
) *model.ExplainNode {
	if md {
		if v, ok := asFloat(blk["cost"]); ok {
			n.EstimatedCost = &v
		}
		if v, ok := asFloat(blk["r_total_time_ms"]); ok {
			n.InclusiveTimeMs = &v
		}
		return n
	}
	if ci, ok := blk["cost_info"].(map[string]any); ok {
		if v, ok := asFloat(ci["query_cost"]); ok {
			n.EstimatedCost = &v
		}
	}
	return n
}

// convertTable converts one "table" leaf. MariaDB's fields are bare
// numbers ("cost", "rows", and, under ANALYZE, "r_rows"/"r_loops"/
// "r_table_time_ms"/"r_other_time_ms"); MySQL's are a mix of numbers
// ("rows_examined_per_scan") and quoted-string numbers inside its own
// nested "cost_info" object ("prefix_cost"). MySQL's own EXPLAIN FORMAT=JSON
// never carries actual-execution fields (see Explain's own doc comment for
// why ANALYZE takes MySQL through the text-tree path instead), so md is
// what selects which field set to read, not az.
func convertTable(
	t map[string]any,
	md bool,
	id func() string,
) *model.ExplainNode {
	access := str(t["access_type"])
	n := &model.ExplainNode{
		ID:        id(),
		Operation: access,
		Relation:  str(t["table_name"]),
		Filter:    str(t["attached_condition"]),
		Loops:     1,
		IsSeqScan: access == "ALL",
	}
	if k, ok := t["key"].(string); ok && k != "" {
		n.Extra = map[string]any{"indexName": k}
	}
	if md {
		if v, ok := asFloat(t["rows"]); ok {
			r := int64(v)
			n.EstimatedRows = &r
		}
		if v, ok := asFloat(t["cost"]); ok {
			n.EstimatedCost = &v
		}
		if v, ok := asFloat(t["r_rows"]); ok {
			r := int64(v)
			n.ActualRows = &r
		}
		if v, ok := asFloat(t["r_loops"]); ok && v > 0 {
			n.Loops = int(v)
		}
		tt, ok1 := asFloat(t["r_table_time_ms"])
		ot, ok2 := asFloat(t["r_other_time_ms"])
		if ok1 || ok2 {
			// r_table_time_ms/r_other_time_ms are already totals across
			// every r_loops iteration, confirmed against a real MariaDB
			// server (a 3-loop inner join table's own reported time does
			// not scale linearly if treated as a per-loop average).
			// explain.Annotate's exclusive-time math is written once,
			// uniformly, against the per-loop-average convention every
			// other source (Postgres, MySQL's own ANALYZE text tree) uses
			// (multiply by Loops for the total); normalize MariaDB's
			// already-total value down to a per-loop average here, at the
			// parser boundary, instead of teaching the shared package a
			// second per-engine time convention for one field.
			tot := (tt + ot) / float64(max(n.Loops, 1))
			n.InclusiveTimeMs = &tot
		}
		return n
	}
	if v, ok := asFloat(t["rows_examined_per_scan"]); ok {
		r := int64(v)
		n.EstimatedRows = &r
	}
	if ci, ok := t["cost_info"].(map[string]any); ok {
		if v, ok := asFloat(ci["prefix_cost"]); ok {
			n.EstimatedCost = &v
		}
	}
	return n
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// asFloat reads a JSON number, or a JSON string holding one (MySQL quotes
// its own cost_info fields; MariaDB does not), as a float64.
func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		f, e := strconv.ParseFloat(x, 64)
		return f, e == nil
	}
	return 0, false
}

var (
	costRe   = regexp.MustCompile(`\(cost=([0-9.]+) rows=([0-9.]+)\)`)
	actualRe = regexp.MustCompile(
		`\(actual time=([0-9.]+)\.\.([0-9.]+) rows=(\d+) loops=(\d+)\)`,
	)
)

// runTextAnalyze parses MySQL's EXPLAIN ANALYZE text tree: one "-> <label>
// (cost=.. rows=..) (actual time=A..B rows=C loops=D)" line per node,
// nested by a fixed 4 spaces of indentation per level (confirmed against a
// real MySQL 8.4 instance; both the cost and actual-time parentheticals are
// independently optional, since an intermediate materialization step, e.g.
// "Table scan on <temporary>", carries actual time but no cost).
func runTextAnalyze(
	ctx context.Context,
	cn *sql.Conn,
	st, ver string,
) (model.ExplainPlan, error) {
	var raw string
	if e := cn.QueryRowContext(ctx, st).Scan(&raw); e != nil {
		return model.ExplainPlan{}, e
	}
	n := 0
	id := func() string { n++; return strconv.Itoa(n) }
	rt := parseTextTree(raw, id)
	plan := model.ExplainPlan{
		Engine: "mysql", EngineVersion: ver, Analyzed: true,
		Root: rt, RawOutput: raw, RawFormat: "text",
	}
	if rt != nil && rt.InclusiveTimeMs != nil {
		v := *rt.InclusiveTimeMs * float64(max(rt.Loops, 1))
		plan.ExecutionTimeMs = &v
	}
	return explain.Annotate(plan), nil
}

// parseTextTree builds a tree from raw's "-> "-prefixed, indentation-nested
// lines using a depth stack: each line's depth pops the stack down to its
// parent, then pushes itself as that parent's next child.
func parseTextTree(raw string, id func() string) *model.ExplainNode {
	type frame struct {
		dep int
		n   *model.ExplainNode
	}
	var rt *model.ExplainNode
	var stk []frame
	for _, l := range strings.Split(raw, "\n") {
		dep, n := parseLine(l, id)
		if n == nil {
			continue
		}
		for len(stk) > 0 && stk[len(stk)-1].dep >= dep {
			stk = stk[:len(stk)-1]
		}
		if len(stk) == 0 {
			rt = n
		} else {
			p := stk[len(stk)-1].n
			n.ParentID = p.ID
			p.Children = append(p.Children, n)
		}
		stk = append(stk, frame{dep, n})
	}
	return rt
}

func parseLine(l string, id func() string) (int, *model.ExplainNode) {
	tr := strings.TrimLeft(l, " ")
	lead := len(l) - len(tr)
	dep := lead / 4
	if !strings.HasPrefix(tr, "-> ") {
		return dep, nil
	}
	body := strings.TrimPrefix(tr, "-> ")
	n := &model.ExplainNode{ID: id(), Loops: 1}
	if m := actualRe.FindStringSubmatch(body); m != nil {
		body = strings.Replace(body, m[0], "", 1)
		if v, e := strconv.ParseFloat(m[2], 64); e == nil {
			n.InclusiveTimeMs = &v
		}
		if v, e := strconv.Atoi(m[3]); e == nil {
			r := int64(v)
			n.ActualRows = &r
		}
		if v, e := strconv.Atoi(m[4]); e == nil {
			n.Loops = v
		}
	}
	if m := costRe.FindStringSubmatch(body); m != nil {
		body = strings.Replace(body, m[0], "", 1)
		if v, e := strconv.ParseFloat(m[1], 64); e == nil {
			n.EstimatedCost = &v
		}
		if v, e := strconv.ParseFloat(m[2], 64); e == nil {
			r := int64(v)
			n.EstimatedRows = &r
		}
	}
	lbl := strings.TrimSpace(body)
	n.Detail = lbl
	var ix string
	n.Operation, n.Relation, n.Filter, ix = classifyLabel(lbl)
	n.IsSeqScan = n.Operation == "ALL"
	setIndex(n, ix)
	return dep, n
}

// classifyLabel recognizes the common MySQL EXPLAIN ANALYZE step phrasings
// (table/index scans, index lookups, a filter) and pulls a relation, index
// name, and filter condition out of them, so those nodes line up with the
// same normalized shape MySQL's own EXPLAIN FORMAT=JSON produces (e.g.
// "ALL" for a full scan, which parseLine also stamps IsSeqScan from).
// Anything else
// (a join algorithm, a sort/aggregate step) keeps its full line as the
// operation label rather than guessing at a relation that is not there.
func classifyLabel(s string) (op, rel, filt, idx string) {
	if strings.HasPrefix(s, "Filter: ") {
		return "Filter", "", strings.TrimPrefix(s, "Filter: "), ""
	}
	if strings.HasPrefix(s, "Table scan on ") {
		return "ALL", strings.TrimPrefix(s, "Table scan on "), "", ""
	}
	if af, ok := trimAny(s, "Covering index scan on ", "Index scan on "); ok {
		tb, ix := splitUsing(af)
		return "index", tb, "", ix
	}
	if af, ok := trimAny(s, "Single-row index lookup on ", "Index lookup on "); ok {
		op := "ref"
		if strings.HasPrefix(s, "Single-row") {
			op = "eq_ref"
		}
		tb, ix := splitUsing(af)
		return op, tb, "", ix
	}
	return s, "", "", ""
}

func trimAny(s string, prefixes ...string) (string, bool) {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return strings.TrimPrefix(s, p), true
		}
	}
	return "", false
}

// splitUsing splits "<table> using <index> (<condition>)" (the condition
// and even the index name are optional) into the table and index name.
func splitUsing(s string) (tb, ix string) {
	i := strings.Index(s, " using ")
	if i < 0 {
		return s, ""
	}
	tb = s[:i]
	af := s[i+len(" using "):]
	if j := strings.IndexAny(af, " ("); j >= 0 {
		return tb, af[:j]
	}
	return tb, af
}

func setIndex(n *model.ExplainNode, ix string) {
	if ix == "" {
		return
	}
	if n.Extra == nil {
		n.Extra = map[string]any{}
	}
	n.Extra["indexName"] = ix
}
