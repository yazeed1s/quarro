// Package mysql implements Quarro's provider.Provider contract for MySQL,
// on top of the shared dialect algorithms in internal/database/dialect.
package mysql

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/dialect"
)

// Provider implements provider.Provider for MySQL. It holds no state; every
// method takes the connection resource it needs as an argument. The
// interface's Handle/Connection parameters are always a *sql.DB/*sql.Conn
// in practice, since this package is the only one that ever creates one (via
// Open/OpenConnection) or type-asserts one back.
type Provider struct{}

// New returns a MySQL Provider.
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
	return dialect.Exec(ctx, conn, "SET SESSION TRANSACTION READ ONLY")
}

func quote(s string) string { return dialect.QuoteBacktick(s) }
func param(int) string      { return "?" }

// literal encodes a string containing a backslash or NUL byte as a hex
// literal instead of a quoted string, so MySQL's backslash-escaping (on by
// default, and unaffected by NO_BACKSLASH_ESCAPES since that mode can be
// changed independently of what this app controls) can never reinterpret an
// embedded backslash as an escape sequence.
func literal(v any) (string, error) {
	if str, ok := v.(string); ok &&
		strings.ContainsAny(str, "\\\x00") {
		return "CONVERT(X'" + hex.EncodeToString(
			[]byte(str),
		) + "' USING utf8mb4)", nil
	}
	return dialect.Literal(v, false)
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
