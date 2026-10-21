package postgres

import "github.com/yazeed1s/quarro/internal/database/dialect"

// Placeholders returns the distinct ":name" parameters in sqlText, in order
// of first appearance, so the frontend can prompt for values before running
// a parameterized statement. A PostgreSQL "::" type cast is never treated
// as a placeholder.
func (Provider) Placeholders(sqlText string) ([]string, error) {
	return dialect.PlaceholderNames(true, sqlText)
}

// BindParams rewrites sqlText's ":name" placeholders into PostgreSQL's
// native "$n" bound-parameter syntax and returns the ordered args to
// execute it with. It never string-interpolates a value into the statement.
func (Provider) BindParams(
	sqlText string,
	values map[string]any,
) (string, []any, error) {
	return dialect.BindParams(true, param, sqlText, values)
}
