package capability

import (
	"context"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// Editor applies a batch of staged row edits (insert/update/delete) to one
// table.
type Editor interface {
	ApplyTableEdits(
		context.Context,
		Connection,
		string,
		[]model.RowEdit,
	) (model.EditResult, error)
	PreviewTableEdits(
		context.Context,
		Connection,
		string,
		[]model.RowEdit,
	) ([]model.EditPreview, error)
}

// DocumentEditor applies whole-document insert/replace/delete to one
// MongoDB collection, keyed by _id. Deliberately not Editor: that
// interface's RowEdit/CellEdit shape is a flat column->value map built for
// SQL's fixed-column model, which a schemaless, arbitrarily nested
// document does not fit.
//
// doc is MongoDB Extended JSON text (the same format Browse's own filter
// parsing already uses), not plain JSON — a bare JSON round trip would
// silently turn a real ObjectID/Date/Binary field into a plain string.
//
// id is the document's _id, plain-JSON-encoded exactly as Browse's own
// sanitize() already hands it to the frontend (a quoted hex string for an
// ObjectID, a bare number/string for anything else) — not Extended JSON.
// An ObjectID's real BSON type is lost by that sanitize() step (it becomes
// a bare hex string indistinguishable from a genuine 24-hex-character
// string _id); the implementation re-derives it with the same heuristic
// Browse's own callers already accept for display purposes, documented at
// the implementation.
type DocumentEditor interface {
	// FetchDocumentJSON returns one document's Extended JSON text by id,
	// for a user to edit.
	FetchDocumentJSON(ctx context.Context, h Connection, collection, id string) (string, error)
	// ApplyDocumentEdit inserts, replaces (by id), or deletes (by id) one
	// document. kind is "insert" | "replace" | "delete". doc is Extended
	// JSON text for insert/replace, ignored for delete.
	ApplyDocumentEdit(ctx context.Context, h Connection, collection, kind, id, doc string) (model.DocumentEditResult, error)
}
