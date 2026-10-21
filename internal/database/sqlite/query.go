package sqlite

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

func literal(
	v any,
) (string, error) {
	return dialect.Literal(v, false)
}

func (Provider) CompileSelect(q model.VisualQuery) (string, error) {
	return dialect.CompileSelect(dialect.QueryDialect{
		Quote:    quoteIdent,
		TableSQL: func(object string) (string, error) { return quoteIdent(object), nil },
		Literal:  literal,
	}, q)
}

func (Provider) FormatLiteral(
	v any,
) (string, error) {
	return literal(v)
}

func (Provider) QualifiedTable(
	object string,
) (string, error) {
	return quoteIdent(object), nil
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
