package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/model"
	"github.com/yazeed1s/quarro/internal/database/sqlite"
	"github.com/yazeed1s/quarro/internal/jobs"
	"github.com/yazeed1s/quarro/internal/sessions"
)

func newTestQueriesService(t *testing.T) (*QueriesService, string) {
	t.Helper()
	registry := sessions.NewRegistry()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "q.db"), false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	registry.Put("session-1", "profile-1", db)
	return NewQueriesService(
		registry,
		jobs.NewRegistry(),
	), "session-1"
}

func TestReadOnlySessionRefusesSQLWritesAndGuardOverrides(t *testing.T) {
	registry := sessions.NewRegistry()
	path := filepath.Join(t.TempDir(), "readonly.db")
	db, err := sqlite.Open(path, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO items VALUES (1, 'safe')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	registry.InstallProvider(
		"readonly-session",
		connections.Profile{ID: "readonly-profile", Engine: connections.EngineSQLite, ReadOnly: true},
		registry.BeginConnect("readonly-session"),
		db,
		sqlite.New(),
	)
	svc := NewQueriesService(registry, jobs.NewRegistry())

	read, err := svc.ExecuteDocument("readonly-session", "doc", "read", `SELECT name FROM items`, 100, 10)
	if err != nil || len(read.Results) != 1 || read.Results[0].Kind != "rows" {
		t.Fatalf("read: results=%+v err=%v", read.Results, err)
	}
	write, err := svc.ExecuteDocument("readonly-session", "doc", "write", `DELETE FROM items`, 100, 10)
	if err != nil || len(write.Results) != 1 || write.Results[0].Kind != "error" || !strings.Contains(strings.ToLower(write.Results[0].Error), "read") {
		t.Fatalf("write: results=%+v err=%v, want database refusal", write.Results, err)
	}
	override, err := svc.ExecuteDocument("readonly-session", "doc", "override", `PRAGMA query_only = OFF`, 100, 10)
	if err != nil || len(override.Results) != 1 || override.Results[0].Kind != "error" || !strings.Contains(override.Results[0].Error, "changing its read-only mode") {
		t.Fatalf("override: results=%+v err=%v, want backend guard refusal", override.Results, err)
	}
	verify, err := svc.ExecuteDocument("readonly-session", "doc", "verify", `SELECT COUNT(*) AS count FROM items`, 100, 10)
	if err != nil || len(verify.Results) != 1 || fmt.Sprint(verify.Results[0].Rows[0]["count"]) != "1" {
		t.Fatalf("verify: results=%+v err=%v", verify.Results, err)
	}
}

func TestExecuteRunsMultipleStatements(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	result, err := svc.Execute(
		sessionID,
		"job-1",
		"CREATE TABLE t (id INTEGER); INSERT INTO t VALUES (1); SELECT id FROM t;",
		100,
		10,
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Results) != 3 {
		t.Fatalf(
			"got %d results, want 3: %+v",
			len(result.Results),
			result.Results,
		)
	}
	if result.Results[0].Kind != "status" ||
		result.Results[1].Kind != "status" {
		t.Errorf(
			"expected DDL/DML as status results: %+v",
			result.Results[:2],
		)
	}
	if result.Results[2].Kind != "rows" ||
		len(result.Results[2].Rows) != 1 {
		t.Errorf(
			"expected one row from the SELECT: %+v",
			result.Results[2],
		)
	}
}

func TestExecuteParameterizedDocumentBindsValues(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	if _, err := svc.ExecuteDocument(sessionID, "doc", "setup",
		"CREATE TABLE users (id INTEGER, name TEXT); INSERT INTO users VALUES (1, 'Ann'), (2, 'Bo');",
		100, 10); err != nil {
		t.Fatalf("setup: %v", err)
	}
	names, err := svc.Placeholders(
		sessionID,
		"SELECT * FROM users WHERE id = :id",
	)
	if err != nil {
		t.Fatalf("Placeholders: %v", err)
	}
	if len(names) != 1 || names[0] != "id" {
		t.Fatalf("names=%v", names)
	}
	result, err := svc.ExecuteParameterizedDocument(
		sessionID,
		"doc",
		"job-param",
		"SELECT * FROM users WHERE id = :id",
		map[string]any{"id": "2"},
		100,
		10,
	)
	if err != nil {
		t.Fatalf("ExecuteParameterizedDocument: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Kind != "rows" ||
		len(result.Results[0].Rows) != 1 {
		t.Fatalf("results=%+v", result.Results)
	}
	if got := result.Results[0].Rows[0]["name"]; got != "Bo" {
		t.Fatalf("name=%v, want Bo", got)
	}
	// The reported statement keeps the original ":id" text for display, even
	// though the database actually ran the bound "?" form.
	if result.Results[0].Statement != "SELECT * FROM users WHERE id = :id" {
		t.Fatalf("statement=%q", result.Results[0].Statement)
	}
}

func TestExecuteParameterizedDocumentMissingValue(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	result, err := svc.ExecuteParameterizedDocument(
		sessionID,
		"doc",
		"job-missing",
		"SELECT :missing",
		map[string]any{},
		100,
		10,
	)
	if err != nil {
		t.Fatalf("ExecuteParameterizedDocument: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Kind != "error" {
		t.Fatalf("expected a bind error, got %+v", result.Results)
	}
}

func TestExecuteStopsAtFirstError(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	result, err := svc.Execute(sessionID, "job-1",
		"CREATE TABLE t (id INTEGER); SELEKT nonsense; SELECT 1;",
		100, 10)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf(
			"got %d results, want 2 (stopped at the error): %+v",
			len(result.Results),
			result.Results,
		)
	}
	if result.Results[1].Kind != "error" {
		t.Errorf(
			"expected the second result to be an error: %+v",
			result.Results[1],
		)
	}
}

func TestExecuteUnknownSessionReturnsError(t *testing.T) {
	svc, _ := newTestQueriesService(t)
	if _, err := svc.Execute("no-such-session", "job-1", "SELECT 1;", 100, 10); err == nil {
		t.Fatal("expected an error for an unknown session")
	}
}

func TestExecuteCancellation(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	jobID := "job-cancel"
	done := make(chan struct{})
	go func() {
		svc.Execute(sessionID, jobID, "SELECT 1;", 100, 30)
		close(done)
	}()
	// Give the goroutine a moment to register the job, then cancel it.
	// This mainly exercises that Cancel doesn't panic/deadlock when raced
	// against a fast-finishing Execute; a slow, cancellable query is
	// exercised at the SplitStatements/jobs.Registry unit level instead.
	time.Sleep(5 * time.Millisecond)
	svc.Cancel(jobID)
	<-done
}

func TestDiscoverSchema(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	if _, err := svc.Execute(sessionID, "job-1", "CREATE TABLE t (id INTEGER);", 100, 10); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	tables, err := svc.DiscoverSchema(sessionID)
	if err != nil {
		t.Fatalf("DiscoverSchema: %v", err)
	}
	if len(tables) != 1 || tables[0].Name != "t" {
		t.Fatalf("got %+v", tables)
	}
}

func TestLazyDiscoveryRejectsUnsupportedProvider(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	if _, err := svc.DiscoverSchemaNames(sessionID); err == nil {
		t.Fatal("SQLite unexpectedly supports names-only schema discovery")
	}
	if _, err := svc.DiscoverTableDetail(sessionID, "main", "t"); err == nil {
		t.Fatal("SQLite unexpectedly supports lazy table detail")
	}
}

func TestConstraintDiscoveryRejectsUnsupportedProviderButDefinitionWorks(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	if _, err := svc.Execute(sessionID, "job-1", "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL);", 100, 10); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// SQLite has no catalog-level named CHECK/UNIQUE constraint concept to
	// query separately from CREATE TABLE's own text (see DiscoverDefinition
	// below), so it does not implement ConstraintDiscoverer at all.
	if _, err := svc.DiscoverConstraints(sessionID, "main", "t"); err == nil {
		t.Fatal("SQLite unexpectedly supports constraint discovery")
	}
	def, err := svc.DiscoverDefinition(sessionID, "main", "t")
	if err != nil {
		t.Fatalf("DiscoverDefinition: %v", err)
	}
	if !def.Verbatim || !strings.Contains(def.SQL, "CREATE TABLE") {
		t.Fatalf("got %+v", def)
	}
	if _, err := svc.DiscoverDefinition(sessionID, "main", "missing"); err == nil {
		t.Fatal("expected an error for a table that does not exist")
	}
}

func TestCompileVisualQuery(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	sql, err := svc.CompileVisualQuery(sessionID, model.VisualQuery{
		Tables: []model.VisualQueryTable{{Object: "t", Alias: "t1"}},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "t1",
					Column: "id",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CompileVisualQuery: %v", err)
	}
	want := `SELECT "t1"."id" FROM "t" AS "t1"`
	if sql != want {
		t.Fatalf("got %q, want %q", sql, want)
	}
}

func TestAggregateVisualQueryMatchesHandWrittenGroupBy(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	if _, err := svc.ExecuteDocument(sessionID, "setup", "setup-aggregate",
		"CREATE TABLE orders (id INTEGER, status TEXT, total REAL); INSERT INTO orders VALUES (1, 'paid', 10.5), (2, 'paid', 4.5), (3, 'open', 7.0);",
		100, 10); err != nil {
		t.Fatalf("setup: %v", err)
	}
	query := model.VisualQuery{
		Tables: []model.VisualQueryTable{
			{Object: "orders", Alias: "o"},
		},
		Columns: []model.VisualQueryColumn{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "o",
					Column: "status",
				},
			},
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "o",
					Column: "id",
				},
				Aggregate: "count",
				Alias:     "n",
			},
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "o",
					Column: "total",
				},
				Aggregate: "sum",
				Alias:     "sum_total",
			},
		},
		GroupBy: []model.VisualQueryRef{
			{Table: "o", Column: "status"},
		},
		OrderBy: []model.VisualQueryOrder{
			{
				VisualQueryRef: model.VisualQueryRef{
					Table:  "o",
					Column: "status",
				},
			},
		},
	}
	agg, err := svc.Aggregate(
		sessionID,
		"aggregate-doc",
		"aggregate-job",
		query,
		100,
		10,
	)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	hand, err := svc.ExecuteDocument(
		sessionID,
		"query-doc",
		"query-job",
		`SELECT status, COUNT(id) AS n, SUM(total) AS sum_total FROM orders GROUP BY status ORDER BY status ASC`,
		100,
		10,
	)
	if err != nil {
		t.Fatalf("ExecuteDocument: %v", err)
	}
	if len(agg.Results) != 1 || len(hand.Results) != 1 ||
		agg.Results[0].Kind != "rows" ||
		hand.Results[0].Kind != "rows" {
		t.Fatalf(
			"unexpected results: agg=%+v hand=%+v",
			agg.Results,
			hand.Results,
		)
	}
	if fmt.Sprint(
		agg.Results[0].Rows,
	) != fmt.Sprint(
		hand.Results[0].Rows,
	) {
		t.Fatalf(
			"aggregate rows differ:\nagg  %+v\nhand %+v",
			agg.Results[0].Rows,
			hand.Results[0].Rows,
		)
	}
	if agg.Results[0].Statement == "" {
		t.Fatal("aggregate result did not keep compiled statement")
	}
}

