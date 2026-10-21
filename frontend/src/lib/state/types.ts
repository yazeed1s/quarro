export type Engine = 'sqlite' | 'postgres' | 'mysql' | 'mongodb' | 'redis';
export interface Capabilities {
    connector: boolean;
    discoverer: boolean;
    sqlExecutor: boolean;
    browser: boolean;
    editor: boolean;
    transactor: boolean;
    keyValueStore: boolean;
    keyValueEditor: boolean;
    aggregator: boolean;
    queryCompiler: boolean;
    explainer: boolean;
    identifierFormatter: boolean;
    routineDiscoverer: boolean;
    lazyDiscoverer: boolean;
    constraintDiscoverer: boolean;
    definitionDiscoverer: boolean;
    documentEditor: boolean;
    nativeQueryExecutor: boolean;
    /** Which native editor a query document uses: 'mongodb-pipeline', 'redis-command', or '' for none. */
    nativeQueryLanguage: string;
    supportsAnalyze: boolean;
    requiresDelimiterWrapping: boolean;
    supportsJoins: boolean;
    supportsGroupBy: boolean;
}
export const emptyCapabilities: Capabilities = {
    connector: false,
    discoverer: false,
    sqlExecutor: false,
    browser: false,
    editor: false,
    transactor: false,
    keyValueStore: false,
    keyValueEditor: false,
    aggregator: false,
    queryCompiler: false,
    explainer: false,
    identifierFormatter: false,
    routineDiscoverer: false,
    lazyDiscoverer: false,
    constraintDiscoverer: false,
    definitionDiscoverer: false,
    documentEditor: false,
    nativeQueryExecutor: false,
    nativeQueryLanguage: '',
    supportsAnalyze: false,
    requiresDelimiterWrapping: false,
    supportsJoins: false,
    supportsGroupBy: false,
};
export interface ConfigField {
    key: string;
    label: string;
    kind: string;
    required: boolean;
    default?: string;
}
export interface DriverMetadata {
    name: Engine;
    displayName: string;
    icon: string;
    sqlDialect?: string;
    configSchema: ConfigField[];
}
export type Theme = 'system' | 'monochrome-light' | 'monochrome';
export type BlobCell = { blob: true; size: number; base64: string };
// A nested object/array only ever comes from a document engine (MongoDB);
// no SQL provider's ResultSet produces one. Typed here rather than widened
// to `any` so call sites that only ever see SQL results keep real narrowing.
export type CellValue = string | number | boolean | null | BlobCell | CellValue[] | { [key: string]: CellValue };
export type Row = Record<string, CellValue>;
export function isBlobCell(value: CellValue): value is BlobCell {
    return typeof value === 'object' && value !== null && 'blob' in value;
}
export interface Column {
    name: string;
    declaredType: string;
}
// KVEntry is one key in a key-value listing: metadata only, never the
// value (see KeyDetail for that). length is the element count, or the byte
// length for a string.
export interface KVEntry {
    key: string;
    type?: string;
    length: number;
    ttlSeconds?: number;
    ttlMillis?: number;
}
// KVString is one stored string or element. A value that is not valid
// UTF-8 arrives as base64 with binary set, since JSON text would corrupt it.
export interface KVString {
    text: string;
    binary?: boolean;
    base64?: string;
    size: number;
}
// KeyMember is one element of a non-string key. Hash: field + value. List:
// index + value. Set: value. Sorted set: value + score. Stream: id + fields.
export interface KeyMember {
    field?: KVString;
    index?: number;
    id?: string;
    value: KVString;
    score?: number;
    fields?: Record<string, KVString>;
}
export interface StreamGroup {
    name: string;
    consumers: number;
    pending: number;
    lastDeliveredId: string;
    lag?: number;
}
export interface StreamInfo {
    firstId: string;
    lastId: string;
    groups: StreamGroup[];
}
// KeyEdit is one change to one Redis key, mirroring model.KeyEdit. The
// old* fields are what the user last saw; the backend refuses the edit
// (conflict) when the key no longer matches them.
export interface KeyEdit {
    key: string;
    op: 'set' | 'hset' | 'hdel' | 'lset' | 'lpush' | 'rpush' | 'lrem' | 'sadd' | 'srem' | 'zadd' | 'zrem' | 'create' | 'del' | 'expire' | 'persist';
    type?: string;
    field?: string;
    oldField?: string;
    index?: number;
    value: string;
    oldValue?: string;
    score?: number;
    oldScore?: number;
    /** The new expiry for "expire"; must be positive. */
    ttlMillis?: number;
}
export interface KeyEditResult {
    applied: boolean;
    conflict: boolean;
    keyRemoved: boolean;
    message?: string;
}
// KeyDetail is one key's value as loaded so far. cursor is '' once every
// page has been read.
export interface KeyDetail {
    key: string;
    type: string;
    exists: boolean;
    length: number;
    ttlMillis?: number;
    value?: KVString;
    members: KeyMember[];
    cursor: string;
    stream?: StreamInfo;
}
// "documents" and "keyvalue" are additive result kinds for a non-SQL
// engine like MongoDB or Redis. No SQL engine emits either today, so
// every existing renderer only ever sees
// "rows" | "status" | "error".
export interface ResultSet {
    kind: 'rows' | 'documents' | 'keyvalue' | 'status' | 'error';
    statement: string;
    columns?: Column[];
    rows?: Row[];
    documents?: Record<string, unknown>[];
    entries?: KVEntry[];
    truncated?: boolean;
    rowsAffected?: number;
    lastInsertId?: number | string;
    stableOrder?: boolean;
    error?: string;
    errorCode?: string;
    connectionLost?: boolean;
}
export type CellEditMode = 'value' | 'null' | 'default';
export interface CellEdit {
    mode: CellEditMode;
    value?: CellValue;
}
export interface RowEdit {
    id: string;
    kind: 'insert' | 'update' | 'delete';
    original?: Row;
    values: Record<string, CellEdit>;
}
export interface EditResult {
    applied: number;
    rowsAffected: number;
    statements: string[];
}
export interface EditPreview {
    kind: RowEdit['kind'];
    sql: string;
    args: string[];
}
// DocumentEditResult reports one whole-document insert/replace/delete for
// a document engine (MongoDB). matched is 0 for a replace/delete that
// found no document anymore — the same conflict signal EditResult's own
// affected-row-count mismatch already gives for SQL.
export interface DocumentEditResult {
    applied: boolean;
    matched: number;
}
export interface SchemaColumn {
    name: string;
    type: string;
    notNull: boolean;
    primaryKey: boolean;
    defaultValue: string | null;
}
export interface SchemaIndex {
    name: string;
    columns: string[];
    unique: boolean;
}
export interface SchemaForeignKey {
    column: string;
    refTable: string;
    refColumn: string;
    // The engine's own referential action text ("CASCADE" | "SET NULL" |
    // "RESTRICT" | "NO ACTION" | "SET DEFAULT"), read from its catalog.
    onDelete?: string;
    onUpdate?: string;
}
export interface SchemaTable {
    schema?: string;
    objectName?: string;
    name: string;
    // 'collection' is MongoDB's, 'keyspace' is Redis's (a "prefix:" group of
    // keys, not a real schema object).
    kind: 'table' | 'view' | 'collection' | 'keyspace';
    columns: SchemaColumn[];
    indexes: SchemaIndex[];
    foreignKeys: SchemaForeignKey[];
}
// Constraint is a table-level CHECK or named UNIQUE constraint, distinct
// from SchemaIndex: a named UNIQUE constraint often has a near-identical
// unique SchemaIndex entry, but a CHECK constraint has no index at all.
export interface Constraint {
    name: string;
    kind: 'check' | 'unique';
    columns?: string[];
    expression?: string;
}
// Definition is one object's CREATE TABLE/CREATE VIEW text. verbatim is
// false only for PostgreSQL's reconstructed table DDL (it has no single
// built-in function for that, unlike its own view text or MySQL/SQLite).
export interface Definition {
    sql: string;
    verbatim: boolean;
}
// SchemaFunction is a stored function or procedure (PostgreSQL/MySQL only).
// Parameters/returnType/definition are engine-formatted best-effort text,
// not a structured type Quarro parses itself.
export interface SchemaFunction {
    name: string;
    schema: string;
    objectName: string;
    kind: 'function' | 'procedure';
    parameters: string;
    returnType: string;
    language: string;
    definition: string;
}
export interface Profile {
    id: string;
    name: string;
    engine: Engine;
    database: string;
    schema: string;
    environment: string;
    folder: string;
    favorite: boolean;
    host: string;
    port: string;
    username: string;
    path: string;
    readOnly: boolean;
    tlsMode?: string;
    tlsCA?: string;
    tlsCert?: string;
    tlsKey?: string;
    tlsServerName?: string;
    socket?: string;
    hasPassword: boolean;

