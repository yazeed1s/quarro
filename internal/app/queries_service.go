package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yazeed1s/quarro/internal/database/capability"
	"github.com/yazeed1s/quarro/internal/database/driver"
	"github.com/yazeed1s/quarro/internal/database/model"
	"github.com/yazeed1s/quarro/internal/database/provider"
	"github.com/yazeed1s/quarro/internal/database/sqlite"
	"github.com/yazeed1s/quarro/internal/jobs"
	"github.com/yazeed1s/quarro/internal/sessions"
)

const (
	defaultMaxRows = 500
	hardMaxRows    = 10000
)

// ExecuteResult is the outcome of running one or more statements.
type ExecuteResult struct {
	Results   []model.ResultSet `json:"results"`
	ElapsedMs int64             `json:"elapsedMs"`
}

// QueriesService is bound to the frontend via Wails: it splits, runs, and
// cancels SQL against a session's live database handle, and discovers
// schema for the object browser and editor completion.
type QueriesService struct {
	sessions *sessions.Registry
	jobs     *jobs.Registry
}

func NewQueriesService(
	sessionRegistry *sessions.Registry,
	jobRegistry *jobs.Registry,
) *QueriesService {
	return &QueriesService{
		sessions: sessionRegistry,
		jobs:     jobRegistry,
	}
}

// SplitStatements exposes the statement splitter so the frontend can find
// "the statement under the cursor" without reimplementing SQL-aware
// splitting in TypeScript.
func (s *QueriesService) SplitStatements(
	sqlText string,
) []model.StatementRange {
	return sqlite.SplitStatements(sqlText)
}

// Execute runs every statement found in sqlText (by SplitStatements)
// against sessionID's database, in order, stopping at the first statement
// that errors. jobID lets a concurrent Cancel call stop it mid-flight.
func (s *QueriesService) Execute(
	sessionID, jobID, sqlText string,
	maxRows, timeoutSeconds int,
) (ExecuteResult, error) {
	return s.ExecuteDocument(
		sessionID,
		"legacy",
		jobID,
		sqlText,
		maxRows,
		timeoutSeconds,
	)
}

func (s *QueriesService) ExecuteDocument(
	sessionID, documentID, jobID, sqlText string,
	maxRows, timeoutSeconds int,
) (ExecuteResult, error) {
	return s.executeDocument(
		sessionID,
		documentID,
		jobID,
		sqlText,
		nil,
		maxRows,
		timeoutSeconds,
	)
}

// ExecuteParameterizedDocument runs sqlText the same way ExecuteDocument
// does, but first binds every ":name" placeholder in each split statement to
// params through the provider (see internal/database/provider/params.go).
// Values are always sent as driver args, never interpolated into the SQL
// text, so this is safe even for a value the user typed verbatim.
func (s *QueriesService) ExecuteParameterizedDocument(
	sessionID, documentID, jobID, sqlText string,
	params map[string]any,
	maxRows, timeoutSeconds int,
) (ExecuteResult, error) {
	return s.executeDocument(
		sessionID,
		documentID,
		jobID,
		sqlText,
		params,
		maxRows,
		timeoutSeconds,
	)
}

// Placeholders returns the distinct ":name" parameters found in sqlText for
// sessionID's engine, in order of first appearance, so the frontend can
// prompt for values before running a parameterized statement.
func (s *QueriesService) Placeholders(
	sessionID, sqlText string,
) ([]string, error) {
	raw, err := s.sessions.Provider(sessionID)
	if err != nil {
		return nil, err
	}
	p, err := requireSQL(raw)
	if err != nil {
		return nil, err
	}
	return p.Placeholders(sqlText)
}

