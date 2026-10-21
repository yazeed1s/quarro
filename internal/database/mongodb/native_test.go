package mongodb

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yazeed1s/quarro/internal/connections"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestExecuteNativeRunsPipelineAndRejectsWrites confirms the native query
// surface runs a real user-authored aggregation pipeline (not one compiled
// from a VisualQuery AST, unlike Aggregate) and refuses a pipeline whose
// first stage is a write (here, $merge) before it touches the database.
func TestExecuteNativeRunsPipelineAndRejectsWrites(t *testing.T) {
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
		Database: "quarro_native_contract",
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
	coll := hd.db.Collection("orders")
	if _, err := coll.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = hd.db.Drop(cleanupCtx)
	})
	docs := []any{
		bson.M{"status": "active", "total": 10},
		bson.M{"status": "active", "total": 5},
		bson.M{"status": "retired", "total": 3},
	}
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := `{"collection":"orders","pipeline":[{"$match":{"status":"active"}},{"$group":{"_id":"$status","sum":{"$sum":"$total"}}},{"$sort":{"sum":-1}},{"$limit":1}]}`
	result := p.ExecuteNative(ctx, conn, req, 10)
	if result.Kind != "rows" {
		t.Fatalf("pipeline result: %+v", result)
	}
	if len(result.Rows) != 1 || result.Rows[0]["sum"] != int32(15) {
		t.Fatalf("grouped result = %+v, want one row summing to 15", result.Rows)
	}

	writeReq := `{"collection":"orders","pipeline":[{"$merge":{"into":"orders_copy"}}]}`
	rejected := p.ExecuteNative(ctx, conn, writeReq, 10)
	if rejected.Kind != "error" {
		t.Fatalf("expected $merge to be rejected, got: %+v", rejected)
	}
	if count, err := hd.db.Collection("orders_copy").CountDocuments(ctx, bson.D{}); err != nil || count != 0 {
		t.Fatalf("$merge ran despite rejection: count=%d err=%v", count, err)
	}
	outReq := `{"collection":"orders","pipeline":[{"$match":{}},{"$out":"orders_copy"}]}`
	if rejected := p.ExecuteNative(ctx, conn, outReq, 10); rejected.Kind != "error" {
		t.Fatalf("expected $out to be rejected, got: %+v", rejected)
	}

	badReq := `{"collection":"orders","pipeline":"not an array"}`
	if bad := p.ExecuteNative(ctx, conn, badReq, 10); bad.Kind != "error" {
		t.Fatalf("expected invalid pipeline to error, got: %+v", bad)
	}
}
