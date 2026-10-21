package provider

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
)

// TestMariaDBExplainContract runs mysql.Provider's Explain against a real
// MariaDB server: ANALYZE FORMAT=JSON, a real JSON tree with r_* actual-
// execution fields, structurally different from MySQL's own EXPLAIN
// ANALYZE text tree, which TestRemoteContracts already covers against a
// real MySQL server. See scripts/test-engine.sh's mariadb mode for the
// disposable container this reads QUARRO_TEST_MARIADB_URL from.
func TestMariaDBExplainContract(t *testing.T) {
	raw := os.Getenv("QUARRO_TEST_MARIADB_URL")
	if raw == "" {
		t.Skip(
			"set QUARRO_TEST_MARIADB_URL to a disposable MariaDB address",
		)
	}
	input, e := ImportURL(raw)
	if e != nil {
		t.Fatal(e)
	}
	p := mustGet(t, connections.EngineMySQL)
	exp, ok := any(p).(Explainer)
	if !ok {
		t.Fatal("mysql.Provider must implement Explainer")
	}
	db, e := p.Open(input)
	if e != nil {
		t.Fatal(e)
	}
	sqlDB := db.(*sql.DB)
	defer sqlDB.Close()
	ctx := context.Background()
	c, e := sqlDB.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()

	run := func(q string) {
		t.Helper()
		if r := p.Execute(ctx, c, q, 10, 4<<20); r.Kind == "error" {
			t.Fatalf("%s: %+v", q, r)
		}
	}
	run("DROP TABLE IF EXISTS quarro_maria_contract")
	defer sqlDB.Exec("DROP TABLE IF EXISTS quarro_maria_contract")
	run(
		"CREATE TABLE quarro_maria_contract (id INT PRIMARY KEY, value VARCHAR(100))",
	)
	run(
		"INSERT INTO quarro_maria_contract VALUES (1,'a'),(2,'b'),(3,'c')",
	)

	q := "SELECT * FROM quarro_maria_contract WHERE value = 'no-index-on-this-column'"
	estimate, e := exp.Explain(ctx, c, q, false, 100)
	if e != nil {
		t.Fatalf("explain (estimate): %v", e)
	}
	if estimate.Engine != "mariadb" {
		t.Fatalf("Engine = %q, want mariadb", estimate.Engine)
	}
	if estimate.Root == nil ||
		!planHasBadge(estimate.Root, "seqScan") {
		t.Fatalf(
			"expected a seqScan badge scanning an unindexed column: %+v",
			estimate.Root,
		)
	}
	if estimate.Root.ActualRows != nil {
		t.Fatalf(
			"plain EXPLAIN must not report actual rows: %+v",
			estimate.Root,
		)
	}

	analyzed, e := exp.Explain(ctx, c, q, true, 100)
	if e != nil {
		t.Fatalf("explain (analyze): %v", e)
	}
	if analyzed.RawFormat != "json" {
		t.Fatalf(
			"MariaDB ANALYZE FORMAT=JSON should report RawFormat=json, got %q",
			analyzed.RawFormat,
		)
	}
	if analyzed.Root == nil || analyzed.Root.ActualRows == nil {
		t.Fatalf(
			"MariaDB ANALYZE FORMAT=JSON must report actual rows: %+v",
			analyzed.Root,
		)
	}
	if analyzed.ExecutionTimeMs == nil {
		t.Fatalf(
			"MariaDB ANALYZE FORMAT=JSON must report an execution time: %+v",
			analyzed,
		)
	}
}
