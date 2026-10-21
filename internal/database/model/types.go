package model

type Column struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	NotNull      bool    `json:"notNull"`
	PrimaryKey   bool    `json:"primaryKey"`
	DefaultValue *string `json:"defaultValue"`
}

type Index struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

type ForeignKey struct {
	Column    string `json:"column"`
	RefTable  string `json:"refTable"`
	RefColumn string `json:"refColumn"`
	// OnDelete/OnUpdate are the engine's own referential action text
	// ("CASCADE" | "SET NULL" | "RESTRICT" | "NO ACTION" | "SET DEFAULT"),
	// read directly from each engine's own catalog, not guessed at when a
	// CREATE TABLE statement omitted an explicit clause (all three engines
	// report an explicit default rule in that case, same as this one does).
	OnDelete string `json:"onDelete,omitempty"`
	OnUpdate string `json:"onUpdate,omitempty"`
}

// Function is a stored function or procedure. Parameters/ReturnType are
// best-effort, formatted the way the engine's own catalog reports them
// (e.g. PostgreSQL's pg_get_function_arguments text); Quarro does not parse
// them into a structured type system.
type Function struct {
	Name       string `json:"name"`
	Schema     string `json:"schema"`
	ObjectName string `json:"objectName"`
	Kind       string `json:"kind"` // "function" | "procedure"
	Parameters string `json:"parameters,omitempty"`
	ReturnType string `json:"returnType,omitempty"`
	Language   string `json:"language,omitempty"`
	Definition string `json:"definition,omitempty"`
}

