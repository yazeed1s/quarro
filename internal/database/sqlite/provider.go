package sqlite

import (
	"context"
	"database/sql"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// Provider implements provider.Provider for SQLite. It holds no state;
// every method takes the connection resource it needs as an argument. The
// interface's Handle/Connection parameters are always a *sql.DB/*sql.Conn
// in practice, since this package is the only one that ever creates one (via
// Open/OpenConnection) or type-asserts one back.
type Provider struct{}

// New returns a SQLite Provider.
func New() Provider { return Provider{} }

func (Provider) Open(
	i connections.Input,
) (any, error) {
	return Open(i.Path, i.ReadOnly)
}

func (Provider) OpenConnection(
	ctx context.Context,
	h any,
) (any, error) {
	return dialect.OpenConnection(ctx, h)
}

func (Provider) DiscardConnection(
	conn any,
) error {
	return dialect.DiscardConnection(conn)
}

func (Provider) Close(h any) error { return dialect.CloseHandle(h) }

func (Provider) Exec(
	ctx context.Context,
	conn any,
	stmt string,
) error {
	return dialect.Exec(ctx, conn, stmt)
}

func (Provider) EnforceReadOnly(ctx context.Context, conn any) error {
	return dialect.Exec(ctx, conn, "PRAGMA query_only = ON")
}

func (Provider) Execute(
	ctx context.Context,
	conn any,
	stmt string,
	n, b int,
	args ...any,
) model.ResultSet {
	return ExecuteOnConn(ctx, conn.(*sql.Conn), stmt, n, b, args...)
}

func (p Provider) ExecuteResults(
	ctx context.Context,
	conn any,
	stmt string,
	n, b int,
	args ...any,
) []model.ResultSet {
	return []model.ResultSet{
		p.Execute(ctx, conn, stmt, n, b, args...),
	}
}

func (Provider) Discover(
	_ context.Context,
	h any,
) ([]model.Table, error) {
	return Discover(h.(*sql.DB))
}

func (Provider) Browse(
	ctx context.Context,
	conn any,
	table, filter, filterColumn, sort string,
	desc bool,
	offset, n int,
) model.ResultSet {
	return Browse(
		ctx,
		conn.(*sql.Conn),
		table,
		filter,
		filterColumn,
		sort,
		desc,
		offset,
		n,
	)
}

func (Provider) Databases(
	context.Context,
	any,
) ([]string, error) {
	return []string{"main"}, nil
}

func (Provider) Split(
	s string,
) ([]model.StatementRange, []string, error) {
	return SplitStatements(s), StatementTexts(s), nil
}

func (Provider) QuoteIdentifier(
	s string,
) string {
	return quoteIdent(s)
}
