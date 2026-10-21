package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// DiscoverConstraints lists schema.table's named CHECK and UNIQUE
// constraints. Primary keys are excluded: Table.Columns already reports
// PrimaryKey per column, and a constraints view exists to surface what
// that does not, not to repeat it.
func (Provider) DiscoverConstraints(
	ctx context.Context,
	h any,
	schema, table string,
) ([]model.Constraint, error) {
	db := h.(*sql.DB)
	conn, e := db.Conn(ctx)
	if e != nil {
		return nil, e
	}
	defer conn.Close()

	out := []model.Constraint{}

	checkRows, e := conn.QueryContext(
		ctx,
		`SELECT con.conname, pg_get_constraintdef(con.oid, true)
		FROM pg_constraint con
		JOIN pg_class t ON t.oid = con.conrelid
		JOIN pg_namespace ns ON ns.oid = t.relnamespace
		WHERE ns.nspname = $1 AND t.relname = $2 AND con.contype = 'c'
		ORDER BY con.conname`,
		schema, table,
	)
	if e != nil {
		return nil, e
	}
	for checkRows.Next() {
		var name, def string
		if e := checkRows.Scan(&name, &def); e != nil {
			checkRows.Close()
			return nil, e
		}
		out = append(out, model.Constraint{Name: name, Kind: "check", Expression: def})
	}
	if e := checkRows.Err(); e != nil {
		checkRows.Close()
		return nil, e
	}
	checkRows.Close()

	// One row per constrained column, the same unnest-and-fan-out shape the
	// foreign key discovery query already uses, so a composite UNIQUE
	// constraint's full column list is read from the catalog rather than
	// parsed out of pg_get_constraintdef's free-text definition.
	uniqueRows, e := conn.QueryContext(
		ctx,
		`SELECT con.conname, a.attname
		FROM pg_constraint con
		JOIN pg_class t ON t.oid = con.conrelid
		JOIN pg_namespace ns ON ns.oid = t.relnamespace
		CROSS JOIN LATERAL unnest(con.conkey) WITH ORDINALITY k(colnum, ord)
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.colnum
		WHERE ns.nspname = $1 AND t.relname = $2 AND con.contype = 'u'
		ORDER BY con.conname, k.ord`,
		schema, table,
	)
	if e != nil {
		return nil, e
	}
	defer uniqueRows.Close()
	byName := map[string]int{}
	for uniqueRows.Next() {
		var name, col string
		if e := uniqueRows.Scan(&name, &col); e != nil {
			return nil, e
		}
		idx, ok := byName[name]
		if !ok {
			out = append(out, model.Constraint{Name: name, Kind: "unique"})
			idx = len(out) - 1
			byName[name] = idx
		}
		out[idx].Columns = append(out[idx].Columns, col)
	}
	if e := uniqueRows.Err(); e != nil {
		return nil, e
	}
	return out, nil
}

// DiscoverDefinition returns a view's verbatim query text via
// pg_get_viewdef, or a reconstructed CREATE TABLE statement for a table
// (PostgreSQL has no single built-in function for the latter, unlike
// MySQL's SHOW CREATE TABLE or SQLite's sqlite_master.sql) explicitly
// marked non-verbatim so a caller never presents it as engine-authoritative
// DDL.
func (p Provider) DiscoverDefinition(
	ctx context.Context,
	h any,
	schema, table string,
) (model.Definition, error) {
	db := h.(*sql.DB)
	conn, e := db.Conn(ctx)
	if e != nil {
		return model.Definition{}, e
	}
	defer conn.Close()

	var kind string
	e = conn.QueryRowContext(
		ctx,
		`SELECT c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $1 AND c.relname = $2`,
		schema, table,
	).Scan(&kind)
	if e == sql.ErrNoRows {
		return model.Definition{}, fmt.Errorf("table or view no longer exists")
	}
	if e != nil {
		return model.Definition{}, e
	}

	if kind == "v" || kind == "m" { // ordinary view or materialized view
		var def string
		if e := conn.QueryRowContext(
			ctx,
			`SELECT pg_get_viewdef(c.oid, true) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $1 AND c.relname = $2`,
			schema, table,
		).Scan(&def); e != nil {
			return model.Definition{}, e
		}
		return model.Definition{SQL: strings.TrimRight(def, "\n"), Verbatim: true}, nil
	}

	tables, e := p.discover(ctx, conn, schema, table, false)
	if e != nil {
		return model.Definition{}, e
	}
	if len(tables) != 1 {
		return model.Definition{}, fmt.Errorf("table or view no longer exists")
	}
	constraints, e := Provider{}.DiscoverConstraints(ctx, db, schema, table)
	if e != nil {
		return model.Definition{}, e
	}
	return model.Definition{SQL: reconstructCreateTable(tables[0], constraints), Verbatim: false}, nil
}

