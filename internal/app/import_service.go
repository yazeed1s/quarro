package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
	"github.com/yazeed1s/quarro/internal/database/provider"
	"github.com/yazeed1s/quarro/internal/jobs"
	"github.com/yazeed1s/quarro/internal/sessions"
)

const (
	importTimeout             = 30 * time.Minute
	importPreviewSample       = 50
	importMaxFileBytes        = 200 << 20
	importMaxBatch            = 5000
	importMaxErrors           = 20
	importMaxValidationIssues = 100
)

// importPreviewRowCap and importDefaultBatch are vars, not consts, so tests
// can shrink them and exercise the row cap and multi-batch behavior without
// seeding huge CSV files.
var (
	// importPreviewRowCap bounds how many rows PreviewCSV will read while
	// counting the file, the same way the app avoids an expensive automatic
	// full-table COUNT(*) elsewhere: past this many rows the preview reports
	// "at least N" instead of scanning an arbitrarily large file to the end.
	importPreviewRowCap = 500_000
	importDefaultBatch  = 500
)

// ColumnMapping assigns one CSV column (by index) to one target table
// column. CSV columns with no entry here are not imported and simply take
// the table's own default for every inserted row, the same as leaving a
// column on "Default" when inserting a row by hand (see workspace insertRow).
type ColumnMapping struct {
	CSVColumn int    `json:"csvColumn"`
	Column    string `json:"column"`
}

// ImportOptions configures how the CSV is read and how empty/placeholder
// text maps to SQL NULL.
type ImportOptions struct {
	Delimiter string `json:"delimiter"`
	HasHeader bool   `json:"hasHeader"`
	NullToken string `json:"nullToken"`
	BatchSize int    `json:"batchSize"`
	Encoding  string `json:"encoding"`
}

type CSVPreview struct {
	Columns         []string   `json:"columns"`
	Sample          [][]string `json:"sample"`
	TotalRows       int        `json:"totalRows"`
	TotalRowsCapped bool       `json:"totalRowsCapped"`
}

type ImportValidationIssue struct {
	Row       int    `json:"row"`
	CSVColumn string `json:"csvColumn"`
	Column    string `json:"column"`
	Value     string `json:"value"`
	Expected  string `json:"expected"`
	Message   string `json:"message"`
}

type ImportValidation struct {
	ValidRows        int                     `json:"validRows"`
	InvalidRows      int                     `json:"invalidRows"`
	Issues           []ImportValidationIssue `json:"issues"`
	UncheckedColumns []string                `json:"uncheckedColumns"`
}

func checkImportFile(p string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	if info.Size() > importMaxFileBytes {
		return fmt.Errorf(
			"this file is larger than %d MiB; import it in smaller pieces",
			importMaxFileBytes>>20,
		)
	}
	return nil
}

// ImportProgress is polled while a job runs. Errors carries up to
// importMaxErrors human-readable batch failures ("rows 501-1000: ..."); a
// batch that fails does not stop later batches, so earlier and later
// successful batches remain committed, matching how staged table-edit
// batches already behave (see internal/sessions/registry.go WithAtomicEdits).
type ImportProgress struct {
	Imported   int64    `json:"imported"`
	Failed     int64    `json:"failed"`
	Done       bool     `json:"done"`
	Cancelled  bool     `json:"cancelled"`
	FatalError string   `json:"fatalError"`
	Errors     []string `json:"errors"`
}

type importJob struct {
	mu       sync.Mutex
	progress ImportProgress
}

func (j *importJob) snapshot() ImportProgress {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := j.progress
	out.Errors = append([]string{}, j.progress.Errors...)
	return out
}
func (j *importJob) recordSuccess(n int64) {
	j.mu.Lock()
	j.progress.Imported += n
	j.mu.Unlock()
}
func (j *importJob) recordFailure(n int64, message string) {
	j.mu.Lock()
	j.progress.Failed += n
	if len(j.progress.Errors) < importMaxErrors {
		j.progress.Errors = append(j.progress.Errors, message)
	}
	j.mu.Unlock()
}
func (j *importJob) finish(err error, cancelled bool) {
	j.mu.Lock()
	j.progress.Done = true
	j.progress.Cancelled = cancelled
	if err != nil {
		j.progress.FatalError = err.Error()
	}
	j.mu.Unlock()
}

