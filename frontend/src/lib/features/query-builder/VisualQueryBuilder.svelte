<script lang="ts">
    import { onMount } from 'svelte';
    import { MarkerType, type Connection, type Edge, type Node } from '@xyflow/svelte';
    import type {
        Document,
        Session,
        SchemaTable,
        VisualQueryAggregate,
        VisualQueryColumn,
        VisualQueryFilter,
        VisualQueryJoin,
        VisualQueryJoinType,
        VisualQueryOperator,
        VisualQueryOrder,
        VisualQueryRef,
    } from '../../state/types';
    import { objectLabel } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import ResizeHandle from '../../components/ResizeHandle.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';
    import QueryTableNode, { type QueryTableNodeData } from './QueryTableNode.svelte';
    import QueryCanvas from './QueryCanvas.svelte';
    import JoinEditor, { type JoinEditorTable } from './JoinEditor.svelte';
    import { COLUMN_DRAG_MIME, TABLE_DRAG_MIME, parseColumnDrag, setTableDragImage } from './dnd';
    import { copy } from '../results/format';

    let { session, document }: { session: Session; document: Document } = $props();
    const nodeTypes = { table: QueryTableNode };
    const defaultEdgeOptions = {
        type: 'smoothstep',
        markerEnd: { type: MarkerType.ArrowClosed, color: 'var(--diagram-edge)' },
        focusable: false,
        interactionWidth: 18,
    };
    const fitViewOptions = { padding: 0.2, minZoom: 0.6, maxZoom: 1, duration: 0 };
    const snapGrid: [number, number] = [16, 16];

    const aggregateLabels: Record<VisualQueryAggregate, string> = {
        '': 'None',
        count: 'Count',
        sum: 'Sum',
        avg: 'Average',
        min: 'Minimum',
        max: 'Maximum',
    };
    const operatorLabels: Record<VisualQueryOperator, string> = {
        eq: '=',
        ne: '≠',
        lt: '<',
        lte: '≤',
        gt: '>',
        gte: '≥',
        like: 'LIKE',
        isNull: 'IS NULL',
        isNotNull: 'IS NOT NULL',
        in: 'IN',
    };
    const aggregateOptions: SelectOption[] = Object.entries(aggregateLabels).map(([value, label]) => ({ value, label }));
    const operatorOptions: SelectOption[] = Object.entries(operatorLabels).map(([value, label]) => ({
        value,
        label,
    }));
    const orderDirectionOptions: SelectOption[] = [
        { value: 'asc', label: 'Ascending' },
        { value: 'desc', label: 'Descending' },
    ];

    let tableSearch = $state('');
    let draggingTable = $state<string | null>(null);
    onMount(() => {
        // WebKitGTK sometimes never fires `dragend` on the source row once a
        // drop lands inside the canvas's transformed (pan/zoom) surface,
        // leaving this row stuck in its dragging visual state. `drop`
        // reliably fires regardless, so use it as a backstop.
        const clear = () => (draggingTable = null);
        window.addEventListener('drop', clear);
        window.addEventListener('dragend', clear);
        return () => {
            window.removeEventListener('drop', clear);
            window.removeEventListener('dragend', clear);
        };
    });
    let columnDropTarget = $state<'columns' | 'filters' | 'orderBy' | null>(null);
    let joinEditor = $state<{
        tables: JoinEditorTable[];
        initial?: VisualQueryJoin;
        index?: number;
        defaultLeftAlias?: string;
    } | null>(null);

    let ast = $derived(document.visualQueryAst);
    let tablesOnCanvas = $derived(
        ast.tables
            .map((t) => ({ ref: t, schema: session.schema.find((s) => s.name === t.object) }))
            .filter((t): t is { ref: (typeof ast.tables)[number]; schema: SchemaTable } => !!t.schema),
    );
    let aliasToSchema = $derived(new Map(tablesOnCanvas.map((t) => [t.ref.alias, t.schema])));
    let availableTables = $derived.by(() => {
        const used = new Set(ast.tables.map((t) => t.object));
        const search = tableSearch.trim().toLowerCase();
        return session.schema.filter((t) => {
            if (t.kind !== 'table' && t.kind !== 'view') return false;
            if (used.has(t.name)) return false;
            if (!search) return true;
            return objectLabel(t.name).toLowerCase().includes(search);
        });
    });
    let hasAggregate = $derived(ast.columns.some((c) => c.aggregate));
    // Mongo's Aggregate rejects a join, and Mongo/Redis have no relational
    // group-by concept at all; these gate the controls that would let a
    // user build an AST the backend can't honor, rather than letting them
    // discover that from a runtime error.
    let supportsJoins = $derived(session.capabilities.supportsJoins);
    let supportsGroupBy = $derived(session.capabilities.supportsGroupBy);
    let columnOptions: SelectOption[] = $derived(
        tablesOnCanvas.flatMap((t) =>
            t.schema.columns.map((c) => ({
                value: `${t.ref.alias}.${c.name}`,
                label: `${t.ref.alias}.${c.name}`,
            })),
        ),
    );

    function nextAlias(): string {
        const used = new Set(ast.tables.map((t) => t.alias));
        let n = 1;
        while (used.has(`t${n}`)) n++;
        return `t${n}`;
    }
    function autoPosition(index: number) {
        return { x: 60 + (index % 3) * 320, y: 60 + Math.floor(index / 3) * 420 };
    }
    function joinKey(a: string, b: string) {
        return [a, b].sort().join('~');
    }
    function refsMatch(a: VisualQueryRef, b: VisualQueryRef) {
        return a.table === b.table && a.column === b.column;
    }

    function addTableAt(object: string, position?: { x: number; y: number }) {
        if (ast.tables.some((t) => t.object === object)) {
            workspace.notify(`${objectLabel(object)} is already on the canvas.`);
            return;
        }
        const alias = nextAlias();
        document.visualQueryAst = { ...ast, tables: [...ast.tables, { object, alias }] };
        if (position) workspace.updateVisualQueryPosition(document, alias, position);
    }
    function addTable(object: string) {
        addTableAt(object);
        tableSearch = '';
    }
    function removeTable(alias: string) {
        document.visualQueryAst = {
            ...ast,
            tables: ast.tables.filter((t) => t.alias !== alias),
            joins: ast.joins.filter((j) => j.left.table !== alias && j.right.table !== alias),
            columns: ast.columns.filter((c) => c.table !== alias),
            filters: ast.filters.filter((f) => f.table !== alias),
            having: ast.having.filter((f) => f.table !== alias),
            orderBy: ast.orderBy.filter((o) => o.table !== alias),
        };
        const positions = { ...document.visualQueryPositions };
        delete positions[alias];
        document.visualQueryPositions = positions;
    }
    function toggleColumn(alias: string, column: string) {
        const exists = ast.columns.some((c) => c.table === alias && c.column === column);
        document.visualQueryAst = {
            ...ast,
            columns: exists
                ? ast.columns.filter((c) => !(c.table === alias && c.column === column))
                : [...ast.columns, { table: alias, column, aggregate: '' as VisualQueryAggregate }],
        };
    }
    function updateColumn(index: number, patch: Partial<VisualQueryColumn>) {
        const columns = ast.columns.slice();
        columns[index] = { ...columns[index], ...patch };
        document.visualQueryAst = { ...ast, columns };
    }
    function removeColumn(index: number) {
        document.visualQueryAst = { ...ast, columns: ast.columns.filter((_, i) => i !== index) };
    }

    function handleColumnsDrop(event: DragEvent) {
        columnDropTarget = null;
        const drag = parseColumnDrag(event);
        if (!drag) return;
        event.preventDefault();
        const additions = drag.columns
            .filter((column) => !ast.columns.some((c) => c.table === drag.table && c.column === column))
            .map((column) => ({
                table: drag.table,
                column,
                aggregate: '' as VisualQueryAggregate,
            }));
        if (additions.length) document.visualQueryAst = { ...ast, columns: [...ast.columns, ...additions] };
    }
    function handleFilterDrop(event: DragEvent) {
        columnDropTarget = null;
        const drag = parseColumnDrag(event);
        if (!drag) return;
        event.preventDefault();
        document.visualQueryAst = {
            ...ast,
            filters: [
                ...ast.filters,
                {
                    table: drag.table,
                    column: drag.columns[0],
                    operator: 'eq' as VisualQueryOperator,
                },
            ],
        };
    }
    function handleOrderDrop(event: DragEvent) {
        columnDropTarget = null;
        const drag = parseColumnDrag(event);
        if (!drag) return;
        event.preventDefault();
        document.visualQueryAst = {
            ...ast,
            orderBy: [...ast.orderBy, { table: drag.table, column: drag.columns[0], descending: false }],
        };
    }
    function handleColumnDragOver(event: DragEvent) {
        if (!event.dataTransfer?.types.includes(COLUMN_DRAG_MIME)) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = 'copy';
    }
    function handleColumnDragEnter(event: DragEvent, target: typeof columnDropTarget) {
        if (!event.dataTransfer?.types.includes(COLUMN_DRAG_MIME)) return;
        columnDropTarget = target;
    }
    function handleColumnDragLeave(event: DragEvent, target: typeof columnDropTarget) {
        if (columnDropTarget !== target) return;
        // Cast to `globalThis.Node`, not the bare name — `Node` is shadowed in
        // this file's type space by the imported xyflow `Node`.
        const related = event.relatedTarget;
        if (related instanceof globalThis.Node && (event.currentTarget as HTMLElement).contains(related)) return;
        columnDropTarget = null;
    }

    function blankRef(): VisualQueryRef {
        const first = tablesOnCanvas[0];
        return { table: first?.ref.alias ?? '', column: first?.schema.columns[0]?.name ?? '' };
    }
    function addFilter() {
        document.visualQueryAst = {
            ...ast,
            filters: [...ast.filters, { ...blankRef(), operator: 'eq' as VisualQueryOperator }],
        };
    }
    function updateFilter(index: number, patch: Partial<VisualQueryFilter>) {
        const filters = ast.filters.slice();
        filters[index] = { ...filters[index], ...patch };
        document.visualQueryAst = { ...ast, filters };
    }
    function removeFilter(index: number) {
        document.visualQueryAst = { ...ast, filters: ast.filters.filter((_, i) => i !== index) };
    }

    function addHaving() {
        document.visualQueryAst = {
            ...ast,
            having: [
                ...ast.having,
                {
                    ...blankRef(),
                    aggregate: 'count' as VisualQueryAggregate,
                    operator: 'gt' as VisualQueryOperator,
                },
            ],
        };
    }
    function updateHaving(index: number, patch: Partial<VisualQueryFilter>) {
        const having = ast.having.slice();
        having[index] = { ...having[index], ...patch };
        document.visualQueryAst = { ...ast, having };
    }
    function removeHaving(index: number) {
        document.visualQueryAst = { ...ast, having: ast.having.filter((_, i) => i !== index) };
    }

    function addOrder() {
        document.visualQueryAst = {
            ...ast,
            orderBy: [...ast.orderBy, { ...blankRef(), descending: false }],
        };
    }
    function updateOrder(index: number, patch: Partial<VisualQueryOrder>) {
        const orderBy = ast.orderBy.slice();
        orderBy[index] = { ...orderBy[index], ...patch };
        document.visualQueryAst = { ...ast, orderBy };
    }
    function removeOrder(index: number) {
        document.visualQueryAst = { ...ast, orderBy: ast.orderBy.filter((_, i) => i !== index) };
    }

    function joinEditorTables(): JoinEditorTable[] {
        return tablesOnCanvas.map((t) => ({
            alias: t.ref.alias,
            label: `${t.ref.alias} · ${objectLabel(t.schema.name)}`,
            columns: t.schema.columns.map((c) => c.name),
        }));
    }
    function openJoinEditorFor(alias: string) {
        joinEditor = { tables: joinEditorTables(), defaultLeftAlias: alias };
    }
    function openJoinEditorAt(index: number) {
        joinEditor = { tables: joinEditorTables(), initial: ast.joins[index], index };
    }
    function saveJoin(join: VisualQueryJoin) {
        if (joinEditor?.index !== undefined) {
            const joins = ast.joins.slice();
            joins[joinEditor.index] = join;
            document.visualQueryAst = { ...ast, joins };
        } else {
            document.visualQueryAst = { ...ast, joins: [...ast.joins, join] };
        }
    }
    function removeJoinAt(index: number) {
        document.visualQueryAst = { ...ast, joins: ast.joins.filter((_, i) => i !== index) };
    }
    function appendJoin(left: VisualQueryRef, right: VisualQueryRef) {
        document.visualQueryAst = {
            ...ast,
            joins: [...ast.joins, { type: 'inner' as VisualQueryJoinType, left, right }],
        };
    }
    function acceptSuggestedJoin(leftAlias: string, rightAlias: string, left: string, right: string) {
        appendJoin({ table: leftAlias, column: left }, { table: rightAlias, column: right });
    }

    function handleEdgeClick(edge: Edge) {
        const data = edge.data as {
            index?: number;
            suggested?: { a: string; b: string; leftCol: string; rightCol: string };
        };
        if (data?.suggested) acceptSuggestedJoin(data.suggested.a, data.suggested.b, data.suggested.leftCol, data.suggested.rightCol);
        else if (data?.index !== undefined) openJoinEditorAt(data.index);
    }

    // Draw-a-line joins: a connection dragged between two column handles on
    // the canvas. `source`/`target` are node ids (table aliases);
    // `sourceHandle`/`targetHandle` are the column names each Handle in
    // QueryTableNode.svelte is id-ed by.
    function isValidJoinConnection(connection: Edge | Connection): boolean {
        if (!supportsJoins || connection.source === connection.target) return false;
        return !joinedPairs.has(joinKey(connection.source, connection.target));
    }
    function handleConnect(connection: Connection) {
        if (!connection.sourceHandle || !connection.targetHandle) return;
        appendJoin({ table: connection.source, column: connection.sourceHandle }, { table: connection.target, column: connection.targetHandle });
    }

    let nodes = $derived<Node[]>(
        tablesOnCanvas.map((t, index) => ({
            id: t.ref.alias,
            type: 'table',
            position: document.visualQueryPositions[t.ref.alias] ?? autoPosition(index),
            data: {
                table: t.schema,
                alias: t.ref.alias,
                selectedColumns: new Set(ast.columns.filter((c) => c.table === t.ref.alias).map((c) => c.column)),
                onToggleColumn: (column: string) => toggleColumn(t.ref.alias, column),
                onJoinTo: supportsJoins ? () => openJoinEditorFor(t.ref.alias) : undefined,
                onRemove: () => removeTable(t.ref.alias),
            } as unknown as Record<string, unknown>,
        })),
    );

    let joinedPairs = $derived(new Set(ast.joins.map((j) => joinKey(j.left.table, j.right.table))));
    let suggestedJoins = $derived.by(() => {
        const out: { a: string; b: string; leftCol: string; rightCol: string }[] = [];
        if (!supportsJoins) return out;
        for (let i = 0; i < tablesOnCanvas.length; i++) {
            for (let j = i + 1; j < tablesOnCanvas.length; j++) {
                const a = tablesOnCanvas[i];
                const b = tablesOnCanvas[j];
                if (joinedPairs.has(joinKey(a.ref.alias, b.ref.alias))) continue;
                const forward = a.schema.foreignKeys.find((fk) => fk.refTable === b.schema.name);
                if (forward) {
                    out.push({
                        a: a.ref.alias,
                        b: b.ref.alias,
                        leftCol: forward.column,
                        rightCol: forward.refColumn,
                    });
                    continue;
                }
                const backward = b.schema.foreignKeys.find((fk) => fk.refTable === a.schema.name);
                if (backward)
                    out.push({
                        a: b.ref.alias,
                        b: a.ref.alias,
                        leftCol: backward.column,
                        rightCol: backward.refColumn,
                    });
            }
        }
        return out;
    });
    let edges = $derived<Edge[]>([
        ...ast.joins.map((j, index) => ({
            id: `join-${index}`,
            source: j.left.table,
            target: j.right.table,
            label: `${j.left.column} ${j.type} ${j.right.column}`,
            class: 'query-edge',
            focusable: false,
            data: { index },
        })),
        ...suggestedJoins.map((s) => ({
            id: `suggest-${s.a}-${s.b}`,
            source: s.a,
            target: s.b,
            label: `Suggested: ${s.leftCol} → ${s.rightCol}`,
            class: 'query-edge suggested',
            style: 'stroke-dasharray: 5 4;',
            focusable: false,
            data: { suggested: s },
        })),
    ]);

    $effect(() => {
        const columns = ast.columns;
        const anyAggregate = columns.some((c) => c.aggregate);
        const plain = columns.filter((c) => !c.aggregate).map((c) => ({ table: c.table, column: c.column }));
        const groupBy = anyAggregate ? plain : [];
        const same = groupBy.length === ast.groupBy.length && groupBy.every((g, i) => refsMatch(g, ast.groupBy[i]));
        if (!same) document.visualQueryAst = { ...ast, groupBy };
    });

    $effect(() => {
        // Track every field the compiler cares about so this effect re-runs on
        // any AST edit, then debounce the actual backend round-trip.
        void JSON.stringify(ast);
        document.visualQuerySql = '';
        document.visualQueryError = '';
        document.sql = '';
        const timer = setTimeout(() => workspace.compileVisualQuery(session, document), 150);
        return () => clearTimeout(timer);
    });