// Constraint is a table-level CHECK or named UNIQUE constraint, distinct
// from Index: a named UNIQUE constraint is often backed by a unique index
// Table.Indexes already reports, but a CHECK constraint has no index at
// all, and both deserve their own named identity in a structure/constraint
// view rather than being inferred from Indexes.
type Constraint struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`                 // "check" | "unique"
	Columns    []string `json:"columns,omitempty"`    // unique constraints only
	Expression string   `json:"expression,omitempty"` // check constraints only, engine-formatted text
}

// Definition is one object's CREATE TABLE/CREATE VIEW text. Verbatim
// reports whether SQL is read directly from the engine's own catalog
// (PostgreSQL views, MySQL, SQLite) or assembled by Quarro from other
// discovery data because the engine has no single built-in function for it
// (PostgreSQL tables); a caller must show that distinction to the user
// rather than presenting both as equally authoritative.
type Definition struct {
	SQL      string `json:"sql"`
	Verbatim bool   `json:"verbatim"`
}

type Table struct {
	Name        string       `json:"name"`
	Schema      string       `json:"schema"`
	ObjectName  string       `json:"objectName"`
	Kind        string       `json:"kind"` // "table" | "view"
	Columns     []Column     `json:"columns"`
	Indexes     []Index      `json:"indexes"`
	ForeignKeys []ForeignKey `json:"foreignKeys"`
}

type ColumnMeta struct {
	Name         string `json:"name"`
	DeclaredType string `json:"declaredType"`
}

// BlobValue represents a BLOB cell. The frontend renders it as a compact
// placeholder rather than attempting to display raw binary as text.
type BlobValue struct {
	Blob   bool   `json:"blob"`
	Size   int    `json:"size"`
	Base64 string `json:"base64"`
}

// KVEntry is one key in a key-value listing (ResultSet.Entries). A scan
// lists metadata only: Value is left nil so listing a keyspace never loads
// every value. Length is the element count, or the byte length for a
// string. TTLSeconds/TTLMillis are nil when the key has no expiry.
type KVEntry struct {
	Key        string `json:"key"`
	Value      any    `json:"value,omitempty"`
	Type       string `json:"type,omitempty"`
	Length     int64  `json:"length"`
	TTLSeconds *int64 `json:"ttlSeconds,omitempty"`
	TTLMillis  *int64 `json:"ttlMillis,omitempty"`
}

// KeyDetail is one key's type-shaped value, read one page at a time.
// Value is set for a string, Members for every other type. Cursor is
// opaque to the caller and "" once there is nothing more to read.
type KeyDetail struct {
	Key       string      `json:"key"`
	Type      string      `json:"type"`
	Exists    bool        `json:"exists"`
	Length    int64       `json:"length"`
	TTLMillis *int64      `json:"ttlMillis,omitempty"`
	Value     *KVString   `json:"value,omitempty"`
	Members   []KeyMember `json:"members,omitempty"`
	Cursor    string      `json:"cursor"`
	Stream    *StreamInfo `json:"stream,omitempty"`
}

// KVString is a stored string or element. Binary is true when the bytes
// are not valid UTF-8: Text is then empty and Base64 holds the exact
// bytes, since JSON would replace invalid bytes and corrupt the value.
type KVString struct {
	Text   string `json:"text"`
	Binary bool   `json:"binary,omitempty"`
	Base64 string `json:"base64,omitempty"`
	Size   int    `json:"size"`
}

// KeyMember is one element of a non-string key. Hash: Field+Value. List:
// Index+Value. Set: Value. Sorted set: Value+Score. Stream: ID+Fields.
type KeyMember struct {
	Field  *KVString           `json:"field,omitempty"`
	Index  *int64              `json:"index,omitempty"`
	ID     string              `json:"id,omitempty"`
	Value  KVString            `json:"value"`
	Score  *float64            `json:"score,omitempty"`
	Fields map[string]KVString `json:"fields,omitempty"`
}

// KeyEdit is one change to one key. Op names the operation ("set",
// "hset", "hdel", "lset", "lpush", "rpush", "lrem", "sadd", "srem",
// "zadd", "zrem", "create", "del", "expire", "persist"); which other
// fields matter depends on Op. The Old* fields are what the user last saw, compared against the
// key's current state so a change that moved underneath them is refused
// instead of overwritten.
type KeyEdit struct {
	Key      string   `json:"key"`
	Op       string   `json:"op"`
	Type     string   `json:"type,omitempty"` // "create" only
	Field    string   `json:"field,omitempty"`
	OldField *string  `json:"oldField,omitempty"`
	Index    *int64   `json:"index,omitempty"`
	Value    string   `json:"value"`
	OldValue *string  `json:"oldValue,omitempty"`
	Score    *float64 `json:"score,omitempty"`
	OldScore *float64 `json:"oldScore,omitempty"`
	// TTLMillis is the new expiry for "expire"; it must be positive.
	TTLMillis *int64 `json:"ttlMillis,omitempty"`
}

// KeyEditResult reports one KeyEdit. Conflict means the key no longer
// matched what the user saw and nothing was written; Message says why.
// KeyRemoved means the key is gone after the edit (a delete, or the last
// element removed, which makes Redis drop the key itself).
type KeyEditResult struct {
	Applied    bool   `json:"applied"`
	Conflict   bool   `json:"conflict"`
	KeyRemoved bool   `json:"keyRemoved"`
	Message    string `json:"message,omitempty"`
}

// StreamInfo summarizes a stream, sent with its first page only.
type StreamInfo struct {
	FirstID string        `json:"firstId"`
	LastID  string        `json:"lastId"`
	Groups  []StreamGroup `json:"groups"`
}

// StreamGroup is one consumer group. Lag is nil when the server does not
// report it (Redis before 7).
type StreamGroup struct {
	Name            string `json:"name"`
	Consumers       int64  `json:"consumers"`
	Pending         int64  `json:"pending"`
	LastDeliveredID string `json:"lastDeliveredId"`
	Lag             *int64 `json:"lag,omitempty"`
}

// ResultSet is a tagged union so a failed statement mid-batch can be
// reported without discarding result sets already collected from earlier
// statements in the same Execute call (a returned Go error would reject the
// whole Wails call and drop everything, per @wailsio/runtime's Call()).
//
// Documents and Entries are additive variants for a non-SQL engine like
// MongoDB or Redis: no SQL engine ever sets
// Kind to "documents" or "keyvalue" today, or populates either field, so
// existing rows/status/error behavior is unchanged. They're carried on the
// same struct rather than a Go-level union (interface{} variant types)
// because this struct crosses the Wails JS boundary as plain JSON; splitting
// it would need a discriminated-union type on the TypeScript side that
// duplicates the same Kind switch this app already does per result.
type ResultSet struct {
	StableOrder bool             `json:"stableOrder,omitempty"`
	Kind        string           `json:"kind"` // "rows" | "documents" | "keyvalue" | "status" | "error"
	Statement   string           `json:"statement"`
	Columns     []ColumnMeta     `json:"columns,omitempty"`
	Rows        []map[string]any `json:"rows,omitempty"`
	// Documents holds one result's payload when Kind is "documents" (e.g. a
	// MongoDB query), each document already flattened to a JSON object the
	// same way a SQL row is.
	Documents []map[string]any `json:"documents,omitempty"`
	// Entries holds one result's payload when Kind is "keyvalue" (e.g. a
	// Redis/RocksDB get or scan).
	Entries        []KVEntry `json:"entries,omitempty"`
	Truncated      bool      `json:"truncated,omitempty"`
	RowsAffected   int64     `json:"rowsAffected,omitempty"`
	LastInsertID   any       `json:"lastInsertId,omitempty"`
	ErrorCode      string    `json:"errorCode,omitempty"`
	ConnectionLost bool      `json:"connectionLost,omitempty"`
	Error          string    `json:"error,omitempty"`
}

type StatementRange struct {
	Text  string `json:"text,omitempty"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// CellEdit represents one edited table cell. Mode is "value", "null", or
