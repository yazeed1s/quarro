package sqlite

import "github.com/yazeed1s/quarro/internal/database/dialect"

// Placeholders returns the distinct ":name" parameters in sqlText, in order
// of first appearance, so the frontend can prompt for values before running
// a parameterized statement.
func (Provider) Placeholders(sqlText string) ([]string, error) {
	return dialect.PlaceholderNames(false, sqlText)
}

// BindParams rewrites sqlText's ":name" placeholders into SQLite's native
// "?" bound-parameter syntax and returns the ordered args to execute it
// with. It never string-interpolates a value into the statement.
func (Provider) BindParams(
	sqlText string,
	values map[string]any,
) (string, []any, error) {
	return dialect.BindParams(
		false,
		func(int) string { return "?" },
		sqlText,
		values,
	)
}
