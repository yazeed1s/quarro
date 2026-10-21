package mongodb

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestContract runs the same discovery/connect/disconnect/browse shapes
// SQL engines' TestRemoteContracts (internal/database/provider/provider_test.go)
// exercises, against a real local MongoDB, to prove the provider contracts
// generalize instead of being SQL-shaped in disguise. It's skipped unless
// QUARRO_TEST_MONGO_URL is set to a "mongodb://host:port" (or
// "host:port") address, see scripts/test-engine.sh's mongo mode for a
// disposable container that sets it.
func TestContract(t *testing.T) {
	raw := os.Getenv("QUARRO_TEST_MONGO_URL")
	if raw == "" {
		t.Skip(
			"set QUARRO_TEST_MONGO_URL to a disposable MongoDB address",
		)
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
		Database: "quarro_contract",
	}}

	h, err := p.Open(input)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close(h)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	conn, err := p.OpenConnection(ctx, h)
	if err != nil {
		t.Fatalf("OpenConnection: %v", err)
	}
	hd := conn.(*handle)
	coll := hd.db.Collection("widgets")
	if _, err := coll.DeleteMany(ctx, map[string]any{}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()
		_ = hd.db.Drop(cleanupCtx)
	})

	docs := []any{
		map[string]any{
			"name":   "widget-a",
			"status": "active",
			"count":  int32(3),
		},
		map[string]any{
			"name":   "widget-b",
			"status": "retired",
			"count":  int32(1),
		},
		map[string]any{
			"name":   "widget-c",
			"status": "active",
			"count":  int32(9),
		},
	}
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tables, err := p.Discover(ctx, h)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	found := false
	for _, table := range tables {
		if table.ObjectName == "widgets" {
			found = true
			if table.Kind != "collection" {
				t.Fatalf("kind = %q, want collection", table.Kind)
			}
			if len(table.Columns) != 0 {
				t.Fatalf(
					"expected no fictitious SQL columns for a collection, got %+v",
					table.Columns,
				)
			}
		}
	}
	if !found {
		t.Fatal("widgets collection missing from Discover")
	}

	dbs, err := p.Databases(ctx, h)
	if err != nil {
		t.Fatalf("Databases: %v", err)
	}
	if !slicesContain(dbs, "quarro_contract") {
		t.Fatalf("Databases() = %v, want quarro_contract", dbs)
	}

	all := p.Browse(ctx, conn, "widgets", "", "", "", false, 0, 10)
	if all.Kind != "rows" || len(all.Rows) != 3 || !all.StableOrder {
		t.Fatalf("browse all: %+v", all)
	}

	filtered := p.Browse(
		ctx,
		conn,
		"widgets",
		`{"status":"active"}`,
		"",
		"count",
		true,
		0,
		10,
	)
	if filtered.Kind != "rows" || len(filtered.Rows) != 2 {
		t.Fatalf("browse filtered: %+v", filtered)
	}
	if filtered.Rows[0]["name"] != "widget-c" {
		t.Fatalf(
			"expected descending count order (widget-c first), got %+v",
			filtered.Rows,
		)
	}

	paged := p.Browse(ctx, conn, "widgets", "", "", "count", false, 0, 1)
	if paged.Kind != "rows" || len(paged.Rows) != 1 ||
		!paged.Truncated {
		t.Fatalf("browse paged: %+v", paged)
	}

	if bad := p.Browse(ctx, conn, "widgets", "{not json", "", "", false, 0, 10); bad.Kind != "error" {
		t.Fatalf("invalid filter accepted: %+v", bad)
	}

	testAggregate(t, ctx, p, conn)
	testExplain(t, ctx, p, conn)
}

// testAggregate exercises provider.Aggregator against the same seeded
// "widgets" collection TestContract already set up (3 documents, 2 active/1
// retired, counts 3/1/9), proving a grouped/aggregated VisualQuery produces
// the same shape of result a SQL engine's Aggregate would over equivalent
// data.
func testAggregate(
	t *testing.T,
	ctx context.Context,
	p Provider,
	conn any,
) {
	t.Helper()

	grouped := p.Aggregate(ctx, conn, model.VisualQuery{
		Tables: []model.VisualQueryTable{
			{Object: "widgets", Alias: "w"},
		},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "w",
					Column: "status",
				},
				Alias: "status",
			},
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "w",
					Column: "count",
				},
				Aggregate: "sum",
				Alias:     "total",
			},
		},
		GroupBy: []model.VisualQueryRef{
			{Table: "w", Column: "status"},
		},
		OrderBy: []model.VisualQueryOrder{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "w",
					Column: "status",
				},
				Aggregate:  "",
				Descending: false,
			},
		},
	}, 100)
	if grouped.Kind != "rows" {
		t.Fatalf("aggregate group by status: %+v", grouped)
	}
	totals := map[string]int64{}
	for _, row := range grouped.Rows {
		status, _ := row["status"].(string)
		switch v := row["total"].(type) {
		case int32:
			totals[status] = int64(v)
		case int64:
			totals[status] = v
		case float64:
			totals[status] = int64(v)
		default:
			t.Fatalf("unexpected total type %T in row %+v", row["total"], row)
		}
	}
	if totals["active"] != 12 || totals["retired"] != 1 {
		t.Fatalf(
			"grouped totals = %+v, want active=12 retired=1",
			totals,
		)
	}

	count := p.Aggregate(ctx, conn, model.VisualQuery{
		Tables: []model.VisualQueryTable{
			{Object: "widgets", Alias: "w"},
		},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "w",
					Column: "name",
				},
				Aggregate: "count",
				Alias:     "n",
			},
		},
		Filters: []model.VisualQueryFilter{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "w",
					Column: "status",
				},
				Operator: "eq",
				Value:    "active",
			},
		},
	}, 100)
	if count.Kind != "rows" || len(count.Rows) != 1 {
		t.Fatalf("aggregate filtered count: %+v", count)
	}
	if n, _ := count.Rows[0]["n"].(int32); n != 2 {
		if n64, ok := count.Rows[0]["n"].(int64); !ok || n64 != 2 {
			t.Fatalf(
				"filtered count = %+v, want 2",
				count.Rows[0]["n"],
			)
		}
	}

	if bad := p.Aggregate(ctx, conn, model.VisualQuery{
		Tables: []model.VisualQueryTable{{Object: "widgets", Alias: "w"}, {Object: "other", Alias: "o"}},
		Columns: []model.VisualQueryColumn{
			{VisualQueryRef: model.VisualQueryRef{Table: "w", Column: "name"}},
		},
	}, 100); bad.Kind != "error" {
		t.Fatalf("multi-table aggregate accepted: %+v", bad)
	}
}