func TestAggregateLargeTableIgnoresGridLoadCap(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	if _, err := svc.ExecuteDocument(sessionID, "setup-large", "setup-large",
		`CREATE TABLE big_orders (id INTEGER PRIMARY KEY, status TEXT);
WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 15050)
INSERT INTO big_orders SELECT n, CASE WHEN n % 2 = 0 THEN 'paid' ELSE 'open' END FROM seq;`,
		100, 10); err != nil {
		t.Fatalf("setup: %v", err)
	}
	result, err := svc.Aggregate(
		sessionID,
		"chart-large",
		"aggregate-large",
		model.VisualQuery{
			Tables: []model.VisualQueryTable{
				{Object: "big_orders", Alias: "t1"},
			},
			Columns: []model.VisualQueryColumn{
				{
					VisualQueryRef: model.VisualQueryRef{
						Table:  "t1",
						Column: "status",
					},
					Alias: "__x",
				},
				{
					VisualQueryRef: model.VisualQueryRef{
						Table:  "t1",
						Column: "id",
					},
					Aggregate: "count",
					Alias:     "__value",
				},
			},
			GroupBy: []model.VisualQueryRef{
				{Table: "t1", Column: "status"},
			},
			OrderBy: []model.VisualQueryOrder{
				{
					VisualQueryRef: model.VisualQueryRef{
						Table:  "t1",
						Column: "status",
					},
				},
			},
		},
		10,
		10,
	)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Kind != "rows" ||
		len(result.Results[0].Rows) != 2 {
		t.Fatalf("unexpected aggregate result: %+v", result.Results)
	}
	total := int64(0)
	for _, row := range result.Results[0].Rows {
		switch v := row["__value"].(type) {
		case int64:
			total += v
		case int:
			total += int64(v)
		default:
			t.Fatalf("unexpected count type %T in %+v", row["__value"], row)
		}
	}
	if total != 15050 {
		t.Fatalf("total=%d, want 15050 from server aggregate", total)
	}
}

