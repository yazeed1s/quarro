package mongodb

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yazeed1s/quarro/internal/connections"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestDocumentEditRoundTripsBSONTypes is the concrete regression this item
// exists to prevent: editing through a naive JSON round trip would turn a
// real ObjectID/Date field into a plain string. It inserts a document with
// a nested sub-document, an ObjectID reference, a Date, and an array,
// edits one field through FetchDocumentJSON -> ApplyDocumentEdit the same
// way the frontend would, and confirms every other field's real BSON type
// survived untouched.
func TestDocumentEditRoundTripsBSONTypes(t *testing.T) {
	raw := os.Getenv("QUARRO_TEST_MONGO_URL")
	if raw == "" {
		t.Skip("set QUARRO_TEST_MONGO_URL to a disposable MongoDB address")
	}
	hostport := strings.TrimPrefix(raw, "mongodb://")
	host, port, ok := strings.Cut(hostport, ":")
	if !ok {
		host, port = hostport, "27017"
	}

	p := New()
	input := connections.Input{Profile: connections.Profile{
		Engine:   connections.EngineMongoDB,
		Host:     host,
		Port:     port,
		Database: "quarro_edit_contract",
	}}
	h, err := p.Open(input)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close(h)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := p.OpenConnection(ctx, h)
	if err != nil {
		t.Fatalf("OpenConnection: %v", err)
	}
	hd := conn.(*handle)
	coll := hd.db.Collection("profiles")
	if _, err := coll.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = hd.db.Drop(cleanupCtx)
	})

	ref := bson.NewObjectID()
	createdAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	seed := bson.M{
		"name":      "Ada",
		"team":      ref,
		"createdAt": bson.DateTime(createdAt.UnixMilli()),
		"address":   bson.M{"city": "London", "zip": "SW1"},
		"tags":      bson.A{"engineer", "writer"},
	}
	result, err := coll.InsertOne(ctx, seed)
	if err != nil {
		t.Fatalf("seed insert: %v", err)
	}
	oid, ok := result.InsertedID.(bson.ObjectID)
	if !ok {
		t.Fatalf("inserted _id is not an ObjectID: %T", result.InsertedID)
	}
	idJSON, err := json.Marshal(oid.Hex())
	if err != nil {
		t.Fatal(err)
	}

	docJSON, err := p.FetchDocumentJSON(ctx, conn, "profiles", string(idJSON))
	if err != nil {
		t.Fatalf("FetchDocumentJSON: %v", err)
	}
	if !strings.Contains(docJSON, `"$oid"`) || !strings.Contains(docJSON, `"$date"`) {
		t.Fatalf("expected Extended JSON wrappers for ObjectID/Date fields, got: %s", docJSON)
	}

	// Edit exactly one field through plain text manipulation, the same
	// crude thing a user typing into the editor would do, then apply.
	edited := strings.Replace(docJSON, `"Ada"`, `"Ada Lovelace"`, 1)
	applyResult, err := p.ApplyDocumentEdit(ctx, conn, "profiles", "replace", string(idJSON), edited)
	if err != nil {
		t.Fatalf("ApplyDocumentEdit replace: %v", err)
	}
	if !applyResult.Applied || applyResult.Matched != 1 {
		t.Fatalf("replace result = %+v, want applied with matched=1", applyResult)
	}

	var reloaded bson.M
	if err := coll.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&reloaded); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded["name"] != "Ada Lovelace" {
		t.Errorf("edited field did not apply: %+v", reloaded)
	}
	if got, ok := reloaded["team"].(bson.ObjectID); !ok || got != ref {
		t.Errorf("team field lost its ObjectID type across the edit: %#v", reloaded["team"])
	}
	if got, ok := reloaded["createdAt"].(bson.DateTime); !ok || got.Time().UnixMilli() != createdAt.UnixMilli() {
		t.Errorf("createdAt field lost its Date type across the edit: %#v", reloaded["createdAt"])
	}
	if reloaded["_id"] != oid {
		t.Errorf("_id changed across a replace: %#v", reloaded["_id"])
	}
	// A nested sub-document decodes as bson.D (ordered), not bson.M, when
	// nested inside an interface{}-typed parent (see explain.go's own asM
	// helper, reused here, for the same normalization).
	address, ok := asM(reloaded["address"])
	if !ok || address["city"] != "London" {
		t.Errorf("nested sub-document did not survive the edit: %#v", reloaded["address"])
	}

	// Insert.
	insertDoc := `{"name": "Grace", "tags": ["admiral"]}`
	insertResult, err := p.ApplyDocumentEdit(ctx, conn, "profiles", "insert", "", insertDoc)
	if err != nil {
		t.Fatalf("ApplyDocumentEdit insert: %v", err)
	}
	if !insertResult.Applied {
		t.Fatalf("insert result = %+v, want applied", insertResult)
	}
	count, err := coll.CountDocuments(ctx, bson.D{{Key: "name", Value: "Grace"}})
	if err != nil || count != 1 {
		t.Fatalf("inserted document not found: count=%d err=%v", count, err)
	}

	// Delete.
	deleteResult, err := p.ApplyDocumentEdit(ctx, conn, "profiles", "delete", string(idJSON), "")
	if err != nil {
		t.Fatalf("ApplyDocumentEdit delete: %v", err)
	}
	if !deleteResult.Applied || deleteResult.Matched != 1 {
		t.Fatalf("delete result = %+v, want applied with matched=1", deleteResult)
	}
	if err := coll.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Err(); err == nil {
		t.Fatal("document still exists after delete")
	}

	// A delete/replace that no longer matches anything is a conflict, not
	// a silent no-op or a fabricated success.
	conflict, err := p.ApplyDocumentEdit(ctx, conn, "profiles", "delete", string(idJSON), "")
	if err != nil {
		t.Fatalf("ApplyDocumentEdit delete (already gone): %v", err)
	}
	if conflict.Applied || conflict.Matched != 0 {
		t.Fatalf("expected no-match result for an already-deleted document, got %+v", conflict)
	}
}
