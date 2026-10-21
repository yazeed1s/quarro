package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// NativeQueryExecutor runs one engine-native query directly, for an
// engine with no SQL text to execute (SQLExecutor's Split/Placeholders/
// BindParams are SQL-text concepts with no equivalent for it). req is a
// small JSON envelope the frontend builds, the same "frontend-built
// envelope through one opaque string slot" shape Explainer's own req
// already uses for MongoDB — never something a user hand-types in the
// envelope's own syntax, even though the query content inside it (e.g. a
// pipeline) is exactly what the user wrote. maxRows bounds the result the
// same way Aggregator.Aggregate's own maxRows already does; there is no
// separate byte-size bound here because Browser/Aggregator do not have
// one for MongoDB either (SQL's byte cap is specific to database/sql
// driver buffering this engine does not go through).
type NativeQueryExecutor interface {
	ExecuteNative(
		ctx context.Context,
		h Connection,
		req string,
		maxRows int,
	) model.ResultSet
}

// NativeWriteDetector reports whether a native query would modify data, so
// a read-only profile can refuse it before it runs. Implemented by an
// engine whose native language mixes reads and writes freely (a Redis
// command line); an engine whose native surface is already read-only
// (MongoDB refuses $merge/$out unconditionally) does not need it.
type NativeWriteDetector interface {
	NativeQueryWrites(ctx context.Context, h Connection, req string) (bool, error)
}