// ImportService reads a CSV file the user picked through a native open
// dialog and inserts it into an existing table in batches, reusing the same
// provider.ApplyTableEdits + sessions.WithAtomicEdits machinery Phase 7's
// staged table editing already established (savepoint-per-batch, so one bad
// batch rolls back only itself). It requires the target table to have a
// primary key, the same identity requirement table editing already has.
type ImportService struct {
	sessions *sessions.Registry
	jobs     *jobs.Registry
	dialogs  OpenFileDialog
	mu       sync.Mutex
	active   map[string]*importJob
}

func NewImportService(
	sessionRegistry *sessions.Registry,
	jobRegistry *jobs.Registry,
	dialogs OpenFileDialog,
) *ImportService {
	return &ImportService{
		sessions: sessionRegistry,
		jobs:     jobRegistry,
		dialogs:  dialogs,
		active:   map[string]*importJob{},
	}
}

func (s *ImportService) ChooseImportFile() (string, error) {
	path, err := s.dialogs.OpenFile(
		"Choose a CSV file to import",
		"CSV files",
		"*.csv",
	)
	if err != nil {
		return "", fmt.Errorf("choosing a file failed: %w", err)
	}
	return path, nil
}

// PreviewCSV reads path's header (if any) and up to importPreviewRowCap data
// rows, returning a small sample for the frontend's column-mapping UI plus a
// row count. Nothing is imported yet.
func (s *ImportService) PreviewCSV(
	path, delimiter, enc string,
	hasHeader bool,
) (CSVPreview, error) {
	if err := checkImportFile(path); err != nil {
		return CSVPreview{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return CSVPreview{}, err
	}
	defer f.Close()
	dec, err := decodedImportReader(f, enc)
	if err != nil {
		return CSVPreview{}, err
	}
	reader := newCSVReader(dec, delimiter)
	var columns []string
	if hasHeader {
		header, err := reader.Read()
		if err != nil {
			return CSVPreview{}, fmt.Errorf(
				"reading the header row: %w",
				err,
			)
		}
		if err := validateUTF8Record(header); err != nil {
			return CSVPreview{}, fmt.Errorf("reading the header row: %w", err)
		}
		columns = header
	}
	preview := CSVPreview{}
	width := len(columns)
	for preview.TotalRows < importPreviewRowCap {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return CSVPreview{}, fmt.Errorf(
				"reading row %d: %w",
				preview.TotalRows+1,
				err,
			)
		}
		if err := validateUTF8Record(record); err != nil {
			return CSVPreview{}, fmt.Errorf("reading row %d: %w", preview.TotalRows+1, err)
		}
		if len(record) > width {
			width = len(record)
		}
		preview.TotalRows++
		if len(preview.Sample) < importPreviewSample {
			preview.Sample = append(preview.Sample, record)
		}
	}
	if preview.TotalRows == importPreviewRowCap {
		if _, err := reader.Read(); err == nil {
			preview.TotalRowsCapped = true
		}
	}
	if !hasHeader {
		columns = make([]string, width)
		for i := range columns {
			columns[i] = fmt.Sprintf("Column %d", i+1)
		}
	}
	preview.Columns = columns
	return preview, nil
}

func newCSVReader(r io.Reader, sep string) *csv.Reader {
	rd := csv.NewReader(r)
	rd.FieldsPerRecord = -1
	if rs := []rune(sep); len(rs) == 1 {
		rd.Comma = rs[0]
	}
	return rd
}

func (s *ImportService) importTable(ctx context.Context, sid, id string) (model.Table, error) {
	db, raw, err := s.sessions.Resources(sid)
	if err != nil {
		return model.Table{}, err
	}
	if lazy, ok := raw.(provider.LazyDiscoverer); ok {
		sch, tbl, err := dialect.ParseObject(id)
		if err == nil {
			return lazy.DiscoverTable(ctx, db, sch, tbl)
		}
	}
	disc, err := requireDiscoverer(raw)
	if err != nil {
		return model.Table{}, err
	}
	tbls, err := disc.Discover(ctx, db)
	if err != nil {
		return model.Table{}, err
	}
	for _, tbl := range tbls {
		if tbl.Name == id {
			return tbl, nil
		}
	}
	return model.Table{}, fmt.Errorf("table no longer exists")
}

