package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// Explainer runs EXPLAIN (or, with analyze true, EXPLAIN ANALYZE, which
// actually executes the statement) and returns an engine-neutral plan
// tree, already annotated by internal/database/explain.Annotate. Every SQL
// engine implements this using its own EXPLAIN syntax and output format;
// mongodb.Provider implements it by translating MongoDB's own explain
// command output the same way its Aggregate translates VisualQuery into an
// aggregation pipeline, proving the capability generalizes past SQL. Not
// composed into SQLProvider: SQLProvider's compile-time checks assert
// every SQL engine satisfies every interface it composes, and Explainer is
// also satisfied by a non-SQL engine.
type Explainer interface {
	Explain(
		context.Context,
		Connection,
		string,
		bool,
		int,
	) (model.ExplainPlan, error)
}
