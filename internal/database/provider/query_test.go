package provider

import (
	"strings"
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

func twoTableJoin() model.VisualQuery {
	return model.VisualQuery{
		Tables: []model.VisualQueryTable{
			{
				Object: dialect.ObjectID("public", "users"),
				Alias:  "t1",
			},
			{
				Object: dialect.ObjectID("public", "orders"),
				Alias:  "t2",
			},
		},
		Joins: []model.VisualQueryJoin{
			{
				Type: "inner",
				Left: model.VisualQueryRef{
					Table:  "t1",
					Column: "id",
				},
				Right: model.VisualQueryRef{
					Table:  "t2",
					Column: "user_id",
				},
			},
		},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "name",
				},
			},
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t2",
					Column: "total",
				},
				Alias: "order_total",
			},
		},
		Filters: []model.VisualQueryFilter{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t2",
					Column: "status",
				},
				Operator: "eq",
				Value:    "paid",
			},
		},
	}
}

func TestCompileSelectDialects(t *testing.T) {
	sqliteQuery := twoTableJoin()
	sqliteQuery.Tables[0].Object = "users"
	sqliteQuery.Tables[1].Object = "orders"

	cases := []struct {
		name   string
		engine connections.Engine
		query  model.VisualQuery
		want   string
	}{
		{
			"sqlite join+filter",
			connections.EngineSQLite,
			sqliteQuery,
			`SELECT "t1"."name", "t2"."total" AS "order_total" FROM "users" AS "t1" JOIN "orders" AS "t2" ON "t1"."id" = "t2"."user_id" WHERE "t2"."status" = 'paid'`,
		},
		{
			"postgres join+filter",
			connections.EnginePostgres,
			twoTableJoin(),
			`SELECT "t1"."name", "t2"."total" AS "order_total" FROM "public"."users" AS "t1" JOIN "public"."orders" AS "t2" ON "t1"."id" = "t2"."user_id" WHERE "t2"."status" = 'paid'`,
		},
		{
			"mysql join+filter",
			connections.EngineMySQL,
			twoTableJoin(),
			"SELECT `t1`.`name`, `t2`.`total` AS `order_total` FROM `public`.`users` AS `t1` JOIN `public`.`orders` AS `t2` ON `t1`.`id` = `t2`.`user_id` WHERE `t2`.`status` = 'paid'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := mustGet(t, c.engine).CompileSelect(c.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestCompileSelectAggregateGroupBy(t *testing.T) {
	q := model.VisualQuery{
		Tables: []model.VisualQueryTable{
			{Object: "orders", Alias: "t1"},
		},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "status",
				},
			},
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "id",
				},
				Aggregate: "count",
				Alias:     "n",
			},
		},
		GroupBy: []model.VisualQueryRef{
			{Table: "t1", Column: "status"},
		},
		Having: []model.VisualQueryFilter{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "id",
				},
				Aggregate: "count",
				Operator:  "gt",
				Value:     float64(1),
			},
		},
		OrderBy: []model.VisualQueryOrder{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "status",
				},
			},
		},
		Limit: 10,
	}
	got, err := mustGet(t, connections.EngineSQLite).CompileSelect(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT "t1"."status", COUNT("t1"."id") AS "n" FROM "orders" AS "t1" GROUP BY "t1"."status" HAVING COUNT("t1"."id") > 1 ORDER BY "t1"."status" ASC LIMIT 10`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestCompileSelectInFilter(t *testing.T) {
	q := model.VisualQuery{
		Tables: []model.VisualQueryTable{
			{Object: "orders", Alias: "t1"},
		},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "id",
				},
			},
		},
		Filters: []model.VisualQueryFilter{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "status",
				},
				Operator: "in",
				Values:   []any{"paid", "shipped"},
			},
		},
	}
	got, err := mustGet(t, connections.EngineSQLite).CompileSelect(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(
		got,
		`"t1"."status" IN ('paid', 'shipped')`,
	) {
		t.Fatalf("got %s", got)
	}
}

func TestCompileSelectValidationErrors(t *testing.T) {
	base := func() model.VisualQuery {
		return model.VisualQuery{
			Tables: []model.VisualQueryTable{
				{Object: "orders", Alias: "t1"},
			},
			Columns: []model.VisualQueryColumn{
				{
					VisualQueryRef: model.VisualQueryRef{
						Table:  "t1",
						Column: "id",
					},
				},
			},
		}
	}
	cases := []struct {
		name  string
		query model.VisualQuery
	}{
		{
			"no tables",
			model.VisualQuery{
				Columns: []model.VisualQueryColumn{
					{
						VisualQueryRef: model.VisualQueryRef{
							Table:  "t1",
							Column: "id",
						},
					},
				},
			},
		},
		{
			"no columns",
			model.VisualQuery{
				Tables: []model.VisualQueryTable{
					{Object: "orders", Alias: "t1"},
				},
			},
		},
		{"unknown alias", func() model.VisualQuery {
			q := base()
			q.Columns[0].Table = "missing"
			return q
		}()},
		{"bad join type", func() model.VisualQuery {
			q := base()
			q.Tables = append(
				q.Tables,
				model.VisualQueryTable{Object: "users", Alias: "t2"},
			)
			q.Joins = []model.VisualQueryJoin{
				{
					Type: "full",
					Left: model.VisualQueryRef{
						Table:  "t1",
						Column: "id",
					},
					Right: model.VisualQueryRef{
						Table:  "t2",
						Column: "id",
					},
				},
			}
			return q
		}()},
		{"unjoined table", func() model.VisualQuery {
			q := base()
			q.Tables = append(
				q.Tables,
				model.VisualQueryTable{Object: "users", Alias: "t2"},
			)
			return q
		}()},
		{"aggregate without group by", func() model.VisualQuery {
			q := base()
			q.Columns = append(
				q.Columns,
				model.VisualQueryColumn{
					VisualQueryRef: model.VisualQueryRef{
						Table:  "t1",
						Column: "total",
					},
					Aggregate: "sum",
				},
			)
			return q
		}()},
		{"empty in", func() model.VisualQuery {
			q := base()
			q.Filters = []model.VisualQueryFilter{
				{
					VisualQueryRef: model.VisualQueryRef{
						Table:  "t1",
						Column: "status",
					},
					Operator: "in",
				},
			}
			return q
		}()},
		{"having without aggregate", func() model.VisualQuery {
			q := base()
			q.Having = []model.VisualQueryFilter{
				{
					VisualQueryRef: model.VisualQueryRef{
						Table:  "t1",
						Column: "status",
					},
					Operator: "eq",
					Value:    "paid",
				},
			}
			return q
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := mustGet(t, connections.EngineSQLite).CompileSelect(c.query); err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
		})
	}
}

func TestReversedOuterJoinPreservesRequestedSide(t *testing.T) {
	q := twoTableJoin()
	q.Joins[0].Type = "left"
	q.Joins[0].Left, q.Joins[0].Right = q.Joins[0].Right, q.Joins[0].Left
	got, err := mustGet(
		t,
		connections.EnginePostgres,
	).CompileSelect(q)
	if err != nil || !strings.Contains(got, "RIGHT JOIN") {
		t.Fatalf("%s: %v", got, err)
	}
}

func TestMySQLBackslashLiteralIsModeIndependent(t *testing.T) {
	q := model.VisualQuery{
		Tables: []model.VisualQueryTable{
			{
				Object: dialect.ObjectID("public", "orders"),
				Alias:  "t1",
			},
		},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "id",
				},
			},
		},
		Filters: []model.VisualQueryFilter{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "note",
				},
				Operator: "eq",
				Value:    "x\\' OR 1=1 --",
			},
		},
	}
	got, err := mustGet(t, connections.EngineMySQL).CompileSelect(q)
	if err != nil || !strings.Contains(got, "CONVERT(X'") ||
		strings.Contains(got, "OR 1=1") {
		t.Fatalf("unsafe literal: %s %v", got, err)
	}
}

func TestAggregateRejectsUngroupedHavingAndOrder(t *testing.T) {
	q := model.VisualQuery{
		Tables: []model.VisualQueryTable{
			{Object: "orders", Alias: "t1"},
		},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "id",
				},
				Aggregate: "count",
			},
		},
	}
	q.Having = []model.VisualQueryFilter{
		{
			VisualQueryRef: model.VisualQueryRef{
				Table:  "t1",
				Column: "status",
			},
			Operator: "eq",
			Value:    "paid",
		},
	}
	if _, err := mustGet(t, connections.EngineSQLite).CompileSelect(q); err == nil {
		t.Fatal("ungrouped having accepted")
	}
	q.Having = nil
	q.OrderBy = []model.VisualQueryOrder{
		{
			VisualQueryRef: model.VisualQueryRef{
				Table:  "t1",
				Column: "status",
			},
		},
	}
	if _, err := mustGet(t, connections.EngineSQLite).CompileSelect(q); err == nil {
		t.Fatal("ungrouped ordering accepted")
	}
}