    // SSH tunnel fields. Only meaningful for PostgreSQL/MySQL profiles.
    sshEnabled?: boolean;
    sshHost?: string;
    sshPort?: string;
    sshUsername?: string;
    sshAuthMethod?: 'password' | 'key';
    sshPrivateKeyPath?: string;
    sshHasPassword?: boolean;
    sshHasPassphrase?: boolean;
    sshHostFingerprint?: string;
}
export type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error';
export type VisualQueryAggregate = '' | 'count' | 'sum' | 'avg' | 'min' | 'max';
export type VisualQueryJoinType = 'inner' | 'left' | 'right';
export type VisualQueryOperator = 'eq' | 'ne' | 'lt' | 'lte' | 'gt' | 'gte' | 'like' | 'isNull' | 'isNotNull' | 'in';
export interface VisualQueryRef {
    table: string;
    column: string;
}
export interface VisualQueryTable {
    object: string;
    alias: string;
}
export interface VisualQueryJoin {
    type: VisualQueryJoinType;
    left: VisualQueryRef;
    right: VisualQueryRef;
}
export interface VisualQueryColumn extends VisualQueryRef {
    aggregate?: VisualQueryAggregate;
    alias?: string;
}
export interface VisualQueryFilter extends VisualQueryRef {
    aggregate?: VisualQueryAggregate;
    operator: VisualQueryOperator;
    value?: CellValue;
    values?: CellValue[];
}
export interface VisualQueryOrder extends VisualQueryRef {
    aggregate?: VisualQueryAggregate;
    descending: boolean;
}
export interface VisualQuery {
    tables: VisualQueryTable[];
    joins: VisualQueryJoin[];
    columns: VisualQueryColumn[];
    filters: VisualQueryFilter[];
    groupBy: VisualQueryRef[];
    having: VisualQueryFilter[];
    orderBy: VisualQueryOrder[];
    limit: number;
}
export function emptyVisualQuery(): VisualQuery {
    return {
        tables: [],
        joins: [],
        columns: [],
        filters: [],
        groupBy: [],
        having: [],
        orderBy: [],
        limit: 0,
    };
}
export type ChartSource =
    | {
          kind: 'snapshot';
          /** The session this series' rows were snapshotted from. A series always
           * resolves against its own source session, never the chart document's
           * session, so a chart can combine series from several sessions/engines. */
          sessionId: string;
          sourceDocumentId: string;
          sourceTitle: string;
          columns: Column[];
          rows: Row[];
      }
    | {
          kind: 'table';
          sessionId: string;
          sourceDocumentId?: string;
          sourceTitle: string;
          object: string;
          columns: Column[];
          rows: Row[];
      };
