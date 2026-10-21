package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"), false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestExecuteOneSelect(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (id INTEGER, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (1, 'a'), (2, 'b')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	result := ExecuteOne(
		ctx,
		db,
		`SELECT id, name FROM t ORDER BY id`,
		100,
	)
	if result.Kind != "rows" {
		t.Fatalf(
			"Kind = %q, want rows (error: %s)",
			result.Kind,
			result.Error,
		)
	}
	if len(result.Rows) != 2 {
		t.Fatalf(
			"got %d rows, want 2: %+v",
			len(result.Rows),
			result.Rows,
		)
	}
	if result.Rows[0]["name"] != "a" {
		t.Errorf("row 0 name = %v", result.Rows[0]["name"])
	}
}

func TestExecuteOneInsertReportsAffectedRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	result := ExecuteOne(
		ctx,
		db,
		`INSERT INTO t VALUES (1), (2), (3)`,
		100,
	)
	if result.Kind != "status" {
		t.Fatalf(
			"Kind = %q, want status (error: %s)",
			result.Kind,
			result.Error,
		)
	}
	if result.RowsAffected != 3 {
		t.Errorf("RowsAffected = %d, want 3", result.RowsAffected)
	}
}

func TestExecuteOneSyntaxErrorReportsErrorKind(t *testing.T) {
	db := openTestDB(t)
	result := ExecuteOne(
		context.Background(),
		db,
		`SELEKT nonsense`,
		100,
	)
	if result.Kind != "error" || result.Error == "" {
		t.Fatalf("got %+v, want a populated error result", result)
	}
}

func TestExecuteOneTruncatesAtMaxRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (?)`, i); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	result := ExecuteOne(ctx, db, `SELECT id FROM t ORDER BY id`, 5)
	if len(result.Rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(result.Rows))
	}
	if !result.Truncated {
		t.Error("expected Truncated=true")
	}
}

func TestExecuteOneNotTruncatedWhenExactlyAtLimit(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (?)`, i); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	result := ExecuteOne(ctx, db, `SELECT id FROM t ORDER BY id`, 5)
	if result.Truncated {
		t.Error(
			"expected Truncated=false when row count equals the cap",
		)
	}
}

func TestExecuteOneNullAndBlobAndBigInt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (n INTEGER, b BLOB, big INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (NULL, X'48656c6c6f', 9007199254740993)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	result := ExecuteOne(ctx, db, `SELECT n, b, big FROM t`, 100)
	if result.Kind != "rows" {
		t.Fatalf("Kind = %q, error: %s", result.Kind, result.Error)
	}
	row := result.Rows[0]
	if row["n"] != nil {
		t.Errorf("n = %v, want nil", row["n"])
	}
	blob, ok := row["b"].(BlobValue)
	if !ok || blob.Size != 5 {
		t.Errorf("b = %+v, want a 5-byte BlobValue", row["b"])
	}
	big, ok := row["big"].(string)
	if !ok || big != "9007199254740993" {
		t.Errorf(
			"big = %v (%T), want the string \"9007199254740993\"",
			row["big"],
			row["big"],
		)
	}
}

func TestExecuteOneSmallIntStaysNumeric(t *testing.T) {
	db := openTestDB(t)
	result := ExecuteOne(
		context.Background(),
		db,
		`SELECT 42 AS n`,
		100,
	)
	if result.Kind != "rows" {
		t.Fatalf("Kind = %q, error: %s", result.Kind, result.Error)
	}
	n, ok := result.Rows[0]["n"].(int64)
	if !ok || n != 42 {
		t.Errorf(
			"n = %v (%T), want int64(42)",
			result.Rows[0]["n"],
			result.Rows[0]["n"],
		)
	}
}

func TestReturningCTEAndDuplicateLabels(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`)
	r := ExecuteOne(
		ctx,
		db,
		`INSERT INTO t DEFAULT VALUES RETURNING id`,
		10,
	)
	if r.Kind != "rows" || len(r.Rows) != 1 {
		t.Fatalf("RETURNING: %+v", r)
	}
	r = ExecuteOne(
		ctx,
		db,
		`WITH x AS (SELECT 9) INSERT INTO t SELECT * FROM x`,
		10,
	)
	if r.Kind != "status" || r.RowsAffected != 1 {
		t.Fatalf("CTE write: %+v", r)
	}
	r = ExecuteOne(ctx, db, `SELECT 1 AS x, 2 AS x, 3 AS "x (2)"`, 10)
	if len(r.Rows[0]) != 3 || r.Rows[0]["x"] != int64(1) ||
		r.Rows[0]["x (2)"] != int64(2) {
		t.Fatalf("duplicate labels lost: %+v", r)
	}
}
func TestResultByteLimit(t *testing.T) {
	db := openTestDB(t)
	r := ExecuteOne(
		context.Background(),
		db,
		`SELECT zeroblob(4000000) AS b`,
		10,
	)
	if !r.Truncated || len(r.Rows) != 0 {
		t.Fatalf("unbounded blob result: %+v", r)
	}
}
func TestQuotedFilePathReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test?#mode=rw.db")
	db, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`CREATE TABLE t(id)`)
	db.Close()
	db, err = Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO t VALUES (1)`); err == nil {
		t.Fatal("read-only URI was bypassed")
	}
}
