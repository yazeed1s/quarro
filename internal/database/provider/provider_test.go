package provider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

// mustGet is for the SQL-dialect tests in this file: it asserts the
// returned engine is a full SQLProvider, which every engine Get can
// construct today except MongoDB is (see TestMongoDoesNotImplementSQL for
// the boundary this asserts).
func mustGet(t *testing.T, engine connections.Engine) Provider {
	t.Helper()
	raw, e := Get(engine)
	if e != nil {
		t.Fatal(e)
	}
	p, ok := raw.(Provider)
	if !ok {
		t.Fatalf("%s does not implement the full SQL surface", engine)
	}
	return p
}

func TestSplitDialects(t *testing.T) {
	cases := []struct {
		engine connections.Engine
		sql    string
		count  int
	}{
		{
			connections.EnginePostgres,
			"-- 😀\nSELECT 'a;'; DO $tag$ BEGIN RAISE NOTICE 'x;'; END $tag$; SELECT 3",
			3,
		},
		{
			connections.EnginePostgres,
			`SELECT E'a\';b'; /* outer /* inner ; */ ok */ SELECT 2`,
			2,
		},
		{
			connections.EngineMySQL,
			"DELIMITER $$\nCREATE PROCEDURE p() BEGIN SELECT 'x;'; SELECT 2; END$$\nDELIMITER ;\nSELECT 3;",
			2,
		},
		{
			connections.EngineMySQL,
			"SELECT `a;b`; # comment ;\n SELECT 'x;';",
			2,
		},
	}
	for _, c := range cases {
		p := mustGet(t, c.engine)
		ranges, parts, e := p.Split(c.sql)
		if e != nil || len(parts) != c.count ||
			len(ranges) != c.count {
			t.Fatalf("%s: %v %#v", c.engine, e, parts)
		}
	}
	if _, _, e := mustGet(t, connections.EnginePostgres).Split("SELECT $x$missing"); e == nil {
		t.Fatal("unterminated quote accepted")
	}
}
func TestURLValidation(t *testing.T) {
	i, e := ImportURL(
		"postgres://alice:p%40ss@localhost:5433/acme?sslmode=verify-full&schema=analytics",
	)
	if e != nil || i.Password != "p@ss" || i.Port != "5433" ||
		i.Schema != "analytics" {
		t.Fatalf("%+v %v", i, e)
	}
	for _, u := range []string{"postgres://u:p@host/db?sslmode=prefer", "mysql://u:p@host/db?tls=skip-verify", "mysql://u:p@host:99999/db", "mysql://u:p@host/db?unknown=yes"} {
		if _, e := ImportURL(u); e == nil {
			t.Fatalf("accepted %s", u)
		}
	}
}
func TestTLSVerification(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Quarro test"},
		DNSNames:              []string{"db.example.test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}
	der, e := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&key.PublicKey,
		key,
	)
	if e != nil {
		t.Fatal(e)
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(
		ca,
		pem.EncodeToMemory(
			&pem.Block{Type: "CERTIFICATE", Bytes: der},
		),
		0600,
	)
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
	listener, e := tls.Listen(
		"tcp",
		"127.0.0.1:0",
		&tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		},
	)
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	go func() {
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); c.(*tls.Conn).Handshake() }()
		}
	}()
	for _, tc := range []struct {
		name, ca string
		ok       bool
	}{{"db.example.test", ca, true}, {"wrong.example.test", ca, false}, {"db.example.test", "", false}} {
		cfg, e := dialect.TLSConfig(
			connections.Profile{Host: tc.name, TLSCA: tc.ca},
		)
		if e != nil {
			t.Fatal(e)
		}
		c, e := tls.Dial("tcp", listener.Addr().String(), cfg)
		if c != nil {
			c.Close()
		}
		if (e == nil) != tc.ok {
			t.Fatalf("TLS %s: %v", tc.name, e)
		}
	}
}
func TestRemoteContracts(t *testing.T) {
	for _, engine := range []connections.Engine{connections.EnginePostgres, connections.EngineMySQL} {
		t.Run(string(engine), func(t *testing.T) {
			raw := os.Getenv(
				"QUARRO_TEST_" + strings.ToUpper(
					string(engine),
				) + "_URL",
			)
			if raw == "" {
				t.Skip("set isolated integration database URL")
			}
			input, e := ImportURL(raw)
			if e != nil {
				t.Fatal(e)
			}
			p := mustGet(t, engine)
			isPG := engine == connections.EnginePostgres
			if input.TLSMode == "verify-full" {
				wrong := input
				wrong.TLSServerName = "wrong.invalid"
				if db, e := p.Open(wrong); e == nil {
					_ = p.Close(db)
					t.Fatal("hostname mismatch accepted")
				}
				untrusted := input
				untrusted.TLSCA = ""
				if db, e := p.Open(untrusted); e == nil {
					_ = p.Close(db)
					t.Fatal("untrusted certificate accepted")
				}
			}
			if socket := os.Getenv("QUARRO_TEST_" + strings.ToUpper(string(engine)) + "_SOCKET"); socket != "" {
				local := input
				local.Socket = socket
				if isPG {
					local.Port = "5432"
				}
				local.TLSMode = "disable"
				local.TLSCA = ""
				socketDB, e := p.Open(local)
				if e != nil {
					t.Fatal(e)
				}
				_ = p.Close(socketDB)
			}
			db, e := p.Open(input)
			if e != nil {
				t.Fatal(e)
			}
			// The test manipulates the raw connection directly below (a
			// second connection to prove temp-table scoping, deliberate
			// cancellation to prove connection-loss reporting), behavior
			// inherent to database/sql itself, not something Provider's
			// bounded Execute/Browse surface is meant to expose.
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
				r := p.Execute(ctx, c, q, 100, 4<<20)
				if r.Kind == "error" {
					t.Fatalf("%s: %+v", q, r)
				}
			}
			run("DROP TABLE IF EXISTS quarro_child")
			run("DROP TABLE IF EXISTS quarro_contract")
			defer func() {
				sqlDB.Exec("DROP TABLE IF EXISTS quarro_child")
				sqlDB.Exec("DROP TABLE IF EXISTS quarro_contract")
			}()
			run(
				"CREATE TABLE quarro_contract (id INTEGER PRIMARY KEY, value VARCHAR(100), amount DECIMAL(30,10))",
			)
			run(
				"CREATE TABLE quarro_child (id INTEGER PRIMARY KEY, parent INTEGER, FOREIGN KEY(parent) REFERENCES quarro_contract(id))",
			)
			run(
				"INSERT INTO quarro_contract VALUES (1,'100%',12345678901234567890.1234567890),(2,'quote''s',NULL),(3,'100%',0)",
			)
			result := p.Execute(
				ctx,
				c,
				"SELECT amount FROM quarro_contract WHERE id=1",
				10,
				4<<20,
			)
			if result.Kind != "rows" || len(result.Rows) != 1 ||
				result.Rows[0]["amount"] != "12345678901234567890.1234567890" {
				t.Fatalf("numeric precision: %+v", result)
			}
			result = p.Execute(
				ctx,
				c,
				"SELECT 1 AS x,2 AS x",
				10,
				4<<20,
			)
			if len(result.Rows[0]) != 2 {
				t.Fatal("duplicate columns lost")
			}
			result = p.Execute(
				ctx,
				c,
				"SELECT * FROM quarro_contract",
				1,
				4<<20,
			)
			if len(result.Rows) != 1 || !result.Truncated {
				t.Fatalf("row bound: %+v", result)
			}
			schema := input.Database
			if isPG {
				schema = "public"
			}
			query := model.VisualQuery{
				Tables: []model.VisualQueryTable{
					{
						Object: dialect.ObjectID(
							schema,
							"quarro_contract",
						),
						Alias: "c",
					},
				},
				Columns: []model.VisualQueryColumn{
					{
						VisualQueryRef: model.VisualQueryRef{
							Table:  "c",
							Column: "value",
						},
					},
					{
						VisualQueryRef: model.VisualQueryRef{
							Table:  "c",
							Column: "id",
						},
						Aggregate: "count",
						Alias:     "n",
					},
				},
				GroupBy: []model.VisualQueryRef{
					{Table: "c", Column: "value"},
				},
				OrderBy: []model.VisualQueryOrder{
					{
						VisualQueryRef: model.VisualQueryRef{
							Table:  "c",
							Column: "value",
						},
					},
				},
			}
			agg := p.Aggregate(ctx, c, query, 10)
			hand := p.Execute(
				ctx,
				c,
				"SELECT value, COUNT(id) AS n FROM quarro_contract GROUP BY value ORDER BY value ASC",
				10,
				4<<20,
			)
			if agg.Kind != "rows" || hand.Kind != "rows" ||
				fmt.Sprint(agg.Rows) != fmt.Sprint(hand.Rows) {
				t.Fatalf(
					"aggregate mismatch: agg=%+v hand=%+v",
					agg,
					hand,
				)
			}
			result = p.Browse(
				ctx,
				c,
				dialect.ObjectID(schema, "quarro_contract"),
				"%",
				"",
				"value",
				false,
				1,
				1,
			)
			if result.Kind != "rows" || len(result.Rows) != 1 ||
				!result.StableOrder ||
				fmtValue(result.Rows[0]["id"]) != "3" {
				t.Fatalf("browse: %+v", result)
			}
			editBatch := []model.RowEdit{
				{
					Kind: "update",
					Original: map[string]any{
						"id":     int64(2),
						"value":  "quote's",
						"amount": nil,
					},
					Values: map[string]model.CellEdit{
						"value": {Mode: "value", Value: "edited"},
					},
				},
				{
					Kind: "insert",
					Values: map[string]model.CellEdit{
						"id":     {Mode: "value", Value: int64(4)},
						"value":  {Mode: "value", Value: "inserted"},
						"amount": {Mode: "null"},
					},
				},
				{
					Kind: "delete",
					Original: map[string]any{
						"id":     int64(1),
						"value":  "100%",
						"amount": "12345678901234567890.1234567890",
					},
					Values: map[string]model.CellEdit{},
				},
			}
			preview, e := p.PreviewTableEdits(
				ctx,
				c,
				dialect.ObjectID(schema, "quarro_contract"),
				editBatch,
			)
			if e != nil || len(preview) != 3 {
				t.Fatalf("preview: %+v %v", preview, e)
			}
			unchanged := p.Execute(ctx, c, "SELECT id FROM quarro_contract WHERE id=4 OR value='edited'", 10, 4<<20)
			if unchanged.Kind != "rows" || len(unchanged.Rows) != 0 {
				t.Fatalf("preview changed table data: %+v", unchanged)
			}
			edits, e := p.ApplyTableEdits(
				ctx,
				c,
				dialect.ObjectID(schema, "quarro_contract"),
				editBatch,
			)
			if e != nil || edits.Applied != 3 ||
				edits.RowsAffected != 3 {
				t.Fatalf("edit: %+v %v", edits, e)
			}
			previewSQL := make([]string, len(preview))
			for i := range preview {
				previewSQL[i] = preview[i].SQL
			}
			if !slices.Equal(previewSQL, edits.Statements) {
				t.Fatalf("preview SQL drifted from applied SQL: preview=%q applied=%q", previewSQL, edits.Statements)
			}
			if _, e := p.ApplyTableEdits(ctx, c, dialect.ObjectID(schema, "quarro_contract"), []model.RowEdit{{
				Kind:     "update",
				Original: map[string]any{"id": int64(2), "value": "quote's", "amount": nil},
				Values:   map[string]model.CellEdit{"value": {Mode: "value", Value: "stale"}},
			}}); e == nil {
				t.Fatal("stale edit did not report a conflict")
			}
			tables, e := p.Discover(ctx, db)
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, table := range tables {
				if table.ObjectName == "quarro_contract" {
					found = true
					if len(table.Columns) != 3 ||
						!table.Columns[0].PrimaryKey {
						t.Fatalf("metadata: %+v", table)
					}
				}
			}
			if !found {
				t.Fatal("table missing")
			}
			lazy, ok := any(p).(LazyDiscoverer)
			if !ok {
				t.Fatalf("%s.Provider must implement LazyDiscoverer", engine)
			}
			names, e := lazy.DiscoverNames(ctx, db)
			if e != nil {
				t.Fatal(e)
			}
			var summary *model.Table
			for i := range names {
				if names[i].ObjectName == "quarro_contract" {
					summary = &names[i]
					break
				}
			}
			if summary == nil {
				t.Fatal("names-only discovery omitted quarro_contract")
			}
			if len(summary.Columns) != 0 || len(summary.Indexes) != 0 || len(summary.ForeignKeys) != 0 {
				t.Fatalf("names-only discovery returned detail: %+v", summary)
			}
			detail, e := lazy.DiscoverTable(ctx, db, schema, "quarro_contract")
			if e != nil {
				t.Fatal(e)
			}
			if len(detail.Columns) != 3 || !detail.Columns[0].PrimaryKey {
				t.Fatalf("lazy table detail: %+v", detail)
			}
			rd, ok := any(p).(RoutineDiscoverer)
			if !ok {
				t.Fatalf("%s.Provider must implement RoutineDiscoverer", engine)
			}
			if isPG {
				run("DROP FUNCTION IF EXISTS quarro_add(integer, integer)")
				run(
					"CREATE FUNCTION quarro_add(a integer, b integer) RETURNS integer AS $$ BEGIN RETURN a + b; END; $$ LANGUAGE plpgsql",
				)
				run("DROP PROCEDURE IF EXISTS quarro_proc()")
				run(
					"CREATE PROCEDURE quarro_proc() LANGUAGE plpgsql AS $$ BEGIN END; $$",
				)
				defer sqlDB.Exec("DROP FUNCTION IF EXISTS quarro_add(integer, integer)")
				defer sqlDB.Exec("DROP PROCEDURE IF EXISTS quarro_proc()")
			} else {
				run("DROP FUNCTION IF EXISTS quarro_add")
				run(
					"CREATE FUNCTION quarro_add(a INT, b INT) RETURNS INT DETERMINISTIC RETURN a + b",
				)
				run("DROP PROCEDURE IF EXISTS quarro_proc")
				run("CREATE PROCEDURE quarro_proc() BEGIN END")
				defer sqlDB.Exec("DROP FUNCTION IF EXISTS quarro_add")
				defer sqlDB.Exec("DROP PROCEDURE IF EXISTS quarro_proc")
			}
			routines, e := rd.DiscoverRoutines(ctx, db)
			if e != nil {
				t.Fatal(e)
			}
			var fn, proc *model.Function
			for i := range routines {
				switch routines[i].ObjectName {
				case "quarro_add":
					fn = &routines[i]
				case "quarro_proc":
					proc = &routines[i]
				}
			}
			if fn == nil || fn.Kind != "function" {
				t.Fatalf("function missing or misclassified: %+v", routines)
			}
			if proc == nil || proc.Kind != "procedure" {
				t.Fatalf("procedure missing or misclassified: %+v", routines)
			}
			if isPG {
				if fn.Language != "plpgsql" ||
					!strings.Contains(fn.Definition, "a + b") {
					t.Fatalf("function signature/body: %+v", fn)
				}
			}

			constraintDiscoverer, ok := any(p).(ConstraintDiscoverer)
			if !ok {
				t.Fatalf("%s.Provider must implement ConstraintDiscoverer", engine)
			}
			run("DROP TABLE IF EXISTS quarro_constrained")
			defer sqlDB.Exec("DROP TABLE IF EXISTS quarro_constrained")
			run(
				"CREATE TABLE quarro_constrained (id INTEGER PRIMARY KEY, email VARCHAR(100), amount DECIMAL(10,2), CONSTRAINT quarro_email_unique UNIQUE (email), CONSTRAINT quarro_amount_check CHECK (amount >= 0))",
			)
			constraints, e := constraintDiscoverer.DiscoverConstraints(ctx, db, schema, "quarro_constrained")
			if e != nil {
				t.Fatal(e)
			}
			var uniqueC, checkC *model.Constraint
			for i := range constraints {
				switch constraints[i].Name {
				case "quarro_email_unique":
					uniqueC = &constraints[i]
				case "quarro_amount_check":
					checkC = &constraints[i]
				}
			}
			if uniqueC == nil || uniqueC.Kind != "unique" || len(uniqueC.Columns) != 1 || uniqueC.Columns[0] != "email" {
				t.Fatalf("%s unique constraint = %+v", engine, constraints)
			}
			if checkC == nil || checkC.Kind != "check" || !strings.Contains(checkC.Expression, "amount") {
				t.Fatalf("%s check constraint = %+v", engine, constraints)
			}

			definitionDiscoverer, ok := any(p).(DefinitionDiscoverer)
			if !ok {
				t.Fatalf("%s.Provider must implement DefinitionDiscoverer", engine)
			}
			tableDef, e := definitionDiscoverer.DiscoverDefinition(ctx, db, schema, "quarro_constrained")
			if e != nil {
				t.Fatal(e)
			}
			// PostgreSQL has no single built-in function for a table's DDL text,
			// so its definition is Quarro-reconstructed and must say so; MySQL's
			// SHOW CREATE TABLE is the engine's own verbatim text.
			if isPG == tableDef.Verbatim {
				t.Fatalf("%s table definition verbatim=%v, want %v", engine, tableDef.Verbatim, !isPG)
			}
			if !strings.Contains(strings.ToUpper(tableDef.SQL), "CREATE TABLE") {
				t.Fatalf("%s table definition missing CREATE TABLE: %+v", engine, tableDef)
			}

			run("DROP VIEW IF EXISTS quarro_constrained_view")
			defer sqlDB.Exec("DROP VIEW IF EXISTS quarro_constrained_view")
			run("CREATE VIEW quarro_constrained_view AS SELECT id, email FROM quarro_constrained")
			viewDef, e := definitionDiscoverer.DiscoverDefinition(ctx, db, schema, "quarro_constrained_view")
			if e != nil {
				t.Fatal(e)
			}
			// Both engines report a view's own query text verbatim, unlike
			// PostgreSQL's reconstructed table path above.
			if !viewDef.Verbatim {
				t.Fatalf("%s view definition should be verbatim: %+v", engine, viewDef)
			}
			if !strings.Contains(strings.ToUpper(viewDef.SQL), "SELECT") {
				t.Fatalf("%s view definition missing SELECT: %+v", engine, viewDef)
			}

			if isPG {
				exp, ok := any(p).(Explainer)
				if !ok {
					t.Fatal(
						"postgres.Provider must implement Explainer",
					)
				}
				estimate, e := exp.Explain(
					ctx,
					c,
					"SELECT * FROM quarro_contract WHERE value = 'no-index-on-this-column'",
					false,
					100,
				)
				if e != nil {
					t.Fatalf("explain (estimate): %v", e)
				}
				if estimate.Root == nil ||
					estimate.Root.Operation == "" {
					t.Fatalf(
						"explain (estimate): empty plan %+v",
						estimate,
					)
				}
				if estimate.Root.ActualRows != nil {
					t.Fatalf(
						"plain EXPLAIN must not report actual rows: %+v",
						estimate.Root,
					)
				}
				if !planHasBadge(estimate.Root, "seqScan") {
					t.Fatalf(
						"expected a seqScan badge scanning an unindexed column: %+v",
						estimate.Root,
					)
				}
				analyzed, e := exp.Explain(
					ctx,
					c,
					"SELECT * FROM quarro_contract WHERE value = 'no-index-on-this-column'",
					true,
					100,
				)
				if e != nil {
					t.Fatalf("explain (analyze): %v", e)
				}
				if analyzed.Root == nil ||
					analyzed.Root.ActualRows == nil {
					t.Fatalf(
						"EXPLAIN ANALYZE must report actual rows: %+v",
						analyzed.Root,
					)
				}
				if analyzed.ExecutionTimeMs == nil {
					t.Fatalf(
						"EXPLAIN ANALYZE must report an execution time: %+v",
						analyzed,
					)
				}
			}
			if !isPG {
				exp, ok := any(p).(Explainer)
				if !ok {
					t.Fatal("mysql.Provider must implement Explainer")
				}
				estimate, e := exp.Explain(
					ctx,
					c,
					"SELECT * FROM quarro_contract WHERE value = 'no-index-on-this-column'",
					false,
					100,
				)
				if e != nil {
					t.Fatalf("explain (estimate): %v", e)
				}
				if estimate.Engine != "mysql" ||
					estimate.Root == nil {
					t.Fatalf("explain (estimate): %+v", estimate)
				}
				if !planHasBadge(estimate.Root, "seqScan") {
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
				// scripts/test-engine.sh's sql mode provisions MySQL 8.4, well
				// past the 8.0.18 EXPLAIN ANALYZE floor.
				analyzed, e := exp.Explain(
					ctx,
					c,
					"SELECT * FROM quarro_contract WHERE value = 'no-index-on-this-column'",
					true,
					100,
				)
				if e != nil {
					t.Fatalf("explain (analyze): %v", e)
				}
				if analyzed.RawFormat != "text" {
					t.Fatalf(
						"MySQL EXPLAIN ANALYZE should report RawFormat=text, got %q",
						analyzed.RawFormat,
					)
				}
				if analyzed.Root == nil ||
					analyzed.Root.ActualRows == nil {
					t.Fatalf(
						"EXPLAIN ANALYZE must report actual rows: %+v",
						analyzed.Root,
					)
				}
			}
			if !isPG {
				run("DROP PROCEDURE IF EXISTS quarro_results")
				run(
					"CREATE PROCEDURE quarro_results() BEGIN SELECT 1 AS first; SELECT 2 AS second; END",
				)
				outputs := p.ExecuteResults(
					ctx,
					c,
					"CALL quarro_results()",
					10,
					4096,
				)
				if len(outputs) < 2 || len(outputs[0].Rows) != 1 ||
					len(outputs[1].Rows) != 1 {
					t.Fatalf("procedure results: %+v", outputs)
				}
				run("DROP PROCEDURE quarro_results")
			}
			run("CREATE TEMPORARY TABLE private_tab (id INTEGER)")
			other, e := sqlDB.Conn(ctx)
			if e != nil {
				t.Fatal(e)
			}
			r := p.Execute(
				ctx,
				other,
				"SELECT * FROM private_tab",
				10,
				4096,
			)
			other.Close()
			if r.Kind != "error" {
				t.Fatal("temp table leaked")
			}
			q := "SELECT SLEEP(10)"
			if isPG {
				q = "SELECT pg_sleep(10)"
			}
			timed, cancel := context.WithTimeout(
				ctx,
				100*time.Millisecond,
			)
			defer cancel()
			start := time.Now()
			r = p.Execute(timed, c, q, 10, 4096)
			if r.Kind != "error" ||
				time.Since(start) > 3*time.Second {
				t.Fatalf("cancellation: %+v", r)
			}
			if !isPG && !r.ConnectionLost {
				t.Fatal(
					"MySQL cancellation must report connection loss",
				)
			}
			t.Logf("tested %s", engine)
		})
	}
}
func planHasBadge(n *model.ExplainNode, kind string) bool {
	for _, b := range n.Badges {
		if b.Kind == kind {
			return true
		}
	}
	for _, c := range n.Children {
		if planHasBadge(c, kind) {
			return true
		}
	}
	return false
}

func fmtValue(v any) string {
	switch v := v.(type) {
	case int64:
		return big.NewInt(v).String()
	case int32:
		return big.NewInt(int64(v)).String()
	case string:
		return v
	}
	return "unknown"
}
