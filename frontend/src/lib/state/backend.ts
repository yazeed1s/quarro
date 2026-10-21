import * as ConnectionsService from '../../../bindings/github.com/yazeed1s/quarro/internal/app/connectionsservice';
import * as QueriesService from '../../../bindings/github.com/yazeed1s/quarro/internal/app/queriesservice';
import * as ExportService from '../../../bindings/github.com/yazeed1s/quarro/internal/app/exportservice';
import * as ImportService from '../../../bindings/github.com/yazeed1s/quarro/internal/app/importservice';
import * as DesktopService from '../../../bindings/github.com/yazeed1s/quarro/internal/app/desktopservice';
import * as AppService from '../../../bindings/github.com/yazeed1s/quarro/internal/app/appservice';
import { Browser } from '@wailsio/runtime';
import type { Engine as BackendEngine, Profile as BackendProfile } from '../../../bindings/github.com/yazeed1s/quarro/internal/connections/models';
import type {
    Table as BackendTable,
    Function as BackendFunction,
    ResultSet as BackendResultSet,
    ExplainNode as BackendExplainNode,
    ExplainPlan as BackendExplainPlan,
} from '../../../bindings/github.com/yazeed1s/quarro/internal/database/model/models';
import type { Capabilities as BackendCapabilities } from '../../../bindings/github.com/yazeed1s/quarro/internal/database/capability/models';
import type { DriverMetadata as BackendDriverMetadata } from '../../../bindings/github.com/yazeed1s/quarro/internal/database/driver/models';
import type {
    Profile,
    Engine,
    Capabilities,
    DriverMetadata,
    ResultSet,
    SchemaTable,
    SchemaFunction,
    Constraint,
    Definition,
    RowEdit,
    EditResult,
    EditPreview,
    DocumentEditResult,
    VisualQuery,
    ExportRequest,
    ExportProgress,
    CSVPreview,
    ColumnMapping,
    ImportOptions,
    ImportProgress,
    ImportValidation,
    ExplainNode,
    ExplainPlan,
    KeyDetail,
    KeyEdit,
    KeyEditResult,
    StreamGroup,
    DesktopChrome,
    AppInfo,
    UpdateInfo,
} from './types';
import { emptyCapabilities } from './types';

function toProfile(p: BackendProfile): Profile {
    return { ...p, engine: p.engine as unknown as Engine, sshAuthMethod: p.sshAuthMethod as unknown as Profile['sshAuthMethod'] };
}

export async function listProfiles(): Promise<Profile[]> {
    return ((await ConnectionsService.ListProfiles()) ?? []).map(toProfile);
}

export interface SaveProfileResult {
    profile: Profile;
    passwordNotPersistent: boolean;
}

export async function saveProfile(profile: Profile, password?: string, sshPassword?: string, sshPassphrase?: string): Promise<SaveProfileResult> {
    const result = await ConnectionsService.SaveProfile({
        ...profile,
        password: password ?? '',
        sshPassword: sshPassword ?? '',
        sshPassphrase: sshPassphrase ?? '',
        tlsMode: profile.tlsMode ?? 'verify-full',
        tlsCA: profile.tlsCA ?? '',
        tlsCert: profile.tlsCert ?? '',
        tlsKey: profile.tlsKey ?? '',
        tlsServerName: profile.tlsServerName ?? '',
        socket: profile.socket ?? '',
        engine: profile.engine as unknown as BackendEngine,
    } as never);
    return {
        profile: toProfile(result.profile),
        passwordNotPersistent: result.passwordNotPersistent,
    };
}

export async function deleteProfile(id: string): Promise<void> {
    await ConnectionsService.DeleteProfile(id);
}

export interface TestResult {
    ok: boolean;
    message: string;
    sshHostFingerprint?: string;
}

export async function testConnection(profile: Profile, password?: string, sshPassword?: string, sshPassphrase?: string): Promise<TestResult> {
    return ConnectionsService.TestConnection({
        ...profile,
        password: password ?? '',
        sshPassword: sshPassword ?? '',
        sshPassphrase: sshPassphrase ?? '',
        tlsMode: profile.tlsMode ?? 'verify-full',
        tlsCA: profile.tlsCA ?? '',
        tlsCert: profile.tlsCert ?? '',
        tlsKey: profile.tlsKey ?? '',
        tlsServerName: profile.tlsServerName ?? '',
        socket: profile.socket ?? '',
        engine: profile.engine as unknown as BackendEngine,
    } as never);
}

