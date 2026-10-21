package mongodb

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var aggregateOps = map[string]string{
	"count": "$sum",
	"sum":   "$sum",
	"avg":   "$avg",
	"min":   "$min",
	"max":   "$max",
}

var filterOps = map[string]string{
	"ne":  "$ne",
	"lt":  "$lt",
	"lte": "$lte",
	"gt":  "$gt",
	"gte": "$gte",
}

// Aggregate implements provider.Aggregator by translating a single-collection
// model.VisualQuery into a $match/$group/$project/$sort/$limit pipeline,
// the same AST the Visual Query Builder and the SQL engines' Aggregate
// already consume, so charting's "on the fly" entry point works the same
// way for MongoDB as it does for SQL, without pretending Mongo has a SQL
// compiler. A multi-table/joined query is rejected with a clear error:
// $lookup is a different feature this method doesn't reach for.
func (Provider) Aggregate(
	ctx context.Context,
	conn any,
	q model.VisualQuery,
	maxRows int,
) model.ResultSet {
	hd := conn.(*handle)

	if len(q.Tables) != 1 {
		return model.ResultSet{
			Kind:  "error",
			Error: "MongoDB aggregation supports exactly one collection, not a multi-table query",
		}
	}
	if len(q.Joins) > 0 {
		return model.ResultSet{
			Kind:  "error",
			Error: "MongoDB aggregation does not support joins",
		}
	}
	if len(q.Having) > 0 {
		return model.ResultSet{
			Kind:  "error",
			Error: "MongoDB aggregation does not support HAVING",
		}
	}
	if q.Limit < 0 {
		return model.ResultSet{
			Kind:  "error",
			Error: "limit cannot be negative",
		}
	}
	if len(q.Columns) == 0 {
		return model.ResultSet{
			Kind:  "error",
			Error: "select at least one output column",
		}
	}
	if maxRows < 1 || maxRows > 100000 {
		maxRows = 1000
	}

	table := q.Tables[0]
	resolve := func(ref model.VisualQueryRef) error {
		if ref.Table != table.Alias {
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

	hasAggregate := false
	for _, c := range q.Columns {
		if err := resolve(c.VisualQueryRef); err != nil {
			return model.ResultSet{Kind: "error", Error: err.Error()}
		}
		if c.Aggregate != "" {
			hasAggregate = true
		}
	}

	if hasAggregate {
		grouped := map[string]bool{}
		for _, g := range q.GroupBy {
			if err := resolve(g); err != nil {
				return model.ResultSet{
					Kind:  "error",
					Error: err.Error(),
				}
			}
			grouped[g.Column] = true
		}
		for _, c := range q.Columns {
			if c.Aggregate == "" && !grouped[c.Column] {
				return model.ResultSet{
					Kind: "error",
					Error: fmt.Sprintf(
						"column %q must be aggregated or added to Group By",
						c.Column,
					),
				}
			}
		}
	} else if len(q.GroupBy) > 0 {
		return model.ResultSet{Kind: "error", Error: "group by requires at least one aggregated column"}
	}

	match, err := matchStage(table.Alias, q.Filters)
	if err != nil {
		return model.ResultSet{Kind: "error", Error: err.Error()}
	}

	pipeline := mongo.Pipeline{}
	if match != nil {
		pipeline = append(
			pipeline,
			bson.D{{Key: "$match", Value: match}},
		)
	}

	project := bson.D{{Key: "_id", Value: 0}}
	if hasAggregate {
		groupID := bson.D(nil)
		if len(q.GroupBy) > 0 {
			for _, g := range q.GroupBy {
				groupID = append(
					groupID,
					bson.E{Key: g.Column, Value: "$" + g.Column},
				)
			}
		}
		groupStage := bson.D{
			{Key: "_id", Value: groupIDValue(groupID)},
		}
		for _, c := range q.Columns {
			if c.Aggregate == "" {
				continue
			}
			op, ok := aggregateOps[c.Aggregate]
			if !ok {
				return model.ResultSet{
					Kind: "error",
					Error: fmt.Sprintf(
						"unsupported aggregate %q",
						c.Aggregate,
					),
				}
			}
			alias := aliasFor(c)
			var expr any
			if c.Aggregate == "count" {
				expr = bson.D{{Key: op, Value: 1}}
			} else {
				expr = bson.D{{Key: op, Value: "$" + c.Column}}
			}
			groupStage = append(
				groupStage,
				bson.E{Key: alias, Value: expr},
			)
		}
		pipeline = append(
			pipeline,
			bson.D{{Key: "$group", Value: groupStage}},
		)
		for _, c := range q.Columns {
			alias := aliasFor(c)
			if c.Aggregate == "" {
				project = append(
					project,
					bson.E{Key: alias, Value: "$_id." + c.Column},
				)
			} else {
				project = append(project, bson.E{Key: alias, Value: 1})
			}
		}
	} else {
		for _, c := range q.Columns {
			project = append(project, bson.E{Key: aliasFor(c), Value: "$" + c.Column})
		}
	}
	pipeline = append(
		pipeline,
		bson.D{{Key: "$project", Value: project}},
	)

	if len(q.OrderBy) > 0 {
		sortDoc := bson.D{}
		for _, o := range q.OrderBy {
			if err := resolve(o.VisualQueryRef); err != nil {
				return model.ResultSet{
					Kind:  "error",
					Error: err.Error(),
				}
			}
			alias, err := aliasForOrder(q.Columns, o)
			if err != nil {
				return model.ResultSet{
					Kind:  "error",
					Error: err.Error(),
				}
			}
			dir := 1
			if o.Descending {
				dir = -1
			}
			sortDoc = append(sortDoc, bson.E{Key: alias, Value: dir})
		}
		pipeline = append(
			pipeline,
			bson.D{{Key: "$sort", Value: sortDoc}},
		)
	}

	effectiveLimit := maxRows
	if q.Limit > 0 && q.Limit < effectiveLimit {
		effectiveLimit = q.Limit
	}
	// Fetch one extra document beyond the effective limit so truncation can
	// be reported honestly, the same way Browse does with its n+1 trick.
	pipeline = append(
		pipeline,
		bson.D{{Key: "$limit", Value: int64(effectiveLimit) + 1}},
	)

	cur, err := hd.db.Collection(table.Object).
		Aggregate(ctx, pipeline)
	if err != nil {
		return model.ResultSet{Kind: "error", Error: err.Error()}
	}
	defer cur.Close(ctx)

	rows := []map[string]any{}
	for cur.Next(ctx) {
		var raw bson.M
		if err := cur.Decode(&raw); err != nil {
			return model.ResultSet{Kind: "error", Error: err.Error()}
		}
		row := make(map[string]any, len(raw))
		for k, v := range raw {
			row[k] = sanitize(v)
		}
		rows = append(rows, row)
	}
	if err := cur.Err(); err != nil {
		return model.ResultSet{Kind: "error", Error: err.Error()}
	}

	truncated := len(rows) > effectiveLimit
	if truncated {
		rows = rows[:effectiveLimit]
	}

	columns := make([]model.ColumnMeta, len(q.Columns))
	for i, c := range q.Columns {
		columns[i] = model.ColumnMeta{
			Name:         aliasFor(c),
			DeclaredType: "mixed",
		}
	}

	return model.ResultSet{
		Kind:        "rows",
		Columns:     columns,
		Rows:        rows,
		Truncated:   truncated,
		StableOrder: len(q.OrderBy) > 0,
	}
}

// groupIDValue returns nil (a single group over the whole collection, SQL's
// "aggregate with no GROUP BY") when id is empty, or id itself otherwise. A
// bare nil bson.D can't be compared directly against a populated one in the
// caller, hence this small helper.
func groupIDValue(id bson.D) any {
	if len(id) == 0 {
		return nil
	}
	return id
}

// aliasFor names one output column: the user-chosen alias if set, else the
// bare column for a plain (non-aggregate) reference, else "<aggregate>_<column>",
// a readable default, matching the shape (if not the exact text) of the
// column name a SQL engine's unaliased aggregate expression would carry.
func aliasFor(c model.VisualQueryColumn) string {
	if c.Alias != "" {
		return c.Alias
	}
	if c.Aggregate != "" {
		return c.Aggregate + "_" + c.Column
	}
	return c.Column
}

// aliasForOrder finds the output alias an ORDER BY entry refers to. Unlike
// the SQL compiler (which can order by any grouped/aggregated expression,
// selected or not, because it emits fresh SQL text), a Mongo pipeline's
// $sort stage runs after $project and only sees the fields that survived
// it, so ordering here is restricted to a column actually present in the
// output. This is a real, honest MongoDB limitation, not an oversight.
func aliasForOrder(
	columns []model.VisualQueryColumn,
	o model.VisualQueryOrder,
) (string, error) {
	for _, c := range columns {
		if c.Column == o.Column && c.Aggregate == o.Aggregate {
			return aliasFor(c), nil
		}
	}
	return "", fmt.Errorf(
		"order by column %q must also be a selected output column for MongoDB aggregation",
		o.Column,
	)
}

// matchStage builds a $match stage from a flat ANDed filter list, wrapping
// every condition in its own {column: condition} document under $and rather
// than one bson.D keyed by column name, since two filters on the same
// column would otherwise collide as duplicate map keys.
func matchStage(
	alias string,
	filters []model.VisualQueryFilter,
) (bson.D, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	conds := bson.A{}
	for _, f := range filters {
		if f.Aggregate != "" {
			return nil, fmt.Errorf(
				"aggregate conditions belong in Having, not Filters",
			)
		}
		if f.Table != alias {
			return nil, fmt.Errorf("unknown table alias %q", f.Table)
		}
		if f.Column == "" {
			return nil, fmt.Errorf(
				"column reference on %q is missing a column name",
				f.Table,
			)
		}
		cond, err := filterCondition(f)
		if err != nil {
			return nil, err
		}
		conds = append(conds, bson.D{{Key: f.Column, Value: cond}})
	}
	return bson.D{{Key: "$and", Value: conds}}, nil
}

// filterCondition translates one VisualQueryFilter's operator into the
// value (or operator document) $match expects for that field. isNull/
// isNotNull compare against nil directly rather than $exists, since
// MongoDB's {field: null} already matches both an explicit null value and a
// missing field, the same "no value" meaning SQL's IS NULL carries.
func filterCondition(f model.VisualQueryFilter) (any, error) {
	switch f.Operator {
	case "eq":
		return f.Value, nil
	case "isNull":
		return nil, nil
	case "isNotNull":
		return bson.D{{Key: "$ne", Value: nil}}, nil
	case "in":
		if len(f.Values) == 0 {
			return nil, fmt.Errorf(
				"%q needs at least one value for IN",
				f.Table+"."+f.Column,
			)
		}
		return bson.D{{Key: "$in", Value: bson.A(f.Values)}}, nil
	case "like":
		return bson.D{
			{Key: "$regex", Value: likeToRegex(f.Value)},
		}, nil
	default:
		op, ok := filterOps[f.Operator]
		if !ok {
			return nil, fmt.Errorf(
				"unsupported filter operator %q",
				f.Operator,
			)
		}
		return bson.D{{Key: op, Value: f.Value}}, nil
	}
}

var regexMeta = regexp.MustCompile(`[.*+?^${}()|[\]\\]`)

// likeToRegex converts a SQL LIKE pattern (% = any run of characters, _ =
// exactly one character) into an anchored, case-sensitive regular
// expression, escaping every other regex metacharacter in the pattern
// literally first.
func likeToRegex(value any) string {
	pattern, _ := value.(string)
	escaped := regexMeta.ReplaceAllStringFunc(
		pattern,
		func(m string) string { return "\\" + m },
	)
	escaped = strings.ReplaceAll(escaped, "%", ".*")
	escaped = strings.ReplaceAll(escaped, "_", ".")
	return "^" + escaped + "$"
}