func (s *QueriesService) executeDocument(
	sessionID, documentID, jobID, sqlText string,
	params map[string]any,
	maxRows, timeoutSeconds int,
) (ExecuteResult, error) {
	if len(sqlText) > 1<<20 {
		return ExecuteResult{}, fmt.Errorf(
			"SQL text exceeds the 1 MiB limit",
		)
	}
	if maxRows <= 0 || maxRows > hardMaxRows {
		maxRows = defaultMaxRows
	}
	if timeoutSeconds <= 0 || timeoutSeconds > 300 {
		timeoutSeconds = 30
	}
	profile, err := s.sessions.Profile(sessionID)
	if err != nil {
		return ExecuteResult{}, err
	}
	ctx, done := s.jobs.StartScoped(
		jobID,
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer done()
	start := time.Now()
	results := []model.ResultSet{}
	err = s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireSQL(raw)
			if err != nil {
				return err
			}
			remaining := sqlite.MaxResultBytes
			_, statements, splitErr := p.Split(sqlText)
			if splitErr != nil {
				return splitErr
			}
			if len(statements) > 100 {
				return fmt.Errorf(
					"a batch may contain at most 100 statements",
				)
			}
			for _, stmt := range statements {
				if remaining <= 0 {
					results = append(
						results,
						model.ResultSet{
							Kind:  "error",
							Error: "Batch result byte limit reached; remaining statements were not executed.",
						},
					)
					break
				}
				if profile.ReadOnly {
					if guardErr := rejectReadOnlyOverride(stmt); guardErr != nil {
						results = append(results, model.ResultSet{
							Kind:      "error",
							Statement: stmt,
							Error:     guardErr.Error(),
						})
						break
					}
				}
				boundStmt, args, bindErr := p.BindParams(stmt, params)
				if bindErr != nil {
					results = append(
						results,
						model.ResultSet{
							Kind:      "error",
							Statement: stmt,
							Error:     bindErr.Error(),
						},
					)
					break
				}
				batch := p.ExecuteResults(
					ctx,
					conn,
					boundStmt,
					maxRows,
					remaining,
					args...)
				failed := false
				for _, result := range batch {
					result.Statement = stmt
					results = append(results, result)
					data, _ := json.Marshal(result)
					remaining -= len(data)
					if result.Kind == "error" {
						failed = true
					}
				}
				if failed {
					break
				}
			}
			return nil
		},
	)
	return ExecuteResult{
		Results:   results,
		ElapsedMs: time.Since(start).Milliseconds(),
	}, err
}

func (s *QueriesService) CloseDocument(sessionID, documentID string) {
	s.sessions.CloseDocument(sessionID, documentID)
}

// Cancel stops an in-flight Execute call for jobID. Returns false if the
// job already finished or was never started.
func (s *QueriesService) Cancel(jobID string) bool {
	return s.jobs.Cancel(jobID)
}

func (s *QueriesService) DiscoverSchema(
	sessionID string,
) ([]model.Table, error) {
	db, raw, err := s.sessions.Resources(sessionID)
	if err != nil {
		return nil, err
	}
	p, err := requireDiscoverer(raw)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()
	return p.Discover(ctx, db)
}

// DiscoverSchemaNames is the cheap object-browser catalog path implemented by
// server SQL engines. It returns every table/view with deliberately empty
// Columns/Indexes/ForeignKeys; whole-schema consumers continue to use
// DiscoverSchema.
func (s *QueriesService) DiscoverSchemaNames(
	sessionID string,
) ([]model.Table, error) {
	db, raw, err := s.sessions.Resources(sessionID)
	if err != nil {
		return nil, err
	}
	p, err := requireLazyDiscoverer(raw)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.DiscoverNames(ctx, db)
}