export type ChartType = 'bar' | 'line' | 'pie' | 'scatter' | 'histogram';
export type ChartAggregate = 'count' | 'sum' | 'avg' | 'min' | 'max' | 'countDistinct';
export interface ChartFieldRole {
    x: string;
    y: string;
    series?: string;
    color?: string;
}
export interface ChartSeriesConfig {
    id: string;
    label: string;
    source: ChartSource;
    chartType: ChartType;
    role: ChartFieldRole;
    aggregate: ChartAggregate;
    pieLimit: number;
    /** Histogram bin count; unused by every other chart type. */
    binCount: number;
}
export interface ExplainBadge {
    kind: string;
    severity: 'info' | 'warning';
    message: string;
}
export interface ExplainNode {
    id: string;
    parentId?: string;
    operation: string;
    relation?: string;
    alias?: string;
    detail?: string;
    filter?: string;
    indexCondition?: string;
    estimatedRows?: number;
    actualRows?: number;
    loops: number;
    estimatedCost?: number;
    inclusiveTimeMs?: number;
    exclusiveTimeMs?: number;
    exclusiveCost?: number;
    extra?: Record<string, unknown>;
    badges: ExplainBadge[];
    children: ExplainNode[];
}
export interface ExplainStats {
    operationCounts: Record<string, number>;
    relationsScanned: string[];
    indexesUsed: string[];
    nodeCount: number;
    totalCost?: number;
    totalTimeMs?: number;
}
export interface ExplainPlan {
    engine: string;
    engineVersion?: string;
    analyzed: boolean;
    root: ExplainNode | null;
    planningTimeMs?: number;
    executionTimeMs?: number;
    rawOutput: string;
    rawFormat: 'json' | 'text';
    warnings: string[];
    stats: ExplainStats;
}
export type ExplainMode = 'estimate' | 'analyze';
export type ExplainView = 'graph' | 'table' | 'diagram' | 'stats' | 'raw';
export interface Document {
    id: string;
    kind: 'query' | 'table' | 'diagram' | 'visualQuery' | 'chart' | 'explain';
    title: string;
    sql: string;
    object: string;
    view: 'data' | 'structure' | 'indexes' | 'relationships';
    filter: string;
    sort: string;
    descending: boolean;
    selected: number;
    selectedColumn: string;
    hasResults: boolean;
    status: 'idle' | 'running' | 'done' | 'cancelled' | 'error';
    message: string;
    elapsed: number;
    results: ResultSet[];
    activeResult: number;
    jobId: string;
    selectionFrom: number;
    selectionTo: number;
    cursor: number;
    /** Server-side "filter the whole table" for table documents. This is distinct
     * from `filter`, which is always a client-side search over loaded rows. */
    tableFilter: string;
    /** When set, tableFilter is an exact match on this one column (relationship
     * navigation's "open related rows") instead of the default free-text
     * substring search across every column. Cleared by any manual filter edit. */
    tableFilterColumn: string;
    /** Bumped by resetTableView() to force the grid to remount and re-read a
     * freshly-cleared layout instead of keeping stale in-memory column state. */
    viewVersion: number;
    edits: RowEdit[];
    transactionActive: boolean;
    outputCollapsed: boolean;
    diagramSearch: string;
    diagramSchema: string;
    diagramRelated: string;
    diagramPositions: Record<string, { x: number; y: number }>;
    diagramCollapsed: string[];
    visualQueryAst: VisualQuery;
    visualQuerySql: string;
    visualQueryError: string;
    visualQueryPositions: Record<string, { x: number; y: number }>;
    chartSeries: ChartSeriesConfig[];
    /** ":name" parameters found in the statement about to run, awaiting
     * values from the parameter prompt dialog; empty once resolved. */
    paramPrompt: string[];
    /** Last-provided value per parameter name, offered as the prompt's
     * default and reused for later runs without an explicit "NULL" name. */
    paramValues: Record<string, string | null>;
    /** The source query document this was forked from ("Explain" toolbar
     * action); kept for "Show in editor"/re-run-after-edit navigation. */
    explainSourceDocumentId: string;
    /** The exact statement being explained, resolved once at fork time from
     * the source document's statement-under-cursor, independent of any later
     * edits to that source document. */
    explainSql: string;
    explainMode: ExplainMode;
    explainPlan: ExplainPlan | null;
    explainView: ExplainView;
    explainSelectedNodeId: string;
    /** Bumped by workspace.revealExplainNode() to ask the editor to select
     * and scroll to selectionFrom/selectionTo once; never written by the
     * editor itself, unlike selectionFrom/selectionTo/cursor, which the
     * editor writes on every user selection change. */
    revealSeq: number;
    /** The Redis key open in the key inspector, with every page loaded so
     * far. Fetched data, so never persisted. */
    keyDetail: KeyDetail | null;
    keyLoading: boolean;
    keyError: string;
}
export type ChartDocument = Document & { kind: 'chart'; chartSeries: ChartSeriesConfig[] };
export type ExplainDocument = Document & { kind: 'explain'; explainPlan: ExplainPlan | null };
export interface GridLayout {
    widths: Record<string, number>;
    order: string[];
    hidden: string[];
    sort: string;
    descending: boolean;
    filter: string;
}
export interface Session {
    id: string;
    profile: Profile;
    activeDocument: string;
    documents: Document[];
    connectionState: ConnectionState;
    connectionError: string;
    schema: SchemaTable[];
    schemaLoading: boolean;
    schemaFullyLoaded: boolean;
    schemaFullLoading: boolean;
    schemaVersion: number;
    tableDetails: Record<string, SchemaTable>;
    tableDetailLoading: Record<string, boolean>;
    tableDetailErrors: Record<string, string>;
    tableConstraints: Record<string, Constraint[]>;
    tableConstraintsLoading: Record<string, boolean>;
    tableConstraintsErrors: Record<string, string>;
    tableDefinitions: Record<string, Definition>;
    tableDefinitionsLoading: Record<string, boolean>;
    tableDefinitionsErrors: Record<string, string>;
    routines: SchemaFunction[];
    capabilities: Capabilities;
    sidebarWidth: number;
    editorHeight: number;
    inspectorWidth: number;
    inspector: boolean;
    sidebar: boolean;
}
export interface Preferences {
    theme: Theme;
    uiFont: string;
    uiSize: number;
    density: 'compact' | 'comfortable';
    zoom: number;
    editorFont: string;
    editorSize: number;
    lineHeight: number;
    ligatures: boolean;
    tabSize: number;
    indentTabs: boolean;
    lineNumbers: boolean;
    wordWrap: boolean;
    activeLine: boolean;
    gridFont: string;
    gridSize: number;
    rowHeight: number;
    stripes: boolean;
    nullDisplay: string;
    dateFormat: 'locale' | 'iso';
    timezone: 'UTC' | 'local';
    reducedMotion: boolean;
    restore: boolean;
    resultLimit: number;
    timeout: number;
    historyDays: number;
    rememberHistory: boolean;
    connectionBehavior: 'new' | 'reuse';
    /** Linux only; Go's window.json is authoritative (see DesktopChrome). */
    titleBar: 'system' | 'custom';
    /** Go's window.json is authoritative (see DesktopChrome). */
    appIcon: 'dark' | 'light';
    /** Daily GitHub check for a newer release (prod builds only). */
    checkUpdates: boolean;
}
/** This build, as reported by AppService. */
export interface AppInfo {
    version: string;
    stage: string;
    os: string;
    arch: string;
}
/** A newer published release; available is false when there is none. */
export interface UpdateInfo {
    available: boolean;
    version: string;
    url: string;
}
/** How the native window is decorated, as reported by DesktopService. */
export interface DesktopChrome {
    platform: string;
    titleBar: 'system' | 'custom';
    /** The title bar choice exists on this OS (Linux). */
    configurable: boolean;
    /** The frontend must draw the drag region and window controls. */
    frameless: boolean;
    /** macOS draws its traffic lights over the session bar's leading edge. */
    trafficLights: boolean;
    appIcon: 'dark' | 'light';
    /** The app icon can be swapped: Dock (macOS), taskbar (Windows), launcher entry (Linux). */
    appIconConfigurable: boolean;
}
export type HistoryOutcome = 'ok' | 'error';
export interface HistoryEntry {
    id: string;
    title: string;
    sql: string;
    at: number;
    profileId: string;
    durationMs: number;
    outcome: HistoryOutcome;
    /** Selected collection for an engine-native query. Absent for SQL and
     * for entries persisted before native queries existed. */
    object?: string;
}
export interface SavedQuery {
    id: string;
    title: string;
    sql: string;
    at: number;
    profileId: string;
    folder: string;
    favorite: boolean;
    /** Selected collection for an engine-native query. */
    object?: string;
}

