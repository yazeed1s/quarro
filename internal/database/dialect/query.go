package dialect

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/model"
)

const DefaultResultBytes = 4 << 20

// SelectCompiler is the small dialect-facing part of a SQL provider that can
// turn a Visual Query AST into executable SELECT text.
type SelectCompiler interface {
	CompileSelect(model.VisualQuery) (string, error)
}

// SQLExecutor is the small dialect-facing part of a SQL provider that can run
// compiled SQL text against the provider-owned connection. It intentionally
// mirrors provider.SQLExecutor without importing provider and creating a cycle.
type SQLExecutor interface {
	Execute(
		context.Context,
		any,
		string,
		int,
		int,
		...any,
	) model.ResultSet
}

// AggregateViaSQL implements provider.Aggregator for SQL engines by compiling
// the same Visual Query AST the builder already uses, then executing the
// generated SELECT through the engine's normal bounded execution path.
func AggregateViaSQL(
	ctx context.Context,
	conn any,
	compiler SelectCompiler,
	executor SQLExecutor,
	q model.VisualQuery,
	maxRows, maxBytes int,
) model.ResultSet {
	stmt, err := compiler.CompileSelect(q)
	if err != nil {
		return model.ResultSet{Kind: "error", Error: err.Error()}
	}
	result := executor.Execute(ctx, conn, stmt, maxRows, maxBytes)
	result.Statement = stmt
	return result
}

// QueryDialect is the Visual Query Builder equivalent of EditDialect: the
// SELECT compiler is engine-agnostic and takes its quoting, table-reference,
// and literal-formatting rules as closures so every engine package can share
// one builder.
type QueryDialect struct {
	Quote    func(string) string
	TableSQL func(string) (string, error)
	Literal  func(any) (string, error)
}

var joinKeywords = map[string]string{
	"inner": "JOIN",
	"left":  "LEFT JOIN",
	"right": "RIGHT JOIN",
}

var filterOperators = map[string]bool{
	"eq": true, "ne": true, "lt": true, "lte": true, "gt": true, "gte": true,
	"like": true, "isNull": true, "isNotNull": true, "in": true,
}

var aggregateFuncs = map[string]string{
	"count": "COUNT", "sum": "SUM", "avg": "AVG", "min": "MIN", "max": "MAX",
}

