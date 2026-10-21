package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// DiscoverDefinition returns one object's verbatim CREATE statement text,
// straight from sqlite_master's own sql column — no reconstruction needed,
// unlike PostgreSQL's table path.
func (Provider) DiscoverDefinition(
	ctx context.Context,
	h any,
	schema, table string,
) (model.Definition, error) {
	db := h.(*sql.DB)
	var sqlText sql.NullString
	err := db.QueryRowContext(
		ctx,
		`SELECT sql FROM sqlite_master WHERE type IN ('table','view') AND name = ?`,
		table,
	).Scan(&sqlText)
	if err == sql.ErrNoRows {
		return model.Definition{}, fmt.Errorf("table or view no longer exists")
	}
	if err != nil {
		return model.Definition{}, err
	}
	// A SQLite internal object (e.g. one backing a virtual table) can have a
	// NULL sql column even though it is listed; report that plainly rather
	// than returning an empty string as if it were the real definition.
	if !sqlText.Valid {
		return model.Definition{}, fmt.Errorf("no definition text is available for this object")
	}
	return model.Definition{SQL: sqlText.String, Verbatim: true}, nil
}
