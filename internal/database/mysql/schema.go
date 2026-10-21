package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

func (Provider) Databases(
	ctx context.Context,
	h any,
) ([]string, error) {
	db := h.(*sql.DB)
	rows, e := db.QueryContext(ctx, "SHOW DATABASES")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if e := rows.Scan(&s); e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p Provider) Discover(
	ctx context.Context,
	h any,
) ([]model.Table, error) {
	db := h.(*sql.DB)
	conn, e := db.Conn(ctx)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	return p.discover(ctx, conn, "", "", false)
}

// DiscoverNames returns only table/view identity. It deliberately stops after
// the first catalog query; columns, indexes, and foreign keys are loaded by
// DiscoverTable when the user opens an object.
func (p Provider) DiscoverNames(ctx context.Context, h any) ([]model.Table, error) {
	db := h.(*sql.DB)
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return p.discover(ctx, conn, "", "", true)
}

func (p Provider) DiscoverTable(ctx context.Context, h any, schema, table string) (model.Table, error) {
	db := h.(*sql.DB)
	conn, err := db.Conn(ctx)
	if err != nil {
		return model.Table{}, err
	}
	defer conn.Close()
	tables, err := p.discover(ctx, conn, schema, table, false)
	if err != nil {
		return model.Table{}, err
	}
	if len(tables) != 1 {
		return model.Table{}, fmt.Errorf("table or view no longer exists")
	}
	return tables[0], nil
}

