package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// DiscoverConstraints lists schema.table's named CHECK and UNIQUE
// constraints (MySQL 8.0.16+; CHECK_CONSTRAINTS did not exist before
// that). Primary keys are excluded: Table.Columns already reports
// PrimaryKey per column.
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
		`SELECT cc.CONSTRAINT_NAME, cc.CHECK_CLAUSE
		FROM information_schema.CHECK_CONSTRAINTS cc
		JOIN information_schema.TABLE_CONSTRAINTS tc
			ON tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA AND tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
		WHERE tc.TABLE_SCHEMA = ? AND tc.TABLE_NAME = ? AND tc.CONSTRAINT_TYPE = 'CHECK'
		ORDER BY cc.CONSTRAINT_NAME`,
		schema, table,
	)
	if e != nil {
		return nil, e
	}
	for checkRows.Next() {
		var name, clause string
		if e := checkRows.Scan(&name, &clause); e != nil {
			checkRows.Close()
			return nil, e
		}
		out = append(out, model.Constraint{Name: name, Kind: "check", Expression: "CHECK (" + clause + ")"})
	}
	if e := checkRows.Err(); e != nil {
		checkRows.Close()
		return nil, e
	}
	checkRows.Close()

	uniqueRows, e := conn.QueryContext(
		ctx,
		`SELECT tc.CONSTRAINT_NAME, kcu.COLUMN_NAME
		FROM information_schema.TABLE_CONSTRAINTS tc
		JOIN information_schema.KEY_COLUMN_USAGE kcu
			ON kcu.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA AND kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME AND kcu.TABLE_NAME = tc.TABLE_NAME
		WHERE tc.TABLE_SCHEMA = ? AND tc.TABLE_NAME = ? AND tc.CONSTRAINT_TYPE = 'UNIQUE'
		ORDER BY tc.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`,
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

// DiscoverDefinition returns a table or view's verbatim DDL text via
// SHOW CREATE TABLE/SHOW CREATE VIEW, both of which MySQL already reports
// directly from its own catalog.
func (Provider) DiscoverDefinition(
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
		`SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`,
		schema, table,
	).Scan(&kind)
	if e == sql.ErrNoRows {
		return model.Definition{}, fmt.Errorf("table or view no longer exists")
	}
	if e != nil {
		return model.Definition{}, e
	}

	target := quote(schema) + "." + quote(table)
	if kind == "VIEW" {
		var name, def, charset, collation string
		if e := conn.QueryRowContext(ctx, "SHOW CREATE VIEW "+target).Scan(&name, &def, &charset, &collation); e != nil {
			return model.Definition{}, e
		}
		return model.Definition{SQL: def, Verbatim: true}, nil
	}
	var name, def string
	if e := conn.QueryRowContext(ctx, "SHOW CREATE TABLE "+target).Scan(&name, &def); e != nil {
		return model.Definition{}, e
	}
	return model.Definition{SQL: def, Verbatim: true}, nil
}