export interface ConnectResult {
    ok: boolean;
    message: string;
    sshHostFingerprint?: string;
}

export async function connect(sessionId: string, profileId: string, database: string): Promise<ConnectResult> {
    return ConnectionsService.ConnectTarget(sessionId, profileId, database);
}

export async function disconnect(sessionId: string): Promise<void> {
    await ConnectionsService.Disconnect(sessionId);
}

function toResultSet(r: BackendResultSet): ResultSet {
    return {
        kind: r.kind as ResultSet['kind'],
        statement: r.statement,
        columns: r.columns ?? undefined,
        rows: (r.rows ?? undefined) as ResultSet['rows'],
        // No SQL engine populates these yet; carried through unchanged so a
        // future document/key-value engine's results don't need another
        // adapter change here.
        documents: (r.documents ?? undefined) as ResultSet['documents'],
        entries: (r.entries ?? undefined) as ResultSet['entries'],
        truncated: r.truncated,
        rowsAffected: r.rowsAffected,
        lastInsertId: r.lastInsertId as number | string,
        stableOrder: r.stableOrder,
        error: r.error,
        errorCode: r.errorCode,
        connectionLost: r.connectionLost,
    };
}

function toSchemaTable(t: BackendTable): SchemaTable {
    return {
        name: t.name,
        schema: t.schema,
        objectName: t.objectName,
        kind: t.kind as SchemaTable['kind'],
        columns: t.columns ?? [],
        indexes: (t.indexes ?? []).map((idx) => ({ ...idx, columns: idx.columns ?? [] })),
        foreignKeys: t.foreignKeys ?? [],
    };
}

function toSchemaFunction(f: BackendFunction): SchemaFunction {
    return {
        name: f.name,
        schema: f.schema,
        objectName: f.objectName,
        kind: f.kind as SchemaFunction['kind'],
        parameters: f.parameters ?? '',
        returnType: f.returnType ?? '',
        language: f.language ?? '',
        definition: f.definition ?? '',
    };
}

export interface StatementRange {
    text?: string;
    start: number;
    end: number;
}

export async function splitStatements(sessionId: string, sql: string): Promise<StatementRange[]> {
    return (await QueriesService.SplitForSession(sessionId, sql)) ?? [];
}

export interface ExecuteResult {
    results: ResultSet[];
    elapsedMs: number;
}

export async function execute(sessionId: string, documentId: string, jobId: string, sql: string, maxRows: number, timeoutSeconds: number): Promise<ExecuteResult> {
    const result = await QueriesService.ExecuteDocument(sessionId, documentId, jobId, sql, maxRows, timeoutSeconds);
    return { results: (result.results ?? []).map(toResultSet), elapsedMs: result.elapsedMs };
}

export async function aggregate(sessionId: string, documentId: string, jobId: string, ast: VisualQuery, maxRows: number, timeoutSeconds: number): Promise<ExecuteResult> {
    const result = await QueriesService.Aggregate(sessionId, documentId, jobId, ast as never, maxRows, timeoutSeconds);
    return { results: (result.results ?? []).map(toResultSet), elapsedMs: result.elapsedMs };
}

export async function executeNative(sessionId: string, documentId: string, jobId: string, req: string, maxRows: number, timeoutSeconds: number): Promise<ExecuteResult> {
    const result = await QueriesService.ExecuteNativeDocument(sessionId, documentId, jobId, req, maxRows, timeoutSeconds);
    return { results: (result.results ?? []).map(toResultSet), elapsedMs: result.elapsedMs };
}

function toCapabilities(c: BackendCapabilities): Capabilities {
    return { ...c };
}

export async function sessionCapabilities(sessionId: string): Promise<Capabilities> {
    const c = await QueriesService.SessionCapabilities(sessionId);
    return c ? toCapabilities(c) : emptyCapabilities;
}

function toDriverMetadata(d: BackendDriverMetadata): DriverMetadata {
    return { ...d, name: d.name as unknown as Engine, configSchema: d.configSchema ?? [] };
}