// "default"; "default" is valid for inserts and schema-capable updates.
type CellEdit struct {
	Mode  string `json:"mode"`
	Value any    `json:"value,omitempty"`
}

// RowEdit is one staged table mutation. Original is the row as last loaded by
// the frontend; providers use it for identity and conflict checks.
type RowEdit struct {
	Kind     string              `json:"kind"` // "insert" | "update" | "delete"
	Original map[string]any      `json:"original,omitempty"`
	Values   map[string]CellEdit `json:"values,omitempty"`
}

type EditResult struct {
	Applied      int      `json:"applied"`
	RowsAffected int64    `json:"rowsAffected"`
	Statements   []string `json:"statements"`
}

// DocumentEditResult reports one whole-document insert/replace/delete for a
// document engine (MongoDB). Matched is the server's own matched-document
// count for a replace/delete; 0 for an insert, and 0 for a replace/delete
// that found no document anymore (the same conflict signal a SQL edit's
// affected-row-count mismatch already gives).
type DocumentEditResult struct {
	Applied bool  `json:"applied"`
	Matched int64 `json:"matched"`
}

// EditPreview is one parameterized statement ApplyEdits would execute. Args
// are display-only strings kept separate from SQL; they are never interpolated
// back into a statement or parsed for execution.
type EditPreview struct {
	Kind string   `json:"kind"` // "insert" | "update" | "delete"
	SQL  string   `json:"sql"`
	Args []string `json:"args"`
}

// VisualQueryRef points at one column on one table already added to a visual
// query (by alias, never by bare column name), so the compiler never has to
// resolve ambiguous references.
type VisualQueryRef struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}

// VisualQueryTable is one table instance placed on the Visual Query Builder
// canvas. Object is the same table identifier already used elsewhere for this
// engine (SchemaTable.Name / model.Table.Name): a plain name for SQLite, a
// JSON schema/name pair for PostgreSQL and MySQL.
type VisualQueryTable struct {
	Object string `json:"object"`
	Alias  string `json:"alias"`
}

// VisualQueryJoin is a single equi-join between two tables already on the
// canvas, addressed by alias. Type is "inner", "left", or "right".
type VisualQueryJoin struct {
	Type  string         `json:"type"`
	Left  VisualQueryRef `json:"left"`
	Right VisualQueryRef `json:"right"`
}

// VisualQueryColumn is one selected output column. Aggregate is "" for a
// plain column, or one of "count","sum","avg","min","max".
type VisualQueryColumn struct {
	VisualQueryRef
	Aggregate string `json:"aggregate,omitempty"`
	Alias     string `json:"alias,omitempty"`
}

// VisualQueryFilter is one ANDed WHERE (or HAVING, when Aggregate is set)
// condition. Operator is one of "eq","ne","lt","lte","gt","gte","like",
// "isNull","isNotNull","in". Value is used by all operators except "in" and
// the null checks; Values is used by "in".
type VisualQueryFilter struct {
	VisualQueryRef
	Aggregate string `json:"aggregate,omitempty"`
	Operator  string `json:"operator"`
	Value     any    `json:"value,omitempty"`
	Values    []any  `json:"values,omitempty"`
}

// VisualQueryOrder is one ORDER BY entry, optionally against an aggregate.
type VisualQueryOrder struct {
	VisualQueryRef
	Aggregate  string `json:"aggregate,omitempty"`
	Descending bool   `json:"descending"`
}

// VisualQuery is the Visual Query Builder's typed AST for a single SELECT.
// GroupBy is derived by the frontend from which output columns are
// unaggregated (see workspace.svelte.ts) rather than picked independently,
// so an aggregate query can never omit a required grouping column.
type VisualQuery struct {
	Tables  []VisualQueryTable  `json:"tables"`
	Joins   []VisualQueryJoin   `json:"joins"`
	Columns []VisualQueryColumn `json:"columns"`
	Filters []VisualQueryFilter `json:"filters"`
	GroupBy []VisualQueryRef    `json:"groupBy"`
	Having  []VisualQueryFilter `json:"having"`
	OrderBy []VisualQueryOrder  `json:"orderBy"`
	Limit   int                 `json:"limit,omitempty"`
}

