package dialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
)

// OpenConnection checks out one *sql.Conn from a *sql.DB handle. h and the
// return value are untyped (provider.Handle/provider.Connection) at the
// provider.Connector boundary; every SQL engine package's handle/connection
// really is a *sql.DB/*sql.Conn, so this one implementation is shared by
// sqlite/postgres/mysql instead of being copied three times.
func OpenConnection(ctx context.Context, h any) (any, error) {
	return h.(*sql.DB).Conn(ctx)
}

// DiscardConnection poisons and closes conn, for a connection whose
// in-flight statement was cancelled or whose outcome is otherwise unknown:
// returning it to the pool could hand a later caller a connection stuck in
// an unexpected server-side state (e.g. mid-transaction).
func DiscardConnection(conn any) error {
	c := conn.(*sql.Conn)
	_ = c.Raw(func(any) error { return driver.ErrBadConn })
	return c.Close()
}

// CloseHandle closes a *sql.DB handle, and any SSH tunnel RegisterTunnel
// recorded for it. Both are attempted even if one fails; the pool's own
// error takes precedence when both do, since that is the failure a caller
// is most likely to need to act on.
func CloseHandle(h any) error {
	db := h.(*sql.DB)
	e := db.Close()
	tunnelsMu.Lock()
	t, ok := tunnels[db]
	if ok {
		delete(tunnels, db)
	}
	tunnelsMu.Unlock()
	if ok {
		if te := t.Close(); te != nil && e == nil {
			e = te
		}
	}
	return e
}

// Exec runs a plain control statement (BEGIN/COMMIT/ROLLBACK/SAVEPOINT and
// friends) against conn, returning a plain error rather than a bounded
// model.ResultSet. internal/sessions uses this for transaction and
// batch-atomicity control, never for a user's own SQL. That goes through
// SQLExecutor.Execute instead, which enforces row/byte bounds a control
// statement doesn't need.
func Exec(ctx context.Context, conn any, stmt string) error {
	_, err := conn.(*sql.Conn).ExecContext(ctx, stmt)
	return err
}