func validateMappings(tbl model.Table, maps []ColumnMapping) (map[string]model.Column, error) {
	cols := make(map[string]model.Column, len(tbl.Columns))
	for _, col := range tbl.Columns {
		cols[col.Name] = col
	}
	srcs := map[int]bool{}
	dsts := map[string]bool{}
	for _, item := range maps {
		if item.CSVColumn < 0 || item.Column == "" {
			return nil, fmt.Errorf("invalid column mapping")
		}
		if srcs[item.CSVColumn] {
			return nil, fmt.Errorf("CSV column %d is mapped more than once", item.CSVColumn+1)
		}
		if dsts[item.Column] {
			return nil, fmt.Errorf("table column %s is mapped more than once", item.Column)
		}
		if _, ok := cols[item.Column]; !ok {
			return nil, fmt.Errorf("column %s no longer exists", item.Column)
		}
		srcs[item.CSVColumn] = true
		dsts[item.Column] = true
	}
	return cols, nil
}

func (s *ImportService) ValidateCSV(
	sessionID, p, tid string,
	maps []ColumnMapping,
	opt ImportOptions,
) (ImportValidation, error) {
	if err := checkImportFile(p); err != nil {
		return ImportValidation{}, err
	}
	ctx, stop := context.WithTimeout(context.Background(), importTimeout)
	defer stop()
	tbl, err := s.importTable(ctx, sessionID, tid)
	if err != nil {
		return ImportValidation{}, err
	}
	cols, err := validateMappings(tbl, maps)
	if err != nil {
		return ImportValidation{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return ImportValidation{}, err
	}
	defer f.Close()
	dec, err := decodedImportReader(f, opt.Encoding)
	if err != nil {
		return ImportValidation{}, err
	}
	rd := newCSVReader(dec, opt.Delimiter)
	hdr := []string{}
	if opt.HasHeader {
		hdr, err = rd.Read()
		if err != nil {
			return ImportValidation{}, fmt.Errorf("reading the header row: %w", err)
		}
		if err := validateUTF8Record(hdr); err != nil {
			return ImportValidation{}, fmt.Errorf("reading the header row: %w", err)
		}
	}
	res := ImportValidation{Issues: []ImportValidationIssue{}, UncheckedColumns: []string{}}
	skip := map[string]bool{}
	row := 0
	for {
		if err := ctx.Err(); err != nil {
			return ImportValidation{}, err
		}
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ImportValidation{}, fmt.Errorf("reading row %d: %w", row+1, err)
		}
		row++
		if err := validateUTF8Record(rec); err != nil {
			return ImportValidation{}, fmt.Errorf("reading row %d: %w", row, err)
		}
		bad := false
		for _, item := range maps {
			if item.CSVColumn >= len(rec) {
				continue
			}
			col := cols[item.Column]
			raw := rec[item.CSVColumn]
			var verr error
			ok := true
			if raw == opt.NullToken {
				if col.NotNull {
					verr = fmt.Errorf("NULL is not allowed")
				}
			} else {
				_, ok, verr = importValue(col, raw)
			}
			if !ok {
				skip[col.Name] = true
			}
			if verr == nil {
				continue
			}
			bad = true
			if len(res.Issues) < importMaxValidationIssues {
				src := fmt.Sprintf("Column %d", item.CSVColumn+1)
				if item.CSVColumn < len(hdr) {
					src = hdr[item.CSVColumn]
				}
				v := raw
				if len(v) > 120 {
					v = v[:119] + "…"
				}
				res.Issues = append(res.Issues, ImportValidationIssue{
					Row: row, CSVColumn: src, Column: col.Name,
					Value: v, Expected: col.Type, Message: verr.Error(),
				})
			}
		}
		if bad {
			res.InvalidRows++
		} else {
			res.ValidRows++
		}
	}
	for _, col := range tbl.Columns {
		if skip[col.Name] {
			res.UncheckedColumns = append(res.UncheckedColumns, col.Name)
		}
	}
	return res, nil
}

// StartImport validates inputs and applies the CSV in the background;
// poll Progress(jobID) and use Cancel(jobID) (shared with ExportService and
// QueriesService, they all cancel through the same job registry) to stop
// it early. It returns as soon as the job is registered.
func (s *ImportService) StartImport(
	sessionID, documentID, jobID, path, table string,
	mapping []ColumnMapping,
	opts ImportOptions,
) error {
	profile, err := s.sessions.Profile(sessionID)
	if err != nil {
		return err
	}
	if profile.ReadOnly {
		return fmt.Errorf("this connection is read-only")
	}
	if len(mapping) == 0 {
		return fmt.Errorf(
			"map at least one CSV column to a table column",
		)
	}
	if opts.BatchSize <= 0 || opts.BatchSize > importMaxBatch {
		opts.BatchSize = importDefaultBatch
	}
	job := &importJob{}
	s.mu.Lock()
	s.active[jobID] = job
	s.mu.Unlock()
	go s.run(
		sessionID,
		documentID,
		jobID,
		path,
		table,
		mapping,
		opts,
		job,
	)
	return nil
}