// ExplainNode is one operation in a query plan, normalized to a common
// shape across engines. Fields an engine's EXPLAIN output doesn't provide
// (ActualRows/ActualTimeMs under a plain, non-ANALYZE EXPLAIN; anything
// beyond Operation/Relation/Detail for SQLite) are left nil/zero rather
// than guessed at.
type ExplainNode struct {
	ID             string `json:"id"`
	ParentID       string `json:"parentId,omitempty"`
	Operation      string `json:"operation"`
	Relation       string `json:"relation,omitempty"`
	Alias          string `json:"alias,omitempty"`
	Detail         string `json:"detail,omitempty"`
	Filter         string `json:"filter,omitempty"`
	IndexCondition string `json:"indexCondition,omitempty"`

	EstimatedRows *int64 `json:"estimatedRows,omitempty"`
	ActualRows    *int64 `json:"actualRows,omitempty"`
	Loops         int    `json:"loops"`

	// IsSeqScan reports whether Operation is this engine's own name for "a
	// full scan of a relation with no index" (Postgres's "Seq Scan",
	// MySQL/MariaDB's "ALL" access type, SQLite's "SCAN" EXPLAIN QUERY PLAN
	// keyword, MongoDB's "COLLSCAN" stage). Each engine's own Explain
	// implementation sets this, since only it knows its own operation-name
	// strings; internal/database/explain.Annotate reads it to attach a
	// seqScan badge without knowing any engine's vocabulary itself.
	IsSeqScan bool `json:"isSeqScan,omitempty"`

	EstimatedCost   *float64 `json:"estimatedCost,omitempty"`
	InclusiveTimeMs *float64 `json:"inclusiveTimeMs,omitempty"`
	ExclusiveTimeMs *float64 `json:"exclusiveTimeMs,omitempty"`
	ExclusiveCost   *float64 `json:"exclusiveCost,omitempty"`

	// Extra holds engine-specific detail that doesn't deserve its own named
	// field on every engine's plan (Postgres buffer counts, MariaDB filesort
	// details, parallel-worker counts). A bag on purpose, not an
	// ever-growing struct most engines would mostly leave empty.
	Extra map[string]any `json:"extra,omitempty"`

	Badges   []ExplainBadge `json:"badges,omitempty"`
	Children []*ExplainNode `json:"children,omitempty"`
}

// ExplainBadge is one diagnostic finding attached to a node by
// internal/database/explain.Annotate.
type ExplainBadge struct {
	Kind     string `json:"kind"`     // "hotspot" | "estimateGap" | "seqScan" | "diskSpill" | "heapFetch" | "underutilizedWorker" | "filterInefficiency"
	Severity string `json:"severity"` // "info" | "warning"
	Message  string `json:"message"`
}

// ExplainPlan is the full result of one EXPLAIN/EXPLAIN ANALYZE run,
// already annotated (exclusive metrics, badges, Stats) by
// internal/database/explain.Annotate before it crosses the Wails boundary.
type ExplainPlan struct {
	Engine          string       `json:"engine"`
	EngineVersion   string       `json:"engineVersion,omitempty"`
	Analyzed        bool         `json:"analyzed"`
	Root            *ExplainNode `json:"root"`
	PlanningTimeMs  *float64     `json:"planningTimeMs,omitempty"`
	ExecutionTimeMs *float64     `json:"executionTimeMs,omitempty"`
	RawOutput       string       `json:"rawOutput"`
	RawFormat       string       `json:"rawFormat"` // "json" | "text"
	Warnings        []string     `json:"warnings,omitempty"`
	Stats           ExplainStats `json:"stats"`
}

// ExplainStats is a plan-wide summary computed once over the whole tree.
type ExplainStats struct {
	OperationCounts  map[string]int `json:"operationCounts"`
	RelationsScanned []string       `json:"relationsScanned,omitempty"`
	IndexesUsed      []string       `json:"indexesUsed,omitempty"`
	NodeCount        int            `json:"nodeCount"`
	TotalCost        *float64       `json:"totalCost,omitempty"`
	TotalTimeMs      *float64       `json:"totalTimeMs,omitempty"`
}
