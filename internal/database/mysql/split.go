package mysql

import (
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// Split respects MySQL DELIMITER directives and backtick identifiers.
// Offsets refer to UTF-16 positions in the editor, never Go byte offsets.
func (Provider) Split(
	s string,
) ([]model.StatementRange, []string, error) {
	return dialect.Split(false, s)
}