func (Provider) discover(
	ctx context.Context,
	c *sql.Conn,
	onlySchema, onlyTable string,
	namesOnly bool,
) ([]model.Table, error) {
	scope := "table_schema = DATABASE()"
	args := []any{}
	if onlySchema != "" {
		scope = "table_schema = ? AND table_name = ?"
		args = []any{onlySchema, onlyTable}
	}
	rows, e := c.QueryContext(
		ctx,
		"SELECT table_schema, table_name, table_type FROM information_schema.tables WHERE "+scope+" ORDER BY table_schema,table_name",
		args...)
	if e != nil {
		return nil, e
	}
	tables := []model.Table{}
	for rows.Next() {
		var s, n, k string
		if e := rows.Scan(&s, &n, &k); e != nil {
			rows.Close()
			return nil, e
		}
		kind := "table"
		if strings.Contains(k, "VIEW") {
			kind = "view"
		}
		tables = append(
			tables,
			model.Table{
				Name:        dialect.ObjectID(s, n),
				Schema:      s,
				ObjectName:  n,
				Kind:        kind,
				Columns:     []model.Column{},
				Indexes:     []model.Index{},
				ForeignKeys: []model.ForeignKey{},
			},
		)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if namesOnly {
		return tables, nil
	}
	// Four catalog queries total, rather than three extra round trips per table.
	lookup := map[string]*model.Table{}
	for i := range tables {
		lookup[tables[i].Name] = &tables[i]
	}
	rows, e = c.QueryContext(
		ctx,
		"SELECT table_schema,table_name,column_name,data_type,is_nullable,column_default FROM information_schema.columns WHERE "+scope+" ORDER BY table_schema,table_name,ordinal_position",
		args...)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var s, t, n, typ, nullable string
		var def sql.NullString
		if e := rows.Scan(&s, &t, &n, &typ, &nullable, &def); e != nil {
			rows.Close()
			return nil, e
		}
		if table := lookup[dialect.ObjectID(s, t)]; table != nil {
			col := model.Column{
				Name:    n,
				Type:    typ,
				NotNull: nullable == "NO",
			}
			if def.Valid {
				col.DefaultValue = &def.String
			}
			table.Columns = append(table.Columns, col)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	indexQuery := `SELECT table_schema,table_name,index_name,column_name,non_unique=0,index_name='PRIMARY' FROM information_schema.statistics WHERE ` + scope + ` ORDER BY table_schema,table_name,index_name,seq_in_index`
	rows, e = c.QueryContext(ctx, indexQuery, args...)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var s, t, n string
		var col sql.NullString
		var unique, pk bool
		if e := rows.Scan(&s, &t, &n, &col, &unique, &pk); e != nil {
			rows.Close()
			return nil, e
		}
		if table := lookup[dialect.ObjectID(s, t)]; table != nil {
			idx := -1
			for j := range table.Indexes {
				if table.Indexes[j].Name == n {
					idx = j
					break
				}
			}
			if idx < 0 {
				table.Indexes = append(
					table.Indexes,
					model.Index{
						Name:    n,
						Unique:  unique,
						Columns: []string{},
					},
				)
				idx = len(table.Indexes) - 1
			}
			table.Indexes[idx].Columns = append(
				table.Indexes[idx].Columns,
				col.String,
			)
			if pk {
				for j := range table.Columns {
					if table.Columns[j].Name == col.String ||
						quote(table.Columns[j].Name) == col.String {
						table.Columns[j].PrimaryKey = true
					}
				}
			}
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	fkScope := "kcu.table_schema = DATABASE()"
	if onlySchema != "" {
		fkScope = "kcu.table_schema = ? AND kcu.table_name = ?"
	}
	fkQuery := `SELECT kcu.table_schema,kcu.table_name,kcu.column_name,kcu.referenced_table_schema,kcu.referenced_table_name,kcu.referenced_column_name,rc.delete_rule,rc.update_rule
		FROM information_schema.key_column_usage kcu
		JOIN information_schema.referential_constraints rc
			ON rc.constraint_schema = kcu.table_schema AND rc.constraint_name = kcu.constraint_name AND rc.table_name = kcu.table_name
		WHERE kcu.referenced_table_name IS NOT NULL AND ` + fkScope + ` ORDER BY kcu.table_schema,kcu.table_name,kcu.constraint_name,kcu.ordinal_position`
	rows, e = c.QueryContext(ctx, fkQuery, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var s, t, n, rs, rt, rn, delRule, updRule string
		if e := rows.Scan(&s, &t, &n, &rs, &rt, &rn, &delRule, &updRule); e != nil {
			return nil, e
		}
		if table := lookup[dialect.ObjectID(s, t)]; table != nil {
			table.ForeignKeys = append(
				table.ForeignKeys,
				model.ForeignKey{
					Column:    n,
					RefTable:  dialect.ObjectID(rs, rt),
					RefColumn: rn,
					// MySQL's own REFERENTIAL_CONSTRAINTS text already matches
					// the engine-neutral vocabulary model.ForeignKey uses, so
					// no code-to-text mapping is needed here (unlike Postgres).
					OnDelete: delRule,
					OnUpdate: updRule,
				},
			)
		}
	}
	return tables, rows.Err()
}

// DiscoverRoutines lists stored functions/procedures in the connected
// database. ROUTINE_DEFINITION can be NULL when the connected user lacks
// the SHOW CREATE ROUTINE privilege even though the routine is listed; that
// leaves Definition empty rather than erroring the whole call over one
// routine's missing body.
func (p Provider) DiscoverRoutines(
	ctx context.Context,
	h any,
) ([]model.Function, error) {
	db := h.(*sql.DB)
	conn, e := db.Conn(ctx)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	rows, e := conn.QueryContext(
		ctx,
		`SELECT ROUTINE_SCHEMA, ROUTINE_NAME, ROUTINE_TYPE,
			DATA_TYPE, ROUTINE_DEFINITION
		FROM information_schema.routines
		WHERE ROUTINE_SCHEMA = DATABASE()
		ORDER BY ROUTINE_SCHEMA, ROUTINE_NAME`,
	)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Function{}
	for rows.Next() {
		var s, n, routineType string
		var ret, def sql.NullString
		if e := rows.Scan(&s, &n, &routineType, &ret, &def); e != nil {
			return nil, e
		}
		kind := "function"
		if routineType == "PROCEDURE" {
			kind = "procedure"
		}
		out = append(out, model.Function{
			Name:       dialect.ObjectID(s, n),
			Schema:     s,
			ObjectName: n,
			Kind:       kind,
			ReturnType: ret.String,
			Language:   "sql",
			Definition: def.String,
		})
	}
	return out, rows.Err()
}

func (p Provider) Browse(
	ctx context.Context,
	conn any,
	id, filter, filterColumn, sort string,
	desc bool,
	offset, n int,
) model.ResultSet {
	c := conn.(*sql.Conn)
	s, t, e := dialect.ParseObject(id)
	if e != nil {
		return failure("", e)
	}
	if offset < 0 || offset > 1000000 {
		return failure("", fmt.Errorf("page offset out of range"))
	}
	if n < 1 || n > 10000 {
		n = 500
	}
	tables, e := p.discover(ctx, c, s, t, false)
	if e != nil {
		return failure("", e)
	}
	if len(tables) != 1 {
		return failure(
			"",
			fmt.Errorf("table or view no longer exists"),
		)
	}
	table := tables[0]
	q := "SELECT * FROM " + quote(s) + "." + quote(t)
	args := []any{}
	parts := []string{}
	order := []string{}
	found := sort == ""
	foundFilterColumn := filterColumn == ""
	stable := false
	for _, col := range table.Columns {
		name := quote(col.Name)
		if filterColumn != "" {
			if col.Name == filterColumn {
				foundFilterColumn = true
				args = append(args, filter)
				parts = append(
					parts,
					"CAST("+name+" AS CHAR) = ?",
				)
			}
		} else if filter != "" {
			args = append(args, filter)
			parts = append(
				parts,
				"LOCATE(lower(?), lower(CAST("+name+" AS CHAR))) > 0",
			)
		}
		if col.Name == sort {
			found = true
		}
		if col.PrimaryKey {
			stable = true
			if col.Name != sort {
				order = append(order, name)
			}
		}
	}
	if !found {
		return failure("", fmt.Errorf("sort column no longer exists"))
	}
	if !foundFilterColumn {
		return failure("", fmt.Errorf("filter column no longer exists"))
	}
	if len(parts) > 0 {
		q += " WHERE (" + strings.Join(parts, " OR ") + ")"
	}
	if sort != "" {
		direction := " ASC"
		if desc {
			direction = " DESC"
		}
		order = append([]string{quote(sort) + direction}, order...)
	}
	if len(order) > 0 {
		q += " ORDER BY " + strings.Join(order, ",")
	}
	args = append(args, n+1, offset)
	q += " LIMIT " + param(
		len(args)-1,
	) + " OFFSET " + param(
		len(args),
	)
	result := p.Execute(ctx, c, q, n, 4<<20, args...)
	result.StableOrder = stable
	return result
}
