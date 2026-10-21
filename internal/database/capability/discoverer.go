package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// Discoverer inspects the engine's catalog: what databases exist, and (for
// the currently selected one) what tables/views/columns/indexes/foreign
// keys it has.
type Discoverer interface {
	Discover(context.Context, Handle) ([]model.Table, error)
	Databases(context.Context, Handle) ([]string, error)
}

// LazyDiscoverer exposes the two cheaper catalog operations used by the
// object browser on server databases: listing object names without loading
// every column/index/foreign key, and loading one object's complete metadata
// on demand. Discover remains the full-schema operation for consumers such as
// the schema diagram and visual query builder.
type LazyDiscoverer interface {
	DiscoverNames(context.Context, Handle) ([]model.Table, error)
	DiscoverTable(context.Context, Handle, string, string) (model.Table, error)
}

// RoutineDiscoverer lists stored functions/procedures. A separate small
// interface from Discoverer, not folded into it, because SQLite/MongoDB/
// Redis never implement it, the same way Aggregator/Explainer are their
// own interfaces rather than required methods on Discoverer.
type RoutineDiscoverer interface {
	DiscoverRoutines(context.Context, Handle) ([]model.Function, error)
}

// ConstraintDiscoverer lists named CHECK/UNIQUE constraints beyond what
// Indexes already reports. A separate small interface from Discoverer,
// mirroring RoutineDiscoverer's own rationale: MongoDB/Redis never
// implement it, and folding it into Discoverer would force every engine
// to answer a question only SQL catalogs have.
type ConstraintDiscoverer interface {
	DiscoverConstraints(ctx context.Context, h Handle, schema, table string) ([]model.Constraint, error)
}

// DefinitionDiscoverer returns one object's engine-native DDL text. See
// model.Definition's Verbatim field for the verbatim/reconstructed
// distinction an implementation must set correctly.
type DefinitionDiscoverer interface {
	DiscoverDefinition(ctx context.Context, h Handle, schema, table string) (model.Definition, error)
}