func TestSessionCapabilities(t *testing.T) {
	svc, sessionID := newTestQueriesService(t)
	caps, err := svc.SessionCapabilities(sessionID)
	if err != nil {
		t.Fatalf("SessionCapabilities: %v", err)
	}
	if !caps.SQLExecutor || !caps.QueryCompiler || !caps.Aggregator ||
		!caps.Browser {
		t.Fatalf("missing an expected SQLite capability: %+v", caps)
	}
	if caps.SupportsAnalyze {
		t.Fatalf("SQLite must not report SupportsAnalyze: %+v", caps)
	}
}

func TestCompileVisualQueryUnknownSession(t *testing.T) {
	svc, _ := newTestQueriesService(t)
	if _, err := svc.CompileVisualQuery("no-such-session", model.VisualQuery{}); err == nil {
		t.Fatal("expected an error for an unknown session")
	}
}

func TestSplitStatementsBinding(t *testing.T) {
	svc, _ := newTestQueriesService(t)
	ranges := svc.SplitStatements("SELECT 1; SELECT 2;")
	if len(ranges) != 2 {
		t.Fatalf("got %+v, want 2 ranges", ranges)
	}
}

func TestDocumentIsolationAndTriggerUnicode(t *testing.T) {
	svc, id := newTestQueriesService(t)
	run := func(doc, sql string) ExecuteResult {
		t.Helper()
		r, e := svc.ExecuteDocument(id, doc, "j-"+doc, sql, 100, 10)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	r := run(
		"a",
		`CREATE TEMP TABLE private (id); INSERT INTO private VALUES (1); SELECT * FROM private`,
	)
	if len(r.Results) != 3 || r.Results[2].Kind != "rows" {
		t.Fatalf("temp table affinity: %+v", r)
	}
	r = run("b", `SELECT * FROM private`)
	if r.Results[0].Kind != "error" {
		t.Fatal("document leaked temp table")
	}
	r = run(
		"a",
		`CREATE TABLE t(id); CREATE TABLE audit(value); CREATE TRIGGER audit_t AFTER INSERT ON t BEGIN INSERT INTO audit VALUES ('😀;'); INSERT INTO audit VALUES (CASE WHEN new.id=1 THEN 'yes' ELSE 'no' END); END; INSERT INTO t VALUES(1); SELECT * FROM audit;`,
	)
	if len(r.Results) != 5 || len(r.Results[4].Rows) != 2 {
		t.Fatalf("trigger splitting: %+v", r)
	}
	svc.CloseDocument(id, "a")
	r = run("a", `SELECT * FROM private`)
	if r.Results[0].Kind != "error" {
		t.Fatal("closed document retained temp state")
	}
}
func TestBrowseBoundValuesAndStablePages(t *testing.T) {
	svc, id := newTestQueriesService(t)
	svc.Execute(
		id,
		"setup",
		`CREATE TABLE p(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO p VALUES(1,'100%'),(2,'100%'),(3,'quote''s');`,
		100,
		10,
	)
	r, e := svc.BrowseTable(
		id,
		"table",
		"browse",
		"p",
		"%",
		"",
		"value",
		false,
		0,
		1,
		10,
	)
	if e != nil || len(r.Results[0].Rows) != 1 ||
		!r.Results[0].Truncated ||
		!r.Results[0].StableOrder {
		t.Fatalf("page: %+v %v", r, e)
	}
	r, e = svc.BrowseTable(
		id,
		"table",
		"browse2",
		"p",
		"%",
		"",
		"value",
		false,
		1,
		1,
		10,
	)
	if e != nil || r.Results[0].Rows[0]["id"] != int64(2) {
		t.Fatalf("next page: %+v %v", r, e)
	}
	r, e = svc.BrowseTable(
		id,
		"table",
		"browse3",
		"p",
		"' OR 1=1 --",
		"",
		"value",
		false,
		0,
		10,
		10,
	)
	if e != nil || len(r.Results[0].Rows) != 0 {
		t.Fatalf("filter injection: %+v %v", r, e)
	}
}

// TestBrowseFilterColumnIsExactNotSubstring confirms relationship
// navigation's equality filter ("open related rows") never falls back to
// the free-text substring search: a request for id "1" must not also
// match ids "10" or "21", which it would under the default OR-across-all-
// columns LIKE/instr behavior filterColumn is meant to bypass.
func TestBrowseFilterColumnIsExactNotSubstring(t *testing.T) {
	svc, id := newTestQueriesService(t)
	svc.Execute(
		id,
		"setup",
		`CREATE TABLE p(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO p VALUES(1,'a'),(10,'b'),(21,'c');`,
		100,
		10,
	)
	r, e := svc.BrowseTable(
		id,
		"table",
		"browse",
		"p",
		"1",
		"id",
		"",
		false,
		0,
		10,
		10,
	)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Results[0].Rows) != 1 || r.Results[0].Rows[0]["id"] != int64(1) {
		t.Fatalf("exact filter on id=1 matched more than the one row: %+v", r.Results[0].Rows)
	}
	// A filter column that no longer exists must error, not silently fall
	// back to an unfiltered browse (the same stale-metadata posture the
	// existing sort-column check already takes).
	stale, e := svc.BrowseTable(id, "table", "browse-stale", "p", "1", "missing_column", "", false, 0, 10, 10)
	if e != nil || stale.Results[0].Kind != "error" {
		t.Fatalf("expected an error result for a filter column that no longer exists: %+v %v", stale, e)
	}
}

