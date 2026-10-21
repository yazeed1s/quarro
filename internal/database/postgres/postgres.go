// Package postgres implements Quarro's provider.Provider contract for
// PostgreSQL, on top of the shared dialect algorithms in
// internal/database/dialect.
package postgres

import (
	"context"
	"fmt"

	"github.com/yazeed1s/quarro/internal/database/dialect"
)

// Provider implements provider.Provider for PostgreSQL. It holds no state;
// every method takes the connection resource it needs as an argument. The
// interface's Handle/Connection parameters are always a *sql.DB/*sql.Conn
// (backed by pgx's stdlib adapter) in practice, since this package is the only
// one that ever creates one (via Open/OpenConnection) or type-asserts one
// back.
type Provider struct{}

// New returns a PostgreSQL Provider.
func New() Provider { return Provider{} }

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
	return dialect.Exec(ctx, conn, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY")
}

func quote(s string) string { return dialect.QuoteDouble(s) }
func param(n int) string    { return fmt.Sprintf("$%d", n) }

func literal(
	v any,
) (string, error) {
	return dialect.Literal(v, true)
}

func (Provider) QuoteIdentifier(
	s string,
) string {
	return quote(s)
}

func (Provider) FormatLiteral(
	v any,
) (string, error) {
	return literal(v)
}
func (Provider) QualifiedTable(object string) (string, error) {
	schema, table, err := dialect.ParseObject(object)
	if err != nil {
		return "", err
	}
	return quote(schema) + "." + quote(table), nil
}