export type ExportScope = 'loaded' | 'filtered' | 'full';
export type ExportFormat = 'csv' | 'json' | 'sql';
export interface ExportRequest {
    kind: 'table' | 'query';
    format: ExportFormat;
    scope: ExportScope;
    table: string;
    filter: string;
    sort: string;
    descending: boolean;
    sql: string;
    delimiter: string;
}
export interface ExportProgress {
    rows: number;
    bytes: number;
    done: boolean;
    error: string;
    cancelled: boolean;
}
export interface CSVPreview {
    columns: string[];
    sample: string[][];
    totalRows: number;
    totalRowsCapped: boolean;
}
export interface ColumnMapping {
    csvColumn: number;
    column: string;
}
export interface ImportOptions {
    delimiter: string;
    hasHeader: boolean;
    nullToken: string;
    batchSize: number;
    encoding: string;
}
export interface ImportValidationIssue {
    row: number;
    csvColumn: string;
    column: string;
    value: string;
    expected: string;
    message: string;
}
export interface ImportValidation {
    validRows: number;
    invalidRows: number;
    issues: ImportValidationIssue[];
    uncheckedColumns: string[];
}
export interface ImportProgress {
    imported: number;
    failed: number;
    done: boolean;
    cancelled: boolean;
    fatalError: string;
    errors: string[];
}

export function objectLabel(name: string): string {
    try {
        const value = JSON.parse(name);
        if (Array.isArray(value) && value.length === 2) return value.join('.');
    } catch {}
    return name;
}