export async function listDrivers(): Promise<DriverMetadata[]> {
    return ((await ConnectionsService.ListDrivers()) ?? []).map(toDriverMetadata);
}

function toExplainNode(n: BackendExplainNode): ExplainNode {
    return {
        id: n.id,
        parentId: n.parentId || undefined,
        operation: n.operation,
        relation: n.relation || undefined,
        alias: n.alias || undefined,
        detail: n.detail || undefined,
        filter: n.filter || undefined,
        indexCondition: n.indexCondition || undefined,
        estimatedRows: n.estimatedRows ?? undefined,
        actualRows: n.actualRows ?? undefined,
        loops: n.loops,
        estimatedCost: n.estimatedCost ?? undefined,
        inclusiveTimeMs: n.inclusiveTimeMs ?? undefined,
        exclusiveTimeMs: n.exclusiveTimeMs ?? undefined,
        exclusiveCost: n.exclusiveCost ?? undefined,
        extra: (n.extra ?? undefined) as ExplainNode['extra'],
        badges: (n.badges ?? []).map((b) => ({
            kind: b.kind,
            severity: b.severity as 'info' | 'warning',
            message: b.message,
        })),
        children: (n.children ?? []).filter((c): c is BackendExplainNode => !!c).map(toExplainNode),
    };
}

function toExplainPlan(p: BackendExplainPlan): ExplainPlan {
    return {
        engine: p.engine,
        engineVersion: p.engineVersion || undefined,
        analyzed: p.analyzed,
        root: p.root ? toExplainNode(p.root) : null,
        planningTimeMs: p.planningTimeMs ?? undefined,
        executionTimeMs: p.executionTimeMs ?? undefined,
        rawOutput: p.rawOutput,
        rawFormat: p.rawFormat as 'json' | 'text',
        warnings: p.warnings ?? [],
        stats: {
            operationCounts: (p.stats.operationCounts ?? {}) as Record<string, number>,
            relationsScanned: p.stats.relationsScanned ?? [],
            indexesUsed: p.stats.indexesUsed ?? [],
            nodeCount: p.stats.nodeCount,
            totalCost: p.stats.totalCost ?? undefined,
            totalTimeMs: p.stats.totalTimeMs ?? undefined,
        },
    };
}

export async function explain(sessionId: string, documentId: string, jobId: string, sql: string, analyze: boolean, maxRows: number, timeoutSeconds: number): Promise<ExplainPlan> {
    const plan = await QueriesService.Explain(sessionId, documentId, jobId, sql, analyze, maxRows, timeoutSeconds);
    return toExplainPlan(plan);
}

export async function cancel(jobId: string): Promise<boolean> {
    return QueriesService.Cancel(jobId);
}

export async function placeholders(sessionId: string, sql: string): Promise<string[]> {
    return (await QueriesService.Placeholders(sessionId, sql)) ?? [];
}

export async function executeParameterized(
    sessionId: string,
    documentId: string,
    jobId: string,
    sql: string,
    params: Record<string, string | null>,
    maxRows: number,
    timeoutSeconds: number,
): Promise<ExecuteResult> {
    const result = await QueriesService.ExecuteParameterizedDocument(sessionId, documentId, jobId, sql, params as never, maxRows, timeoutSeconds);
    return { results: (result.results ?? []).map(toResultSet), elapsedMs: result.elapsedMs };
}

export async function discoverRoutines(sessionId: string): Promise<SchemaFunction[]> {
    return ((await QueriesService.DiscoverRoutines(sessionId)) ?? []).map(toSchemaFunction);
}

export async function discoverSchema(sessionId: string): Promise<SchemaTable[]> {
    return ((await QueriesService.DiscoverSchema(sessionId)) ?? []).map(toSchemaTable);
}

export async function discoverSchemaNames(sessionId: string): Promise<SchemaTable[]> {
    return ((await QueriesService.DiscoverSchemaNames(sessionId)) ?? []).map(toSchemaTable);
}

export async function discoverTableDetail(sessionId: string, schema: string, table: string): Promise<SchemaTable> {
    return toSchemaTable(await QueriesService.DiscoverTableDetail(sessionId, schema, table));
}

