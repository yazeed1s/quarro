package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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
	var engine string
	if err := conn.QueryRowContext(ctx, "SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", schema, table).Scan(&engine); err != nil {
		return model.EditResult{}, err
	}
	if !strings.EqualFold(engine, "InnoDB") {
		return model.EditResult{}, fmt.Errorf(
			"staged editing requires InnoDB; %s cannot guarantee atomic rollback",
			engine,
		)
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
		DefaultValuesSQL: " () VALUES ()",
		NullSafe:         func(column, param string) string { return column + " <=> " + param },
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
	var engine string
	if err := conn.QueryRowContext(ctx, "SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", schema, table).Scan(&engine); err != nil {
		return nil, err
	}
	if !strings.EqualFold(engine, "InnoDB") {
		return nil, fmt.Errorf(
			"staged editing requires InnoDB; %s cannot guarantee atomic rollback",
			engine,
		)
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
		DefaultValuesSQL: " () VALUES ()",
		NullSafe:         func(column, param string) string { return column + " <=> " + param },
	}, tables[0], edits)
}
