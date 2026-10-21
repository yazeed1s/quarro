package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

func (p Provider) ApplyTableEdits(
	ctx context.Context,
	c any,
	id string,
	edits []model.RowEdit,
) (model.EditResult, error) {
	conn := c.(*sql.Conn)
	schema, table, err := dialect.ParseObject(id)
	if err != nil {
		return model.EditResult{}, err
	}
	tables, err := p.discover(ctx, conn, schema, table, false)
	if err != nil {
		return model.EditResult{}, err
	}
	if len(tables) != 1 {
		return model.EditResult{}, fmt.Errorf(
			"table no longer exists",
		)
	}
	return dialect.ApplyEdits(ctx, conn, dialect.EditDialect{
		Quote:            quote,
		Param:            param,
		TableName:        quote(schema) + "." + quote(table),
		DefaultValuesSQL: " DEFAULT VALUES",
		NullSafe:         func(column, param string) string { return column + " IS NOT DISTINCT FROM " + param },
	}, tables[0], edits)
}

func (p Provider) PreviewTableEdits(
	ctx context.Context,
	c any,
	id string,
	edits []model.RowEdit,
) ([]model.EditPreview, error) {
	conn := c.(*sql.Conn)
	schema, table, err := dialect.ParseObject(id)
	if err != nil {
		return nil, err
	}
	tables, err := p.discover(ctx, conn, schema, table, false)
	if err != nil {
		return nil, err
	}
	if len(tables) != 1 {
		return nil, fmt.Errorf("table no longer exists")
	}
	return dialect.PreviewEdits(dialect.EditDialect{
		Quote:            quote,
		Param:            param,
		TableName:        quote(schema) + "." + quote(table),
		DefaultValuesSQL: " DEFAULT VALUES",
		NullSafe:         func(column, param string) string { return column + " IS NOT DISTINCT FROM " + param },
	}, tables[0], edits)
}
