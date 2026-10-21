package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// Browser pages through one table/view's rows with an optional filter,
// sort, and stable-ordering hint. filterColumn, when non-empty, restricts
// filter to an exact match on that one column (used by relationship
// navigation to open a related table at the row a foreign key actually
// points to); empty keeps the existing free-text substring search across
// every column.
type Browser interface {
	Browse(
		context.Context,
		Connection,
		string,
		string,
		string,
		string,
		bool,
		int,
		int,
	) model.ResultSet
}