export async function discoverConstraints(sessionId: string, schema: string, table: string): Promise<Constraint[]> {
    return ((await QueriesService.DiscoverConstraints(sessionId, schema, table)) ?? []) as Constraint[];
}

export async function discoverDefinition(sessionId: string, schema: string, table: string): Promise<Definition> {
    return (await QueriesService.DiscoverDefinition(sessionId, schema, table)) as Definition;
}

export async function closeDocument(sessionId: string, documentId: string) {
    await QueriesService.CloseDocument(sessionId, documentId);
}
export async function browseTable(
    sessionId: string,
    documentId: string,
    jobId: string,
    table: string,
    filter: string,
    filterColumn: string,
    sort: string,
    descending: boolean,
    offset: number,
    pageSize: number,
    timeout: number,
): Promise<ExecuteResult> {
    const result = await QueriesService.BrowseTable(sessionId, documentId, jobId, table, filter, filterColumn, sort, descending, offset, pageSize, timeout);
    return { results: (result.results ?? []).map(toResultSet), elapsedMs: result.elapsedMs };
}

export async function inspectKey(sessionId: string, documentId: string, jobId: string, key: string, cursor: string, count: number, timeout: number): Promise<KeyDetail> {
    const d = await QueriesService.InspectKey(sessionId, documentId, jobId, key, cursor, count, timeout);
    return {
        key: d.key,
        type: d.type,
        exists: d.exists,
        length: d.length,
        ttlMillis: d.ttlMillis ?? undefined,
        value: d.value ?? undefined,
        members: (d.members ?? []) as KeyDetail['members'],
        cursor: d.cursor,
        stream: d.stream ? { ...d.stream, groups: (d.stream.groups ?? []) as StreamGroup[] } : undefined,
    };
}

export async function applyKeyEdit(sessionId: string, documentId: string, edit: KeyEdit): Promise<KeyEditResult> {
    return QueriesService.ApplyKeyEdit(sessionId, documentId, edit);
}

export async function keyValueScan(sessionId: string, documentId: string, jobId: string, prefix: string, group: boolean, limit: number, timeout: number): Promise<ExecuteResult> {
    const result = await QueriesService.KeyValueScan(sessionId, documentId, jobId, prefix, group, limit, timeout);
    return { results: (result.results ?? []).map(toResultSet), elapsedMs: result.elapsedMs };
}

export async function applyTableEdits(sessionId: string, documentId: string, table: string, edits: RowEdit[], timeout: number): Promise<EditResult> {
    const payload = editPayload(edits);
    const result = await QueriesService.ApplyTableEdits(sessionId, documentId, table, payload as never, timeout);
    return { ...result, statements: result.statements ?? [] };
}

function editPayload(edits: RowEdit[]) {
    return edits.map((edit) => ({
        kind: edit.kind,
        original: edit.original ?? {},
        values: edit.values ?? {},
    }));
}

export async function previewTableEdits(sessionId: string, documentId: string, table: string, edits: RowEdit[]): Promise<EditPreview[]> {
    const result = (await QueriesService.PreviewTableEdits(sessionId, documentId, table, editPayload(edits) as never)) ?? [];
    return result.map((preview) => ({
        kind: preview.kind as EditPreview['kind'],
        sql: preview.sql,
        args: preview.args ?? [],
    }));
}

export async function fetchDocumentJSON(sessionId: string, documentId: string, collection: string, id: string): Promise<string> {
    return await QueriesService.FetchDocumentJSON(sessionId, documentId, collection, id);
}

export async function applyDocumentEdit(sessionId: string, documentId: string, collection: string, kind: string, id: string, doc: string): Promise<DocumentEditResult> {
    return await QueriesService.ApplyDocumentEdit(sessionId, documentId, collection, kind, id, doc);
}

export async function beginTransaction(sessionId: string, documentId: string): Promise<void> {
    await QueriesService.BeginTransaction(sessionId, documentId);
}

export async function commitTransaction(sessionId: string, documentId: string): Promise<void> {
    await QueriesService.CommitTransaction(sessionId, documentId);
}

export async function rollbackTransaction(sessionId: string, documentId: string): Promise<void> {
    await QueriesService.RollbackTransaction(sessionId, documentId);
}