// DiscoverTableDetail loads current metadata for exactly one table/view.
func (s *QueriesService) DiscoverTableDetail(
	sessionID, schema, table string,
) (model.Table, error) {
	db, raw, err := s.sessions.Resources(sessionID)
	if err != nil {
		return model.Table{}, err
	}
	p, err := requireLazyDiscoverer(raw)
	if err != nil {
		return model.Table{}, err
	}
	if schema == "" || table == "" {
		return model.Table{}, fmt.Errorf("schema and table are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.DiscoverTable(ctx, db, schema, table)
}

// DiscoverRoutines lists stored functions/procedures for sessionID's engine,
// alongside the table/view shape DiscoverSchema already returns. Only
// PostgreSQL and MySQL implement provider.RoutineDiscoverer; any other
// engine's session gets requireRoutineDiscoverer's clear error instead.
func (s *QueriesService) DiscoverRoutines(
	sessionID string,
) ([]model.Function, error) {
	db, raw, err := s.sessions.Resources(sessionID)
	if err != nil {
		return nil, err
	}
	p, err := requireRoutineDiscoverer(raw)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()
	return p.DiscoverRoutines(ctx, db)
}

// DiscoverConstraints lists one table's named CHECK/UNIQUE constraints,
// fetched lazily when a table's Structure view is opened (the same lazy
// per-table pattern DiscoverTableDetail already uses) rather than as part
// of the eager/names-only schema walk. Only PostgreSQL and MySQL implement
// provider.ConstraintDiscoverer.
func (s *QueriesService) DiscoverConstraints(
	sessionID, schema, table string,
) ([]model.Constraint, error) {
	db, raw, err := s.sessions.Resources(sessionID)
	if err != nil {
		return nil, err
	}
	p, err := requireConstraintDiscoverer(raw)
	if err != nil {
		return nil, err
	}
	if schema == "" || table == "" {
		return nil, fmt.Errorf("schema and table are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.DiscoverConstraints(ctx, db, schema, table)
}

// DiscoverDefinition returns one object's engine-native DDL text, fetched
// lazily the same way DiscoverConstraints is. SQLite, PostgreSQL, and
// MySQL all implement provider.DefinitionDiscoverer.
func (s *QueriesService) DiscoverDefinition(
	sessionID, schema, table string,
) (model.Definition, error) {
	db, raw, err := s.sessions.Resources(sessionID)
	if err != nil {
		return model.Definition{}, err
	}
	p, err := requireDefinitionDiscoverer(raw)
	if err != nil {
		return model.Definition{}, err
	}
	if table == "" {
		return model.Definition{}, fmt.Errorf("table is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.DiscoverDefinition(ctx, db, schema, table)
}

// CompileVisualQuery compiles a Visual Query Builder AST into dialect-aware
// SQL text for sessionID's engine. It is a pure, synchronous compile with no
// database access, existence-in-the-database errors (a removed table, stale
// metadata) surface the normal way, through ExecuteDocument, when the user
// runs the generated SQL.
func (s *QueriesService) CompileVisualQuery(
	sessionID string,
	query model.VisualQuery,
) (string, error) {
	raw, err := s.sessions.Provider(sessionID)
	if err != nil {
		return "", err
	}
	p, err := requireSQL(raw)
	if err != nil {
		return "", err
	}
	return p.CompileSelect(query)
}

// SessionCapabilities reports the backend capabilities supported by the
// session's provider. It is safe for the frontend to cache until reconnect,
// because capabilities are tied to the provider instance installed for that
// session. Reading it from the session's driver, rather than reflecting on
// the live provider.Engine value directly, is what also surfaces the
// quirk flags (SupportsAnalyze and friends) that a driver declares rather
// than a Go interface expresses.
func (s *QueriesService) SessionCapabilities(
	sessionID string,
) (capability.Capabilities, error) {
	profile, err := s.sessions.Profile(sessionID)
	if err != nil {
		return capability.Capabilities{}, err
	}
	d, err := driver.Get(string(profile.Engine))
	if err != nil {
		return capability.Capabilities{}, err
	}
	return d.Capabilities(), nil
}

// Aggregate runs a Visual Query AST through the provider's aggregation
// capability. SQL engines compile the AST to their dialect and execute it;
// callers see the same bounded ResultSet shape as a normal query run.
func (s *QueriesService) Aggregate(
	sessionID, documentID, jobID string,
	query model.VisualQuery,
	maxRows, timeoutSeconds int,
) (ExecuteResult, error) {
	if maxRows <= 0 || maxRows > hardMaxRows {
		maxRows = defaultMaxRows
	}
	if timeoutSeconds <= 0 || timeoutSeconds > 300 {
		timeoutSeconds = 30
	}
	ctx, done := s.jobs.StartScoped(
		jobID,
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer done()
	start := time.Now()
	var result model.ResultSet
	err := s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireAggregator(raw)
			if err != nil {
				return err
			}
			result = p.Aggregate(ctx, conn, query, maxRows)
			return nil
		},
	)
	return ExecuteResult{
		Results:   []model.ResultSet{result},
		ElapsedMs: time.Since(start).Milliseconds(),
	}, err
}

// ExecuteNativeDocument runs one engine-native query (req, a small JSON
// envelope the frontend built) directly, the same job-registry/timeout/
// pinned-connection path ExecuteDocument uses for SQL. MongoDB (pipelines)
// and Redis (command lines) implement provider.NativeQueryExecutor; any
// other engine's session gets requireNativeQueryExecutor's clear error.
func (s *QueriesService) ExecuteNativeDocument(
	sessionID, documentID, jobID, req string,
	maxRows, timeoutSeconds int,
) (ExecuteResult, error) {
	if len(req) > 1<<20 {
		return ExecuteResult{}, fmt.Errorf(
			"query text exceeds the 1 MiB limit",
		)
	}
	if maxRows <= 0 || maxRows > hardMaxRows {
		maxRows = defaultMaxRows
	}
	if timeoutSeconds <= 0 || timeoutSeconds > 300 {
		timeoutSeconds = 30
	}
	profile, err := s.sessions.Profile(sessionID)
	if err != nil {
		return ExecuteResult{}, err
	}
	ro := profile.ReadOnly
	ctx, done := s.jobs.StartScoped(
		jobID,
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer done()
	start := time.Now()
	var result model.ResultSet
	err = s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireNativeQueryExecutor(raw)
			if err != nil {
				return err
			}
			// A read-only profile refuses a native write before it runs,
			// for an engine whose native language can write at all.
			if wd, ok := raw.(provider.NativeWriteDetector); ok && ro {
				w, err := wd.NativeQueryWrites(ctx, conn, req)
				if err != nil {
					return err
				}
				if w {
					result = model.ResultSet{
						Kind:  "error",
						Error: "this connection is read-only; that command can modify data",
					}
					return nil
				}
			}
			result = p.ExecuteNative(ctx, conn, req, maxRows)
			return nil
		},
	)
	if err != nil {
		return ExecuteResult{}, err
	}
	return ExecuteResult{
		Results:   []model.ResultSet{result},
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}

// Explain runs EXPLAIN (or, with analyze true, EXPLAIN ANALYZE, which
// actually executes sqlText) against sessionID's database and returns the
// engine-neutral, already-annotated plan. Routed through the same
// job-registry/timeout/pinned-connection path ExecuteDocument uses, since
// EXPLAIN ANALYZE can run for as long as the query itself and must be
// cancellable the same way.
func (s *QueriesService) Explain(
	sessionID, documentID, jobID, sqlText string,
	analyze bool,
	maxRows, timeoutSeconds int,
) (model.ExplainPlan, error) {
	if maxRows <= 0 || maxRows > hardMaxRows {
		maxRows = defaultMaxRows
	}
	if timeoutSeconds <= 0 || timeoutSeconds > 300 {
		timeoutSeconds = 30
	}
	ctx, done := s.jobs.StartScoped(
		jobID,
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer done()
	var plan model.ExplainPlan
	err := s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireExplainer(raw)
			if err != nil {
				return err
			}
			plan, err = p.Explain(
				ctx,
				conn,
				sqlText,
				analyze,
				maxRows,
			)
			return err
		},
	)
	return plan, err
}

func (s *QueriesService) BrowseTable(
	sessionID, documentID, jobID, table, filter, filterColumn, sort string,
	descending bool,
	offset, pageSize, timeoutSeconds int,
) (ExecuteResult, error) {
	if timeoutSeconds <= 0 || timeoutSeconds > 300 {
		timeoutSeconds = 30
	}
	ctx, done := s.jobs.StartScoped(
		jobID,
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer done()
	start := time.Now()
	var result model.ResultSet
	err := s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireBrowser(raw)
			if err != nil {
				return err
			}
			result = p.Browse(
				ctx,
				conn,
				table,
				filter,
				filterColumn,
				sort,
				descending,
				offset,
				pageSize,
			)
			return nil
		},
	)
	return ExecuteResult{
		Results:   []model.ResultSet{result},
		ElapsedMs: time.Since(start).Milliseconds(),
	}, err
}

func (s *QueriesService) ApplyTableEdits(
	sessionID, documentID, table string,
	edits []model.RowEdit,
	timeoutSeconds int,
) (model.EditResult, error) {
	if timeoutSeconds <= 0 || timeoutSeconds > 300 {
		timeoutSeconds = 30
	}
	profile, err := s.sessions.Profile(sessionID)
	if err != nil {
		return model.EditResult{}, err
	}
	if profile.ReadOnly {
		return model.EditResult{}, fmt.Errorf(
			"this connection is read-only",
		)
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer cancel()
	var result model.EditResult
	err = s.sessions.WithAtomicEdits(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireSQL(raw)
			if err != nil {
				return err
			}
			var applyErr error
			result, applyErr = p.ApplyTableEdits(
				ctx,
				conn,
				table,
				edits,
			)
			return applyErr
		},
	)
	if err != nil {
		return model.EditResult{}, err
	}
	return result, nil
}

// PreviewTableEdits resolves current table metadata on the document's pinned
// connection and builds the exact parameterized statements ApplyTableEdits
// would use. It deliberately uses WithExecution rather than WithAtomicEdits:
// previewing opens no transaction and executes no mutation.
func (s *QueriesService) PreviewTableEdits(
	sessionID, documentID, table string,
	edits []model.RowEdit,
) ([]model.EditPreview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result := []model.EditPreview{}
	err := s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireSQL(raw)
			if err != nil {
				return err
			}
			var previewErr error
			result, previewErr = p.PreviewTableEdits(ctx, conn, table, edits)
			return previewErr
		},
	)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// FetchDocumentJSON returns one MongoDB document's Extended JSON text by
// id, on the document's pinned connection (consistent connection affinity
// with ApplyTableEdits, not because Mongo itself needs transactional
// state here). Only MongoDB implements provider.DocumentEditor.
func (s *QueriesService) FetchDocumentJSON(
	sessionID, documentID, collection, id string,
) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var result string
	err := s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireDocumentEditor(raw)
			if err != nil {
				return err
			}
			var fetchErr error
			result, fetchErr = p.FetchDocumentJSON(ctx, conn, collection, id)
			return fetchErr
		},
	)
	if err != nil {
		return "", err
	}
	return result, nil
}

