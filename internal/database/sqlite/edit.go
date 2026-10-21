package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

func (Provider) ApplyTableEdits(
	ctx context.Context,
	c any,
	table string,
	edits []model.RowEdit,
) (model.EditResult, error) {
	conn := c.(*sql.Conn)
	meta, err := tableForEdit(ctx, conn, table)
	if err != nil {
		return model.EditResult{}, err
	}
	return dialect.ApplyEdits(ctx, conn, dialect.EditDialect{
		Quote:            quoteIdent,
		Param:            func(int) string { return "?" },
		TableName:        quoteIdent(table),
		DefaultValuesSQL: " DEFAULT VALUES",
		NullSafe:         func(column, param string) string { return column + " IS " + param },
	}, meta, edits)
}

func (Provider) PreviewTableEdits(
	ctx context.Context,
	c any,
	table string,
	edits []model.RowEdit,
) ([]model.EditPreview, error) {
	conn := c.(*sql.Conn)
	meta, err := tableForEdit(ctx, conn, table)
	if err != nil {
		return nil, err
	}
	return dialect.PreviewEdits(dialect.EditDialect{
		Quote:            quoteIdent,
		Param:            func(int) string { return "?" },
		TableName:        quoteIdent(table),
		DefaultValuesSQL: " DEFAULT VALUES",
		NullSafe:         func(column, param string) string { return column + " IS " + param },
	}, meta, edits)
}

func tableForEdit(
	ctx context.Context,
	conn *sql.Conn,
	table string,
) (model.Table, error) {
	var kind string
	if err := conn.QueryRowContext(ctx, `SELECT type FROM sqlite_schema WHERE name = ? AND type IN ('table','view')`, table).Scan(&kind); err != nil {
		return model.Table{}, fmt.Errorf(
			"table or view no longer exists",
		)
	}
	rows, err := conn.QueryContext(
		ctx,
		`SELECT name, type, "notnull", pk, dflt_value FROM pragma_table_xinfo(?)`,
		table,
	)
	if err != nil {
		return model.Table{}, err
	}
	defer rows.Close()
	out := model.Table{
		Name:       table,
		ObjectName: table,
		Kind:       kind,
		Columns:    []model.Column{},
	}
	for rows.Next() {
		var name, typ string
		var notNull, pk int
		var def sql.NullString
		if err := rows.Scan(&name, &typ, &notNull, &pk, &def); err != nil {
			return model.Table{}, err
		}
		col := model.Column{
			Name:       name,
			Type:       typ,
			NotNull:    notNull != 0,
			PrimaryKey: pk != 0,
		}
		if def.Valid {
			col.DefaultValue = &def.String
		}
		out.Columns = append(out.Columns, col)
	}
	return out, rows.Err()
}
