package postgres

import (
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// Split respects PostgreSQL dollar quoting. Offsets refer to UTF-16
// positions in the editor, never Go byte offsets.
func (Provider) Split(
	s string,
) ([]model.StatementRange, []string, error) {
	return dialect.Split(true, s)
}
