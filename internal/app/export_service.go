package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/provider"
	"github.com/yazeed1s/quarro/internal/jobs"
	"github.com/yazeed1s/quarro/internal/sessions"
)

// should it be??
const exportTimeout = 30 * time.Minute

// These are vars, not consts, so tests can shrink them and exercise
// pagination/cap behavior without seeding huge tables.
var (
	exportPageSize = 2000
	// exportRowSanityCap stops a runaway loop; it is not a user-facing
	// limit, a table this large would already take a very long time.
	exportRowSanityCap = 20_000_000
	// exportQueryMaxRows/Bytes bound a "full" query export the same way
	// interactive execution is bounded (see internal/app/queries_service.go):
	// arbitrary SQL has no safe re-pagination without a guaranteed stable
	// order, so a query export is one larger, but still bounded, read.
	exportQueryMaxRows  = 10000
	exportQueryMaxBytes = 64 << 20
)

// ExportRequest describes one export job. Kind selects the source: "table"
// browses like the Data tab does (Scope "filtered" keeps the document's
// current table filter/sort, "full" ignores it); "query" re-runs SQL once,
// bounded the same way interactive execution is.
type ExportRequest struct {
	Kind       string `json:"kind"`
	Format     string `json:"format"`
	Scope      string `json:"scope"`
	Table      string `json:"table"`
	Filter     string `json:"filter"`
	Sort       string `json:"sort"`
	Descending bool   `json:"descending"`
	SQL        string `json:"sql"`
	Delimiter  string `json:"delimiter"`
}

// ExportProgress is polled by the frontend while a job runs; Done is set
// exactly once, alongside Error when the job failed or was cancelled.
type ExportProgress struct {
	Rows      int64  `json:"rows"`
	Bytes     int64  `json:"bytes"`
	Done      bool   `json:"done"`
	Error     string `json:"error"`
	Cancelled bool   `json:"cancelled"`
}

type exportJob struct {
	mu       sync.Mutex
	progress ExportProgress
}

func (j *exportJob) snapshot() ExportProgress {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.progress
}
func (j *exportJob) update(rows int64, bytes int64) {
	j.mu.Lock()
	j.progress.Rows += rows
	j.progress.Bytes = bytes
	j.mu.Unlock()
}
func (j *exportJob) finish(err error, cancelled bool) {
	j.mu.Lock()
	j.progress.Done = true
	j.progress.Cancelled = cancelled
	if err != nil {
		j.progress.Error = err.Error()
	}
	j.mu.Unlock()
}

// ExportService streams table/query results directly to a file the user
// picks through a native save dialog, without ever holding the full result
// set in memory.
// see internal/app/export_writer.go for the per-format writers.
// Progress is polled rather than pushed (this app has no
// Wails-event channel yet) and cancellation reuses the same job registry as
// query execution, so Cancel(jobID) works for either.
type ExportService struct {
	sessions *sessions.Registry
	jobs     *jobs.Registry
	dialogs  SaveFileDialog
	mu       sync.Mutex
	active   map[string]*exportJob
}

func NewExportService(
	sessionRegistry *sessions.Registry,
	jobRegistry *jobs.Registry,
	dialogs SaveFileDialog,
) *ExportService {
	return &ExportService{
		sessions: sessionRegistry,
		jobs:     jobRegistry,
		dialogs:  dialogs,
		active:   map[string]*exportJob{},
	}
}

// ChooseExportDestination opens a native "Save File" dialog. An empty path
// with a nil error means the user cancelled; a non-nil error means dialogs
// are not available at all in this build (e.g. the Playwright test harness's
// server mode). The frontend should show that message rather than retry.
func (s *ExportService) ChooseExportDestination(
	defaultName, filterLabel, filterPattern string,
) (string, error) {
	path, err := s.dialogs.SaveFile(
		defaultName,
		filterLabel,
		filterPattern,
	)
	if err != nil {
		return "", fmt.Errorf(
			"choosing a destination failed: %w",
			err,
		)
	}
	return path, nil
}

// SaveBytes writes content the frontend already rendered (diagram SVG/PNG,
// loaded result rows) to a path chosen with ChooseExportDestination.
func (s *ExportService) SaveBytes(path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("no destination chosen")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("saving file failed: %w", err)
	}
	return nil
}

// StartExport validates the request and destination, then streams rows to
// disk in a background goroutine; poll Progress(jobID) for status and
// Cancel(jobID) to stop it early. It returns as soon as the job is
// registered, not when the export finishes.
func (s *ExportService) StartExport(
	sessionID, documentID, jobID, destination string,
	req ExportRequest,
) error {
	if destination == "" {
		return fmt.Errorf("no export destination was chosen")
	}
	if req.Kind != "table" && req.Kind != "query" {
		return fmt.Errorf("unsupported export source %q", req.Kind)
	}
	if req.Format == "sql" && req.Table == "" {
		return fmt.Errorf(
			"a SQL export needs a target table; export a table document, or a query's Data grid, not an arbitrary join",
		)
	}
	if req.Delimiter == "" {
		req.Delimiter = ","
	}
	job := &exportJob{}
	s.mu.Lock()
	s.active[jobID] = job
	s.mu.Unlock()
	go s.run(sessionID, documentID, jobID, destination, req, job)
	return nil
}

// Progress reports a job's status. A job that has never been started (a
// stale or unknown jobID) reports Done so a poll loop stops cleanly instead
// of retrying forever.
func (s *ExportService) Progress(jobID string) ExportProgress {
	s.mu.Lock()
	job := s.active[jobID]
	s.mu.Unlock()
	if job == nil {
		return ExportProgress{Done: true}
	}
	return job.snapshot()
}