</script>

<section class="visual-query-builder" style:flex-grow={document.outputCollapsed ? 1 : undefined} style:max-height={document.outputCollapsed ? 'none' : undefined}>
    <header class="visual-query-toolbar">
        <div class="diagram-title">
            <Icon name="columns" size={18} />
            <div>
                <strong>Visual query</strong>
                <span>{tablesOnCanvas.length} tables · {ast.joins.length} joins</span>
            </div>
        </div>
        <label class="search-field diagram-search">
            <Icon name="search" size={14} />
            <input aria-label="Add a table to the query" placeholder="Add a table…" bind:value={tableSearch} />
        </label>
        {#if tableSearch && availableTables.length}<div class="table-picker" role="listbox">
                {#each availableTables.slice(0, 8) as t}<button
                        role="option"
                        aria-selected="false"
                        class:dragging={draggingTable === t.name}
                        draggable="true"
                        ondragstart={(e) => {
                            e.dataTransfer?.setData(TABLE_DRAG_MIME, t.name);
                            setTableDragImage(e, objectLabel(t.name));
                            draggingTable = t.name;
                        }}
                        ondragend={() => (draggingTable = null)}
                        onclick={() => addTable(t.name)}
                    >
                        <Icon name="table" size={13} />{objectLabel(t.name)}
                    </button>{/each}
            </div>{/if}
        <Button
            icon="play"
            variant="primary"
            label="Run visual query"
            disabled={session.connectionState !== 'connected' || document.status === 'running' || !!document.visualQueryError || !document.visualQuerySql}
            onclick={() => workspace.run(session, document, 'all')}
        >
            Run
        </Button>
        <Button icon="code" label="Open generated SQL in a new query" disabled={!document.visualQuerySql} onclick={() => workspace.openVisualQueryAsEditor(session, document)}>
            Open in editor
        </Button>
        <Button icon="copy" label="Copy generated SQL" disabled={!document.visualQuerySql} onclick={() => copy(document.visualQuerySql, (s) => workspace.notify(s))}>
            Copy SQL
        </Button>
    </header>
    <div class="query-body">
        <QueryCanvas
            documentId={document.id}
            hasTables={!!tablesOnCanvas.length}
            {nodes}
            {edges}
            {nodeTypes}
            {defaultEdgeOptions}
            {fitViewOptions}
            {snapGrid}
            onNodeDragStop={(alias, position) => workspace.updateVisualQueryPosition(document, alias, position)}
            onEdgeClick={handleEdgeClick}
            onDropTable={addTableAt}
            isValidConnection={isValidJoinConnection}
            onConnect={handleConnect}
        />
        <ResizeHandle value={session.inspectorWidth} min={260} max={480} reverse label="Query panel width" onchange={(v) => (session.inspectorWidth = v)} />
        {#if joinEditor}<div class="query-panel-wrapper" style:width={`${session.inspectorWidth}px`}>
                {#key joinEditor}<JoinEditor
                        tables={joinEditor.tables}
                        initial={joinEditor.initial}
                        defaultLeftAlias={joinEditor.defaultLeftAlias}
                        onsave={saveJoin}
                        onremove={joinEditor.index !== undefined ? () => removeJoinAt(joinEditor!.index!) : undefined}
                        onclose={() => (joinEditor = null)}
                    />{/key}
            </div>
        {:else}<aside class="query-panel" style:width={`${session.inspectorWidth}px`}>
                <section
                    class="query-panel-section"
                    class:drag-active={columnDropTarget === 'columns'}
                    ondragover={handleColumnDragOver}
                    ondragenter={(e) => handleColumnDragEnter(e, 'columns')}
                    ondragleave={(e) => handleColumnDragLeave(e, 'columns')}
                    ondrop={handleColumnsDrop}
                    role="group"
                >
                    <h3>
                        Columns <span class="count">{ast.columns.length}</span>
                    </h3>
                    {#if !ast.columns.length}<p class="query-panel-hint">Check columns on a table node, or drag column rows here, to add them to the output.</p>{/if}
                    {#each ast.columns as column, index}<div class="query-column-row">
                            <span class="query-column-ref">{column.table}.{column.column}</span>
                            <Select
                                value={column.aggregate || ''}
                                options={aggregateOptions}
                                placeholder={`Aggregate for ${column.table}.${column.column}`}
                                compact
                                disabled={!supportsGroupBy}
                                onchange={(value) =>
                                    updateColumn(index, {
                                        aggregate: value as VisualQueryAggregate,
                                    })}
                            />
                            <input
                                type="text"
                                placeholder="alias"
                                value={column.alias ?? ''}
                                oninput={(e) =>
                                    updateColumn(index, {
                                        alias: (e.currentTarget as HTMLInputElement).value,
                                    })}
                            />
                            <Button icon="close" label="Remove column" onclick={() => removeColumn(index)} />
                        </div>{/each}
                </section>

                <section
                    class="query-panel-section"
                    class:drag-active={columnDropTarget === 'filters'}
                    ondragover={handleColumnDragOver}
                    ondragenter={(e) => handleColumnDragEnter(e, 'filters')}
                    ondragleave={(e) => handleColumnDragLeave(e, 'filters')}
                    ondrop={handleFilterDrop}
                    role="group"
                >
                    <div class="query-panel-heading">
                        <h3>
                            Filters <span class="count">{ast.filters.length}</span>
                        </h3>
                        <Button icon="plus" label="Add filter" disabled={!tablesOnCanvas.length} onclick={addFilter} />
                    </div>
                    {#each ast.filters as filter, index}<div class="query-condition-row">
                            <Select
                                value={`${filter.table}.${filter.column}`}
                                options={columnOptions}
                                placeholder="Filter column"
                                compact
                                onchange={(value) => {
                                    const [table, ...parts] = value.split('.');
                                    updateFilter(index, { table, column: parts.join('.') });
                                }}
                            />
                            <Select
                                value={filter.operator}
                                options={operatorOptions}
                                placeholder="Filter operator"
                                compact
                                onchange={(value) => updateFilter(index, { operator: value as VisualQueryOperator })}
                            />
                            {#if filter.operator === 'in'}<input
                                    type="text"
                                    placeholder="comma-separated values"
                                    value={(filter.values ?? []).join(', ')}
                                    oninput={(e) =>
                                        updateFilter(index, {
                                            values: (e.currentTarget as HTMLInputElement).value
                                                .split(',')
                                                .map((v) => v.trim())
                                                .filter(Boolean),
                                        })}
                                />{:else if filter.operator !== 'isNull' && filter.operator !== 'isNotNull'}<input
                                    type="text"
                                    placeholder="value"
                                    value={filter.value ?? ''}
                                    oninput={(e) =>
                                        updateFilter(index, {
                                            value: (e.currentTarget as HTMLInputElement).value,
                                        })}
                                />{/if}
                            <Button icon="close" label="Remove filter" onclick={() => removeFilter(index)} />
                        </div>{/each}
                </section>

                {#if supportsGroupBy}<section class="query-panel-section">
                        <h3>
                            Group by <span class="count">{ast.groupBy.length}</span>
                        </h3>
                        <p class="query-panel-hint">
                            {#if hasAggregate}Derived automatically from the non-aggregated output columns.
                            {:else}Add an aggregate (Count, Sum, …) to an output column to enable grouping.{/if}
                        </p>
                        {#if ast.groupBy.length}<ul class="query-panel-list">
                                {#each ast.groupBy as g}<li>{g.table}.{g.column}</li>{/each}
                            </ul>{/if}
                    </section>

                    <section class="query-panel-section">
                        <div class="query-panel-heading">
                            <h3>
                                Having <span class="count">{ast.having.length}</span>
                            </h3>
                            <Button icon="plus" label="Add having condition" disabled={!hasAggregate} onclick={addHaving} />
                        </div>
                        {#if !hasAggregate}<p class="query-panel-hint">Requires at least one aggregated output column.</p>{/if}
                        {#each ast.having as condition, index}<div class="query-condition-row">
                                <Select
                                    value={`${condition.table}.${condition.column}`}
                                    options={columnOptions}
                                    placeholder="Having column"
                                    compact
                                    onchange={(value) => {
                                        const [table, ...parts] = value.split('.');
                                        updateHaving(index, { table, column: parts.join('.') });
                                    }}
                                />
                                <Select
                                    value={condition.aggregate || ''}
                                    options={aggregateOptions}
                                    placeholder="Having aggregate"
                                    compact
                                    onchange={(value) =>
                                        updateHaving(index, {
                                            aggregate: value as VisualQueryAggregate,
                                        })}
                                />
                                <Select
                                    value={condition.operator}
                                    options={operatorOptions}
                                    placeholder="Having operator"
                                    compact
                                    onchange={(value) =>
                                        updateHaving(index, {
                                            operator: value as VisualQueryOperator,
                                        })}
                                />
                                {#if condition.operator === 'in'}
                                    <input
                                        aria-label="Having values"
                                        placeholder="comma-separated values"
                                        value={(condition.values ?? []).join(', ')}
                                        oninput={(e) =>
                                            updateHaving(index, {
                                                values: e.currentTarget.value
                                                    .split(',')
                                                    .map((v) => v.trim())
                                                    .filter(Boolean),
                                            })}
                                    />
                                {:else if condition.operator !== 'isNull' && condition.operator !== 'isNotNull'}
                                    <input type="text" placeholder="value" value={condition.value ?? ''} oninput={(e) => updateHaving(index, { value: e.currentTarget.value })} />
                                {/if}
                                <Button icon="close" label="Remove having condition" onclick={() => removeHaving(index)} />
                            </div>{/each}
                    </section>{/if}

                <section
                    class="query-panel-section"
                    class:drag-active={columnDropTarget === 'orderBy'}
                    ondragover={handleColumnDragOver}
                    ondragenter={(e) => handleColumnDragEnter(e, 'orderBy')}
                    ondragleave={(e) => handleColumnDragLeave(e, 'orderBy')}
                    ondrop={handleOrderDrop}
                    role="group"
                >
                    <div class="query-panel-heading">
                        <h3>
                            Order by <span class="count">{ast.orderBy.length}</span>
                        </h3>
                        <Button icon="plus" label="Add order by" disabled={!tablesOnCanvas.length} onclick={addOrder} />
                    </div>
                    {#each ast.orderBy as order, index}<div class="query-condition-row">
                            <Select
                                value={`${order.table}.${order.column}`}
                                options={columnOptions}
                                placeholder="Order by column"
                                compact
                                onchange={(value) => {
                                    const [table, ...parts] = value.split('.');
                                    updateOrder(index, { table, column: parts.join('.') });
                                }}
                            />
                            <Select
                                value={order.descending ? 'desc' : 'asc'}
                                options={orderDirectionOptions}
                                placeholder="Order direction"
                                compact
                                onchange={(value) => updateOrder(index, { descending: value === 'desc' })}
                            />
                            <Button icon="close" label="Remove order by" onclick={() => removeOrder(index)} />
                        </div>{/each}
                </section>

                <section class="query-panel-section">
                    <h3>Limit</h3>
                    <input
                        type="number"
                        min="0"
                        class="query-limit"
                        value={ast.limit || ''}
                        placeholder="No limit"
                        oninput={(e) =>
                            (document.visualQueryAst = {
                                ...ast,
                                limit: Number((e.currentTarget as HTMLInputElement).value) || 0,
                            })}
                    />
                </section>
            </aside>{/if}
    </div>
    <div class="query-sql-preview">
        <div class="query-sql-heading">
            <span>Generated SQL</span>
            {#if document.visualQueryError && tablesOnCanvas.length}<span class="query-sql-error">
                    {document.visualQueryError}
                </span>{/if}
        </div>
        <!-- svelte-ignore a11y_no_noninteractive_tabindex (The read-only SQL preview is keyboard-focusable so it can be selected and copied without a mouse.) -->
        <pre tabindex="0" aria-label="Generated SQL">{document.visualQuerySql || '-- Add tables and columns to generate SQL --'}</pre>
    </div>
</section>
