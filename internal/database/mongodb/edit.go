package mongodb

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/yazeed1s/quarro/internal/database/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// objectIDHex matches sanitize()'s own hex rendering of a real ObjectID
// (browse.go's sanitize, case bson.ObjectID: x.Hex()). A 24-character lower
// -hex string id is treated as that ObjectID, not a literal string, the
// same assumption the rest of this package already makes implicitly by
// using ObjectID as MongoDB's own default _id type; a collection that
// deliberately uses a real 24-hex-character string as its _id (not an
// ObjectID) is the one case this heuristic gets wrong, same limitation
// DocumentEditor's own doc comment states.
var objectIDHex = regexp.MustCompile(`^[0-9a-f]{24}$`)

// idFilterValue decodes id (plain JSON, exactly as Browse's sanitize()
// already hands a document's _id to the frontend) into the real value a
// Mongo filter needs.
func idFilterValue(id string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(id), &value); err != nil {
		return nil, fmt.Errorf("invalid id: %w", err)
	}
	if s, ok := value.(string); ok && objectIDHex.MatchString(s) {
		if oid, err := bson.ObjectIDFromHex(s); err == nil {
			return oid, nil
		}
	}
	return value, nil
}

// FetchDocumentJSON returns one document's relaxed Extended JSON text by
// id, re-reading the raw document rather than the grid's own sanitized
// row: sanitize() is lossy and display-oriented (an ObjectID becomes a
// bare hex string, a Date becomes an RFC3339 string), not something an
// edit should round-trip through.
func (Provider) FetchDocumentJSON(
	ctx context.Context,
	conn any,
	collection, id string,
) (string, error) {
	hd := conn.(*handle)
	value, err := idFilterValue(id)
	if err != nil {
		return "", err
	}
	var raw bson.M
	if err := hd.db.Collection(collection).FindOne(ctx, bson.D{{Key: "_id", Value: value}}).Decode(&raw); err != nil {
		return "", fmt.Errorf("document no longer exists")
	}
	out, err := bson.MarshalExtJSONIndent(raw, false, false, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ApplyDocumentEdit inserts, replaces (by id), or deletes (by id) one
// document. doc is relaxed Extended JSON text for insert/replace.
func (Provider) ApplyDocumentEdit(
	ctx context.Context,
	conn any,
	collection, kind, id, doc string,
) (model.DocumentEditResult, error) {
	hd := conn.(*handle)
	coll := hd.db.Collection(collection)
	switch kind {
	case "insert":
		var value bson.M
		if err := bson.UnmarshalExtJSON([]byte(doc), false, &value); err != nil {
			return model.DocumentEditResult{}, fmt.Errorf("invalid document: %w", err)
		}
		if _, err := coll.InsertOne(ctx, value); err != nil {
			return model.DocumentEditResult{}, err
		}
		return model.DocumentEditResult{Applied: true}, nil
	case "replace":
		idValue, err := idFilterValue(id)
		if err != nil {
			return model.DocumentEditResult{}, err
		}
		var value bson.M
		if err := bson.UnmarshalExtJSON([]byte(doc), false, &value); err != nil {
			return model.DocumentEditResult{}, fmt.Errorf("invalid document: %w", err)
		}
		// _id is immutable once set; replacing it with whatever the edited
		// text happens to contain would either be rejected by the server
		// or silently target a different document than the one being
		// edited, so this path always keeps the original id.
		delete(value, "_id")
		result, err := coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: idValue}}, value)
		if err != nil {
			return model.DocumentEditResult{}, err
		}
		return model.DocumentEditResult{Applied: result.MatchedCount > 0, Matched: result.MatchedCount}, nil
	case "delete":
		idValue, err := idFilterValue(id)
		if err != nil {
			return model.DocumentEditResult{}, err
		}
		result, err := coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: idValue}})
		if err != nil {
			return model.DocumentEditResult{}, err
		}
		return model.DocumentEditResult{Applied: result.DeletedCount > 0, Matched: result.DeletedCount}, nil
	default:
		return model.DocumentEditResult{}, fmt.Errorf("unknown edit kind %q", kind)
	}
}
