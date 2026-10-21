package capability

import "context"

// Transactor runs a plain control statement (BEGIN/COMMIT/ROLLBACK/SAVEPOINT
// and friends) against a connection, returning a plain error instead of a
// bounded model.ResultSet. internal/sessions uses this for transaction and
// batch-atomicity control, never for a user's own SQL. That always goes
// through SQLExecutor.Execute, which enforces the row/byte bounds a control
// statement doesn't need.
type Transactor interface {
	Exec(context.Context, Connection, string) error
}