// CompileSelect compiles a Visual Query Builder AST into one SELECT
// statement using d's dialect rules.
func CompileSelect(
	d QueryDialect,
	q model.VisualQuery,
) (string, error) {
	if q.Limit < 0 {
		return "", fmt.Errorf("limit cannot be negative")
	}
	if len(q.Tables) == 0 {
		return "", fmt.Errorf("add at least one table")
	}
	tables := map[string]model.VisualQueryTable{}
	for _, t := range q.Tables {
		if t.Alias == "" {
			return "", fmt.Errorf(
				"table %q is missing an alias",
				t.Object,
			)
		}
		if _, dup := tables[t.Alias]; dup {
			return "", fmt.Errorf("duplicate table alias %q", t.Alias)
		}
		tables[t.Alias] = t
	}
	resolve := func(ref model.VisualQueryRef) error {
		if _, ok := tables[ref.Table]; !ok {
			return fmt.Errorf("unknown table alias %q", ref.Table)
		}
		if ref.Column == "" {
			return fmt.Errorf(
				"column reference on %q is missing a column name",
				ref.Table,
			)
		}
		return nil
	}
	col := func(ref model.VisualQueryRef) string { return d.Quote(ref.Table) + "." + d.Quote(ref.Column) }

	aggregatedCol := func(aggregate string, ref model.VisualQueryRef) (string, error) {
		if aggregate == "" {
			return col(ref), nil
		}
		fn, ok := aggregateFuncs[aggregate]
		if !ok {
			return "", fmt.Errorf(
				"unsupported aggregate %q",
				aggregate,
			)
		}
		return fn + "(" + col(ref) + ")", nil
	}

	condition := func(f model.VisualQueryFilter) (string, error) {
		if err := resolve(f.VisualQueryRef); err != nil {
			return "", err
		}
		if !filterOperators[f.Operator] {
			return "", fmt.Errorf(
				"unsupported filter operator %q",
				f.Operator,
			)
		}
		expr, err := aggregatedCol(f.Aggregate, f.VisualQueryRef)
		if err != nil {
			return "", err
		}
		switch f.Operator {
		case "isNull":
			return expr + " IS NULL", nil
		case "isNotNull":
			return expr + " IS NOT NULL", nil
		case "in":
			if len(f.Values) == 0 {
				return "", fmt.Errorf(
					"%q needs at least one value for IN",
					f.Table+"."+f.Column,
				)
			}
			parts := make([]string, len(f.Values))
			for i, v := range f.Values {
				lit, err := d.Literal(v)
				if err != nil {
					return "", err
				}
				parts[i] = lit
			}
			return expr + " IN (" + strings.Join(
				parts,
				", ",
			) + ")", nil
		default:
			lit, err := d.Literal(f.Value)
			if err != nil {
				return "", err
			}
			ops := map[string]string{
				"eq":   "=",
				"ne":   "<>",
				"lt":   "<",
				"lte":  "<=",
				"gt":   ">",
				"gte":  ">=",
				"like": "LIKE",
			}
			return expr + " " + ops[f.Operator] + " " + lit, nil
		}
	}

	var b strings.Builder
	b.WriteString("SELECT ")
	if len(q.Columns) == 0 {
		return "", fmt.Errorf("select at least one output column")
	}
	selectParts := make([]string, len(q.Columns))
	for i, c := range q.Columns {
		if err := resolve(c.VisualQueryRef); err != nil {
			return "", err
		}
		expr, err := aggregatedCol(c.Aggregate, c.VisualQueryRef)
		if err != nil {
			return "", err
		}
		if c.Alias != "" {
			expr += " AS " + d.Quote(c.Alias)
		}
		selectParts[i] = expr
	}
	b.WriteString(strings.Join(selectParts, ", "))

	first := q.Tables[0]
	fromSQL, err := d.TableSQL(first.Object)
	if err != nil {
		return "", err
	}
	b.WriteString(" FROM " + fromSQL + " AS " + d.Quote(first.Alias))

	joined := map[string]bool{first.Alias: true}
	for _, j := range q.Joins {
		keyword, ok := joinKeywords[j.Type]
		if !ok {
			return "", fmt.Errorf("unsupported join type %q", j.Type)
		}
		if err := resolve(j.Left); err != nil {
			return "", err
		}
		if err := resolve(j.Right); err != nil {
			return "", err
		}
		// Exactly one side of the join must be a table not yet in the FROM
		// clause; that side is the one this JOIN clause introduces.
		leftKnown, rightKnown := joined[j.Left.Table], joined[j.Right.Table]
		var newAlias, newObject string
		switch {
		case leftKnown && !rightKnown:
			newAlias, newObject = j.Right.Table, tables[j.Right.Table].Object
		case rightKnown && !leftKnown:
			if j.Type == "left" {
				keyword = "RIGHT JOIN"
			} else if j.Type == "right" {
				keyword = "LEFT JOIN"
			}
			newAlias, newObject = j.Left.Table, tables[j.Left.Table].Object
		case leftKnown && rightKnown:
			return "", fmt.Errorf(
				"tables %q and %q are already joined",
				j.Left.Table,
				j.Right.Table,
			)
		default:
			return "", fmt.Errorf(
				"join between %q and %q has no table already on the canvas",
				j.Left.Table,
				j.Right.Table,
			)
		}
		joinSQL, err := d.TableSQL(newObject)
		if err != nil {
			return "", err
		}
		b.WriteString(
			" " + keyword + " " + joinSQL + " AS " + d.Quote(
				newAlias,
			),
		)
		b.WriteString(" ON " + col(j.Left) + " = " + col(j.Right))
		joined[newAlias] = true
	}
	for alias := range tables {
		if !joined[alias] {
			return "", fmt.Errorf(
				"table %q has no join connecting it to the query",
				alias,
			)
		}
	}

	if len(q.Filters) > 0 {
		parts := make([]string, len(q.Filters))
		for i, f := range q.Filters {
			if f.Aggregate != "" {
				return "", fmt.Errorf(
					"aggregate conditions belong in Having, not Filters",
				)
			}
			expr, err := condition(f)
			if err != nil {
				return "", err
			}
			parts[i] = expr
		}
		b.WriteString(" WHERE " + strings.Join(parts, " AND "))
	}

	hasAggregate := false
	for _, c := range q.Columns {
		if c.Aggregate != "" {
			hasAggregate = true
		}
	}
	if hasAggregate {
		plain := map[string]bool{}
		for _, c := range q.Columns {
			if c.Aggregate == "" {
				plain[col(c.VisualQueryRef)] = true
			}
		}
		grouped := map[string]bool{}
		for _, g := range q.GroupBy {
			if err := resolve(g); err != nil {
				return "", err
			}
			grouped[col(g)] = true
		}
		for key := range plain {
			if !grouped[key] {
				return "", fmt.Errorf(
					"column %q must be aggregated or added to Group By",
					key,
				)
			}
		}
		if len(q.GroupBy) > 0 {
			parts := make([]string, len(q.GroupBy))
			for i, g := range q.GroupBy {
				parts[i] = col(g)
			}
			b.WriteString(" GROUP BY " + strings.Join(parts, ", "))
		}
	} else if len(q.GroupBy) > 0 {
		return "", fmt.Errorf("group by requires at least one aggregated column")
	}

	validateGrouped := func(ref model.VisualQueryRef, aggregate string) error {
		if aggregate != "" && !hasAggregate {
			return fmt.Errorf(
				"aggregate ordering requires an aggregate query",
			)
		}
		if hasAggregate && aggregate == "" {
			for _, g := range q.GroupBy {
				if g == ref {
					return nil
				}
			}
			return fmt.Errorf(
				"column %s must be grouped or aggregated",
				col(ref),
			)
		}
		return nil
	}

	if len(q.Having) > 0 {
		if !hasAggregate {
			return "", fmt.Errorf(
				"having requires at least one aggregated column",
			)
		}
		parts := make([]string, len(q.Having))
		for i, f := range q.Having {
			if err := validateGrouped(f.VisualQueryRef, f.Aggregate); err != nil {
				return "", err
			}
			expr, err := condition(f)
			if err != nil {
				return "", err
			}
			parts[i] = expr
		}
		b.WriteString(" HAVING " + strings.Join(parts, " AND "))
	}

	if len(q.OrderBy) > 0 {
		parts := make([]string, len(q.OrderBy))
		for i, o := range q.OrderBy {
			if err := validateGrouped(o.VisualQueryRef, o.Aggregate); err != nil {
				return "", err
			}
			if err := resolve(o.VisualQueryRef); err != nil {
				return "", err
			}
			expr, err := aggregatedCol(o.Aggregate, o.VisualQueryRef)
			if err != nil {
				return "", err
			}
			if o.Descending {
				expr += " DESC"
			} else {
				expr += " ASC"
			}
			parts[i] = expr
		}
		b.WriteString(" ORDER BY " + strings.Join(parts, ", "))
	}

	if q.Limit > 0 {
		b.WriteString(" LIMIT " + strconv.Itoa(q.Limit))
	}

	return b.String(), nil
}

// Literal formats a Go value (already validated to one of the types below)
// as a SQL literal. pgBool selects PostgreSQL's TRUE/FALSE spelling over the
// 1/0 every other engine here uses.
func Literal(v any, pgBool bool) (string, error) {
	switch x := v.(type) {
	case nil:
		return "NULL", nil
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'", nil
	case bool:
		if pgBool {
			if x {
				return "TRUE", nil
			}
			return "FALSE", nil
		}
		if x {
			return "1", nil
		}
		return "0", nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return "", fmt.Errorf("filter number must be finite")
		}
		return strconv.FormatFloat(x, 'g', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32), nil
	case int:
		return strconv.Itoa(x), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case json.Number:
		if _, err := x.Float64(); err != nil {
			return "", fmt.Errorf("invalid filter number")
		}
		return x.String(), nil
	default:
		return "", fmt.Errorf("unsupported filter value type %T", v)
	}
}