func (s *ImportService) Progress(jobID string) ImportProgress {
	s.mu.Lock()
	job := s.active[jobID]
	s.mu.Unlock()
	if job == nil {
		return ImportProgress{Done: true}
	}
	return job.snapshot()
}

func (s *ImportService) Cancel(jobID string) bool {
	return s.jobs.Cancel(jobID)
}

func (s *ImportService) run(
	sessionID, documentID, jobID, path, table string,
	mapping []ColumnMapping,
	opts ImportOptions,
	job *importJob,
) {
	ctx, done := s.jobs.StartScoped(jobID, importTimeout)
	defer done()
	if err := checkImportFile(path); err != nil {
		job.finish(err, false)
		return
	}
	tbl, err := s.importTable(ctx, sessionID, table)
	if err != nil {
		job.finish(err, false)
		return
	}
	cols, err := validateMappings(tbl, mapping)
	if err != nil {
		job.finish(err, false)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		job.finish(err, false)
		return
	}
	defer f.Close()
	dec, err := decodedImportReader(f, opts.Encoding)
	if err != nil {
		job.finish(err, false)
		return
	}
	reader := newCSVReader(dec, opts.Delimiter)
	if opts.HasHeader {
		hdr, err := reader.Read()
		if err != nil && err != io.EOF {
			job.finish(
				fmt.Errorf("reading the header row: %w", err),
				false,
			)
			return
		}
		if err := validateUTF8Record(hdr); err != nil {
			job.finish(fmt.Errorf("reading the header row: %w", err), false)
			return
		}
	}

	flush := func(batch []model.RowEdit, from, to int) {
		if len(batch) == 0 {
			return
		}
		err := s.sessions.WithAtomicEdits(
			ctx,
			sessionID,
			documentID,
			func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
				p, capErr := requireSQL(raw)
				if capErr != nil {
					return capErr
				}
				_, applyErr := p.ApplyTableEdits(
					ctx,
					conn,
					table,
					batch,
				)
				return applyErr
			},
		)
		if err != nil {
			job.recordFailure(
				int64(len(batch)),
				fmt.Sprintf("rows %d-%d: %v", from, to, err),
			)
		} else {
			job.recordSuccess(int64(len(batch)))
		}
	}

	batch := make([]model.RowEdit, 0, opts.BatchSize)
	rowNum := 0
	batchStart := 1
	for {
		if err := ctx.Err(); err != nil {
			break
		}
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			job.finish(
				fmt.Errorf("reading row %d: %w", rowNum+1, err),
				false,
			)
			return
		}
		rowNum++
		if err := validateUTF8Record(record); err != nil {
			job.finish(fmt.Errorf("reading row %d: %w", rowNum, err), false)
			return
		}
		values := map[string]model.CellEdit{}
		ok := true
		for _, m := range mapping {
			if m.Column == "" || m.CSVColumn < 0 ||
				m.CSVColumn >= len(record) {
				continue
			}
			raw := record[m.CSVColumn]
			col := cols[m.Column]
			if raw == opts.NullToken {
				if col.NotNull {
					job.recordFailure(1, fmt.Sprintf("row %d, column %s: NULL is not allowed", rowNum, col.Name))
					ok = false
					break
				}
				values[m.Column] = model.CellEdit{Mode: "null"}
			} else {
				v, _, err := importValue(col, raw)
				if err != nil {
					job.recordFailure(1, fmt.Sprintf("row %d, column %s: %v", rowNum, col.Name, err))
					ok = false
					break
				}
				values[m.Column] = model.CellEdit{Mode: "value", Value: v}
			}
		}
		if !ok {
			continue
		}
		batch = append(
			batch,
			model.RowEdit{Kind: "insert", Values: values},
		)
		if len(batch) >= opts.BatchSize {
			flush(batch, batchStart, rowNum)
			batch = batch[:0]
			batchStart = rowNum + 1
		}
	}
	flush(batch, batchStart, rowNum)
	job.finish(nil, ctx.Err() != nil)
}
