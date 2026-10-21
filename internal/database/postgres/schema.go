package postgres

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
	rows, e := db.QueryContext(
		ctx,
		"SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate AND has_database_privilege(datname, 'CONNECT') ORDER BY datname",
	)
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
	scope := "table_schema NOT IN ('pg_catalog','information_schema') AND table_schema NOT LIKE 'pg_toast%'"
	args := []any{}
	if onlySchema != "" {
		scope = "table_schema = " + param(
			1,
		) + " AND table_name = " + param(
			2,
		)
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
	indexQuery := `SELECT ns.nspname,t.relname,i.relname,pg_get_indexdef(ix.indexrelid,k.n,true),ix.indisunique,ix.indisprimary FROM pg_index ix JOIN pg_class t ON t.oid=ix.indrelid JOIN pg_namespace ns ON ns.oid=t.relnamespace JOIN pg_class i ON i.oid=ix.indexrelid CROSS JOIN LATERAL generate_series(1,ix.indnkeyatts) k(n) WHERE ns.nspname NOT IN ('pg_catalog','information_schema') ORDER BY ns.nspname,t.relname,i.relname,k.n`
	var indexArgs []any
	if onlySchema != "" {
		indexQuery = strings.Replace(
			indexQuery,
			" ORDER BY",
			" AND ns.nspname=$1 AND t.relname=$2 ORDER BY",
			1,
		)
		indexArgs = args
	}
	rows, e = c.QueryContext(ctx, indexQuery, indexArgs...)
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
	fkQuery := `SELECT ns.nspname,t.relname,a.attname,rns.nspname,rt.relname,ra.attname,c.confdeltype,c.confupdtype FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid JOIN pg_namespace ns ON ns.oid=t.relnamespace JOIN pg_class rt ON rt.oid=c.confrelid JOIN pg_namespace rns ON rns.oid=rt.relnamespace CROSS JOIN LATERAL unnest(c.conkey,c.confkey) WITH ORDINALITY k(local,foreigncol,ord) JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=k.local JOIN pg_attribute ra ON ra.attrelid=rt.oid AND ra.attnum=k.foreigncol WHERE c.contype='f' ORDER BY ns.nspname,t.relname,c.conname,k.ord`
	if onlySchema != "" {
		fkQuery = strings.Replace(
			fkQuery,
			" ORDER BY",
			" AND ns.nspname=$1 AND t.relname=$2 ORDER BY",
			1,
		)
	}
	rows, e = c.QueryContext(ctx, fkQuery, indexArgs...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var s, t, n, rs, rt, rn, delType, updType string
		if e := rows.Scan(&s, &t, &n, &rs, &rt, &rn, &delType, &updType); e != nil {
			return nil, e
		}
		if table := lookup[dialect.ObjectID(s, t)]; table != nil {
			table.ForeignKeys = append(
				table.ForeignKeys,
				model.ForeignKey{
					Column:    n,
					RefTable:  dialect.ObjectID(rs, rt),
					RefColumn: rn,
					OnDelete:  referentialAction(delType),
					OnUpdate:  referentialAction(updType),
				},
			)
		}
	}
	return tables, rows.Err()
}

// referentialAction maps pg_constraint's single-character confdeltype/
// confupdtype code to the engine-neutral action text model.ForeignKey uses.
func referentialAction(code string) string {
	switch code {
	case "c":
		return "CASCADE"
	case "n":
		return "SET NULL"
	case "d":
		return "SET DEFAULT"
	case "r":
		return "RESTRICT"
	default: // "a" (no action) or anything unrecognized
		return "NO ACTION"
	}
}

// DiscoverRoutines lists stored functions/procedures in every user schema
// (the same scope Discover itself excludes pg_catalog/information_schema/
// pg_toast% from). prokind distinguishes a function ('f') from a procedure
// ('p'); pg_get_function_arguments/pg_get_function_result give formatted
// signature text without parsing pg_proc.proargtypes here. prosrc is left
// empty for a C/internal-language routine rather than guessed at.
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
		`SELECT ns.nspname, p.proname, p.prokind,
			pg_get_function_arguments(p.oid),
			pg_get_function_result(p.oid),
			l.lanname, p.prosrc
		FROM pg_proc p
		JOIN pg_namespace ns ON ns.oid = p.pronamespace
		JOIN pg_language l ON l.oid = p.prolang
		WHERE ns.nspname NOT IN ('pg_catalog','information_schema')
			AND ns.nspname NOT LIKE 'pg_toast%'
			AND p.prokind IN ('f','p')
		ORDER BY ns.nspname, p.proname`,
	)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Function{}
	for rows.Next() {
		var s, n, k, params, lang, src string
		var ret sql.NullString
		if e := rows.Scan(&s, &n, &k, &params, &ret, &lang, &src); e != nil {
			return nil, e
		}
		kind := "function"
		if k == "p" {
			kind = "procedure"
		}
		// prosrc holds the C symbol name for a C/internal-language routine,
		// not a real body; only a sql/plpgsql routine's prosrc is its actual
		// source text.
		definition := ""
		if lang == "sql" || lang == "plpgsql" {
			definition = src
		}
		// pg_get_function_result returns NULL for a procedure, which has no
		// return type at all.
		out = append(out, model.Function{
			Name:       dialect.ObjectID(s, n),
			Schema:     s,
			ObjectName: n,
			Kind:       kind,
			Parameters: params,
			ReturnType: ret.String,
			Language:   lang,
			Definition: definition,
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
					"CAST("+name+" AS text) = "+param(len(args)),
				)
			}
		} else if filter != "" {
			args = append(args, filter)
			parts = append(
				parts,
				"strpos(lower(CAST("+name+" AS text)), lower("+param(
					len(args),
				)+")) > 0",
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
