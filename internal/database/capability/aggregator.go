package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// Aggregator runs a Visual Query AST directly through the provider. SQL
// engines implement this by compiling the AST to dialect SQL and executing
// it; later non-SQL engines can implement the same capability without
// exposing SQL.
type Aggregator interface {
	Aggregate(
		context.Context,
		Connection,
		model.VisualQuery,
		int,
	) model.ResultSet
}
