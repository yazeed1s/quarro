package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// SQLExecutor runs SQL text against a connection. Only an engine that
// actually has SQL text to run implements this. A future document or
// key-value engine would not.
type SQLExecutor interface {
	Execute(
		context.Context,
		Connection,
		string,
		int,
		int,
		...any,
	) model.ResultSet
	ExecuteResults(
		context.Context,
		Connection,
		string,
		int,
		int,
		...any,
	) []model.ResultSet
	Split(string) ([]model.StatementRange, []string, error)
	Placeholders(string) ([]string, error)
	BindParams(string, map[string]any) (string, []any, error)
}
