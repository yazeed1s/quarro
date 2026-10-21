package capability

import "context"

// ReadOnlyEnforcer configures one checked-out connection so the database
// itself refuses mutations. The session registry invokes it before exposing
// a connection belonging to a read-only profile to any query service.
//
// This is intentionally separate from SQLExecutor: MongoDB and Redis enforce
// their native write surfaces in the application service, while SQL engines
// can ask the database connection to enforce the invariant as well.
type ReadOnlyEnforcer interface {
	EnforceReadOnly(context.Context, Connection) error
}