func TestApplyTableEditsStagesUpdateInsertDelete(t *testing.T) {
	svc, id := newTestQueriesService(t)
	if _, err := svc.Execute(id, "setup", `CREATE TABLE p(id INTEGER PRIMARY KEY, value TEXT, note TEXT DEFAULT 'auto'); INSERT INTO p(id,value,note) VALUES(1,'old','keep'),(2,'delete','bye');`, 100, 10); err != nil {
		t.Fatal(err)
	}
	edits := []model.RowEdit{
		{
			Kind: "update",
			Original: map[string]any{
				"id":    int64(1),
				"value": "old",
				"note":  "keep",
			},
			Values: map[string]model.CellEdit{
				"value": {Mode: "value", Value: "new"},
			},
		},
		{
			Kind: "insert",
			Values: map[string]model.CellEdit{
				"id":    {Mode: "value", Value: int64(3)},
				"value": {Mode: "null"},
				"note":  {Mode: "default"},
			},
		},
		{
			Kind: "delete",
			Original: map[string]any{
				"id":    int64(2),
				"value": "delete",
				"note":  "bye",
			},
			Values: map[string]model.CellEdit{},
		},
	}
	preview, err := svc.PreviewTableEdits(id, "table", "p", edits)
	if err != nil {
		t.Fatalf("PreviewTableEdits: %v", err)
	}
	if len(preview) != 3 || len(preview[0].Args) == 0 ||
		strings.Contains(preview[0].SQL, "new") {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	active, err := svc.TransactionActive(id, "table")
	if err != nil || active {
		t.Fatalf("preview opened a transaction: active=%v err=%v", active, err)
	}
	before, err := svc.BrowseTable(id, "table", "before-preview", "p", "", "", "id", false, 0, 10, 10)
	if err != nil || len(before.Results[0].Rows) != 2 || before.Results[0].Rows[0]["value"] != "old" {
		t.Fatalf("preview changed table data: %+v %v", before, err)
	}

	result, err := svc.ApplyTableEdits(id, "table", "p", edits, 10)
	if err != nil {
		t.Fatalf("ApplyTableEdits: %v", err)
	}
	if result.Applied != 3 || result.RowsAffected != 3 {
		t.Fatalf("unexpected edit result: %+v", result)
	}
	previewSQL := make([]string, len(preview))
	for i := range preview {
		previewSQL[i] = preview[i].SQL
	}
	if !slices.Equal(previewSQL, result.Statements) {
		t.Fatalf("preview SQL drifted from applied SQL: preview=%q applied=%q", previewSQL, result.Statements)
	}
	rows, err := svc.BrowseTable(
		id,
		"table",
		"browse",
		"p",
		"",
		"",
		"id",
		false,
		0,
		10,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}
	got := rows.Results[0].Rows
	if len(got) != 2 || got[0]["value"] != "new" ||
		got[1]["id"] != int64(3) ||
		got[1]["note"] != "auto" {
		t.Fatalf("edits not reflected: %+v", got)
	}
}

func TestApplyTableEditsDetectsConflict(t *testing.T) {
	svc, id := newTestQueriesService(t)
	if _, err := svc.Execute(id, "setup", `CREATE TABLE p(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO p VALUES(1,'old'); UPDATE p SET value='changed' WHERE id=1;`, 100, 10); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ApplyTableEdits(id, "table", "p", []model.RowEdit{{
		Kind:     "update",
		Original: map[string]any{"id": int64(1), "value": "old"},
		Values: map[string]model.CellEdit{
			"value": {Mode: "value", Value: "new"},
		},
	}}, 10)
	if err == nil {
		t.Fatal("expected conflict")
	}
}

func TestDocumentTransactionRollback(t *testing.T) {
	svc, id := newTestQueriesService(t)
	if _, err := svc.Execute(id, "setup", `CREATE TABLE p(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO p VALUES(1,'old');`, 100, 10); err != nil {
		t.Fatal(err)
	}
	if err := svc.BeginTransaction(id, "table"); err != nil {
		t.Fatal(err)
	}
	if active, err := svc.TransactionActive(id, "table"); err != nil ||
		!active {
		t.Fatalf("transaction state: %v %v", active, err)
	}
	if _, err := svc.ApplyTableEdits(id, "table", "p", []model.RowEdit{{
		Kind:     "update",
		Original: map[string]any{"id": int64(1), "value": "old"},
		Values:   map[string]model.CellEdit{"value": {Mode: "value", Value: "new"}},
	}}, 10); err != nil {
		t.Fatal(err)
	}
	if err := svc.RollbackTransaction(id, "table"); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.BrowseTable(
		id,
		"table",
		"browse",
		"p",
		"",
		"",
		"id",
		false,
		0,
		10,
		10,
	)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Results[0].Rows[0]["value"] != "old" {
		t.Fatalf(
			"rollback did not restore row: %+v",
			rows.Results[0].Rows,
		)
	}
}

func TestCancelBeforeExecute(t *testing.T) {
	svc, id := newTestQueriesService(t)
	svc.Cancel("early")
	r, e := svc.ExecuteDocument(
		id,
		"a",
		"early",
		`CREATE TABLE should_not_exist(id)`,
		100,
		10,
	)
	if e == nil &&
		(len(r.Results) == 0 || r.Results[0].Kind != "error") {
		t.Fatalf("cancelled query ran: %+v", r)
	}
}

func TestCancelSlowQueryLeavesDocumentUsable(t *testing.T) {
	svc, id := newTestQueriesService(t)
	done := make(chan error, 1)
	go func() {
		_, err := svc.ExecuteDocument(
			id,
			"slow",
			"slow-job",
			`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n) SELECT sum(x) FROM n`,
			100,
			30,
		)
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for !svc.jobs.Running("slow-job") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	svc.Cancel("slow-job")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("slow query ignored cancellation")
	}
	r, err := svc.ExecuteDocument(
		id,
		"slow",
		"next-job",
		`SELECT 42 AS n`,
		100,
		10,
	)
	if err != nil || len(r.Results) != 1 ||
		r.Results[0].Rows[0]["n"] != int64(42) {
		t.Fatalf("cancel poisoned connection: %+v %v", r, err)
	}
}

func TestEditBatchFailureRollsBackAndPreservesTransaction(
	t *testing.T,
) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			svc, id := newTestQueriesService(t)
			svc.Execute(
				id,
				"setup",
				"CREATE TABLE p(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO p VALUES(1,'old');",
				100,
				10,
			)
			if explicit {
				if err := svc.BeginTransaction(id, "table"); err != nil {
					t.Fatal(err)
				}
				svc.Execute(
					id,
					"table",
					"INSERT INTO p VALUES(2,'prior');",
					100,
					10,
				)
			}
			result, err := svc.ApplyTableEdits(
				id,
				"table",
				"p",
				[]model.RowEdit{
					{
						Kind: "insert",
						Values: map[string]model.CellEdit{
							"id": {Mode: "value", Value: 3},
						},
					},
					{
						Kind: "update",
						Original: map[string]any{
							"id":    int64(1),
							"value": "stale",
						},
						Values: map[string]model.CellEdit{
							"value": {Mode: "value", Value: "wrong"},
						},
					},
				},
				10,
			)
			if err == nil || result.Applied != 0 {
				t.Fatalf(
					"expected rolled-back conflict: %+v %v",
					result,
					err,
				)
			}
			rows, err := svc.BrowseTable(
				id,
				"table",
				"check",
				"p",
				"",
				"",
				"id",
				false,
				0,
				10,
				10,
			)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if explicit {
				want = 2
			}
			if len(rows.Results[0].Rows) != want ||
				rows.Results[0].Rows[0]["value"] != "old" {
				t.Fatalf(
					"batch was not rolled back: %+v",
					rows.Results,
				)
			}
			if explicit {
				active, _ := svc.TransactionActive(id, "table")
				if !active {
					t.Fatal("user transaction was closed")
				}
				if err := svc.RollbackTransaction(id, "table"); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
