package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yazeed1s/quarro/internal/database/sqlite"
	"github.com/yazeed1s/quarro/internal/jobs"
	"github.com/yazeed1s/quarro/internal/sessions"
)

// fakeDialogs is the test double for SaveFileDialog/OpenFileDialog.
// see internal/app/dialogs.go. path/err let a test control exactly what a
// "user's" dialog choice looks like without any real GUI.
type fakeDialogs struct {
	path string
	err  error
}

func (f fakeDialogs) SaveFile(
	defaultName, filterLabel, filterPattern string,
) (string, error) {
	return f.path, f.err
}

func (f fakeDialogs) OpenFile(
	title, filterLabel, filterPattern string,
) (string, error) {
	return f.path, f.err
}

func newTestExportService(
	t *testing.T,
) (*ExportService, string, string, *sql.DB) {
	t.Helper()
	registry := sessions.NewRegistry()
	db, err := sqlite.Open(
		filepath.Join(t.TempDir(), "export.db"),
		false,
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	registry.Put("session-1", "profile-1", db)
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, note TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return NewExportService(
		registry,
		jobs.NewRegistry(),
		fakeDialogs{},
	), "session-1", "doc-1", db
}

func waitForExport(
	t *testing.T,
	s *ExportService,
	jobID string,
) ExportProgress {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p := s.Progress(jobID)
		if p.Done {
			return p
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("export job %s did not finish in time", jobID)
	return ExportProgress{}
}

func TestChooseExportDestinationDelegatesToDialog(t *testing.T) {
	svc := NewExportService(
		sessions.NewRegistry(),
		jobs.NewRegistry(),
		fakeDialogs{path: "/tmp/out.csv"},
	)
	path, err := svc.ChooseExportDestination(
		"out.csv",
		"CSV files",
		"*.csv",
	)
	if err != nil || path != "/tmp/out.csv" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	svc = NewExportService(
		sessions.NewRegistry(),
		jobs.NewRegistry(),
		fakeDialogs{
			err: fmt.Errorf(
				"file dialogs not available in server mode",
			),
		},
	)
	if _, err := svc.ChooseExportDestination("out.csv", "CSV files", "*.csv"); err == nil {
		t.Fatal("expected an error when the dialog itself fails")
	}
}

func TestExportTableCSVPaginatesAllRows(t *testing.T) {
	svc, sessionID, docID, db := newTestExportService(t)
	for i := 1; i <= 25; i++ {
		if _, err := db.Exec(`INSERT INTO items (id, name) VALUES (?, ?)`, i, fmt.Sprintf("item-%d", i)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	old := exportPageSize
	exportPageSize = 10 // force three pages for 25 rows
	defer func() { exportPageSize = old }()

	dest := filepath.Join(t.TempDir(), "items.csv")
	req := ExportRequest{
		Kind:   "table",
		Format: "csv",
		Scope:  "full",
		Table:  "items",
	}
	if err := svc.StartExport(sessionID, docID, "job-csv", dest, req); err != nil {
		t.Fatalf("StartExport: %v", err)
	}
	progress := waitForExport(t, svc, "job-csv")
	if progress.Error != "" {
		t.Fatalf("export failed: %s", progress.Error)
	}
	if progress.Rows != 25 {
		t.Fatalf("progress.Rows = %d, want 25", progress.Rows)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading export: %v", err)
	}
	lines := strings.Split(
		strings.TrimRight(string(data), "\n"),
		"\n",
	)
	if len(lines) != 26 { // header + 25 rows
		t.Fatalf("got %d lines, want 26:\n%s", len(lines), data)
	}
	if lines[0] != "id,name,note" {
		t.Fatalf("header = %q", lines[0])
	}
}

func TestExportTableFilteredScopeOnlyMatchingRows(t *testing.T) {
	svc, sessionID, docID, db := newTestExportService(t)
	for i, name := range []string{"apple", "banana", "apricot"} {
		if _, err := db.Exec(`INSERT INTO items (id, name) VALUES (?, ?)`, i+1, name); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	dest := filepath.Join(t.TempDir(), "filtered.csv")
	req := ExportRequest{
		Kind:   "table",
		Format: "csv",
		Scope:  "filtered",
		Table:  "items",
		Filter: "ap",
	}
	if err := svc.StartExport(sessionID, docID, "job-filtered", dest, req); err != nil {
		t.Fatalf("StartExport: %v", err)
	}
	progress := waitForExport(t, svc, "job-filtered")
	if progress.Error != "" {
		t.Fatalf("export failed: %s", progress.Error)
	}
	if progress.Rows != 2 {
		t.Fatalf(
			"progress.Rows = %d, want 2 (apple, apricot)",
			progress.Rows,
		)
	}
}

func TestExportQueryJSONAndSQL(t *testing.T) {
	svc, sessionID, docID, db := newTestExportService(t)
	if _, err := db.Exec(`INSERT INTO items (id, name, note) VALUES (1, 'Ann', NULL)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	jsonDest := filepath.Join(t.TempDir(), "out.json")
	req := ExportRequest{
		Kind:   "query",
		Format: "json",
		SQL:    "SELECT * FROM items",
	}
	if err := svc.StartExport(sessionID, docID, "job-json", jsonDest, req); err != nil {
		t.Fatalf("StartExport: %v", err)
	}
	if p := waitForExport(t, svc, "job-json"); p.Error != "" {
		t.Fatalf("export failed: %s", p.Error)
	}
	data, err := os.ReadFile(jsonDest)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("invalid JSON export: %v\n%s", err, data)
	}
	if len(rows) != 1 || rows[0]["name"] != "Ann" ||
		rows[0]["note"] != nil {
		t.Fatalf("rows = %+v", rows)
	}

	sqlDest := filepath.Join(t.TempDir(), "out.sql")
	req = ExportRequest{
		Kind:   "query",
		Format: "sql",
		SQL:    "SELECT * FROM items",
		Table:  "items",
	}
	if err := svc.StartExport(sessionID, docID, "job-sql", sqlDest, req); err != nil {
		t.Fatalf("StartExport: %v", err)
	}
	if p := waitForExport(t, svc, "job-sql"); p.Error != "" {
		t.Fatalf("export failed: %s", p.Error)
	}
	sqlData, err := os.ReadFile(sqlDest)
	if err != nil {
		t.Fatal(err)
	}
	dump := string(sqlData)
	if !strings.Contains(dump, `INSERT INTO "items"`) ||
		!strings.Contains(dump, "'Ann'") ||
		!strings.Contains(dump, "NULL") {
		t.Fatalf("unexpected SQL dump:\n%s", dump)
	}
}

func TestExportUnknownJobIsDone(t *testing.T) {
	svc, _, _, _ := newTestExportService(t)
	if p := svc.Progress("no-such-job"); !p.Done {
		t.Fatalf("expected an unknown job to report Done, got %+v", p)
	}
}

func TestExportCancellationStopsAndCleansUp(t *testing.T) {
	svc, sessionID, docID, db := newTestExportService(t)
	for i := 1; i <= 5; i++ {
		if _, err := db.Exec(`INSERT INTO items (id, name) VALUES (?, ?)`, i, fmt.Sprintf("item-%d", i)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	dest := filepath.Join(t.TempDir(), "cancelled.csv")
	req := ExportRequest{
		Kind:   "table",
		Format: "csv",
		Scope:  "full",
		Table:  "items",
	}
	if err := svc.StartExport(sessionID, docID, "job-cancel", dest, req); err != nil {
		t.Fatalf("StartExport: %v", err)
	}
	// jobs.Registry.Cancel records a "pending cancel" if the job hasn't
	// called StartScoped yet, so calling Cancel immediately after
	// StartExport still reliably stops it. See internal/jobs/registry.go.
	svc.Cancel("job-cancel")
	progress := waitForExport(t, svc, "job-cancel")
	if !progress.Cancelled {
		t.Fatalf("expected Cancelled=true, got %+v", progress)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf(
			"expected the partial file to be removed, stat err=%v",
			err,
		)
	}
}

func TestExportRemovesPartialFileOnFailure(t *testing.T) {
	svc, sessionID, docID, _ := newTestExportService(t)
	dest := filepath.Join(t.TempDir(), "missing-table.csv")
	req := ExportRequest{
		Kind:   "table",
		Format: "csv",
		Scope:  "full",
		Table:  "does_not_exist",
	}
	if err := svc.StartExport(sessionID, docID, "job-fail", dest, req); err != nil {
		t.Fatalf("StartExport: %v", err)
	}
	progress := waitForExport(t, svc, "job-fail")
	if progress.Error == "" {
		t.Fatal("expected the export to fail for a missing table")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf(
			"expected the partial file to be removed, stat err=%v",
			err,
		)
	}
}