// reconstructCreateTable composes a readable, not necessarily byte-exact,
// CREATE TABLE approximation from discovery data already available
// (columns, primary key, foreign keys, named constraints). Indexes are
// listed separately as a trailing comment, since PostgreSQL issues them as
// their own CREATE INDEX statements rather than inline in CREATE TABLE.
func reconstructCreateTable(t model.Table, constraints []model.Constraint) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s.%s (\n", quote(t.Schema), quote(t.ObjectName))
	lines := []string{}
	var pk []string
	for _, c := range t.Columns {
		line := "    " + quote(c.Name) + " " + c.Type
		if c.NotNull {
			line += " NOT NULL"
		}
		if c.DefaultValue != nil {
			line += " DEFAULT " + *c.DefaultValue
		}
		lines = append(lines, line)
		if c.PrimaryKey {
			pk = append(pk, quote(c.Name))
		}
	}
	if len(pk) > 0 {
		lines = append(lines, "    PRIMARY KEY ("+strings.Join(pk, ", ")+")")
	}
	for _, fk := range t.ForeignKeys {
		refTable := fk.RefTable
		if s, n, e := dialect.ParseObject(fk.RefTable); e == nil {
			refTable = quote(s) + "." + quote(n)
		}
		line := fmt.Sprintf("    FOREIGN KEY (%s) REFERENCES %s(%s)", quote(fk.Column), refTable, quote(fk.RefColumn))
		if fk.OnDelete != "" && fk.OnDelete != "NO ACTION" {
			line += " ON DELETE " + fk.OnDelete
		}
		if fk.OnUpdate != "" && fk.OnUpdate != "NO ACTION" {
			line += " ON UPDATE " + fk.OnUpdate
		}
		lines = append(lines, line)
	}
	for _, c := range constraints {
		switch c.Kind {
		case "unique":
			cols := make([]string, len(c.Columns))
			for i, name := range c.Columns {
				cols[i] = quote(name)
			}
			lines = append(lines, fmt.Sprintf("    CONSTRAINT %s UNIQUE (%s)", quote(c.Name), strings.Join(cols, ", ")))
		case "check":
			lines = append(lines, fmt.Sprintf("    CONSTRAINT %s %s", quote(c.Name), c.Expression))
		}
	}
	b.WriteString(strings.Join(lines, ",\n"))
	b.WriteString("\n);")
	if len(t.Indexes) > 0 {
		b.WriteString("\n\n-- Indexes:\n")
		for _, idx := range t.Indexes {
			unique := ""
			if idx.Unique {
				unique = "UNIQUE "
			}
			cols := make([]string, len(idx.Columns))
			for i, name := range idx.Columns {
				cols[i] = quote(name)
			}
			fmt.Fprintf(&b, "-- CREATE %sINDEX %s ON %s.%s (%s)\n", unique, quote(idx.Name), quote(t.Schema), quote(t.ObjectName), strings.Join(cols, ", "))
		}
	}
	return b.String()
}
