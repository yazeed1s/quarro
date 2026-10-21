<script lang="ts">
    import { objectLabel } from '../../state/types';
    import { untrack } from 'svelte';
    import type { Document, Row, Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import Input from '../../components/Input.svelte';
    import Modal from '../../components/Modal.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';
    import { closeOnOutsideClick } from '../../components/dropdown';

    const insertModeOptions: SelectOption[] = [
        { value: 'default', label: 'Default' },
        { value: 'value', label: 'Value' },
        { value: 'null', label: 'NULL' },
    ];
    import ResizeHandle from '../../components/ResizeHandle.svelte';
    import QueryEditor from '../editor/QueryEditor.svelte';
    import NativeQueryEditor from '../editor/NativeQueryEditor.svelte';
    import RedisCommandEditor from '../editor/RedisCommandEditor.svelte';
    import ParamPrompt from '../editor/ParamPrompt.svelte';
    import TransferPanel from '../transfer/TransferPanel.svelte';
    import ChartDocument from '../charts/ChartDocument.svelte';
    import SchemaDiagram from '../diagram/SchemaDiagram.svelte';
    import VisualQueryBuilder from '../query-builder/VisualQueryBuilder.svelte';
    import ExplainDocument from '../explain/ExplainDocument.svelte';
    import { findNodeRangesInDocument } from '../explain/reveal';
    import DataGrid from '../results/DataGrid.svelte';
    import RowDetails from '../results/RowDetails.svelte';
    import DocumentEditor from '../results/DocumentEditor.svelte';
    import KeyList from '../keyvalue/KeyList.svelte';
    import KeyInspector from '../keyvalue/KeyInspector.svelte';
    let { session, document }: { session: Session; document: Document } = $props();
    let output = $state('results');
    let inspectorMode = $state<'inspect' | 'edit'>('inspect');
    let transferPanel = $state<'none' | 'export' | 'import'>('none');
    let transferTab = $state<'export' | 'import'>('export');
    let schemaTable = $derived(session.tableDetails[document.object] ?? session.schema.find((t) => t.name === document.object));
    let tableDetailLoading = $derived(document.kind === 'table' && !!session.tableDetailLoading[document.object]);
    let tableDetailError = $derived(document.kind === 'table' ? session.tableDetailErrors[document.object] : '');
    let constraints = $derived(session.tableConstraints[document.object] ?? []);
    let constraintsLoading = $derived(document.kind === 'table' && !!session.tableConstraintsLoading[document.object]);
    let constraintsError = $derived(document.kind === 'table' ? session.tableConstraintsErrors[document.object] : '');
    let viewingDefinition = $state(false);
    let definition = $derived(session.tableDefinitions[document.object]);
    let definitionLoading = $derived(!!session.tableDefinitionsLoading[document.object]);
    let definitionError = $derived(session.tableDefinitionsErrors[document.object]);
    let reverseForeignKeys = $derived(session.schema.flatMap((t) => t.foreignKeys.filter((fk) => fk.refTable === document.object).map((fk) => ({ table: t.name, fk }))));
    let layoutKey = $derived(document.kind === 'table' ? JSON.stringify([JSON.stringify([session.profile.id, session.profile.database]), document.object]) : undefined);
    // svelte-ignore state_referenced_locally -- intentional one-time seed of
    // the grid's column layout from persisted storage at mount; layoutKey is
    // stable for a mounted document's lifetime (each open document has its
    // own DocumentView instance, see Workspace.svelte's keyed each block).
    let gridLayout = $state(layoutKey ? { ...workspace.getGridLayout(layoutKey) } : workspace.getGridLayout(''));
    $effect(() => {
        // Read widths/order/hidden to track them; read the existing stored
        // layout via untrack so writing it back doesn't also re-trigger this
        // same effect (which would otherwise loop).
        const snapshot = {
            widths: { ...gridLayout.widths },
            order: [...gridLayout.order],
            hidden: [...gridLayout.hidden],
        };
        if (!layoutKey) return;
        untrack(() => workspace.updateGridLayout(layoutKey, snapshot));
    });
    function toggleColumnHidden(column: string) {
        gridLayout.hidden = gridLayout.hidden.includes(column) ? gridLayout.hidden.filter((c) => c !== column) : [...gridLayout.hidden, column];
    }
    function moveColumn(column: string, direction: -1 | 1) {
        const next = [...allColumnsOrdered];
        const index = next.indexOf(column);
        const target = index + direction;
        if (index < 0 || target < 0 || target >= next.length) return;
        [next[index], next[target]] = [next[target], next[index]];
        gridLayout.order = next;
    }
    let activeResultSet = $derived(document.results[document.activeResult]);
    let allRows = $derived(activeResultSet?.rows ?? []);
    let resultColumns = $derived(activeResultSet?.columns ?? []);
    let canVisualize = $derived(document.kind === 'query' && activeResultSet?.kind === 'rows' && !!activeResultSet.rows?.length);
    // Every open 'explain' document forked from this one, mapping its plan's
    // hotspot-badged nodes back onto this document's own SQL text. An
    // informational marker, not tied to whichever explain document happens
    // to be active right now.
    let hotspotRanges = $derived(
        document.kind === 'query'
            ? session.documents
                  .filter((d) => d.kind === 'explain' && d.explainSourceDocumentId === document.id && d.explainPlan)
                  .flatMap((d) => findNodeRangesInDocument(document.sql, d.explainSql, d.explainPlan!.root, (n) => n.badges.some((b) => b.kind === 'hotspot')))
            : [],
    );
    let editableColumns = $derived(workspace.editableColumns(session, document));
    let tableEditable = $derived(document.kind === 'table' && schemaTable?.kind === 'table' && editableColumns.length > 0 && !session.profile.readOnly);
    let documentEditable = $derived(document.kind === 'table' && schemaTable?.kind === 'collection' && session.capabilities.documentEditor && !session.profile.readOnly);
    let insertingDocument = $state(false);
    let filtered = $derived(
        allRows.filter((row) =>
            Object.values(row).some((value) =>
                String(value ?? '')
                    .toLowerCase()
                    .includes(document.filter.toLowerCase()),
            ),
        ),
    );
    let rows = $derived.by(() => {
        const result = [...filtered];
        // Table documents are already sorted server-side (see
        // workspace.runTableQuery); query documents sort the loaded window
        // client-side since we never rewrite arbitrary user SQL.
        if (document.kind === 'query' && document.sort)
            result.sort((a, b) => {
                const av = a[document.sort],
                    bv = b[document.sort];
                const compared =
                    typeof av === 'number' && typeof bv === 'number'
                        ? av - bv
                        : String(av ?? '').localeCompare(String(bv ?? ''), undefined, {
                              numeric: true,
                          });
                return document.descending ? -compared : compared;
            });
        return result;
    });
    // A key listing has entries, not rows.
    let countLabel = $derived(
        activeResultSet?.kind === 'keyvalue' ? `${activeResultSet.entries?.length ?? 0} key${activeResultSet.entries?.length === 1 ? '' : 's'}` : `${rows.length} rows`,
    );
    let selectedRow = $derived(rows[document.selected]);
    let liveColumns = $derived(resultColumns.length ? resultColumns.map((c) => c.name) : Object.keys(rows[0] ?? {}));
    let allColumnsOrdered = $derived.by(() => {
        if (!gridLayout.order.length) return liveColumns;
        const known = new Set(liveColumns);
        const ordered = gridLayout.order.filter((c) => known.has(c));
        const extra = liveColumns.filter((c) => !gridLayout.order.includes(c));
        return [...ordered, ...extra];
    });
    // Connection name, database and schema often repeat each other (e.g. a
    // profile named after its database, or a schema that shares the
    // database's name), collapse consecutive duplicates so the breadcrumb
    // doesn't spell out the same word two or three times in a row, then
    // always end on the open document (table/query/diagram) itself.
    let breadcrumb = $derived(
        [session.profile.name, session.profile.database, session.profile.schema].filter((part, i, all) => part && part !== all[i - 1]).concat(document.title),
    );
    let hasStableOrder = $derived(document.kind !== 'table' || activeResultSet?.stableOrder === true);
    $effect(() => {
        if (
            document.kind === 'table' &&
            session.connectionState === 'connected' &&
            session.capabilities.lazyDiscoverer &&
            !session.tableDetails[document.object] &&
            !session.tableDetailLoading[document.object] &&
            !session.tableDetailErrors[document.object]
        ) {
            untrack(() => void workspace.loadTableDetail(session, document.object));
        }
    });
    $effect(() => {
        if (document.kind === 'table' && document.view === 'data' && session.connectionState === 'connected' && document.status === 'idle' && document.results.length === 0) {
            // untrack: runTableQuery reads session.schema internally, and schema
            // discovery updates that value shortly after connecting, without
            // untrack, that nested read makes schema a dependency of this
            // effect too, re-triggering it (and re-fetching) every time schema
            // changes, which caused an infinite update loop.
            untrack(() => workspace.runTableQuery(session, document));
        }
    });
    function runPrimary() {
        output = 'results';
        document.outputCollapsed = false;
        workspace.run(session, document, document.selectionFrom === document.selectionTo ? 'statement' : 'selection');
    }
    function runNative() {
        output = 'results';
        document.outputCollapsed = false;
        workspace.runNativeQuery(session, document);
    }
    function runMode(mode: 'all' | 'selection' | 'statement') {
        output = 'results';
        document.outputCollapsed = false;
        workspace.run(session, document, mode);
    }
    function inspect() {
        document.outputCollapsed = false;
        if (document.selected < 0 && rows.length) document.selected = 0;
        inspectorMode = 'inspect';
        transferPanel = 'none';
        session.inspector = !session.inspector;
    }
    function editCell(row: Row, column: string) {
        document.selected = rows.indexOf(row);
        document.selectedColumn = column;
        inspectorMode = tableEditable ? 'edit' : 'inspect';
        transferPanel = 'none';
        session.inspector = true;
    }
    function toggleOutputPanel() {
        document.outputCollapsed = !document.outputCollapsed;
        if (document.outputCollapsed) session.inspector = false;
    }
    function openTransfer(kind: 'export' | 'import') {
        session.inspector = false;
        transferPanel = kind;
        transferTab = kind;
    }
</script>

{#if document.paramPrompt.length}<ParamPrompt {session} {document} />{/if}
<div class="document-view" class:output-collapsed={document.outputCollapsed}>
    <div class="query-toolbar">
        <div class="target">
            <Icon name="database" size={14} />
            {#each breadcrumb as part, i}{#if i > 0}<span class="target-separator"> / </span>{/if}{#if i === breadcrumb.length - 1}<strong>{part}</strong>{:else}<span>
                        {part}
                    </span>{/if}{/each}
            <span class="environment" class:stage={session.profile.environment === 'Stage'}>
                {session.profile.environment}
            </span>
        </div>
        <div class="toolbar-actions">
            {#if document.kind === 'query' && session.capabilities.sqlExecutor}<Button icon="bookmark" label="Save query" onclick={() => workspace.save(session, document)} />
                <Button icon="wand" label="Format SQL" onclick={() => workspace.formatQuery(session, document)} />
                <Button icon="chart" label="Visualize query results" disabled={!canVisualize} onclick={() => workspace.visualizeQueryResult(session, document)}>Visualize</Button>
                {#if session.capabilities.explainer}<Button
                        icon="explain"
                        label="Explain statement"
                        disabled={session.connectionState !== 'connected'}
                        onclick={() => workspace.explainQuery(session, document)}
                    >
                        Explain
                    </Button>{/if}
                <div class="run-group">
                    <Button
                        icon="play"
                        variant="primary"
                        label="Run query"
                        disabled={session.connectionState !== 'connected' || document.status === 'running'}
                        onclick={runPrimary}
                    >
                        Run <kbd>⌘ ↵</kbd>
                    </Button>
                    <details class="dropdown" use:closeOnOutsideClick>
                        <summary aria-label="Run options"><Icon name="chevron" size={12} /></summary>
                        <div class="menu">
                            <button onclick={() => runMode('selection')}>Run Selection</button>
                            <button onclick={() => runMode('statement')}>Run Current Statement</button>
                            <button onclick={() => runMode('all')}>Run All</button>
                        </div>
                    </details>
                </div>
                <Button
                    icon="stop"
                    label="Stop query"
                    disabled={document.status !== 'running'}
                    onclick={() => workspace.stop(document)}
                />{:else if document.kind === 'query' && session.capabilities.nativeQueryExecutor}<Button
                    icon="bookmark"
                    label="Save query"
                    onclick={() => workspace.save(session, document)}
                />
                <Button icon="chart" label="Visualize query results" disabled={!canVisualize} onclick={() => workspace.visualizeQueryResult(session, document)}>Visualize</Button>
                <Button icon="play" variant="primary" label="Run query" disabled={session.connectionState !== 'connected' || document.status === 'running'} onclick={runNative}>
                    Run <kbd>⌘ ↵</kbd>
                </Button>
                <Button
                    icon="stop"
                    label="Stop query"
                    disabled={document.status !== 'running'}
                    onclick={() => workspace.stop(document)}
                />{:else if document.kind === 'diagram'}<span class="read-only-label">READ ONLY</span>
                <Button
                    icon="refresh"
                    label="Refresh schema"
                    disabled={session.connectionState !== 'connected'}
                    onclick={() => workspace.loadSchema(session)}
                />{:else if document.kind === 'visualQuery'}<span class="read-only-label"> VISUAL QUERY </span>{:else if document.kind === 'chart'}<span class="read-only-label">
                    CHART PREVIEW
                </span>{:else if document.kind === 'explain'}<span class="read-only-label"> EXPLAIN </span>{:else}{#if document.transactionActive}<span class="transaction-label">
                        TRANSACTION
                    </span>{:else if !tableEditable && !documentEditable}<span class="read-only-label">READ ONLY</span>{/if}
                <Button
                    icon="play"
                    label="Begin transaction"
                    disabled={session.connectionState !== 'connected' || document.transactionActive || session.profile.readOnly}
                    onclick={() => workspace.beginTransaction(session, document)}
                >
                    Begin
                </Button>
                <Button icon="check" label="Commit transaction" disabled={!document.transactionActive} onclick={() => workspace.commitTransaction(session, document)}>
                    Commit
                </Button>
                <Button icon="stop" label="Rollback transaction" disabled={!document.transactionActive} onclick={() => workspace.rollbackTransaction(session, document)}>
                    Rollback
                </Button>
                <Button
                    icon="chart"
                    label="Visualize table"
                    disabled={session.connectionState !== 'connected'}
                    onclick={() => workspace.visualizeTable(session, document.object, document)}
                >
                    Visualize
                </Button>
                {#if session.capabilities.explainer && !session.capabilities.sqlExecutor}<Button
                        icon="explain"
                        label="Explain collection"
                        disabled={session.connectionState !== 'connected'}
                        onclick={() => workspace.explainTable(session, document)}
                    >
                        Explain
                    </Button>{/if}
                {#if session.capabilities.definitionDiscoverer}<Button
                        icon="code"
                        label="View definition"
                        disabled={session.connectionState !== 'connected'}
                        onclick={() => {
                            viewingDefinition = true;
                            workspace.loadTableDefinition(session, document.object);
                        }}
                    >
                        Definition
                    </Button>{/if}
                <Button icon="refresh" label="Refresh table" disabled={session.connectionState !== 'connected'} onclick={() => workspace.runTableQuery(session, document)} />{/if}
        </div>
    </div>
    {#if session.connectionState === 'connecting'}<div class="connection-warning">
            <Icon name="disconnect" />Connecting…
        </div>{:else if session.connectionState === 'error'}<div class="connection-warning">
            <Icon name="disconnect" />{session.connectionError || 'Connection failed.'}
            <button onclick={() => workspace.reconnect(session)}>Retry</button>
        </div>{:else if session.connectionState === 'disconnected'}<div class="connection-warning">
            <Icon name="disconnect" />This session is disconnected. Your work is preserved.
            <button onclick={() => workspace.reconnect(session)}>Reconnect</button>
        </div>{/if}
    {#if document.kind === 'diagram'}
        <SchemaDiagram {session} {document} />
    {:else if document.kind === 'visualQuery'}
        <VisualQueryBuilder {session} {document} />
    {:else if document.kind === 'chart'}
        <ChartDocument {session} {document} />
    {:else if document.kind === 'explain'}
        <ExplainDocument {session} {document} />
    {:else if document.kind === 'query'}
        <section class="editor-pane" style:height={document.outputCollapsed ? undefined : `${session.editorHeight}px`} aria-label="Query editor">
            {#if session.capabilities.nativeQueryLanguage === 'redis-command'}
                <RedisCommandEditor {document} {session} onrun={runNative} />
            {:else if session.capabilities.nativeQueryLanguage === 'mongodb-pipeline'}
                <NativeQueryEditor {document} {session} onrun={runNative} />
            {:else}
                <QueryEditor
                    {document}
                    engine={session.profile.engine}
                    schema={session.schema}
                    profileId={session.profile.id}
                    sessionId={session.id}
                    {hotspotRanges}
                    onrun={runPrimary}
                />
            {/if}
            <div class="editor-caption">
                <span>
                    {workspace.engineDisplayName(session.profile.engine)}
                </span>
                <span>
                    {session.capabilities.nativeQueryLanguage === 'redis-command'
                        ? 'Redis command'
                        : session.capabilities.nativeQueryLanguage === 'mongodb-pipeline'
                          ? 'Aggregation pipeline · JSON'
                          : 'SQL'} · UTF-8
                    <span class="caption-divider">|</span>
                    {workspace.preferences.restore ? 'Draft saved locally' : 'Unsaved local draft'}
                </span>
            </div>
        </section>
        {#if !document.outputCollapsed}<ResizeHandle
                value={session.editorHeight}
                min={120}
                max={520}
                orientation="horizontal"
                label="Editor height"
                onchange={(v) => (session.editorHeight = v)}
            />{/if}
    {:else}
        <!--
      The "schema / TABLE" eyebrow + big table-name heading was fully
      redundant with the session target bar above and the object tree in
      the sidebar, and ate a lot of vertical space in the document view.
      Commented out rather than deleted in case we want a slimmer version
      of it back later.
    <div class="object-heading">
      <div>
        <div class="eyebrow">{session.profile.schema || session.profile.database} / TABLE</div>
        <h1>
          <Icon name="table" size={23} />{objectLabel(document.object)}
        </h1>
      </div>
    </div>
    -->
        <nav class="object-views" aria-label="Table views">
            {#each ['data', 'structure', 'indexes', 'relationships'] as view}<button
                    class:active={document.view === view}
                    onclick={() => {
                        document.view = view as Document['view'];
                        if (view === 'structure') workspace.loadTableConstraints(session, document.object);
                    }}
                >
                    {view[0].toUpperCase() + view.slice(1)}
                </button>{/each}
        </nav>
    {/if}
    {#if tableDetailLoading}
        <div class="metadata-view">
            <div class="empty-state">
                <span class="spinner"></span>
                <h3>Loading table details</h3>
                <p>Fetching columns, indexes, and relationships…</p>
            </div>
        </div>
    {:else if tableDetailError}
        <div class="metadata-view">
            <div class="empty-state">
                <Icon name="alert" size={28} />
                <h3>Table details unavailable</h3>
                <p>{tableDetailError}</p>
            </div>
        </div>
    {:else if document.kind === 'chart' || document.kind === 'explain'}
        <!-- Chart and Explain documents own the full document body; they do not use the query/table results panel. -->
    {:else if document.kind === 'table' && document.view !== 'data'}
        <div class="metadata-view">
            {#if document.view === 'structure'}<h3>
                    Columns <span class="count">{schemaTable?.columns.length ?? 0}</span>
                </h3>
                <table class="metadata-table">
                    <thead>
                        <tr>
                            <th>Column</th>
                            <th>Type</th>
                            <th>Nullable</th>
                            <th>Key</th>
                        </tr>
                    </thead>
                    <tbody>
                        {#each schemaTable?.columns ?? [] as column}<tr>
                                <td>
                                    <Icon name={column.primaryKey ? 'key' : 'table'} size={13} />{column.name}
                                </td>
                                <td><code>{column.type}</code></td>
                                <td>{column.notNull ? 'No' : 'Yes'}</td>
                                <td>{column.primaryKey ? 'Primary key' : '-'}</td>
                            </tr>{/each}
                    </tbody>
                </table>
                {#if session.capabilities.constraintDiscoverer}<h3 class="constraints-heading">
                        Constraints <span class="count">{constraints.length}</span>
                    </h3>
                    {#if constraintsLoading}<div class="empty-state">
                            <span class="spinner"></span>
                            <p>Fetching constraints…</p>
                        </div>{:else if constraintsError}<div class="empty-state">
                            <Icon name="alert" size={24} />
                            <p>{constraintsError}</p>
                        </div>{:else if constraints.length}<table class="metadata-table">
                            <thead>
                                <tr>
                                    <th>Name</th>
                                    <th>Kind</th>
                                    <th>Detail</th>
                                </tr>
                            </thead>
                            <tbody>
                                {#each constraints as constraint}<tr>
                                        <td>{constraint.name}</td>
                                        <td>{constraint.kind === 'check' ? 'Check' : 'Unique'}</td>
                                        <td><code>{constraint.kind === 'check' ? constraint.expression : (constraint.columns ?? []).join(', ')}</code></td>
                                    </tr>{/each}
                            </tbody>
                        </table>{:else}<div class="empty-state">
                            <Icon name="key" size={24} />
                            <p>No CHECK or named UNIQUE constraints beyond the indexes above.</p>
                        </div>{/if}{/if}{:else if document.view === 'indexes'}<h3>
                    Indexes <span class="count">{schemaTable?.indexes.length ?? 0}</span>
                </h3>
                {#if schemaTable?.indexes.length}<table class="metadata-table">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Columns</th>
                                <th>Unique</th>
                            </tr>
                        </thead>
                        <tbody>
                            {#each schemaTable.indexes as index}<tr>
                                    <td>{index.name}</td>
                                    <td>{index.columns.join(', ')}</td>
                                    <td>{index.unique ? 'Yes' : 'No'}</td>
                                </tr>{/each}
                        </tbody>
                    </table>{:else}<div class="empty-state">
                        <Icon name="key" size={28} />
                        <h3>No indexes</h3>
                        <p>This table has no indexes beyond its primary key, if any.</p>
                    </div>{/if}{:else}<h3>Relationships</h3>
                {#each schemaTable?.foreignKeys ?? [] as fk}<button
                        class="relationship-card"
                        title={selectedRow && selectedRow[fk.column] != null
                            ? `Open ${objectLabel(fk.refTable)} filtered to this row's ${fk.column}`
                            : `Open ${objectLabel(fk.refTable)}`}
                        onclick={() =>
                            selectedRow && selectedRow[fk.column] != null
                                ? workspace.openRelatedRows(session, fk, selectedRow[fk.column])
                                : workspace.openDocument(session, 'table', fk.refTable)}
                    >
                        <Icon name="link" />
                        <span>{objectLabel(document.object)}.{fk.column}</span>
                        <Icon name="arrow" />
                        <span>{objectLabel(fk.refTable)}.{fk.refColumn}</span>
                        {#if fk.onDelete || fk.onUpdate}<small>ON DELETE {fk.onDelete ?? 'NO ACTION'} · ON UPDATE {fk.onUpdate ?? 'NO ACTION'}</small>{/if}
                    </button>{/each}
                {#each reverseForeignKeys as { table, fk }}<button
                        class="relationship-card"
                        title={selectedRow && selectedRow[fk.refColumn] != null
                            ? `Open ${objectLabel(table)} filtered to this row's ${fk.refColumn}`
                            : `Open ${objectLabel(table)}`}
                        onclick={() =>
                            selectedRow && selectedRow[fk.refColumn] != null
                                ? workspace.openRelatedRows(session, { ...fk, refTable: table, refColumn: fk.column }, selectedRow[fk.refColumn])
                                : workspace.openDocument(session, 'table', table)}
                    >
                        <Icon name="link" />
                        <span>{objectLabel(table)}.{fk.column}</span>
                        <Icon name="arrow" />
                        <span>{objectLabel(document.object)}.{fk.refColumn}</span>
                        {#if fk.onDelete || fk.onUpdate}<small>ON DELETE {fk.onDelete ?? 'NO ACTION'} · ON UPDATE {fk.onUpdate ?? 'NO ACTION'}</small>{/if}
                    </button>{/each}
                {#if !(schemaTable?.foreignKeys.length || reverseForeignKeys.length)}<div class="empty-state">
                        <Icon name="link" size={28} />
                        <h3>No relationships</h3>
                        <p>This table has no foreign-key relationships.</p>
                    </div>{/if}{/if}
        </div>
    {:else}
        <div class="results-header" class:collapsed={document.outputCollapsed}>
            <div class="result-tabs">
                <button class:active={output === 'results'} onclick={() => (output = 'results')}>
                    <Icon name="table" size={14} />Results
                    <span class="count">{document.results.length}</span>
                </button>
                <button class:active={output === 'messages'} onclick={() => (output = 'messages')}>
                    Messages{#if document.status === 'error'}<span class="error-dot"></span>{/if}
                </button>
            </div>
            <span class="execution-info">
                {#if document.status === 'running'}<span class="spinner"></span>
                    Running…{:else if document.hasResults}<span class="status-dot"></span>
                    {countLabel}
                    <span>·</span>
                    {document.elapsed || '-'} ms{/if}
            </span>
            <Button
                icon={document.outputCollapsed ? 'chevronUp' : 'chevronDown'}
                label={document.outputCollapsed ? 'Show results panel' : 'Hide results panel'}
                active={!document.outputCollapsed}
                onclick={toggleOutputPanel}
            />
            <Button icon="panel" label="Toggle row inspector" active={session.inspector} disabled={document.outputCollapsed} onclick={inspect} />
        </div>
        {#if document.outputCollapsed}<button class="results-collapsed-strip" onclick={toggleOutputPanel}>
                <Icon name="chevronUp" size={14} />
                <span>Results hidden</span>
                {#if document.hasResults}<strong>
                        {countLabel} · {document.elapsed || '-'} ms
                    </strong>{:else}<strong>Show panel</strong>{/if}
            </button>
        {:else if output === 'messages'}<div class="messages" role="log">
                <span class="eyebrow">EXECUTION LOG</span>
                <p class:error-text={document.status === 'error'}>
                    {document.message || 'Write a query and run it to see results here.'}
                </p>
                <small>Target: {session.profile.name} / {session.profile.database}</small>
            </div>
        {:else if !document.hasResults}<div class="empty-state">
                <span class="empty-icon"><Icon name="code" size={28} /></span>
                <h3>Room for your next discovery</h3>
                <p>Write a query, then run it against your database.</p>
                <Button icon="play" variant="outline" disabled={session.connectionState !== 'connected'} onclick={runPrimary}>Run</Button>
            </div>
        {:else}
            {#if document.results.length > 1}<div class="segmented result-set-tabs">
                    {#each document.results as r, i}<button class:active={document.activeResult === i} onclick={() => (document.activeResult = i)}>
                            Statement {i + 1}{#if r.kind === 'error'}<span class="error-dot"></span>{/if}
                        </button>{/each}
                </div>{/if}
            {#if activeResultSet?.kind === 'status'}<div class="empty-state">
                    <Icon name="check" size={28} />
                    <h3>{activeResultSet.rowsAffected ?? 0} rows affected</h3>
                    {#if activeResultSet.lastInsertId}<p>
                            Last insert ID: {activeResultSet.lastInsertId}
                        </p>{/if}
                </div>
            {:else if activeResultSet?.kind === 'error'}<div class="empty-state">
                    <Icon name="disconnect" size={28} />
                    <h3>Statement failed</h3>
                    <p class="error-text">{activeResultSet.error}</p>
                </div>
            {:else if activeResultSet?.kind === 'documents'}<div class="empty-state">
                    <Icon name="table" size={28} />
                    <h3>This result isn't a table</h3>
                    <p>Document results aren't viewable here yet.</p>
                </div>
            {:else if activeResultSet?.kind === 'keyvalue'}<div class="results-body">
                    <div class="grid-container"><KeyList {session} {document} result={activeResultSet} /></div>
                    {#if session.inspector}<div class="inspector-resizer">
                            <ResizeHandle value={session.inspectorWidth} min={240} max={640} reverse label="Inspector width" onchange={(v) => (session.inspectorWidth = v)} />
                        </div>
                        <div class="inspector-wrapper" style:width={`${session.inspectorWidth}px`}>
                            <KeyInspector {session} {document} onclose={() => (session.inspector = false)} />
                        </div>{/if}
                </div>
            {:else}
                <div class="grid-toolbar">
                    <label class="search-field filter-field">
                        <Icon name="search" size={14} />
                        <input
                            aria-label="Search loaded rows"
                            placeholder="Search loaded rows…"
                            value={document.filter}
                            oninput={(e) => {
                                document.filter = e.currentTarget.value;
                                document.selected = -1;
                            }}
                        />
                        {#if document.filter}<button aria-label="Clear search" onclick={() => (document.filter = '')}>
                                <Icon name="close" size={12} />
                            </button>{/if}
                    </label>
                    {#if document.kind === 'table'}<label class="search-field filter-field">
                            <Icon name="filter" size={14} />
                            <input
                                aria-label="Filter table"
                                placeholder="Filter table…"
                                value={document.tableFilter}
                                onchange={(e) => {
                                    document.tableFilter = e.currentTarget.value;
                                    document.tableFilterColumn = '';
                                    workspace.runTableQuery(session, document);
                                }}
                            />
                            {#if document.tableFilterColumn}<span class="filter-field-exact" title={`Exact match on ${document.tableFilterColumn}`}
                                    >= {document.tableFilterColumn}</span
                                >{/if}
                            {#if document.tableFilter}<button
                                    aria-label="Clear table filter"
                                    onclick={() => {
                                        document.tableFilter = '';
                                        document.tableFilterColumn = '';
                                        workspace.runTableQuery(session, document);
                                    }}
                                >
                                    <Icon name="close" size={12} />
                                </button>{/if}
                        </label>
                        <Button variant="outline" onclick={() => workspace.resetTableView(session, document)}>Reset view</Button>
                        <Button
                            icon="plus"
                            variant="outline"
                            disabled={!tableEditable && !documentEditable}
                            onclick={() => (documentEditable ? (insertingDocument = true) : workspace.insertRow(session, document))}
                        >
                            {documentEditable ? 'Insert document' : 'Insert row'}
                        </Button>
                        <Button
                            variant="danger"
                            disabled={(!tableEditable && !documentEditable) || !selectedRow}
                            onclick={() => (documentEditable ? workspace.deleteDocument(session, document, selectedRow) : workspace.deleteRow(session, document, selectedRow))}
                        >
                            {documentEditable ? 'Delete document' : 'Delete row'}
                        </Button>
                        <Button icon="link" variant="outline" disabled={!tableEditable} onclick={() => openTransfer('import')}>Import CSV</Button>{:else}<span
                            class="grid-toolbar-note">Read only</span
                        >{/if}
                    <Button icon="download" label="Export results" onclick={() => openTransfer('export')}>Export…</Button>
                    <details class="dropdown columns-dropdown" use:closeOnOutsideClick>
                        <summary aria-label="Columns"><Icon name="columns" /></summary>
                        <div class="menu columns-menu">
                            {#each allColumnsOrdered as column}<div class="columns-menu-row">
                                    <Input type="checkbox" bind:checked={() => !gridLayout.hidden.includes(column), () => toggleColumnHidden(column)} label={column} />
                                    <button type="button" aria-label={`Move ${column} left`} disabled={allColumnsOrdered[0] === column} onclick={() => moveColumn(column, -1)}>
                                        ←
                                    </button>
                                    <button
                                        type="button"
                                        aria-label={`Move ${column} right`}
                                        disabled={allColumnsOrdered[allColumnsOrdered.length - 1] === column}
                                        onclick={() => moveColumn(column, 1)}
                                    >
                                        →
                                    </button>
                                </div>{/each}
                        </div>
                    </details>
                </div>
                {#if document.kind === 'table' && document.edits.length}<div class="edit-stage">
                        <div class="edit-stage-summary">
                            <strong>
                                {document.edits.length} staged change{document.edits.length === 1 ? '' : 's'}
                            </strong>
                            <span> Review before applying. Updates/deletes check the original loaded row for conflicts. </span>
                            <Button variant="primary" icon="check" disabled={document.status === 'running'} onclick={() => workspace.applyEdits(session, document)}>Apply</Button>
                            <Button variant="outline" disabled={document.status === 'running'} onclick={() => workspace.discardEdits(document)}>Discard</Button>
                        </div>
                        {#each document.edits as edit}<div class="edit-card">
                                <span class="edit-kind">{edit.kind}</span>
                                {#if edit.kind === 'insert'}<div class="insert-grid">
                                        {#each editableColumns as column}<label>
                                                <span>{column.name}</span>
                                                <Select
                                                    value={edit.values[column.name]?.mode ?? 'default'}
                                                    options={insertModeOptions}
                                                    onchange={(mode) =>
                                                        workspace.setInsertValue(
                                                            document,
                                                            edit.id,
                                                            column.name,
                                                            mode as 'value' | 'null' | 'default',
                                                            edit.values[column.name]?.value,
                                                        )}
                                                />
                                                {#if edit.values[column.name]?.mode === 'value'}<input
                                                        value={String(edit.values[column.name]?.value ?? '')}
                                                        oninput={(e) => workspace.setInsertValue(document, edit.id, column.name, 'value', e.currentTarget.value)}
                                                    />{/if}
                                            </label>{/each}
                                    </div>{:else if edit.kind === 'update'}<span>
                                        Update {Object.keys(edit.values).join(', ')}; expects exactly 1 row.
                                    </span>{:else}<span>
                                        Delete; expects exactly 1 row. Identity:
                                        {schemaTable?.columns
                                            .filter((column) => column.primaryKey)
                                            .map((column) => `${column.name}=${edit.original?.[column.name] ?? ''}`)
                                            .join(', ')}
                                    </span>{/if}
                            </div>{/each}
                    </div>{/if}
                <div class="results-body">
                    <div class="grid-container">
                        {#if rows.length}{#key document.viewVersion}<DataGrid
                                    {rows}
                                    {resultColumns}
                                    {document}
                                    layout={gridLayout}
                                    onsort={document.kind === 'table' ? () => workspace.runTableQuery(session, document) : undefined}
                                    onedit={document.kind === 'table' ? editCell : undefined}
                                    oninspect={() => {
                                        inspectorMode = 'inspect';
                                        session.inspector = true;
                                        if (document.selected < 0) document.selected = 0;
                                    }}
                                />{/key}{:else}<div class="empty-state">
                                <Icon name="search" size={28} />
                                <h3>No matching rows</h3>
                                <p>Try a different filter to find what you need.</p>
                                <Button variant="outline" onclick={() => (document.filter = '')}>Clear filter</Button>
                            </div>{/if}
                    </div>
                    {#if session.inspector || transferPanel !== 'none'}<div class="inspector-resizer">
                            <ResizeHandle value={session.inspectorWidth} min={240} max={480} reverse label="Inspector width" onchange={(v) => (session.inspectorWidth = v)} />
                        </div>
                        <div class="inspector-wrapper" style:width={`${session.inspectorWidth}px`}>
                            {#if transferPanel !== 'none'}
                                <TransferPanel {session} {document} bind:tab={transferTab} onclose={() => (transferPanel = 'none')} />
                            {:else}
                                <RowDetails
                                    row={selectedRow}
                                    columns={editableColumns}
                                    staged={document.edits.find(
                                        (edit) => edit.kind === 'update' && edit.original && selectedRow && workspace.rowKey(edit.original) === workspace.rowKey(selectedRow),
                                    )?.values ?? {}}
                                    bind:mode={inspectorMode}
                                    editable={documentEditable
                                        ? !!selectedRow
                                        : tableEditable &&
                                          !!selectedRow &&
                                          !Object.values(selectedRow).some((v) => typeof v === 'object' && v !== null) &&
                                          !document.edits.some(
                                              (e) => e.kind === 'delete' && e.original && selectedRow && workspace.rowKey(e.original) === workspace.rowKey(selectedRow),
                                          )}
                                    reason={session.profile.readOnly
                                        ? 'This connection is read-only.'
                                        : document.kind !== 'table'
                                          ? 'Open the source table to edit records safely.'
                                          : documentEditable
                                            ? 'Select a document to edit it.'
                                            : 'Editing requires a primary key, a complete row without binary values, and no pending deletion.'}
                                    busy={document.status === 'running'}
                                    focusColumn={document.selectedColumn}
                                    rowNumber={document.selected + 1}
                                    pendingCount={document.edits.length}
                                    previewVersion={JSON.stringify(document.edits)}
                                    documentEdit={documentEditable
                                        ? {
                                              sessionId: session.id,
                                              documentId: document.id,
                                              collection: document.object,
                                              onapplied: () => workspace.runTableQuery(session, document),
                                          }
                                        : null}
                                    onclose={() => (session.inspector = false)}
                                    onchange={(column, value) => selectedRow && workspace.setCellEdit(session, document, selectedRow, column, value)}
                                    onapply={() => workspace.applyEdits(session, document)}
                                    onpreview={() => workspace.previewEdits(session, document)}
                                    ondiscard={() => workspace.discardEdits(document)}
                                />
                            {/if}
                        </div>{/if}
                </div>
                <div class="results-footer">
                    <span>
                        {rows.length ? `1–${rows.length}` : '0'} of {filtered.length} loaded rows{#if activeResultSet?.truncated}
                            · more available{/if}{#if document.kind === 'table' && !hasStableOrder}
                            · <span class="error-text"> no stable order. Row order may vary between pages </span>{/if}
                    </span>
                    <span>
                        {#if document.kind === 'table' && activeResultSet?.truncated}<button
                                class="text-button"
                                onclick={() => workspace.runTableQuery(session, document, { append: true })}
                            >
                                Load more rows
                            </button>
                            <span class="footer-divider">|</span>{/if}
                        {document.selected >= 0 ? `Row ${document.selected + 1} selected` : 'Select a cell to inspect'}
                    </span>
                </div>
            {/if}
        {/if}
    {/if}
</div>
{#if viewingDefinition}<Modal
        title={objectLabel(document.object)}
        subtitle={definition?.verbatim === false ? 'Reconstructed DDL' : 'Definition'}
        onclose={() => (viewingDefinition = false)}
        wide
    >
        <div class="function-detail">
            {#if definitionLoading}<div class="empty-state">
                    <span class="spinner"></span>
                    <p>Fetching definition…</p>
                </div>{:else if definitionError}<p class="function-detail-empty">{definitionError}</p>{:else if definition}
                {#if definition.verbatim === false}<p class="function-detail-empty">
                        PostgreSQL has no single built-in function for a table's DDL text; this is assembled from the same column/constraint/index metadata the Structure tab shows,
                        not read verbatim from the engine's own catalog.
                    </p>{/if}
                <pre class="function-detail-body">{definition.sql}</pre>
            {/if}
        </div>
    </Modal>{/if}
{#if insertingDocument}<Modal title="Insert document" subtitle={document.object} onclose={() => (insertingDocument = false)} wide>
        <DocumentEditor
            sessionId={session.id}
            documentId={document.id}
            collection={document.object}
            id={null}
            onclose={() => (insertingDocument = false)}
            onapplied={() => workspace.runTableQuery(session, document)}
        />
    </Modal>{/if}