// ApplyDocumentEdit inserts, replaces, or deletes one MongoDB document by
// id. See QueriesService.ApplyTableEdits for the equivalent read-only-
// connection check; document editing enforces the same rule.
func (s *QueriesService) ApplyDocumentEdit(
	sessionID, documentID, collection, kind, id, doc string,
) (model.DocumentEditResult, error) {
	profile, err := s.sessions.Profile(sessionID)
	if err != nil {
		return model.DocumentEditResult{}, err
	}
	if profile.ReadOnly {
		return model.DocumentEditResult{}, fmt.Errorf(
			"this connection is read-only",
		)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var result model.DocumentEditResult
	err = s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireDocumentEditor(raw)
			if err != nil {
				return err
			}
			var applyErr error
			result, applyErr = p.ApplyDocumentEdit(ctx, conn, collection, kind, id, doc)
			return applyErr
		},
	)
	if err != nil {
		return model.DocumentEditResult{}, err
	}
	return result, nil
}

func (s *QueriesService) BeginTransaction(
	sessionID, documentID string,
) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()
	return s.sessions.BeginDocumentTransaction(
		ctx,
		sessionID,
		documentID,
	)
}

func (s *QueriesService) CommitTransaction(
	sessionID, documentID string,
) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()
	return s.sessions.CommitDocumentTransaction(
		ctx,
		sessionID,
		documentID,
	)
}