func (s *ExportService) Cancel(jobID string) bool {
	return s.jobs.Cancel(jobID)
}

func (s *ExportService) run(
	sessionID, documentID, jobID, destination string,
	req ExportRequest,
	job *exportJob,
) {
	ctx, done := s.jobs.StartScoped(jobID, exportTimeout)
	defer done()

	file, err := os.Create(destination)
	if err != nil {
		job.finish(
			fmt.Errorf("creating the export file: %w", err),
			false,
		)
		return
	}
	cleanupPartial := true
	defer func() {
		_ = file.Close()
		if cleanupPartial {
			_ = os.Remove(destination)
		}
	}()
	buffered := bufio.NewWriterSize(file, 64<<10)
	counting := &countingWriter{w: buffered}

	err = s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, capErr := requireSQL(raw)
			if capErr != nil {
				return capErr
			}
			profile, profileErr := s.sessions.Profile(sessionID)
			if profileErr != nil {
				return profileErr
			}
			writer, writerErr := newRowWriter(
				req.Format,
				req.Delimiter,
				p.QuoteIdentifier,
				p.FormatLiteral,
				blobLiteralFor(profile.Engine),
				sqlDumpTable(p, req),
				counting,
			)
			if writerErr != nil {
				return writerErr
			}
			if req.Kind == "table" {
				if execErr := s.exportTable(ctx, conn, p, req, writer, counting, job); execErr != nil {
					return execErr
				}
			} else if execErr := s.exportQuery(ctx, conn, p, req, writer, counting, job); execErr != nil {
				return execErr
			}
			return writer.Close()
		},
	)
	if err == nil {
		err = buffered.Flush()
	}
	cancelled := ctx.Err() != nil
	if err != nil {
		job.finish(err, cancelled)
		return
	}
	cleanupPartial = false
	job.finish(nil, false)
}

func sqlDumpTable(
	p provider.IdentifierFormatter,
	req ExportRequest,
) string {
	if req.Table == "" {
		return ""
	}
	qualified, err := p.QualifiedTable(req.Table)
	if err != nil {
		return p.QuoteIdentifier(req.Table)
	}
	return qualified
}

// blobLiteralFor returns this engine's own binary-literal syntax. SQLite and
// MySQL share hex-string literals; PostgreSQL uses its bytea escape syntax.
func blobLiteralFor(engine connections.Engine) func([]byte) string {
	if engine == connections.EnginePostgres {
		return func(raw []byte) string { return "'\\x" + hexString(raw) + "'::bytea" }
	}
	return func(raw []byte) string { return "X'" + hexString(raw) + "'" }
}

func hexString(raw []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(raw)*2)
	for i, b := range raw {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0xf]
	}
	return string(out)
}

func (s *ExportService) exportTable(
	ctx context.Context,
	conn provider.Connection,
	p provider.Browser,
	req ExportRequest,
	writer rowWriter,
	counting *countingWriter,
	job *exportJob,
) error {
	filter := ""
	if req.Scope == "filtered" {
		filter = req.Filter
	}
	offset := 0
	wroteHeader := false
	for offset < exportRowSanityCap {
		if err := ctx.Err(); err != nil {
			return err
		}
		page := p.Browse(
			ctx,
			conn,
			req.Table,
			filter,
			"",
			req.Sort,
			req.Descending,
			offset,
			exportPageSize,
		)
		if page.Kind == "error" {
			if page.Error == "" {
				return fmt.Errorf(
					"export failed while browsing %s",
					req.Table,
				)
			}
			return fmt.Errorf("%s", page.Error)
		}
		if !wroteHeader {
			if err := writer.WriteHeader(page.Columns); err != nil {
				return err
			}
			wroteHeader = true
		}
		for _, row := range page.Rows {
			if err := writer.WriteRow(page.Columns, row); err != nil {
				return err
			}
		}
		job.update(int64(len(page.Rows)), counting.bytes)
		if !page.Truncated {
			return nil
		}
		offset += exportPageSize
	}
	return fmt.Errorf(
		"export stopped after %d rows; narrow the table filter to continue",
		exportRowSanityCap,
	)
}

func (s *ExportService) exportQuery(
	ctx context.Context,
	conn provider.Connection,
	p provider.SQLExecutor,
	req ExportRequest,
	writer rowWriter,
	counting *countingWriter,
	job *exportJob,
) error {
	result := p.Execute(
		ctx,
		conn,
		req.SQL,
		exportQueryMaxRows,
		exportQueryMaxBytes,
	)
	if result.Kind == "error" {
		if result.Error == "" {
			return fmt.Errorf("export failed while running the query")
		}
		return fmt.Errorf("%s", result.Error)
	}
	if result.Kind != "rows" {
		return fmt.Errorf("this statement has no rows to export")
	}
	if err := writer.WriteHeader(result.Columns); err != nil {
		return err
	}
	for _, row := range result.Rows {
		if err := writer.WriteRow(result.Columns, row); err != nil {
			return err
		}
	}
	job.update(int64(len(result.Rows)), counting.bytes)
	if result.Truncated {
		return fmt.Errorf(
			"this query has more than %d rows; query exports are bounded like interactive execution, narrow the SQL (e.g. add a WHERE clause) to export the rest",
			exportQueryMaxRows,
		)
	}
	return nil
}

// countingWriter tracks bytes written so far so ExportProgress.Bytes is
// meaningful without every writer implementation tracking it separately.
type countingWriter struct {
	w     *bufio.Writer
	bytes int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.bytes += int64(n)
	return n, err
}
