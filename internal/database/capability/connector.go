package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/connections"
)

// Connector opens a live database handle from a connection profile, checks
// individual connections out of and back into it, and closes it.
// OpenConnection/DiscardConnection/Close exist so internal/sessions never
// imports database/sql itself: it owns connection lifecycle (which
// document has which connection, when to open/discard/close one) without
// knowing what a connection concretely is.
type Connector interface {
	Open(connections.Input) (Handle, error)
	OpenConnection(context.Context, Handle) (Connection, error)
	// DiscardConnection poisons and closes a connection whose outcome is
	// unknown (a cancelled or failed in-flight statement), so it is never
	// returned to a pool in a state a later caller could be surprised by.
	DiscardConnection(Connection) error
	Close(Handle) error
}