export async function importURL(raw: string) {
    const result = await ConnectionsService.ImportURL(raw);
    const { password, ...profile } = result;
    return { profile: toProfile(profile), password: password ?? '' };
}
export interface CompileVisualQueryResult {
    sql: string;
    error: string;
}

export async function compileVisualQuery(sessionId: string, ast: VisualQuery): Promise<CompileVisualQueryResult> {
    try {
        const sql = await QueriesService.CompileVisualQuery(sessionId, ast as never);
        return { sql, error: '' };
    } catch (err) {
        return { sql: '', error: String(err) };
    }
}

export async function listDatabases(sessionId: string) {
    return (await QueriesService.ListDatabases(sessionId)) ?? [];
}
export async function openDatabase(sourceId: string, sessionId: string, database: string) {
    return ConnectionsService.OpenDatabase(sourceId, sessionId, database);
}

export async function chooseExportDestination(defaultName: string, filterLabel: string, filterPattern: string): Promise<string> {
    return ExportService.ChooseExportDestination(defaultName, filterLabel, filterPattern);
}
export async function saveBytes(path: string, base64: string): Promise<void> {
    await ExportService.SaveBytes(path, base64);
}
export async function startExport(sessionId: string, documentId: string, jobId: string, destination: string, request: ExportRequest): Promise<void> {
    await ExportService.StartExport(sessionId, documentId, jobId, destination, request as never);
}
export async function exportProgress(jobId: string): Promise<ExportProgress> {
    return ExportService.Progress(jobId);
}

export async function chooseImportFile(): Promise<string> {
    return ImportService.ChooseImportFile();
}
export async function previewCSV(path: string, delimiter: string, encoding: string, hasHeader: boolean): Promise<CSVPreview> {
    const result = await ImportService.PreviewCSV(path, delimiter, encoding, hasHeader);
    return {
        ...result,
        columns: result.columns ?? [],
        sample: (result.sample ?? []) as string[][],
    };
}
export async function validateCSV(sessionId: string, path: string, table: string, mapping: ColumnMapping[], opts: ImportOptions): Promise<ImportValidation> {
    const result = await ImportService.ValidateCSV(sessionId, path, table, mapping as never, opts);
    return { ...result, issues: result.issues ?? [], uncheckedColumns: result.uncheckedColumns ?? [] } as ImportValidation;
}
export async function startImport(sessionId: string, documentId: string, jobId: string, path: string, table: string, mapping: ColumnMapping[], opts: ImportOptions): Promise<void> {
    await ImportService.StartImport(sessionId, documentId, jobId, path, table, mapping as never, opts);
}
export async function importProgress(jobId: string): Promise<ImportProgress> {
    const result = await ImportService.Progress(jobId);
    return { ...result, errors: result.errors ?? [] };
}

// Desktop integration. Each call degrades instead of throwing where a plain
// browser preview (no Go backend) has to keep working.
export async function desktopChrome(): Promise<DesktopChrome | null> {
    try {
        return (await DesktopService.Chrome()) as DesktopChrome;
    } catch {
        return null;
    }
}
export async function setTitleBar(titleBar: DesktopChrome['titleBar']): Promise<DesktopChrome> {
    return (await DesktopService.SetTitleBar(titleBar)) as DesktopChrome;
}
export async function setAppIcon(appIcon: DesktopChrome['appIcon']): Promise<DesktopChrome> {
    return (await DesktopService.SetAppIcon(appIcon)) as DesktopChrome;
}
export async function setWindowBackground(hex: string): Promise<void> {
    await DesktopService.SetBackground(hex);
}
/** False when there is no native clipboard (server mode, browser preview). */
export async function copyText(text: string): Promise<boolean> {
    try {
        return await DesktopService.CopyText(text);
    } catch {
        return false;
    }
}

export async function appInfo(): Promise<AppInfo | null> {
    try {
        return await AppService.Info();
    } catch {
        return null;
    }
}
export async function checkForUpdate(): Promise<UpdateInfo> {
    return AppService.CheckForUpdate();
}
/** Opens a URL in the user's browser, not inside the app's webview. */
export async function openExternal(url: string): Promise<void> {
    try {
        await Browser.OpenURL(url);
    } catch {
        window.open(url, '_blank', 'noopener');
    }
}
