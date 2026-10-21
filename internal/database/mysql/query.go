package mysql

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

func (Provider) CompileSelect(q model.VisualQuery) (string, error) {
	return dialect.CompileSelect(dialect.QueryDialect{
		Quote: quote,
		TableSQL: func(object string) (string, error) {
			schema, table, err := dialect.ParseObject(object)
			if err != nil {
				return "", err
			}
			return quote(schema) + "." + quote(table), nil
		},
		Literal: literal,
	}, q)
}

func (p Provider) Aggregate(
	ctx context.Context,
	conn any,
	q model.VisualQuery,
	maxRows int,
) model.ResultSet {
	return dialect.AggregateViaSQL(
		ctx,
		conn,
		p,
		p,
		q,
		maxRows,
		dialect.DefaultResultBytes,
	)
}