func (s *QueriesService) RollbackTransaction(
	sessionID, documentID string,
) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()
	return s.sessions.RollbackDocumentTransaction(
		ctx,
		sessionID,
		documentID,
	)
}

func (s *QueriesService) TransactionActive(
	sessionID, documentID string,
) (bool, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()
	return s.sessions.DocumentTransactionActive(
		ctx,
		sessionID,
		documentID,
	)
}

func (s *QueriesService) SplitForSession(
	sessionID, text string,
) ([]model.StatementRange, error) {
	raw, e := s.sessions.Provider(sessionID)
	if e != nil {
		return nil, e
	}
	p, e := requireSQL(raw)
	if e != nil {
		return nil, e
	}
	ranges, _, e := p.Split(text)
	return ranges, e
}

func (s *QueriesService) ListDatabases(
	sessionID string,
) ([]string, error) {
	db, raw, e := s.sessions.Resources(sessionID)
	if e != nil {
		return nil, e
	}
	p, e := requireDiscoverer(raw)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()
	return p.Databases(ctx, db)
}

// InspectKey reads one page of a key's value from a key-value session
// (Redis). cursor is the previous page's KeyDetail.Cursor ("" for the
// first page). It uses the same pinned-connection, cancellable job path as
// BrowseTable, since one large key can take a while to read.
func (s *QueriesService) InspectKey(
	sessionID, documentID, jobID, key, cursor string,
	count, timeoutSeconds int,
) (model.KeyDetail, error) {
	if timeoutSeconds <= 0 || timeoutSeconds > 300 {
		timeoutSeconds = 30
	}
	ctx, done := s.jobs.StartScoped(
		jobID,
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer done()
	var res model.KeyDetail
	err := s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireKeyValue(raw)
			if err != nil {
				return err
			}
			res, err = p.InspectKey(ctx, conn, key, cursor, count)
			return err
		},
	)
	return res, err
}

