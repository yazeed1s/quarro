import { format as formatSql, type SqlLanguage } from 'sql-formatter';
import { objectLabel, emptyVisualQuery, emptyCapabilities } from './types';
import { appStage, workspaceStorageKey } from './stage';
import { parseUpdateCache, shouldCheck, visibleUpdate, type UpdateCache } from './updates';
import { defaultChartSeries } from '../features/charts/aggregate';
import { findNodeRangesInDocument } from '../features/explain/reveal';
import { mergeKeyPage } from '../features/keyvalue/pages';
import { commandLines, commandNamesFrom } from '../features/editor/redisCommand';
import * as backend from './backend';
import type {
    Document,
    KeyEdit,
    KeyEditResult,
    Preferences,
    DesktopChrome,
    AppInfo,
    UpdateInfo,
    Profile,
    Session,
    HistoryEntry,
    SavedQuery,
    ResultSet,
    GridLayout,
    Row,
    CellValue,
    Column,
    SchemaTable,
    SchemaForeignKey,
    ExportRequest,
    ExportProgress,
    CSVPreview,
    ColumnMapping,
    ImportOptions,
    ImportProgress,
    ImportValidation,
    ChartType,
    ChartSeriesConfig,
    ExplainNode,
    Theme,
    DriverMetadata,
    Engine,
    EditPreview,
} from './types';
const validThemes: Theme[] = ['system', 'monochrome-light', 'monochrome'];
// Keyed by DriverMetadata.sqlDialect (backend truth), not the engine name:
// sql-formatter names the same dialects differently ("postgresql", not
// "postgres").
const formatDialects: Record<string, SqlLanguage> = {
    postgres: 'postgresql',
    mysql: 'mysql',
    sqlite: 'sqlite',
};
export function gridLayoutKey(profileId: string, object: string): string {
    return JSON.stringify([profileId, object]);
}
function defaultGridLayout(): GridLayout {
    return { widths: {}, order: [], hidden: [], sort: '', descending: false, filter: '' };
}
function summarizeResults(results: ResultSet[], elapsedMs: number): string {
    const error = results.find((r) => r.kind === 'error');
    if (error) return error.error || 'The query failed.';
    const parts: string[] = [];
    for (const r of results) {
        if (r.kind === 'rows') parts.push(`${r.rows?.length ?? 0}${r.truncated ? '+' : ''} rows`);
        else if (r.kind === 'status') parts.push(`${r.rowsAffected ?? 0} rows affected`);
    }
    return parts.length ? `${parts.join(' · ')} · ${elapsedMs} ms` : `Completed in ${elapsedMs} ms.`;
}
export const defaultPreferences: Preferences = {
    theme: 'monochrome',
    uiFont: 'Inter',
    uiSize: 13,
    density: 'comfortable',
    zoom: 100,
    editorFont: 'monospace',
    editorSize: 14,
    lineHeight: 1.65,
    ligatures: false,
    tabSize: 2,
    indentTabs: false,
    lineNumbers: true,
    wordWrap: false,
    activeLine: true,
    gridFont: 'Inter',
    gridSize: 13,
    rowHeight: 34,
    stripes: false,
    nullDisplay: 'NULL',
    dateFormat: 'iso',
    timezone: 'UTC',
    reducedMotion: false,
    restore: true,
    resultLimit: 500,
    timeout: 30,
    historyDays: 30,
    rememberHistory: true,
    connectionBehavior: 'new',
    titleBar: 'system',
    appIcon: 'dark',
    checkUpdates: true,
};
const storageKey = workspaceStorageKey;
const updateStorageKey = `quarro.${appStage}.updates`;
const id = () => crypto.randomUUID();
export function makeDocument(kind: 'query' | 'table' | 'diagram' | 'visualQuery' | 'chart' | 'explain' = 'query', object = 'users', title = 'Query 1'): Document {
    return {
        id: id(),
        kind,
        object,
        title: kind === 'table' ? objectLabel(object) : kind === 'diagram' ? 'Schema diagram' : kind === 'visualQuery' ? 'Visual query' : title,
        sql: '',
        view: 'data',
        filter: '',
        sort: '',
        descending: false,
        selected: -1,
        selectedColumn: '',
        hasResults: kind === 'table',
        status: 'idle',
        message: '',
        elapsed: 0,
        results: [],
        activeResult: 0,
        jobId: '',
        selectionFrom: 0,
        selectionTo: 0,
        cursor: 0,
        tableFilter: '',
        tableFilterColumn: '',
        viewVersion: 0,
        edits: [],
        transactionActive: false,
        outputCollapsed: false,
        diagramSearch: '',
        diagramSchema: '',
        diagramRelated: '',
        diagramPositions: {},
        diagramCollapsed: [],
        visualQueryAst: emptyVisualQuery(),
        visualQuerySql: '',
        visualQueryError: '',
        visualQueryPositions: {},
        chartSeries: [],
        paramPrompt: [],
        paramValues: {},
        explainSourceDocumentId: '',
        explainSql: '',
        explainMode: 'estimate',
        explainPlan: null,
        explainView: 'graph',
        explainSelectedNodeId: '',
        revealSeq: 0,
        keyDetail: null,
        keyLoading: false,
        keyError: '',
    };
}
function schemaColumnsForChart(table: SchemaTable): Column[] {
    return table.columns.map((column) => ({ name: column.name, declaredType: column.type }));
}
// The `[A-Za-z0-9_]` run touching `pos`, for Go to Definition — a plain
// word lookup rather than QueryEditor's own alias-aware resolver (which
// needs a live CodeMirror syntax tree this state module has no reason to
// depend on). Good enough for "jump to the table or column named here";
// alias-qualified precision stays in the editor's hover tooltips.
function wordAt(text: string, pos: number): string {
    const isWord = (c: string) => /[A-Za-z0-9_]/.test(c);
    let start = pos;
    let end = pos;
    while (start > 0 && isWord(text[start - 1])) start--;
    while (end < text.length && isWord(text[end])) end++;
    return text.slice(start, end);
}
function findSchemaObject(tables: SchemaTable[], word: string): SchemaTable | undefined {
    if (!word) return undefined;
    const lower = word.toLowerCase();
    return (
        tables.find((t) => t.name.toLowerCase() === lower || (t.objectName ?? '').toLowerCase() === lower) ??
        tables.find((t) => t.columns.some((c) => c.name.toLowerCase() === lower))
    );
}
// queryChartSource/tableChartSource build one series' ChartSource from a
// query document's loaded results or a schema table/collection, tagged with
// the session it actually came from. Shared by the single-series "Visualize"
// entry points and by "Add series", which needs the same source shapes for
// a session other than the chart document's own.
function queryChartSource(session: Session, source: Document, sourceTitle: string): ChartSeriesConfig['source'] | undefined {
    const result = source.results[source.activeResult];
    if (source.kind !== 'query' || result?.kind !== 'rows' || !result.rows?.length) return undefined;
    return {
        kind: 'snapshot',
        sessionId: session.id,
        sourceDocumentId: source.id,
        sourceTitle,
        columns: result.columns ?? [],
        rows: result.rows,
    };
}
async function tableChartSource(session: Session, tableName: string, source: Document | undefined, sourceTitle: string): Promise<ChartSeriesConfig['source'] | undefined> {
    const table = session.schema.find((item) => item.name === tableName);
    if (!table || (table.kind !== 'table' && table.kind !== 'view' && table.kind !== 'collection')) {
        return undefined;
    }
    const loaded = source?.kind === 'table' && source.results[source.activeResult]?.kind === 'rows' ? (source.results[source.activeResult].rows ?? []) : [];
    let columns = schemaColumnsForChart(table);
    let rows = loaded;
    // A schemaless collection (MongoDB) never has a declared column list (its
    // documents can each have different fields) — discover one from a small
    // sample browse instead, the same union-of-keys shape the table grid
    // already uses for it. SQL engines always have declared columns, so this
    // extra round trip never runs for them.
    if (!columns.length) {
        try {
            const result = await backend.browseTable(session.id, id(), id(), table.name, '', '', '', false, 0, 50, 30);
            const sample = result.results[0];
            if (sample?.kind !== 'rows' || !sample.columns?.length) return undefined;
            columns = sample.columns;
            if (!rows.length) rows = sample.rows ?? [];
        } catch {
            return undefined;
        }
    }
    if (!columns.length) return undefined;
    return {
        kind: 'table',
        sessionId: session.id,
        sourceDocumentId: source?.id,
        sourceTitle,
        object: table.name,
        columns,
        rows,
    };
}
// Seeded into the demo SQLite profile's first query tab only, so a first
// run has something to look at instead of a blank editor.
const demoSQL = `-- A little clarity in your data.\nSELECT\n  id, name, email, role, status,\n  created_at, last_login\nFROM users\nORDER BY id;`;
function makeSession(profile: Profile): Session {
    const query = makeDocument();
    if (profile.id === 'demo-sqlite') query.sql = demoSQL;
    return {
        id: id(),
        profile: { ...profile },
        activeDocument: query.id,
        documents: [query],
        connectionState: 'connecting',
        connectionError: '',
        schema: [],
        schemaLoading: false,
        schemaFullyLoaded: false,
        schemaFullLoading: false,
        schemaVersion: 0,
        tableDetails: {},
        tableDetailLoading: {},
        tableDetailErrors: {},
        tableConstraints: {},
        tableConstraintsLoading: {},
        tableConstraintsErrors: {},
        tableDefinitions: {},
        tableDefinitionsLoading: {},
        tableDefinitionsErrors: {},
        routines: [],
        capabilities: emptyCapabilities,
        sidebarWidth: 232,
        editorHeight: 255,
        inspectorWidth: 300,
        inspector: false,
        // Sessions can now be created asynchronously (after connecting), well
        // after App.svelte's one-time mount-time narrow-viewport collapse so
        // each new session must decide its own initial sidebar visibility.
        sidebar: typeof window === 'undefined' || !window.matchMedia('(max-width: 1000px)').matches,
    };
}
function restore(): {
    preferences: Preferences;
    sessions: Session[];
    active: string;
    history: HistoryEntry[];
    saved: SavedQuery[];
    gridLayouts: Record<string, GridLayout>;
} {
    const fallback = {
        preferences: { ...defaultPreferences },
        sessions: [] as Session[],
        active: '',
        history: [] as HistoryEntry[],
        saved: [] as SavedQuery[],
        gridLayouts: {},
    };
    try {
        const raw = JSON.parse(localStorage.getItem(storageKey) || 'null');
        if (!raw || ![1, 2, 3, 4].includes(raw.version)) return fallback;
        const preferences = { ...defaultPreferences, ...raw.preferences };
        if (!validThemes.includes(preferences.theme)) preferences.theme = defaultPreferences.theme;
        if (!['system', 'custom'].includes(preferences.titleBar)) preferences.titleBar = defaultPreferences.titleBar;
        if (!['dark', 'light'].includes(preferences.appIcon)) preferences.appIcon = defaultPreferences.appIcon;
        // The old Compact spacing is now Comfortable. Migrate once so existing
        // workspaces keep that baseline; future Compact selections stay compact.
        if (raw.densityVersion !== 2 || !['compact', 'comfortable'].includes(preferences.density)) preferences.density = 'comfortable';
        // Validate persisted preferences before applying CSS or query limits.
        for (const [key, min, max] of [
            ['uiSize', 11, 20],
            ['zoom', 75, 150],
            ['editorSize', 11, 24],
            ['lineHeight', 1.2, 2.2],
            ['tabSize', 1, 8],
            ['gridSize', 11, 20],
            ['rowHeight', 28, 52],
            ['resultLimit', 1, 10000],
            ['timeout', 1, 300],
            ['historyDays', 1, 365],
        ] as const)
            preferences[key] = Math.min(max, Math.max(min, Number(preferences[key]) || defaultPreferences[key]));
        const restored = preferences.restore && Array.isArray(raw.sessions) ? raw.sessions : fallback.sessions;
        // Sessions restore disconnected: never auto-reconnect or replay queries
        // on startup. The user reconnects each session explicitly.
        for (const s of restored) {
            s.connectionState = 'disconnected';
            s.connectionError = '';
            s.schema = [];
            s.schemaLoading = false;
            s.schemaFullyLoaded = false;
            s.schemaFullLoading = false;
            s.schemaVersion = 0;
            s.tableDetails = {};
            s.tableDetailLoading = {};
            s.tableDetailErrors = {};
            s.tableConstraints = {};
            s.tableConstraintsLoading = {};
            s.tableConstraintsErrors = {};
            s.tableDefinitions = {};
            s.tableDefinitionsLoading = {};
            s.tableDefinitionsErrors = {};
            s.routines = [];
            s.capabilities = emptyCapabilities;
            for (const d of s.documents) {
                d.results = [];
                d.keyDetail = null;
                d.keyLoading = false;
                d.keyError = '';
                d.jobId = '';
                d.hasResults = false;
                d.tableFilter ??= '';
                d.tableFilterColumn ??= '';
                d.viewVersion ??= 0;
                d.edits = [];
                d.transactionActive = false;
                d.outputCollapsed ??= false;
                d.diagramSearch ??= '';
                d.diagramSchema ??= '';
                d.diagramRelated ??= '';
                d.diagramPositions ??= {};
                d.diagramCollapsed ??= [];
                d.visualQueryAst ??= emptyVisualQuery();
                d.visualQuerySql ??= '';
                d.visualQueryError ??= '';
                d.visualQueryPositions ??= {};
                // Older persisted chart series predate ChartSource.sessionId and
                // ChartSeriesConfig.binCount (Phases F/G); backfill both so a chart
                // saved before either existed keeps resolving/rendering unchanged.
                // Fetched rows are never meant to survive a restart (Phase H;
                // persistenceSnapshot() already strips them going forward) — clear
                // them here too so a legacy blob saved before that strip existed
                // shrinks immediately on the next save instead of persisting a
                // multi-megabyte snapshot forever. A 'snapshot' series with no rows
                // is exactly the signal ChartDocument.svelte's resolveSeries uses to
                // show "needs refresh" instead of a misleadingly-empty chart.
                d.chartSeries = (d.chartSeries ?? []).map((series: ChartSeriesConfig) => ({
                    ...series,
                    source: {
                        ...series.source,
                        sessionId: series.source.sessionId ?? s.id,
                        rows: [],
                    },
                    binCount: series.binCount ?? 10,
                }));
                d.paramPrompt = [];
                d.paramValues ??= {};
                d.explainSourceDocumentId ??= '';
                d.explainSql ??= '';
                d.explainMode ??= 'estimate';
                d.explainPlan ??= null;
                d.explainView ??= 'graph';
                d.explainSelectedNodeId ??= '';
                d.revealSeq ??= 0;
                if (d.status === 'running') {
                    d.status = 'cancelled';
                    d.message = 'Interrupted by restart. Run it again when ready.';
                } else {
                    d.status = 'idle';
                }
            }
        }
        return {
            preferences,
            sessions: restored,
            active: restored.some((s: Session) => s.id === raw.active) ? raw.active : (restored[0]?.id ?? ''),
            history: (raw.history ?? [])
                .filter((h: HistoryEntry) => h.at > Date.now() - preferences.historyDays * 86400000)
                .map((h: HistoryEntry) => ({
                    ...h,
                    durationMs: h.durationMs ?? 0,
                    outcome: h.outcome ?? 'ok',
                })),
            saved: (raw.saved ?? []).map((q: SavedQuery) => ({
                ...q,
                folder: q.folder ?? '',
                favorite: q.favorite ?? false,
            })),
            gridLayouts: raw.gridLayouts ?? {},
        };
    } catch {
        return fallback;
    }
}
const initial = restore();
class Workspace {
    preferences = $state<Preferences>(initial.preferences);
    profiles = $state<Profile[]>([]);
    profilesLoading = $state(true);
    drivers = $state<DriverMetadata[]>([]);
    sessions = $state<Session[]>(initial.sessions);
    active = $state(initial.active);
    history = $state<HistoryEntry[]>(initial.history);
    saved = $state<SavedQuery[]>(initial.saved);
    gridLayouts = $state<Record<string, GridLayout>>(initial.gridLayouts);
    settings = $state(false);
    connections = $state(false);
    commandPalette = $state(false);
    notice = $state('');
    storageError = $state(false);
    /** Null until DesktopService answers, and in a plain browser preview. */
    desktop = $state<DesktopChrome | null>(null);
    appInfo = $state<AppInfo | null>(null);
    /** A newer release to announce; null when there is none or it was dismissed. */
    update = $state<UpdateInfo | null>(null);
    private generations = new Map<string, number>();
    private owns(session: Session) {
        return this.sessions.some((s) => s.id === session.id);
    }
    get session() {
        return this.sessions.find((s) => s.id === this.active);
    }
    get document() {
        return this.session?.documents.find((d) => d.id === this.session?.activeDocument);
    }
    notify(message: string) {
        this.notice = message;
    }
    getGridLayout(key: string): GridLayout {
        return this.gridLayouts[key] ?? defaultGridLayout();
    }
    updateGridLayout(key: string, patch: Partial<GridLayout>) {
        this.gridLayouts[key] = { ...this.getGridLayout(key), ...patch };
    }
    async loadProfiles() {
        this.profilesLoading = true;
        try {
            this.profiles = await backend.listProfiles();
            if (appStage !== 'prod' && this.sessions.length === 0) {
                const demo = this.profiles.find((p) => p.id === 'demo-sqlite');
                if (demo) await this.openProfile(demo);
            }
        } catch {
            this.profiles = [];
            this.notify('Connecting to the application backend failed. Run this app with wails3 dev or a packaged build to manage real connections.');
        } finally {
            this.profilesLoading = false;
        }
    }
    async loadDesktop() {
        this.desktop = await backend.desktopChrome();
        // Go's window.json decides how the window was created, so it wins
        // over the localStorage mirror (which clearing site data can reset).
        if (this.desktop?.configurable) this.preferences.titleBar = this.desktop.titleBar;
        if (this.desktop?.appIconConfigurable) this.preferences.appIcon = this.desktop.appIcon;
    }
    async applyTitleBar(titleBar: Preferences['titleBar']) {
        const current = this.desktop;
        if (!current?.configurable || current.titleBar === titleBar) return;
        try {
            this.desktop = await backend.setTitleBar(titleBar);
        } catch (error) {
            this.preferences.titleBar = current.titleBar;
            this.notify(`Changing the title bar failed: ${error instanceof Error ? error.message : String(error)}`);
        }
    }
    async applyAppIcon(appIcon: Preferences['appIcon']) {
        const current = this.desktop;
        if (!current?.appIconConfigurable || current.appIcon === appIcon) return;
        try {
            this.desktop = await backend.setAppIcon(appIcon);
        } catch (error) {
            this.preferences.appIcon = current.appIcon;
            this.notify(`Changing the app icon failed: ${error instanceof Error ? error.message : String(error)}`);
        }
    }
    async loadAppInfo() {
        this.appInfo = await backend.appInfo();
    }
    // The update cache is a per-device convenience, so it lives outside the
    // workspace snapshot, and losing it only means one extra check.
    private readUpdateCache(): UpdateCache {
        try {
            return parseUpdateCache(localStorage.getItem(updateStorageKey));
        } catch {
            return parseUpdateCache(null);
        }
    }
    private writeUpdateCache(cache: UpdateCache) {
        try {
            localStorage.setItem(updateStorageKey, JSON.stringify(cache));
        } catch {
            // Storage unavailable: the next launch simply checks again.
        }
    }
    async checkForUpdates() {
        if (!this.preferences.checkUpdates) {
            this.update = null;
            return;
        }
        let cache = this.readUpdateCache();
        const now = Date.now();
        if (shouldCheck(cache, now)) {
            try {
                const latest = await backend.checkForUpdate();
                cache = { ...cache, checkedAt: now, latest: latest.available ? latest : null };
                this.writeUpdateCache(cache);
            } catch {
                // Offline or rate-limited: stay quiet and retry next launch.
                return;
            }
        }
        if (!this.preferences.checkUpdates) return;
        this.update = visibleUpdate(cache, this.appInfo?.version ?? '');
    }
    dismissUpdate() {
        if (!this.update) return;
        this.writeUpdateCache({ ...this.readUpdateCache(), dismissed: this.update.version });
        this.update = null;
    }
    async loadDrivers() {
        try {
            this.drivers = await backend.listDrivers();
        } catch {
            this.drivers = [];
        }
    }
    engineDisplayName(engine: Engine): string {
        return this.drivers.find((d) => d.name === engine)?.displayName ?? engine;
    }
    async loadCapabilities(session: Session) {
        const generation = this.generations.get(session.id);
        try {
            const capabilities = await backend.sessionCapabilities(session.id);
            if (this.owns(session) && this.generations.get(session.id) === generation) session.capabilities = capabilities;
        } catch {
            if (this.owns(session) && this.generations.get(session.id) === generation) session.capabilities = emptyCapabilities;
        }
    }
    async loadSchema(session: Session) {
        const generation = this.generations.get(session.id);
        const schemaVersion = session.schemaVersion + 1;
        session.schemaVersion = schemaVersion;
        session.schemaLoading = true;
        session.schemaFullyLoaded = false;
        session.schemaFullLoading = false;
        session.tableDetails = {};
        session.tableDetailLoading = {};
        session.tableDetailErrors = {};
        session.tableConstraints = {};
        session.tableConstraintsLoading = {};
        session.tableConstraintsErrors = {};
        session.tableDefinitions = {};
        session.tableDefinitionsLoading = {};
        session.tableDefinitionsErrors = {};
        try {
            const schema = session.capabilities.lazyDiscoverer ? await backend.discoverSchemaNames(session.id) : await backend.discoverSchema(session.id);
            if (this.owns(session) && this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) {
                session.schema = schema;
                if (!session.capabilities.lazyDiscoverer) {
                    session.schemaFullyLoaded = true;
                    session.tableDetails = Object.fromEntries(schema.map((table) => [table.name, table]));
                }
            }
        } catch (error) {
            if (this.owns(session) && this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) {
                session.schema = [];
                this.notify(`Schema refresh failed: ${String(error)}`);
            }
        } finally {
            if (this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) session.schemaLoading = false;
        }
        if (session.documents.some((doc) => doc.kind === 'diagram' || doc.kind === 'visualQuery')) await this.loadFullSchema(session);
        if (session.capabilities.routineDiscoverer) {
            try {
                const routines = await backend.discoverRoutines(session.id);
                if (this.owns(session) && this.generations.get(session.id) === generation) session.routines = routines;
            } catch {
                if (this.owns(session) && this.generations.get(session.id) === generation) session.routines = [];
            }
        } else if (this.owns(session) && this.generations.get(session.id) === generation) session.routines = [];
    }
    async loadFullSchema(session: Session, refresh = false) {
        if (!session.capabilities.lazyDiscoverer) {
            if (refresh) await this.loadSchema(session);
            return;
        }
        if (session.schemaFullyLoaded && !refresh) return;
        if (session.schemaFullLoading) return;
        const generation = this.generations.get(session.id);
        if (refresh) {
            session.schemaVersion += 1;
            session.tableDetails = {};
            session.tableDetailLoading = {};
            session.tableDetailErrors = {};
            session.tableConstraints = {};
            session.tableConstraintsLoading = {};
            session.tableConstraintsErrors = {};
            session.tableDefinitions = {};
            session.tableDefinitionsLoading = {};
            session.tableDefinitionsErrors = {};
            session.schemaFullyLoaded = false;
        }
        const schemaVersion = session.schemaVersion;
        session.schemaFullLoading = true;
        try {
            const schema = await backend.discoverSchema(session.id);
            if (!this.owns(session) || this.generations.get(session.id) !== generation || session.schemaVersion !== schemaVersion) return;
            session.schema = schema;
            session.tableDetails = Object.fromEntries(schema.map((table) => [table.name, table]));
            session.schemaFullyLoaded = true;
        } catch (error) {
            if (this.owns(session) && this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion)
                this.notify(`Full schema refresh failed: ${String(error)}`);
        } finally {
            if (this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) session.schemaFullLoading = false;
        }
    }
    async loadTableDetail(session: Session, object: string) {
        if (!session.capabilities.lazyDiscoverer || session.tableDetails[object] || session.tableDetailLoading[object] || session.tableDetailErrors[object]) return;
        const summary = session.schema.find((table) => table.name === object);
        if (!summary?.schema || !summary.objectName) return;
        const generation = this.generations.get(session.id);
        const schemaVersion = session.schemaVersion;
        session.tableDetailLoading = { ...session.tableDetailLoading, [object]: true };
        const errors = { ...session.tableDetailErrors };
        delete errors[object];
        session.tableDetailErrors = errors;
        try {
            const detail = await backend.discoverTableDetail(session.id, summary.schema, summary.objectName);
            if (!this.owns(session) || this.generations.get(session.id) !== generation || session.schemaVersion !== schemaVersion) return;
            session.tableDetails = { ...session.tableDetails, [object]: detail };
            session.schema = session.schema.map((table) => (table.name === object ? detail : table));
        } catch (error) {
            if (this.owns(session) && this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) {
                session.tableDetailErrors = { ...session.tableDetailErrors, [object]: String(error) };
                this.notify(`Table metadata failed: ${String(error)}`);
            }
        } finally {
            if (this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) {
                const loading = { ...session.tableDetailLoading };
                delete loading[object];
                session.tableDetailLoading = loading;
            }
        }
    }
    // Fetched only when a table document's Structure tab is actually
    // opened, same lazy-per-table trigger loadTableDetail already uses, not
    // part of the eager/names-only schema walk. Only PostgreSQL/MySQL
    // implement this, gated by capabilities.constraintDiscoverer.
    async loadTableConstraints(session: Session, object: string) {
        if (!session.capabilities.constraintDiscoverer || session.tableConstraints[object] || session.tableConstraintsLoading[object] || session.tableConstraintsErrors[object])
            return;
        const summary = session.schema.find((table) => table.name === object);
        if (!summary) return;
        const schema = summary.schema ?? '';
        const tableName = summary.objectName || object;
        const generation = this.generations.get(session.id);
        const schemaVersion = session.schemaVersion;
        session.tableConstraintsLoading = { ...session.tableConstraintsLoading, [object]: true };
        const errors = { ...session.tableConstraintsErrors };
        delete errors[object];
        session.tableConstraintsErrors = errors;
        try {
            const constraints = await backend.discoverConstraints(session.id, schema, tableName);
            if (!this.owns(session) || this.generations.get(session.id) !== generation || session.schemaVersion !== schemaVersion) return;
            session.tableConstraints = { ...session.tableConstraints, [object]: constraints };
        } catch (error) {
            if (this.owns(session) && this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) {
                session.tableConstraintsErrors = { ...session.tableConstraintsErrors, [object]: String(error) };
            }
        } finally {
            if (this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) {
                const loading = { ...session.tableConstraintsLoading };
                delete loading[object];
                session.tableConstraintsLoading = loading;
            }
        }
    }
    // Fetched only when the user explicitly opens "View Definition" — never
    // automatically, since it is a secondary inspection action, not part of
    // the normal Structure/Indexes/Relationships browsing flow.
    async loadTableDefinition(session: Session, object: string) {
        if (!session.capabilities.definitionDiscoverer || session.tableDefinitions[object] || session.tableDefinitionsLoading[object] || session.tableDefinitionsErrors[object])
            return;
        const summary = session.schema.find((table) => table.name === object);
        if (!summary) return;
        const schema = summary.schema ?? '';
        const tableName = summary.objectName || object;
        const generation = this.generations.get(session.id);
        const schemaVersion = session.schemaVersion;
        session.tableDefinitionsLoading = { ...session.tableDefinitionsLoading, [object]: true };
        const errors = { ...session.tableDefinitionsErrors };
        delete errors[object];
        session.tableDefinitionsErrors = errors;
        try {
            const definition = await backend.discoverDefinition(session.id, schema, tableName);
            if (!this.owns(session) || this.generations.get(session.id) !== generation || session.schemaVersion !== schemaVersion) return;
            session.tableDefinitions = { ...session.tableDefinitions, [object]: definition };
        } catch (error) {
            if (this.owns(session) && this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) {
                session.tableDefinitionsErrors = { ...session.tableDefinitionsErrors, [object]: String(error) };
            }
        } finally {
            if (this.generations.get(session.id) === generation && session.schemaVersion === schemaVersion) {
                const loading = { ...session.tableDefinitionsLoading };
                delete loading[object];
                session.tableDefinitionsLoading = loading;
            }
        }
    }
    persistenceSnapshot() {
        return {
            version: 4,
            densityVersion: 2,
            preferences: this.preferences,
            sessions: this.preferences.restore
                ? this.sessions.map((session) => ({
                      id: session.id,
                      profile: session.profile,
                      activeDocument: session.activeDocument,
                      sidebarWidth: session.sidebarWidth,
                      editorHeight: session.editorHeight,
                      inspectorWidth: session.inspectorWidth,
                      sidebar: session.sidebar,
                      inspector: session.inspector,
                      documents: session.documents.map((doc) => ({
                          id: doc.id,
                          kind: doc.kind,
                          title: doc.title,
                          sql: doc.sql,
                          object: doc.object,
                          view: doc.view,
                          filter: doc.filter,
                          sort: doc.sort,
                          descending: doc.descending,
                          selected: -1,
                          selectedColumn: '',
                          status: 'idle',
                          message: '',
                          elapsed: 0,
                          activeResult: 0,
                          selectionFrom: 0,
                          selectionTo: 0,
                          cursor: 0,
                          tableFilter: doc.tableFilter,
                          viewVersion: 0,
                          edits: [],
                          transactionActive: false,
                          outputCollapsed: doc.outputCollapsed ?? false,
                          diagramSearch: doc.diagramSearch ?? '',
                          diagramSchema: doc.diagramSchema ?? '',
                          diagramRelated: doc.diagramRelated ?? '',
                          diagramPositions: doc.diagramPositions ?? {},
                          diagramCollapsed: doc.diagramCollapsed ?? [],
                          visualQueryAst: doc.visualQueryAst ?? emptyVisualQuery(),
                          visualQuerySql: doc.visualQuerySql ?? '',
                          visualQueryError: doc.visualQueryError ?? '',
                          visualQueryPositions: doc.visualQueryPositions ?? {},
                          // Persist a series' full config (source identity, columns,
                          // chart type, field roles, aggregate/bin settings) but never
                          // its fetched rows: a table document's rows already stay out
                          // of localStorage, and a chart snapshot can be considerably
                          // larger (measured ~1.7 MB for 10,000 rows of a typical
                          // seven-column table) — multiplied across several open charts,
                          // that risks blowing the whole origin's localStorage quota and
                          // silently breaking persistence for the entire workspace, not
                          // just this chart. A restored 'snapshot' series with no rows
                          // is what ChartDocument.svelte's "Refresh" action re-resolves.
                          chartSeries: (doc.chartSeries ?? []).map((series) => ({
                              ...series,
                              source: { ...series.source, rows: [] },
                          })),
                          // An EXPLAIN plan's own serialized size is a tree of tens,
                          // not thousands, of small node objects (typically tens of KB
                          // even for a real multi-join query). Nowhere near a chart
                          // snapshot's row-count-driven size, so it is persisted as-is
                          // rather than stripped like chartSeries' rows above.
                          explainSourceDocumentId: doc.explainSourceDocumentId ?? '',
                          explainSql: doc.explainSql ?? '',
                          explainMode: doc.explainMode ?? 'estimate',
                          explainPlan: doc.explainPlan ?? null,
                          explainView: doc.explainView ?? 'graph',
                          explainSelectedNodeId: '',
                          revealSeq: 0,
                      })),
                  }))
                : [],
            active: this.active,
            history: this.preferences.rememberHistory ? this.history.filter((h) => h.at > Date.now() - this.preferences.historyDays * 86400000) : [],
            saved: this.saved,
            gridLayouts: this.gridLayouts,
        };
    }
    persist() {
        try {
            localStorage.setItem(storageKey, JSON.stringify(this.persistenceSnapshot()));
            this.storageError = false;
        } catch {
            this.storageError = true;
        }
    }
    async openDatabase(source: Session, database: string) {
        const session = makeSession({
            ...source.profile,
            database,
            name: `${source.profile.name} / ${database}`,
        });
        this.sessions.push(session);
        this.active = session.id;
        const target = this.sessions.find((s) => s.id === session.id)!;
        this.generations.set(target.id, 1);
        try {
            const result = await backend.openDatabase(source.id, target.id, database);
            if (!this.owns(target)) {
                await backend.disconnect(target.id);
                return;
            }
            if (!result.ok) throw new Error(result.message);
            await this.loadCapabilities(target);
            await this.loadSchema(target);
            if (this.owns(target) && this.generations.get(target.id) === 1) target.connectionState = 'connected';
        } catch (e) {
            if (this.owns(target) && this.generations.get(target.id) === 1) {
                target.connectionState = 'error';
                target.connectionError = String(e);
            }
        }
    }
    async openProfile(profile: Profile) {
        const existing = this.sessions.find((s) => s.profile.id === profile.id);
        if (existing && this.preferences.connectionBehavior === 'reuse') {
            this.active = existing.id;
            this.connections = false;
            if (existing.connectionState !== 'connected') await this.reconnect(existing);
            return;
        }
        const session = makeSession(profile);
        this.sessions.push(session);
        this.active = session.id;
        this.connections = false;
        // Re-fetch the just-pushed session: .push() wraps it into a new
        // reactive proxy distinct from the local `session` reference, so
        // mutating `session` further wouldn't be observed by the UI.
        const reactive = this.sessions.find((s) => s.id === session.id)!;
        await this.establish(reactive);
    }
    private async establish(session: Session) {
        const generation = (this.generations.get(session.id) ?? 0) + 1;
        this.generations.set(session.id, generation);
        for (const doc of session.documents) this.stop(doc);
        session.connectionState = 'connecting';
        session.connectionError = '';
        session.capabilities = emptyCapabilities;
        try {
            const result = await backend.connect(session.id, session.profile.id, session.profile.database);
            if (!this.owns(session)) {
                await backend.disconnect(session.id);
                return;
            }
            if (this.generations.get(session.id) !== generation) return;
            if (result.ok) {
                await this.loadCapabilities(session);
                await this.loadSchema(session);
                if (session.documents.some((doc) => doc.kind === 'diagram' || doc.kind === 'visualQuery')) await this.loadFullSchema(session);
                if (!this.owns(session) || this.generations.get(session.id) !== generation) return;
                for (const doc of session.documents)
                    if (doc.kind === 'table') {
                        doc.results = [];
                        doc.status = 'idle';
                        doc.hasResults = false;
                    }
                session.connectionState = 'connected';
            } else {
                session.connectionState = 'error';
                session.connectionError = result.message || 'Connection failed.';
            }
        } catch {
            if (!this.owns(session) || this.generations.get(session.id) !== generation) return;
            session.connectionState = 'error';
            session.connectionError = 'Backend unavailable. Run this app with wails3 dev or a packaged build to connect to real databases.';
        }
    }
    async reconnect(session: Session) {
        await this.establish(session);
    }
    async disconnect(session: Session) {
        if (!this.confirmCloseWork(session)) return;
        this.generations.set(session.id, (this.generations.get(session.id) ?? 0) + 1);
        for (const d of session.documents) this.stop(d);
        session.connectionState = 'disconnected';
        session.connectionError = '';
        session.capabilities = emptyCapabilities;
        try {
            await backend.disconnect(session.id);
        } catch {
            // Best-effort: the session is already marked disconnected locally.
        }
    }
    closeSession(session: Session) {
        if (!this.confirmCloseWork(session)) return;
        this.generations.set(session.id, (this.generations.get(session.id) ?? 0) + 1);
        for (const d of session.documents) this.stop(d);
        backend.disconnect(session.id).catch(() => {});
        this.sessions = this.sessions.filter((s) => s.id !== session.id);
        if (this.active === session.id) this.active = this.sessions[0]?.id ?? '';
    }
    openDocument(session: Session, kind: 'query' | 'table' | 'diagram' | 'visualQuery' | 'chart' = 'query', object = 'users') {
        if (kind === 'diagram' || kind === 'visualQuery') void this.loadFullSchema(session);
        if (kind === 'table') void this.loadTableDetail(session, object);
        const existing = (kind === 'table' || kind === 'diagram') && session.documents.find((d) => d.kind === kind && d.object === object);
        if (existing) {
            session.activeDocument = existing.id;
            return existing;
        }
        const doc = makeDocument(kind, object, `Query ${session.documents.filter((d) => d.kind === 'query').length + 1}`);
        if (kind === 'table') {
            const layout = this.gridLayouts[gridLayoutKey(JSON.stringify([session.profile.id, session.profile.database]), object)];
            if (layout) {
                doc.sort = layout.sort;
                doc.descending = layout.descending;
                doc.tableFilter = layout.filter;
            }
        }
        session.documents.push(doc);
        session.activeDocument = doc.id;
        // Same reason as in openProfile: return the reactive proxy, not the
        // plain object that was pushed.
        return session.documents.find((d) => d.id === doc.id)!;
    }
    // Follows a foreign key to the row it actually points to: opens (or
    // reuses) the target table document and filters it to an exact match on
    // the referenced column, via BrowseTable's filterColumn (never the
    // default free-text substring search, which would also match unrelated
    // rows for a numeric id like "1" against "10"/"21").
    openRelatedRows(session: Session, fk: SchemaForeignKey, value: CellValue) {
        const doc = this.openDocument(session, 'table', fk.refTable);
        doc.tableFilter = value === null ? '' : String(value);
        doc.tableFilterColumn = fk.refColumn;
        doc.sort = '';
        doc.descending = false;
        doc.results = [];
        doc.hasResults = false;
        doc.viewVersion += 1;
        void this.runTableQuery(session, doc);
        return doc;
    }
    openDiagram(session: Session, related = '') {
        const doc = this.openDocument(session, 'diagram', 'schema');
        if (related) doc.diagramRelated = related;
        return doc;
    }
    updateDiagramPosition(doc: Document, table: string, position: { x: number; y: number }) {
        doc.diagramPositions = { ...doc.diagramPositions, [table]: position };
    }
    toggleDiagramCollapse(doc: Document, table: string) {
        doc.diagramCollapsed = doc.diagramCollapsed.includes(table) ? doc.diagramCollapsed.filter((name) => name !== table) : [...doc.diagramCollapsed, table];
    }
    updateVisualQueryPosition(doc: Document, alias: string, position: { x: number; y: number }) {
        doc.visualQueryPositions = { ...doc.visualQueryPositions, [alias]: position };
    }
    private visualQuerySeq = new Map<string, number>();
    async compileVisualQuery(session: Session, doc: Document) {
        const seq = (this.visualQuerySeq.get(doc.id) ?? 0) + 1;
        this.visualQuerySeq.set(doc.id, seq);
        const generation = this.generations.get(session.id);
        const snapshot = JSON.stringify(doc.visualQueryAst);
        const { sql, error } = await backend.compileVisualQuery(session.id, JSON.parse(snapshot));
        if (JSON.stringify(doc.visualQueryAst) !== snapshot) return;
        // A newer compile may have been requested (and even resolved) while this
        // one was in flight; never let a stale response clobber fresher state.
        if (this.visualQuerySeq.get(doc.id) !== seq) return;
        if (this.generations.get(session.id) !== generation || !this.owns(session)) return;
        doc.visualQuerySql = sql;
        doc.visualQueryError = error;
        doc.sql = sql;
        return !error && !!sql;
    }
    async openVisualQueryAsEditor(session: Session, doc: Document) {
        if (!(await this.compileVisualQuery(session, doc))) return;
        if (doc.visualQueryError || !doc.visualQuerySql) return;
        const opened = this.openDocument(session, 'query');
        opened.sql = doc.visualQuerySql;
        return opened;
    }
    visualizeQueryResult(session: Session, source: Document) {
        const chartSource = queryChartSource(session, source, source.title);
        if (!chartSource) {
            this.notify('Run a query with rows before visualizing it.');
            return;
        }
        const count = session.documents.filter((d) => d.kind === 'chart').length + 1;
        const series = defaultChartSeries(id(), source.title, chartSource);
        const doc = makeDocument('chart', source.id, `Chart ${count}`);
        doc.chartSeries = [series];
        session.documents.push(doc);
        session.activeDocument = doc.id;
        return session.documents.find((d) => d.id === doc.id)!;
    }
    async visualizeTable(session: Session, tableName: string, source?: Document) {
        const chartSource = await tableChartSource(session, tableName, source, objectLabel(tableName));
        if (!chartSource) {
            this.notify(
                session.schema.some((item) => item.name === tableName) ? 'This object has no discovered fields to chart.' : 'Pick a table or collection before visualizing it.',
            );
            return;
        }
        const count = session.documents.filter((d) => d.kind === 'chart').length + 1;
        const series = defaultChartSeries(id(), objectLabel(tableName), chartSource);
        const doc = makeDocument('chart', tableName, `Chart ${count}`);
        doc.chartSeries = [series];
        session.documents.push(doc);
        session.activeDocument = doc.id;
        return session.documents.find((d) => d.id === doc.id)!;
    }
    // addChartSeriesFromQuery/addChartSeriesFromTable append an
    // independently-resolved series to an existing chart document, from any
    // connected session, not necessarily the one
    // the chart document itself was opened in. Each new series inherits the
    // document's current chart type (bar/line only; ChartDocument.svelte does
    // not offer "Add series" for pie/scatter) so every series in one document
    // renders on the same shared canvas.
    addChartSeriesFromQuery(chartDoc: Document, session: Session, source: Document) {
        const label = `${session.profile.name} · ${source.title}`;
        const chartSource = queryChartSource(session, source, label);
        if (!chartSource) {
            this.notify('Run a query with rows before adding it as a series.');
            return;
        }
        this.pushChartSeries(chartDoc, defaultChartSeries(id(), label, chartSource));
    }
    async addChartSeriesFromTable(chartDoc: Document, session: Session, tableName: string, source?: Document) {
        const label = `${session.profile.name} · ${objectLabel(tableName)}`;
        const chartSource = await tableChartSource(session, tableName, source, label);
        if (!chartSource) {
            this.notify(
                session.schema.some((item) => item.name === tableName)
                    ? 'This object has no discovered fields to chart.'
                    : 'Pick a table or collection before adding it as a series.',
            );
            return;
        }
        this.pushChartSeries(chartDoc, defaultChartSeries(id(), label, chartSource));
    }
    private pushChartSeries(chartDoc: Document, series: ChartSeriesConfig) {
        const chartType = chartDoc.chartSeries[0]?.chartType ?? 'bar';
        chartDoc.chartSeries = [...chartDoc.chartSeries, { ...series, chartType }];
    }
    removeChartSeries(chartDoc: Document, seriesId: string) {
        if (chartDoc.chartSeries.length <= 1) return;
        chartDoc.chartSeries = chartDoc.chartSeries.filter((s) => s.id !== seriesId);
    }
    setChartType(chartDoc: Document, chartType: ChartType) {
        chartDoc.chartSeries = chartDoc.chartSeries.map((s) => ({ ...s, chartType }));
    }
    // explainQuery forks a new 'explain' document from a query document's
    // statement under the cursor (the same resolution rule Format SQL uses,
    // reused rather than reinvented) and runs the default estimate mode
    // immediately, mirroring visualizeQueryResult's one-way fork pattern.
    async explainQuery(session: Session, source: Document) {
        if (session.connectionState !== 'connected') return;
        let sql = source.sql;
        try {
            const ranges = await backend.splitStatements(session.id, source.sql);
            const range = ranges.find((r) => source.cursor >= r.start && source.cursor <= r.end) ?? ranges.find((r) => r.start >= source.cursor);
            if (range) sql = range.text ?? source.sql.slice(range.start, range.end);
        } catch {
            // Fall back to the whole document below.
        }
        if (!sql.trim()) {
            this.notify('Write or select a statement to explain first.');
            return;
        }
        const count = session.documents.filter((d) => d.kind === 'explain').length + 1;
        const doc = makeDocument('explain', source.id, `Explain ${count}`);
        doc.explainSourceDocumentId = source.id;
        doc.explainSql = sql;
        session.documents.push(doc);
        session.activeDocument = doc.id;
        const proxyDoc = session.documents.find((d) => d.id === doc.id)!;
        await this.runExplain(session, proxyDoc);
        return session.documents.find((d) => d.id === doc.id)!;
    }
    // explainTable is explainQuery's counterpart for a table document on an
    // engine with no SQL query-document alternative to fork from (today:
    // MongoDB). Gated in the UI on "explainer without sqlExecutor" rather
    // than an engine name (DocumentView.svelte), so a future non-SQL engine
    // that also implements Explainer gets this entry point automatically.
    // The request text is a small JSON envelope, not SQL; see
    // mongodb.Provider.Explain's own doc comment for why the shared
    // Explainer interface's one string slot needs one here.
    async explainTable(session: Session, source: Document) {
        if (session.connectionState !== 'connected') return;
        let filter: unknown = {};
        if (source.tableFilter.trim()) {
            try {
                filter = JSON.parse(source.tableFilter);
            } catch {
                this.notify('The current filter is not valid JSON; fix or clear it before explaining.');
                return;
            }
        }
        const count = session.documents.filter((d) => d.kind === 'explain').length + 1;
        const doc = makeDocument('explain', source.id, `Explain ${count}`);
        doc.explainSourceDocumentId = source.id;
        doc.explainSql = JSON.stringify({ collection: source.object, filter });
        session.documents.push(doc);
        session.activeDocument = doc.id;
        const proxyDoc = session.documents.find((d) => d.id === doc.id)!;
        await this.runExplain(session, proxyDoc);
        return session.documents.find((d) => d.id === doc.id)!;
    }
    // A plain, non-authoritative check used only to decide whether to warn
    // before EXPLAIN ANALYZE actually runs a statement's real effects. The
    // backend enforces nothing based on this; it is a UX nicety, the same
    // "heuristic warning, not authorization" posture this app already takes
    // for read-only-connection UI elsewhere.
    looksMutating(sql: string): boolean {
        const first = sql.replace(/^(\s|--[^\n]*\n|\/\*[\s\S]*?\*\/)+/, '').split(/\s+/)[0] ?? '';
        return !['SELECT', 'WITH'].includes(first.toUpperCase());
    }
    async runExplain(session: Session, doc: Document) {
        if (session.connectionState !== 'connected' || !doc.explainSql.trim() || doc.status === 'running') return;
        const jobId = id();
        const generation = this.generations.get(session.id);
        doc.jobId = jobId;
        doc.status = 'running';
        doc.message = 'Running…';
        try {
            const plan = await backend.explain(session.id, doc.id, jobId, doc.explainSql, doc.explainMode === 'analyze', this.preferences.resultLimit, this.preferences.timeout);
            if (doc.jobId !== jobId || !this.owns(session) || this.generations.get(session.id) !== generation) return;
            doc.explainPlan = plan;
            doc.explainSelectedNodeId = '';
            doc.status = 'done';
            doc.message = '';
            doc.jobId = '';
        } catch (error) {
            if (doc.jobId !== jobId || !this.owns(session)) return;
            doc.status = 'error';
            doc.message = String(error);
            doc.jobId = '';
        }
    }
    // revealExplainNode jumps from a plan node (the detail panel's "Show in
    // editor" action) to the exact FROM/JOIN clause it came from, in the
    // query document Explain was forked from. Switches tabs even when the
    // reference can't be located (the source document is genuinely useful to
    // see either way), but only sets a selection when it can.
    revealExplainNode(session: Session, doc: Document, node: ExplainNode) {
        const source = session.documents.find((d) => d.id === doc.explainSourceDocumentId);
        if (!source) {
            this.notify('The original query document is no longer open.');
            return;
        }
        session.activeDocument = source.id;
        if (!node.relation) {
            this.notify('This node has no table reference to jump to.');
            return;
        }
        const ranges = findNodeRangesInDocument(source.sql, doc.explainSql, doc.explainPlan?.root ?? null, (n) => n.id === node.id);
        if (!ranges.length) {
            this.notify('Could not find this reference in the query; it may have changed since Explain last ran.');
            return;
        }
        source.selectionFrom = ranges[0].from;
        source.selectionTo = ranges[0].to;
        source.cursor = ranges[0].from;
        source.revealSeq = (source.revealSeq ?? 0) + 1;
    }
    closeDocument(session: Session, doc: Document) {
        if (!this.confirmCloseWork(session, doc)) return;
        this.stop(doc);
        backend.closeDocument(session.id, doc.id).catch(() => {});
        this.visualQuerySeq.delete(doc.id);
        this.paramPending.delete(doc.id);
        session.documents = session.documents.filter((d) => d.id !== doc.id);
        if (session.activeDocument === doc.id) session.activeDocument = session.documents[0]?.id ?? '';
    }
    private confirmCloseWork(session: Session, doc?: Document) {
        const docs = doc ? [doc] : session.documents;
        const hasRisk = docs.some((d) => d.edits.length > 0 || d.transactionActive);
        if (!hasRisk || typeof window === 'undefined') return true;
        return window.confirm('This will discard staged edits or close an active transaction for this workspace. Continue?');
    }
    editableColumns(session: Session, doc: Document) {
        const table = session.schema.find((t) => t.name === doc.object);
        if (!table || table.kind !== 'table') return [];
        if (!table.columns.some((c) => c.primaryKey)) return [];
        return table.columns;
    }
    rowKey(row: Row) {
        return JSON.stringify(row);
    }
    setCellEdit(session: Session, doc: Document, row: Row, column: string, value: CellValue) {
        if (session.profile.readOnly) {
            this.notify('This connection is read-only.');
            return;
        }
        if (!this.editableColumns(session, doc).some((c) => c.name === column)) {
            this.notify('This table needs a primary key before Quarro can edit it safely.');
            return;
        }
        if (doc.status === 'running' || Object.values(row).some((v) => typeof v === 'object' && v !== null)) return;
        const key = this.rowKey(row);
        if (doc.edits.some((e) => e.kind === 'delete' && e.original && this.rowKey(e.original) === key)) return;
        let edit = doc.edits.find((e) => e.kind === 'update' && e.original && this.rowKey(e.original) === key);
        if (value === row[column] || (typeof row[column] === 'number' && String(value) === String(row[column]))) {
            if (edit) {
                delete edit.values[column];
                if (!Object.keys(edit.values).length) doc.edits = doc.edits.filter((e) => e.id !== edit?.id);
            }
            return;
        }
        if (!edit) {
            edit = { id: id(), kind: 'update', original: { ...row }, values: {} };
            doc.edits.push(edit);
        }
        edit.values[column] = {
            mode: value === null ? 'null' : 'value',
            value: value ?? undefined,
        };
    }
    setInsertValue(doc: Document, editId: string, column: string, mode: 'value' | 'null' | 'default', value?: CellValue) {
        const edit = doc.edits.find((e) => e.id === editId && e.kind === 'insert');
        if (!edit) return;
        edit.values[column] = { mode, value };
    }
    insertRow(session: Session, doc: Document) {
        if (session.profile.readOnly) {
            this.notify('This connection is read-only.');
            return;
        }
        const columns = this.editableColumns(session, doc);
        if (!columns.length) {
            this.notify('This table needs a primary key before Quarro can edit it safely.');
            return;
        }
        const values = Object.fromEntries(columns.map((c) => [c.name, { mode: 'default' as const }]));
        doc.edits.push({ id: id(), kind: 'insert', values });
    }
    deleteRow(session: Session, doc: Document, row: Row | undefined) {
        if (!row) return;
        if (session.profile.readOnly) {
            this.notify('This connection is read-only.');
            return;
        }
        const columns = this.editableColumns(session, doc);
        if (!columns.length) {
            this.notify('This table needs a primary key before Quarro can edit it safely.');
            return;
        }
        if (doc.status === 'running' || Object.values(row).some((v) => typeof v === 'object' && v !== null)) return;
        const key = this.rowKey(row);
        doc.edits = doc.edits.filter((e) => !e.original || this.rowKey(e.original) !== key);
        doc.edits.push({ id: id(), kind: 'delete', original: { ...row }, values: {} });
    }
    // deleteDocument is deleteRow's MongoDB counterpart: there is no staged-
    // edit batch to review before applying (a document delete is already
    // atomic and immediate server-side), so this confirms once and deletes
    // right away instead of queuing it into doc.edits.
    async deleteDocument(session: Session, doc: Document, row: Row | undefined) {
        if (!row) return;
        if (session.profile.readOnly) {
            this.notify('This connection is read-only.');
            return;
        }
        if (typeof window !== 'undefined' && !window.confirm('Delete this document? This cannot be undone.')) return;
        try {
            const result = await backend.applyDocumentEdit(session.id, doc.id, doc.object, 'delete', JSON.stringify(row._id), '');
            if (!result.applied) {
                this.notify('This document no longer exists — it may have already been deleted.');
                return;
            }
            this.notify('Document deleted.');
            await this.runTableQuery(session, doc);
        } catch (error) {
            this.notify(String(error));
        }
    }
    // The latest inspectKey request per document, so a slow page for a key
    // the user already clicked away from never overwrites the current one.
    private keyRequests = new Map<string, string>();
    // inspectKey opens a Redis key in the key inspector, or with more set
    // loads its next page onto what is already shown.
    async inspectKey(session: Session, doc: Document, key: string, more = false) {
        const loaded = doc.keyDetail;
        const cursor = more && loaded?.key === key ? loaded.cursor : '';
        if (more && !cursor) return;
        const request = id();
        this.keyRequests.set(doc.id, request);
        if (!more) doc.keyDetail = null;
        doc.keyLoading = true;
        doc.keyError = '';
        try {
            const page = await backend.inspectKey(session.id, doc.id, request, key, cursor, 100, this.preferences.timeout);
            if (this.keyRequests.get(doc.id) !== request || !this.owns(session)) return;
            doc.keyDetail = more && doc.keyDetail ? mergeKeyPage(doc.keyDetail, page) : page;
        } catch (error) {
            if (this.keyRequests.get(doc.id) !== request) return;
            doc.keyError = String(error);
        } finally {
            if (this.keyRequests.get(doc.id) === request) {
                this.keyRequests.delete(doc.id);
                doc.keyLoading = false;
            }
        }
    }
    // applyKeyEdit writes one change to a Redis key, then re-reads the key
    // from its first page and refreshes the key list (sizes change, and a
    // create or delete adds or removes a row). A conflict comes back as a
    // result for the caller to show, not as a notification.
    async applyKeyEdit(session: Session, doc: Document, edit: KeyEdit): Promise<KeyEditResult | undefined> {
        if (session.profile.readOnly) {
            this.notify('This connection is read-only.');
            return undefined;
        }
        try {
            const result = await backend.applyKeyEdit(session.id, doc.id, edit);
            if (!result.applied || !this.owns(session)) return result;
            if (result.keyRemoved) {
                if (doc.keyDetail?.key === edit.key) this.closeKey(doc);
            } else {
                await this.inspectKey(session, doc, edit.key);
            }
            void this.runTableQuery(session, doc);
            return result;
        } catch (error) {
            this.notify(String(error));
            return undefined;
        }
    }
    closeKey(doc: Document) {
        this.keyRequests.delete(doc.id);
        doc.keyDetail = null;
        doc.keyLoading = false;
        doc.keyError = '';
    }
    setSelectedNull(session: Session, doc: Document, row: Row | undefined) {
        if (!row || !doc.selectedColumn) return;
        this.setCellEdit(session, doc, row, doc.selectedColumn, null);
    }
    discardEdits(doc: Document) {
        doc.edits = [];
    }
    async previewEdits(session: Session, doc: Document): Promise<EditPreview[]> {
        if (session.connectionState !== 'connected' || !doc.edits.length) return [];
        return backend.previewTableEdits(session.id, doc.id, doc.object, doc.edits);
    }
    async applyEdits(session: Session, doc: Document) {
        if (session.profile.readOnly) {
            this.notify('This connection is read-only.');
            return;
        }
        if (session.connectionState !== 'connected' || !doc.edits.length || doc.status === 'running') return;
        doc.status = 'running';
        doc.message = 'Applying staged edits…';
        try {
            const result = await backend.applyTableEdits(session.id, doc.id, doc.object, doc.edits, this.preferences.timeout);
            doc.edits = [];
            doc.status = 'done';
            doc.message = `${result.applied} staged change${result.applied === 1 ? '' : 's'} applied · ${result.rowsAffected} rows affected.`;
            this.notify(doc.message);
            await this.loadSchema(session);
            await this.runTableQuery(session, doc);
        } catch (error) {
            doc.status = 'error';
            doc.message = String(error);
            this.notify(doc.message);
        }
    }
    async beginTransaction(session: Session, doc: Document) {
        if (session.profile.readOnly) {
            this.notify('This connection is read-only.');
            return;
        }
        try {
            await backend.beginTransaction(session.id, doc.id);
            doc.transactionActive = true;
            this.notify('Transaction started for this document.');
        } catch (error) {
            this.notify(String(error));
        }
    }
    async commitTransaction(session: Session, doc: Document) {
        try {
            await backend.commitTransaction(session.id, doc.id);
            doc.transactionActive = false;
            this.notify('Transaction committed.');
            if (doc.kind === 'table') await this.runTableQuery(session, doc);
        } catch (error) {
            this.notify(String(error));
        }
    }
    async rollbackTransaction(session: Session, doc: Document) {
        try {
            await backend.rollbackTransaction(session.id, doc.id);
            doc.transactionActive = false;
            doc.edits = [];
            this.notify('Transaction rolled back.');
            if (doc.kind === 'table') await this.runTableQuery(session, doc);
        } catch (error) {
            this.notify(String(error));
        }
    }
    private async resolveSql(session: Session, doc: Document, mode: 'all' | 'selection' | 'statement'): Promise<string> {
        if (mode === 'all') return doc.sql;
        if (mode === 'selection' && doc.selectionFrom !== doc.selectionTo) {
            return doc.sql.slice(Math.min(doc.selectionFrom, doc.selectionTo), Math.max(doc.selectionFrom, doc.selectionTo));
        }
        if (mode === 'selection') return '';
        const text = doc.sql,
            cursor = doc.cursor;
        const ranges = await backend.splitStatements(session.id, text);
        const range = ranges.find((r) => cursor >= r.start && cursor <= r.end) ?? ranges.find((r) => r.start >= cursor);
        if (!range) return '';
        const statement = range.text ?? text.slice(range.start, range.end);
        if (session.capabilities.requiresDelimiterWrapping) {
            let delimiter = '$quarro$';
            while (statement.includes(delimiter)) delimiter += '$';
            return `DELIMITER ${delimiter}\n${statement}${delimiter}\nDELIMITER ;`;
        }
        return statement;
    }
    /** True when doc is not the tab the user is currently looking at  a job
     * finishing there should surface a notification rather than steal focus. */
    private isBackground(session: Session, doc: Document) {
        return this.active !== session.id || session.activeDocument !== doc.id;
    }
    cancelParamPrompt(doc: Document) {
        doc.paramPrompt = [];
        this.paramPending.delete(doc.id);
    }
    private paramPending = new Map<string, { mode: 'all' | 'selection' | 'statement'; opts: { append?: boolean } }>();
    submitParams(session: Session, doc: Document, values: Record<string, string | null>) {
        doc.paramValues = { ...doc.paramValues, ...values };
        doc.paramPrompt = [];
        const pending = this.paramPending.get(doc.id);
        this.paramPending.delete(doc.id);
        this.run(session, doc, pending?.mode ?? 'statement', pending?.opts ?? {});
    }
    async run(session: Session, doc: Document, mode: 'all' | 'selection' | 'statement' = 'statement', opts: { append?: boolean } = {}) {
        if (session.connectionState !== 'connected' || doc.status === 'running') return;
        if (doc.kind === 'visualQuery') {
            if (!(await this.compileVisualQuery(session, doc))) return;
            if (doc.visualQueryError || !doc.visualQuerySql || String(doc.status) === 'running') return;
            doc.outputCollapsed = false;
        }
        const jobId = id();
        const generation = this.generations.get(session.id);
        doc.jobId = jobId;
        doc.status = 'running';
        doc.message = opts.append ? 'Loading more…' : 'Running…';
        try {
            const sqlText = doc.kind === 'table' ? doc.sql : await this.resolveSql(session, doc, mode);
            if (doc.jobId !== jobId || !this.owns(session) || this.generations.get(session.id) !== generation) return;
            if (doc.kind !== 'table' && !sqlText.trim()) {
                doc.status = 'error';
                doc.message = 'Nothing to run  write or select a statement first.';
                doc.jobId = '';
                return;
            }
            let paramNames: string[] = [];
            if (doc.kind === 'query') {
                paramNames = await backend.placeholders(session.id, sqlText);
                if (doc.jobId !== jobId || !this.owns(session) || this.generations.get(session.id) !== generation) return;
                if (paramNames.some((name) => !(name in doc.paramValues))) {
                    doc.paramPrompt = paramNames;
                    doc.status = 'idle';
                    doc.jobId = '';
                    this.paramPending.set(doc.id, { mode, opts });
                    return;
                }
            }
            const result =
                doc.kind === 'table' && session.capabilities.keyValueStore
                    ? // Redis has no offset/skip equivalent — SCAN is cursor-based, so
                      // every run starts over rather than resuming a prior page (no
                      // "Load More" append support, unlike browseTable below).
                      await backend.keyValueScan(
                          session.id,
                          doc.id,
                          jobId,
                          // With no filter typed, list the clicked keyspace group
                          // ("group:*" plus the bare key). A typed filter is a literal
                          // Redis prefix scan, not a substring search like a SQL
                          // table's filter.
                          doc.tableFilter || doc.object,
                          !doc.tableFilter,
                          this.preferences.resultLimit,
                          this.preferences.timeout,
                      )
                    : doc.kind === 'table'
                      ? await backend.browseTable(
                            session.id,
                            doc.id,
                            jobId,
                            doc.object,
                            doc.tableFilter,
                            doc.tableFilterColumn,
                            doc.sort,
                            doc.descending,
                            opts.append ? (doc.results[0]?.rows?.length ?? 0) : 0,
                            this.preferences.resultLimit,
                            this.preferences.timeout,
                        )
                      : paramNames.length
                        ? await backend.executeParameterized(
                              session.id,
                              doc.id,
                              jobId,
                              sqlText,
                              Object.fromEntries(paramNames.map((name) => [name, doc.paramValues[name]])),
                              this.preferences.resultLimit,
                              this.preferences.timeout,
                          )
                        : await backend.execute(session.id, doc.id, jobId, sqlText, this.preferences.resultLimit, this.preferences.timeout);
            if (doc.jobId !== jobId) return; // superseded by stop() or a newer run
            const existing = doc.results[0];
            const incoming = result.results[0];
            if (opts.append && existing?.kind === 'rows' && incoming?.kind === 'rows') {
                const combined = [...(existing.rows ?? []), ...(incoming.rows ?? [])];
                if (combined.length > 10000 || new TextEncoder().encode(JSON.stringify(combined)).byteLength > 16 * 1024 * 1024) {
                    doc.status = 'done';
                    doc.jobId = '';
                    doc.message = 'Loaded-result limit reached (10,000 rows / 16 MiB). Narrow the table filter to continue.';
                    this.notify(doc.message);
                    return;
                }
                doc.results = [
                    {
                        ...existing,
                        rows: [...(existing.rows ?? []), ...(incoming.rows ?? [])],
                        truncated: incoming.truncated,
                    },
                    ...result.results.slice(1),
                ];
            } else {
                doc.results = result.results;
                doc.activeResult = 0;
            }
            doc.hasResults = doc.results.length > 0;
            doc.elapsed = result.elapsedMs;
            const failed = result.results.some((r) => r.kind === 'error');
            doc.status = failed ? 'error' : 'done';
            doc.message = summarizeResults(doc.results, result.elapsedMs);
            doc.jobId = '';
            const background = this.isBackground(session, doc);
            if (background) this.notify(failed ? `"${doc.title}" failed while running in the background.` : `"${doc.title}" finished in the background: ${doc.message}`);
            if (!opts.append && this.preferences.rememberHistory) {
                this.history.unshift({
                    id: id(),
                    title: doc.title,
                    sql: sqlText,
                    at: Date.now(),
                    profileId: session.profile.id,
                    durationMs: result.elapsedMs,
                    outcome: failed ? 'error' : 'ok',
                });
                this.history = this.history.slice(0, 200);
            }
        } catch (error) {
            if (doc.jobId !== jobId) return;
            doc.status = 'error';
            doc.message = String(error);
            doc.jobId = '';
            if (this.isBackground(session, doc)) this.notify(`"${doc.title}" failed while running in the background.`);
        }
    }
    // runNativeQuery is run()'s counterpart for a query document on an
    // engine with a native language instead of SQL: a kept-separate method
    // rather than another branch inside run()'s own SQL-shaped
    // mode/placeholder/split logic, the same "explainQuery and explainTable
    // are two methods, not one branching method" precedent already used
    // elsewhere. doc.sql holds exactly what the user wrote (not a wrapped
    // envelope, so Save Query/history show that).
    async runNativeQuery(session: Session, doc: Document) {
        if (session.capabilities.nativeQueryLanguage === 'redis-command') return this.runRedisCommands(session, doc);
        return this.runPipeline(session, doc);
    }
    // runPipeline runs a MongoDB aggregation pipeline; doc.object holds the
    // selected collection.
    private async runPipeline(session: Session, doc: Document) {
        if (session.connectionState !== 'connected' || doc.status === 'running') return;
        const jobId = id();
        const generation = this.generations.get(session.id);
        doc.jobId = jobId;
        doc.status = 'running';
        doc.message = 'Running…';
        if (!doc.object) {
            doc.status = 'error';
            doc.message = 'Select a collection first.';
            doc.jobId = '';
            return;
        }
        let pipeline: unknown;
        try {
            pipeline = JSON.parse(doc.sql.trim() || '[]');
        } catch (error) {
            doc.status = 'error';
            doc.message = `Invalid pipeline JSON: ${String(error)}`;
            doc.jobId = '';
            return;
        }
        const req = JSON.stringify({ collection: doc.object, pipeline });
        try {
            const result = await backend.executeNative(session.id, doc.id, jobId, req, this.preferences.resultLimit, this.preferences.timeout);
            if (doc.jobId !== jobId || !this.owns(session) || this.generations.get(session.id) !== generation) return;
            this.finishNativeRun(session, doc, result.results, result.elapsedMs, doc.sql, doc.object);
        } catch (error) {
            this.failNativeRun(session, doc, jobId, error);
        }
    }
    // runRedisCommands runs the command line under the cursor, or each line
    // of the selection in order, one backend job per line (a Redis command
    // line is one command), each landing as its own result tab. Stop
    // cancels the line in flight and skips the rest.
    private async runRedisCommands(session: Session, doc: Document) {
        if (session.connectionState !== 'connected' || doc.status === 'running') return;
        const lines = commandLines(doc.sql, doc.selectionFrom, doc.selectionTo);
        if (!lines.length) {
            doc.status = 'error';
            doc.message = 'Nothing to run: put the cursor on a command, or select several lines.';
            return;
        }
        const jobId = id();
        const generation = this.generations.get(session.id);
        doc.jobId = jobId;
        doc.status = 'running';
        doc.message = 'Running…';
        const results: ResultSet[] = [];
        let elapsed = 0;
        try {
            for (const line of lines) {
                const result = await backend.executeNative(session.id, doc.id, jobId, JSON.stringify({ command: line }), this.preferences.resultLimit, this.preferences.timeout);
                if (doc.jobId !== jobId || !this.owns(session) || this.generations.get(session.id) !== generation) return;
                results.push(...result.results);
                elapsed += result.elapsedMs;
            }
            this.finishNativeRun(session, doc, results, elapsed, lines.join('\n'), undefined);
        } catch (error) {
            this.failNativeRun(session, doc, jobId, error);
        }
    }
    private finishNativeRun(session: Session, doc: Document, results: ResultSet[], elapsedMs: number, ran: string, object: string | undefined) {
        doc.results = results;
        doc.activeResult = 0;
        doc.hasResults = doc.results.length > 0;
        doc.elapsed = elapsedMs;
        const failed = results.some((r) => r.kind === 'error');
        doc.status = failed ? 'error' : 'done';
        doc.message = summarizeResults(doc.results, elapsedMs);
        doc.jobId = '';
        const background = this.isBackground(session, doc);
        if (background) this.notify(failed ? `"${doc.title}" failed while running in the background.` : `"${doc.title}" finished in the background: ${doc.message}`);
        if (this.preferences.rememberHistory) {
            this.history.unshift({
                id: id(),
                title: doc.title,
                sql: ran,
                at: Date.now(),
                profileId: session.profile.id,
                durationMs: elapsedMs,
                outcome: failed ? 'error' : 'ok',
                object,
            });
            this.history = this.history.slice(0, 200);
        }
    }
    private failNativeRun(session: Session, doc: Document, jobId: string, error: unknown) {
        if (doc.jobId !== jobId) return;
        doc.status = 'error';
        doc.message = String(error);
        doc.jobId = '';
        if (this.isBackground(session, doc)) this.notify(`"${doc.title}" failed while running in the background.`);
    }
    // The server's command names per session, for the Redis command
    // editor's completion. COMMAND LIST is Redis 7+; older servers answer
    // the plain COMMAND. Failure means no completion, never an error.
    private commandNames = new Map<string, Promise<string[]>>();
    redisCommandNames(session: Session, doc: Document): Promise<string[]> {
        let names = this.commandNames.get(session.id);
        if (!names) {
            const read = async (command: string) => {
                const result = await backend.executeNative(session.id, doc.id, id(), JSON.stringify({ command }), 5000, this.preferences.timeout);
                const rs = result.results[0];
                return rs?.kind === 'rows' ? commandNamesFrom(rs.rows ?? []) : [];
            };
            names = read('COMMAND LIST')
                .then((list) => (list.length ? list : read('COMMAND')))
                .catch(() => []);
            this.commandNames.set(session.id, names);
        }
        return names;
    }
    async runTableQuery(session: Session, doc: Document, opts: { append?: boolean } = {}) {
        if (opts.append && doc.status === 'running') return;
        if (!opts.append) this.stop(doc);
        const visibleRows = () =>
            (doc.results[doc.activeResult]?.rows ?? []).filter((row) =>
                Object.values(row).some((value) =>
                    String(value ?? '')
                        .toLowerCase()
                        .includes(doc.filter.toLowerCase()),
                ),
            );
        const selected = visibleRows()[doc.selected];
        const keys =
            session.schema
                .find((table) => table.name === doc.object)
                ?.columns.filter((column) => column.primaryKey)
                .map((column) => column.name) ?? [];
        doc.selected = -1;
        if (!opts.append) {
            const key = gridLayoutKey(JSON.stringify([session.profile.id, session.profile.database]), doc.object);
            this.updateGridLayout(key, {
                sort: doc.sort,
                descending: doc.descending,
                filter: doc.tableFilter,
            });
        }
        await this.run(session, doc, 'all', opts);
        if (selected && keys.length) {
            doc.selected = visibleRows().findIndex((row) => keys.every((key) => row[key] === selected[key]));
        }
    }
    resetTableView(session: Session, doc: Document) {
        const key = gridLayoutKey(JSON.stringify([session.profile.id, session.profile.database]), doc.object);
        delete this.gridLayouts[key];
        doc.sort = '';
        doc.descending = false;
        doc.tableFilter = '';
        doc.tableFilterColumn = '';
        doc.results = [];
        doc.hasResults = false;
        doc.viewVersion += 1;
        this.runTableQuery(session, doc);
    }
    stop(doc: Document) {
        if (!doc.jobId) return;
        const jobId = doc.jobId;
        doc.jobId = '';
        doc.status = 'cancelled';
        doc.message = 'Cancellation requested. The server outcome is not yet confirmed; previous results are preserved.';
        backend.cancel(jobId).catch(() => {});
    }
    // Formats the selection when there is one, otherwise just the statement
    // the cursor is inside (found the same way Run Current Statement finds
    // it) — reformatting a whole multi-statement script when the user only
    // meant to clean up one of them would be a surprising side effect.
    // Falls back to the whole document when neither is available (an empty
    // selection with no statement boundaries to find, e.g. a single
    // unterminated fragment).
    async formatQuery(session: Session, doc: Document) {
        const dialectName = this.drivers.find((d) => d.name === session.profile.engine)?.sqlDialect;
        const dialect: SqlLanguage = (dialectName && formatDialects[dialectName]) || 'sqlite';
        let from = 0;
        let to = doc.sql.length;
        if (doc.selectionFrom !== doc.selectionTo) {
            from = Math.min(doc.selectionFrom, doc.selectionTo);
            to = Math.max(doc.selectionFrom, doc.selectionTo);
        } else {
            try {
                const ranges = await backend.splitStatements(session.id, doc.sql);
                const range = ranges.find((r) => doc.cursor >= r.start && doc.cursor <= r.end) ?? ranges.find((r) => r.start >= doc.cursor);
                if (range) {
                    from = range.start;
                    to = range.end;
                }
            } catch {
                // Fall back to the whole document below.
            }
        }
        try {
            const formatted = formatSql(doc.sql.slice(from, to), {
                language: dialect,
                tabWidth: 2,
            });
            doc.sql = doc.sql.slice(0, from) + formatted + doc.sql.slice(to);
        } catch (error) {
            this.notify(`Could not format this SQL: ${String(error)}`);
        }
    }
    // Opens the Structure view of whichever table or view the identifier at
    // `pos` (default: the document's own cursor) names — matched directly by
    // table/view name first, then by owning a column with that name. See
    // findSchemaObject's own comment for why this is a plain word lookup
    // rather than the editor's own alias-aware hover resolution.
    goToDefinition(session: Session, doc: Document, pos: number = doc.cursor) {
        const word = wordAt(doc.sql, pos);
        const target = findSchemaObject(session.schema, word);
        if (!target) {
            this.notify(word ? `No table or column named "${word}" found in this schema.` : 'Place the cursor on a table or column name first.');
            return;
        }
        const opened = this.openDocument(session, 'table', target.name);
        opened.view = 'structure';
    }
    save(session: Session, doc: Document) {
        // A MongoDB pipeline only means something with its collection; a SQL
        // query or a Redis command line stands alone.
        const pipeline = session.capabilities.nativeQueryLanguage === 'mongodb-pipeline';
        const existing = this.saved.find((q) => q.sql === doc.sql && q.profileId === session.profile.id && q.object === (pipeline ? doc.object : undefined));
        if (!existing)
            this.saved.push({
                id: id(),
                title: doc.title,
                sql: doc.sql,
                at: Date.now(),
                profileId: session.profile.id,
                folder: '',
                favorite: false,
                object: pipeline ? doc.object : undefined,
            });
        this.notify('Query saved to this connection.');
    }
    renameSaved(query: SavedQuery, title: string) {
        const trimmed = title.trim();
        if (trimmed) query.title = trimmed;
    }
    setSavedFolder(query: SavedQuery, folder: string) {
        query.folder = folder.trim();
    }
    toggleSavedFavorite(query: SavedQuery) {
        query.favorite = !query.favorite;
    }
    deleteSaved(query: SavedQuery) {
        this.saved = this.saved.filter((q) => q.id !== query.id);
    }
    deleteHistoryEntry(entry: HistoryEntry) {
        this.history = this.history.filter((h) => h.id !== entry.id);
    }
    clearHistory(profileId?: string) {
        this.history = profileId ? this.history.filter((h) => h.profileId !== profileId) : [];
    }
    nextSession(direction: 1 | -1 = 1) {
        if (this.sessions.length < 2) return;
        const i = this.sessions.findIndex((s) => s.id === this.active);
        const next = (i + direction + this.sessions.length) % this.sessions.length;
        this.active = this.sessions[next].id;
    }
    resetLayout() {
        if (this.session)
            Object.assign(this.session, {
                sidebarWidth: 232,
                editorHeight: 255,
                inspectorWidth: 300,
                inspector: false,
                sidebar: true,
            });
    }
    async chooseExportDestination(defaultName: string, filterLabel: string, filterPattern: string): Promise<string> {
        try {
            return await backend.chooseExportDestination(defaultName, filterLabel, filterPattern);
        } catch (error) {
            this.notify(String(error));
            return '';
        }
    }
    /**
     * Asks where to save, then writes the blob there. Returns false if the
     * user cancelled or the write failed. Where native dialogs are
     * unavailable (e.g. the server-mode test harness) it falls back to a
     * plain browser download.
     */
    async saveBlobWithPicker(blob: Blob, defaultName: string, filterLabel: string, extension: string): Promise<boolean> {
        let destination: string;
        try {
            destination = await backend.chooseExportDestination(defaultName, filterLabel, `*.${extension}`);
        } catch {
            const url = URL.createObjectURL(blob);
            const a = window.document.createElement('a');
            a.href = url;
            a.download = defaultName;
            window.document.body.appendChild(a);
            a.click();
            setTimeout(() => {
                a.remove();
                URL.revokeObjectURL(url);
            }, 1000);
            return true;
        }
        if (!destination) return false;
        try {
            const bytes = new Uint8Array(await blob.arrayBuffer());
            let binary = '';
            for (let i = 0; i < bytes.length; i += 0x8000) {
                binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
            }
            await backend.saveBytes(destination, btoa(binary));
            return true;
        } catch (error) {
            this.notify(String(error));
            return false;
        }
    }
    async startExport(session: Session, doc: Document, destination: string, request: ExportRequest): Promise<string> {
        const jobId = id();
        try {
            await backend.startExport(session.id, doc.id, jobId, destination, request);
            return jobId;
        } catch (error) {
            this.notify(String(error));
            return '';
        }
    }
    async exportProgress(jobId: string): Promise<ExportProgress> {
        return backend.exportProgress(jobId);
    }
    async chooseImportFile(): Promise<string> {
        try {
            return await backend.chooseImportFile();
        } catch (error) {
            this.notify(String(error));
            return '';
        }
    }
    async previewImport(path: string, delimiter: string, encoding: string, hasHeader: boolean): Promise<CSVPreview | undefined> {
        try {
            return await backend.previewCSV(path, delimiter, encoding, hasHeader);
        } catch (error) {
            this.notify(String(error));
            return undefined;
        }
    }
    async validateImport(session: Session, path: string, table: string, mapping: ColumnMapping[], opts: ImportOptions): Promise<ImportValidation | undefined> {
        try {
            return await backend.validateCSV(session.id, path, table, mapping, opts);
        } catch (error) {
            this.notify(String(error));
            return undefined;
        }
    }
    async startImport(session: Session, doc: Document, path: string, table: string, mapping: ColumnMapping[], opts: ImportOptions): Promise<string> {
        const jobId = id();
        try {
            await backend.startImport(session.id, doc.id, jobId, path, table, mapping, opts);
            return jobId;
        } catch (error) {
            this.notify(String(error));
            return '';
        }
    }
    async importProgress(jobId: string): Promise<ImportProgress> {
        return backend.importProgress(jobId);
    }
    cancelBackgroundJob(jobId: string) {
        backend.cancel(jobId).catch(() => {});
    }
}
export const workspace = new Workspace();