// testExplain exercises provider.Explainer against the same seeded
// "widgets" collection (3 documents, no index on "status"), proving
// MongoDB's own explain command produces the same shape of annotated plan
// a SQL engine's Explain would: a real seqScan badge on the unindexed
// COLLSCAN under a plain estimate, and real actual rows/execution time
// once analyzed.
func testExplain(
	t *testing.T,
	ctx context.Context,
	p Provider,
	conn any,
) {
	t.Helper()

	req := `{"collection":"widgets","filter":{"status":"active"}}`
	estimate, e := p.Explain(ctx, conn, req, false, 100)
	if e != nil {
		t.Fatalf("explain (estimate): %v", e)
	}
	if estimate.Engine != "mongodb" || estimate.Root == nil {
		t.Fatalf("explain (estimate): %+v", estimate)
	}
	if estimate.Root.Operation != "COLLSCAN" ||
		estimate.Root.Relation != "widgets" {
		t.Fatalf(
			"expected a COLLSCAN on widgets (no index on status): %+v",
			estimate.Root,
		)
	}
	found := false
	for _, b := range estimate.Root.Badges {
		if b.Kind == "seqScan" {
			found = true
		}
	}
	if !found {
		t.Fatalf(
			"expected a seqScan badge on an unindexed COLLSCAN: %+v",
			estimate.Root.Badges,
		)
	}
	if estimate.Root.ActualRows != nil {
		t.Fatalf(
			"plain explain must not report actual rows: %+v",
			estimate.Root,
		)
	}

	analyzed, e := p.Explain(ctx, conn, req, true, 100)
	if e != nil {
		t.Fatalf("explain (analyze): %v", e)
	}
	if analyzed.Root == nil || analyzed.Root.ActualRows == nil ||
		*analyzed.Root.ActualRows != 2 {
		t.Fatalf(
			"explain (analyze): expected 2 actual rows, got %+v",
			analyzed.Root,
		)
	}
	if analyzed.ExecutionTimeMs == nil {
		t.Fatalf(
			"explain (analyze): expected an execution time, got %+v",
			analyzed,
		)
	}

	if _, e := p.Explain(ctx, conn, `{"filter":{}}`, false, 100); e == nil {
		t.Fatal(
			"expected an error for a request with no collection name",
		)
	}
}

func slicesContain(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// TestSanitizeHandlesCommonBSONTypes is a fast, no-server unit test for the
// ObjectID/DateTime/binary conversions Browse relies on to make a MongoDB
// document JSON-safe. Every real document has an ObjectID _id, so getting
// this wrong would break browsing every collection, not just an edge case.
func TestSanitizeHandlesCommonBSONTypes(t *testing.T) {
	id := bson.NewObjectID()
	if got := sanitize(id); got != id.Hex() {
		t.Fatalf(
			"ObjectID: got %#v, want hex string %q",
			got,
			id.Hex(),
		)
	}

	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := sanitize(bson.NewDateTimeFromTime(when)); got != "2026-01-02T03:04:05Z" {
		t.Fatalf("DateTime: got %#v", got)
	}

	blob := sanitize(bson.Binary{Data: []byte("hi")})
	want := model.BlobValue{
		Blob:   true,
		Size:   2,
		Base64: base64.StdEncoding.EncodeToString([]byte("hi")),
	}
	if blob != want {
		t.Fatalf("Binary: got %#v, want %#v", blob, want)
	}

	nested := sanitize(bson.M{"id": id, "tags": bson.A{"a", id}})
	m, ok := nested.(map[string]any)
	if !ok {
		t.Fatalf(
			"nested document did not sanitize to a map: %#v",
			nested,
		)
	}
	if m["id"] != id.Hex() {
		t.Fatalf("nested ObjectID not sanitized: %#v", m["id"])
	}
	tags, ok := m["tags"].([]any)
	if !ok || len(tags) != 2 || tags[1] != id.Hex() {
		t.Fatalf("nested array not sanitized: %#v", m["tags"])
	}

	if got := sanitize("plain string"); got != "plain string" {
		t.Fatalf(
			"unrelated types must pass through unchanged, got %#v",
			got,
		)
	}
}