// ApplyKeyEdit applies one change to one key in a key-value session
// (Redis), refused up front on a read-only connection the same way
// ApplyDocumentEdit is. A conflict (the key no longer matches what the
// user saw) is a normal result with Conflict set, not an error.
func (s *QueriesService) ApplyKeyEdit(
	sessionID, documentID string,
	edit model.KeyEdit,
) (model.KeyEditResult, error) {
	profile, err := s.sessions.Profile(sessionID)
	if err != nil {
		return model.KeyEditResult{}, err
	}
	if profile.ReadOnly {
		return model.KeyEditResult{}, fmt.Errorf(
			"this connection is read-only",
		)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var res model.KeyEditResult
	err = s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireKeyValueEditor(raw)
			if err != nil {
				return err
			}
			res, err = p.ApplyKeyEdit(ctx, conn, edit)
			return err
		},
	)
	if err != nil {
		return model.KeyEditResult{}, err
	}
	return res, nil
}

// KeyValueScan lists key metadata for keys matching prefix (an empty
// prefix matches every key) from a key-value session, up to limit keys.
// group treats prefix as a keyspace group name ("prefix:*" plus the bare
// key) rather than a literal prefix. Unlike
// BrowseTable there is no offset/skip: Redis's SCAN is cursor-based, with no
// stable equivalent of "page 3" the way a SQL table's rows have.
func (s *QueriesService) KeyValueScan(
	sessionID, documentID, jobID, prefix string,
	group bool,
	limit, timeoutSeconds int,
) (ExecuteResult, error) {
	if timeoutSeconds <= 0 || timeoutSeconds > 300 {
		timeoutSeconds = 30
	}
	ctx, done := s.jobs.StartScoped(
		jobID,
		time.Duration(timeoutSeconds)*time.Second,
	)
	defer done()
	start := time.Now()
	var result model.ResultSet
	err := s.sessions.WithExecution(
		ctx,
		sessionID,
		documentID,
		func(ctx context.Context, conn provider.Connection, raw provider.Engine) error {
			p, err := requireKeyValue(raw)
			if err != nil {
				return err
			}
			result = p.Scan(ctx, conn, prefix, group, limit)
			return nil
		},
	)
	return ExecuteResult{
		Results:   []model.ResultSet{result},
		ElapsedMs: time.Since(start).Milliseconds(),
	}, err
}
