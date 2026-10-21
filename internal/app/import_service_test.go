package app

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yazeed1s/quarro/internal/database/sqlite"
	"github.com/yazeed1s/quarro/internal/jobs"
	"github.com/yazeed1s/quarro/internal/sessions"
)

func newTestImportService(
	t *testing.T,
) (*ImportService, string, string, *sql.DB) {
	t.Helper()
	registry := sessions.NewRegistry()
	db, err := sqlite.Open(
		filepath.Join(t.TempDir(), "import.db"),
		false,
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	registry.Put("session-1", "profile-1", db)
	if _, err := db.Exec(`CREATE TABLE people (id INTEGER PRIMARY KEY, name TEXT NOT NULL, note TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return NewImportService(
		registry,
		jobs.NewRegistry(),
		fakeDialogs{},
	), "session-1", "doc-1", db
}

func writeCSV(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.csv")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing csv: %v", err)
	}
	return path
}

func writeCSVBytes(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.csv")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatalf("writing csv: %v", err)
	}
	return p
}

func waitForImport(
	t *testing.T,
	s *ImportService,
	jobID string,
) ImportProgress {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p := s.Progress(jobID)
		if p.Done {
			return p
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("import job %s did not finish in time", jobID)
	return ImportProgress{}
}

func TestChooseImportFileDelegatesToDialog(t *testing.T) {
	svc := NewImportService(
		sessions.NewRegistry(),
		jobs.NewRegistry(),
		fakeDialogs{path: "/tmp/in.csv"},
	)
	path, err := svc.ChooseImportFile()
	if err != nil || path != "/tmp/in.csv" {
		t.Fatalf("path=%q err=%v", path, err)
	}
}

func TestPreviewCSVWithHeader(t *testing.T) {
	svc, _, _, _ := newTestImportService(t)
	path := writeCSV(t, "id,name,note\n1,Ann,\n2,Bo,hello\n")
	preview, err := svc.PreviewCSV(path, ",", "auto", true)
	if err != nil {
		t.Fatalf("PreviewCSV: %v", err)
	}
	if len(preview.Columns) != 3 || preview.Columns[1] != "name" {
		t.Fatalf("columns = %v", preview.Columns)
	}
	if preview.TotalRows != 2 || len(preview.Sample) != 2 {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestPreviewCSVWithoutHeaderSynthesizesColumns(t *testing.T) {
	svc, _, _, _ := newTestImportService(t)
	path := writeCSV(t, "1,Ann\n2,Bo\n")
	preview, err := svc.PreviewCSV(path, ",", "auto", false)
	if err != nil {
		t.Fatalf("PreviewCSV: %v", err)
	}
	if len(preview.Columns) != 2 || preview.Columns[0] != "Column 1" {
		t.Fatalf("columns = %v", preview.Columns)
	}
}

func TestPreviewCSVRowCapReportsCapped(t *testing.T) {
	svc, _, _, _ := newTestImportService(t)
	old := importPreviewRowCap
	importPreviewRowCap = 3
	defer func() { importPreviewRowCap = old }()
	path := writeCSV(t, "id\n1\n2\n3\n4\n5\n")
	preview, err := svc.PreviewCSV(path, ",", "auto", true)
	if err != nil {
		t.Fatalf("PreviewCSV: %v", err)
	}
	if preview.TotalRows != 3 || !preview.TotalRowsCapped {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestPreviewCSVDecodesSelectedEncoding(t *testing.T) {
	svc, _, _, _ := newTestImportService(t)
	p := writeCSVBytes(t, []byte("name\nJos\xe9\n"))
	pre, err := svc.PreviewCSV(p, ",", "windows-1252", true)
	if err != nil {
		t.Fatalf("PreviewCSV: %v", err)
	}
	if got := pre.Sample[0][0]; got != "José" {
		t.Fatalf("decoded value = %q", got)
	}
	if _, err := svc.PreviewCSV(p, ",", "utf-8", true); err == nil {
		t.Fatal("expected invalid UTF-8 to require an encoding selection")
	}
}

func TestPreviewCSVAutoDetectsUTF16BOM(t *testing.T) {
	svc, _, _, _ := newTestImportService(t)
	p := writeCSVBytes(t, []byte{0xff, 0xfe, 'n', 0, 'a', 0, 'm', 0, 'e', 0, '\n', 0, 'A', 0, 'n', 0, 'n', 0, '\n', 0})
	pre, err := svc.PreviewCSV(p, ",", "auto", true)
	if err != nil {
		t.Fatalf("PreviewCSV: %v", err)
	}
	if got := pre.Sample[0][0]; got != "Ann" {
		t.Fatalf("decoded value = %q", got)
	}
}

func TestValidateCSVReportsTypedRowsWithoutWriting(t *testing.T) {
	svc, sid, _, db := newTestImportService(t)
	if _, err := db.Exec(`CREATE TABLE typed_import (id INTEGER PRIMARY KEY, enabled BOOLEAN NOT NULL, joined DATE, payload JSON)`); err != nil {
		t.Fatalf("create typed table: %v", err)
	}
	p := writeCSV(t, "id,enabled,joined,payload\n1,true,2026-10-20,true\nbad,perhaps,2026-19-40,not-json\n")
	maps := []ColumnMapping{
		{CSVColumn: 0, Column: "id"},
		{CSVColumn: 1, Column: "enabled"},
		{CSVColumn: 2, Column: "joined"},
		{CSVColumn: 3, Column: "payload"},
	}
	res, err := svc.ValidateCSV(sid, p, "typed_import", maps, ImportOptions{Delimiter: ",", HasHeader: true, Encoding: "auto"})
	if err != nil {
		t.Fatalf("ValidateCSV: %v", err)
	}
	if res.ValidRows != 1 || res.InvalidRows != 1 || len(res.Issues) != 4 {
		t.Fatalf("validation = %+v", res)
	}
	var cnt int
	if err := db.QueryRow(`SELECT count(*) FROM typed_import`).Scan(&cnt); err != nil || cnt != 0 {
		t.Fatalf("validation wrote rows: count=%d err=%v", cnt, err)
	}
}

func TestImportRejectsInvalidTypedRowWithoutPoisoningBatch(t *testing.T) {
	svc, sid, did, db := newTestImportService(t)
	if _, err := db.Exec(`CREATE TABLE typed_import (id INTEGER PRIMARY KEY, enabled BOOLEAN NOT NULL)`); err != nil {
		t.Fatalf("create typed table: %v", err)
	}
	p := writeCSV(t, "id,enabled\n1,true\nbad,false\n2,false\n")
	maps := []ColumnMapping{{CSVColumn: 0, Column: "id"}, {CSVColumn: 1, Column: "enabled"}}
	opt := ImportOptions{Delimiter: ",", HasHeader: true, Encoding: "auto"}
	if err := svc.StartImport(sid, did, "job-types", p, "typed_import", maps, opt); err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	prog := waitForImport(t, svc, "job-types")
	if prog.Imported != 2 || prog.Failed != 1 || len(prog.Errors) != 1 {
		t.Fatalf("progress = %+v", prog)
	}
	var ids, bls int
	if err := db.QueryRow(`SELECT count(*) FILTER (WHERE typeof(id)='integer'), count(*) FILTER (WHERE typeof(enabled)='integer') FROM typed_import`).Scan(&ids, &bls); err != nil {
		t.Fatalf("query typed rows: %v", err)
	}
	if ids != 2 || bls != 2 {
		t.Fatalf("typed values: ids=%d bools=%d", ids, bls)
	}
}

func TestImportAppliesMappedRowsWithNullToken(t *testing.T) {
	svc, sessionID, docID, db := newTestImportService(t)
	path := writeCSV(t, "name,note\nAnn,\nBo,hello\n")
	mapping := []ColumnMapping{
		{CSVColumn: 0, Column: "name"},
		{CSVColumn: 1, Column: "note"},
	}
	opts := ImportOptions{
		Delimiter: ",",
		HasHeader: true,
		NullToken: "",
	}
	if err := svc.StartImport(sessionID, docID, "job-import", path, "people", mapping, opts); err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	progress := waitForImport(t, svc, "job-import")
	if progress.FatalError != "" || len(progress.Errors) != 0 {
		t.Fatalf("progress = %+v", progress)
	}
	if progress.Imported != 2 {
		t.Fatalf("Imported = %d, want 2", progress.Imported)
	}
	var note *string
	if err := db.QueryRow(`SELECT note FROM people WHERE name = 'Ann'`).Scan(&note); err != nil {
		t.Fatalf("query: %v", err)
	}
	if note != nil {
		t.Fatalf("expected Ann's note to be NULL, got %v", *note)
	}
	if err := db.QueryRow(`SELECT note FROM people WHERE name = 'Bo'`).Scan(&note); err != nil {
		t.Fatalf("query: %v", err)
	}
	if note == nil || *note != "hello" {
		t.Fatalf("expected Bo's note to be 'hello', got %v", note)
	}
}

func TestImportContinuesPastAFailingBatch(t *testing.T) {
	svc, sessionID, docID, db := newTestImportService(t)
	old := importDefaultBatch
	importDefaultBatch = 1 // one row per batch, so only the bad row's batch fails
	defer func() { importDefaultBatch = old }()
	// name is NOT NULL; the middle row maps an empty (null-token) value to
	// it, so its batch must fail while the first and third rows still
	// import. A second, non-empty column keeps the middle row from being a
	// blank line, which encoding/csv would otherwise skip outright.
	path := writeCSV(t, "name,note\nAnn,\n,has a note\nBo,\n")
	mapping := []ColumnMapping{
		{CSVColumn: 0, Column: "name"},
		{CSVColumn: 1, Column: "note"},
	}
	opts := ImportOptions{
		Delimiter: ",",
		HasHeader: true,
		NullToken: "",
	}
	if err := svc.StartImport(sessionID, docID, "job-partial", path, "people", mapping, opts); err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	progress := waitForImport(t, svc, "job-partial")
	if progress.Imported != 2 {
		t.Fatalf(
			"Imported = %d, want 2 (Ann and Bo)",
			progress.Imported,
		)
	}
	if progress.Failed != 1 || len(progress.Errors) != 1 {
		t.Fatalf("progress = %+v", progress)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM people`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestImportRequiresAtLeastOneMappedColumn(t *testing.T) {
	svc, sessionID, docID, _ := newTestImportService(t)
	path := writeCSV(t, "name\nAnn\n")
	if err := svc.StartImport(sessionID, docID, "job-empty", path, "people", nil, ImportOptions{}); err == nil {
		t.Fatal("expected an error for an empty column mapping")
	}
}

func TestImportUnknownJobIsDone(t *testing.T) {
	svc, _, _, _ := newTestImportService(t)
	if p := svc.Progress("no-such-job"); !p.Done {
		t.Fatalf("expected an unknown job to report Done, got %+v", p)
	}
}
