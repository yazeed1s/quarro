<script lang="ts">
    import { Background, BackgroundVariant, Controls, ConnectionLineType, MarkerType, MiniMap, PanOnScrollMode, SvelteFlow, type Edge, type Node } from '@xyflow/svelte';
    import type { Document, SchemaForeignKey, SchemaTable, Session } from '../../state/types';
    import { objectLabel } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import Input from '../../components/Input.svelte';
    import Modal from '../../components/Modal.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';
    import { renderSchemaDiagramSVG, svgToPNG, type DiagramExportOptions, type DiagramExportPalette } from './export';
    import SchemaTableNode, { type TableNodeData } from './SchemaTableNode.svelte';

    let { session, document }: { session: Session; document: Document } = $props();
    const nodeTypes = { table: SchemaTableNode };
    type DiagramBackground = 'cross' | 'dots' | 'lines' | 'none';
    type DiagramScrollMode = 'pan' | 'zoom' | 'locked';
    type DiagramRenderMode = 'auto' | 'visible' | 'full';
    type DiagramEdgeStyle = 'smoothstep' | 'step' | 'straight' | 'bezier';
    type DiagramZoomRange = 'near' | 'balanced' | 'wide';
    type DiagramNodeDetail = 'full' | 'names' | 'keys';
    const backgroundOptions: { value: DiagramBackground; label: string }[] = [
        { value: 'cross', label: 'Cross' },
        { value: 'dots', label: 'Dots' },
        { value: 'lines', label: 'Lines' },
        { value: 'none', label: 'None' },
    ];
    const scrollOptions: { value: DiagramScrollMode; label: string }[] = [
        { value: 'pan', label: 'Pan' },
        { value: 'zoom', label: 'Zoom' },
        { value: 'locked', label: 'Lock' },
    ];
    const renderOptions: { value: DiagramRenderMode; label: string }[] = [
        { value: 'auto', label: 'Auto' },
        { value: 'visible', label: 'Cull' },
        { value: 'full', label: 'Full' },
    ];
    const edgeOptions: { value: DiagramEdgeStyle; label: string }[] = [
        { value: 'smoothstep', label: 'Smooth' },
        { value: 'step', label: 'Step' },
        { value: 'straight', label: 'Line' },
        { value: 'bezier', label: 'Curve' },
    ];
    const zoomOptions: { value: DiagramZoomRange; label: string }[] = [
        { value: 'near', label: 'Near' },
        { value: 'balanced', label: 'Normal' },
        { value: 'wide', label: 'Wide' },
    ];
    const detailOptions: { value: DiagramNodeDetail; label: string }[] = [
        { value: 'full', label: 'Full' },
        { value: 'names', label: 'Names' },
        { value: 'keys', label: 'Keys' },
    ];
    const snapGrid: [number, number] = [16, 16];

    let configOpen = $state(false);
    let advancedOpen = $state(false);
    let showMinimap = $state(true);
    let showControls = $state(true);
    let edgeLabels = $state(true);
    let edgeMarkers = $state(true);
    let lockNodes = $state(false);
    let minimapInteractive = $state(false);
    let background = $state<DiagramBackground>('cross');
    let scrollMode = $state<DiagramScrollMode>('pan');
    let renderMode = $state<DiagramRenderMode>('auto');
    let edgeStyle = $state<DiagramEdgeStyle>('smoothstep');
    let zoomRange = $state<DiagramZoomRange>('balanced');
    let nodeDetail = $state<DiagramNodeDetail>('full');
    let nodeWidth = $state(278);
    let columnHeight = $state(28);
    let nodeColumnHeight = $state(260);
    let gridGap = $state(22);
    let gridMarkSize = $state(2);
    let edgeWidth = $state(1.5);
    let relatedEdgeWidth = $state(2.2);
    let labelSize = $state(11);
    let minimapWidth = $state(170);
    let minimapHeight = $state(110);
    let fitPadding = $state(18);

    // Keyboard navigation operates on this canvas's own focus state rather
    // than SvelteFlow's native node/edge focusability, so one roving-focus
    // model drives both the visible outline and Enter/Arrow/Tab behavior
    // without a second, competing DOM focus system.
    let focusedTable = $state('');
    let focusedEdgeId = $state('');
    let inspectingFk = $state<{ sourceTable: string; fk: SchemaForeignKey } | null>(null);
    let exportingPNG = $state(false);

    function openFkInspector(sourceTable: string, fk: SchemaForeignKey) {
        inspectingFk = { sourceTable, fk };
    }

    let schemas = $derived([...new Set(session.schema.map((table) => table.schema).filter(Boolean))]);
    let schemaOptions: SelectOption[] = $derived([
        { value: '', label: 'All schemas' },
        ...schemas.filter((schema): schema is string => !!schema).map((schema) => ({ value: schema, label: schema })),
    ]);
    let relatedSet = $derived.by(() => {
        if (!document.diagramRelated) return new Set<string>();
        const related = new Set<string>([document.diagramRelated]);
        for (const table of session.schema) {
            for (const fk of table.foreignKeys) {
                if (table.name === document.diagramRelated) related.add(fk.refTable);
                if (fk.refTable === document.diagramRelated) related.add(table.name);
            }
        }
        return related;
    });
    let visibleTables = $derived.by(() => {
        const search = document.diagramSearch.trim().toLowerCase();
        return session.schema.filter((table) => {
            if (table.kind !== 'table') return false;
            if (document.diagramSchema && table.schema !== document.diagramSchema) return false;
            if (document.diagramRelated && !relatedSet.has(table.name)) return false;
            if (!search) return true;
            return objectLabel(table.name).toLowerCase().includes(search) || table.columns.some((column) => column.name.toLowerCase().includes(search));
        });
    });
    let visibleNames = $derived(new Set(visibleTables.map((table) => table.name)));
    let selfReferenceCount = $derived(visibleTables.reduce((count, table) => count + table.foreignKeys.filter((fk) => fk.refTable === table.name).length, 0));
    let hasSearch = $derived(!!document.diagramSearch.trim());
    let isFiltered = $derived(hasSearch || !!document.diagramSchema || !!document.diagramRelated);
    let flowKey = $derived(`${document.diagramSchema}|${document.diagramRelated}|${document.diagramSearch.trim()}`);

    let automaticPositions = $derived.by(() => {
        const positions: Record<string, { x: number; y: number }> = {};
        let y = 70;
        for (let offset = 0; offset < visibleTables.length; offset += 4) {
            const group = visibleTables.slice(offset, offset + 4);
            let height = 0;
            group.forEach((table, col) => {
                positions[table.name] = { x: 80 + col * 330, y };
                const collapsed = document.diagramCollapsed.includes(table.name);
                height = Math.max(
                    height,
                    collapsed ? 110 : 110 + Math.min(260, Math.min(table.columns.length, 42) * 32 + table.foreignKeys.filter((fk) => fk.refTable === table.name).length * 30),
                );
            });
            y += height + 64;
        }
        return positions;
    });

    let nodes = $derived<Node[]>(
        visibleTables.map((table, index) => {
            const collapsed = document.diagramCollapsed.includes(table.name);
            return {
                id: table.name,
                type: 'table',
                position: document.diagramPositions[table.name] ?? automaticPositions[table.name],
                data: {
                    table,
                    collapsed,
                    detail: nodeDetail,
                    columnLimit: Math.max(8, Math.round(nodeColumnHeight / columnHeight) * 2),
                    related: !!document.diagramRelated && table.name === document.diagramRelated,
                    dimmed: !!document.diagramRelated && !relatedSet.has(table.name),
                    selfReferences: table.foreignKeys.filter((fk) => fk.refTable === table.name),
                    kbdFocused: table.name === focusedTable,
                    onOpen: (name: string) => workspace.openDocument(session, 'table', name),
                    onToggle: (name: string) => workspace.toggleDiagramCollapse(document, name),
                    onFocusRelated: (name: string) => (document.diagramRelated = document.diagramRelated === name ? '' : name),
                } as unknown as Record<string, unknown>,
            };
        }),
    );
    let edges = $derived<Edge[]>(
        session.schema.flatMap((table) =>
            table.foreignKeys
                .filter((fk) => fk.refTable !== table.name && visibleNames.has(table.name) && visibleNames.has(fk.refTable))
                .map((fk, index) => {
                    const id = `${table.name}:${fk.column}:${fk.refTable}:${fk.refColumn}:${index}`;
                    const classes = ['diagram-edge'];
                    if (document.diagramRelated && (table.name === document.diagramRelated || fk.refTable === document.diagramRelated)) classes.push('related');
                    if (id === focusedEdgeId) classes.push('kbd-focused');
                    return {
                        id,
                        source: table.name,
                        target: fk.refTable,
                        label: edgeLabels ? `${fk.column} → ${fk.refColumn}` : undefined,
                        class: classes.join(' '),
                        focusable: false,
                        data: { sourceTable: table.name, fk } as unknown as Record<string, unknown>,
                    };
                }),
        ),
    );
    // The focused node's own outgoing relationships, in the order Tab cycles
    // through them  the same edges this node's diagram connections already
    // draw, not a second relationship lookup.
    let edgesForFocusedTable = $derived(focusedTable ? edges.filter((edge) => edge.source === focusedTable) : []);
    let relationshipCount = $derived(edges.length + selfReferenceCount);
    let backgroundVariant = $derived(background === 'dots' ? BackgroundVariant.Dots : background === 'lines' ? BackgroundVariant.Lines : BackgroundVariant.Cross);
    let onlyRenderVisibleElements = $derived(renderMode === 'visible' || (renderMode === 'auto' && visibleTables.length > 18));
    let connectionLineType = $derived(
        edgeStyle === 'straight'
            ? ConnectionLineType.Straight
            : edgeStyle === 'step'
              ? ConnectionLineType.Step
              : edgeStyle === 'bezier'
                ? ConnectionLineType.Bezier
                : ConnectionLineType.SmoothStep,
    );
    let defaultEdgeOptions = $derived({
        type: edgeStyle,
        markerEnd: edgeMarkers ? { type: MarkerType.ArrowClosed, color: 'var(--diagram-edge)' } : undefined,
        focusable: false,
        interactionWidth: 18,
    });
    let minZoom = $derived(zoomRange === 'wide' ? 0.22 : zoomRange === 'near' ? 0.65 : 0.45);
    let maxZoom = $derived(zoomRange === 'wide' ? 1.6 : zoomRange === 'near' ? 1 : 1.2);
    let fitViewOptions = $derived({
        padding: fitPadding / 100,
        minZoom: zoomRange === 'wide' ? 0.35 : zoomRange === 'near' ? 0.78 : 0.65,
        maxZoom: zoomRange === 'near' ? 1 : 1.1,
        duration: 0,
    });
    let diagramStyle = $derived(
        [
            `--diagram-node-width: ${nodeWidth}px`,
            `--diagram-column-height: ${columnHeight}px`,
            `--diagram-node-column-max: ${nodeColumnHeight}px`,
            `--diagram-edge-width: ${edgeWidth}`,
            `--diagram-related-edge-width: ${relatedEdgeWidth}`,
            `--diagram-label-size: ${labelSize}px`,
        ].join('; '),
    );
    let interactionSummary = $derived(
        `${scrollMode === 'pan' ? 'Scroll pans' : scrollMode === 'zoom' ? 'Scroll zooms' : 'Navigation locked'} · ${lockNodes ? 'Nodes locked' : 'Nodes movable'}`,
    );

    function focusCanvas() {
        if (!focusedTable && nodes.length) focusedTable = nodes[0].id;
    }
    // Nearest-node-in-direction search over each node's own current rendered
    // position (manual drag position if set, else the automatic layout)
    // the same position map the canvas already renders from, not a second
    // graph structure.
    function moveFocus(direction: 'up' | 'down' | 'left' | 'right') {
        if (!nodes.length) return;
        if (!focusedTable) {
            focusedTable = nodes[0].id;
            return;
        }
        const current = nodes.find((node) => node.id === focusedTable);
        if (!current) {
            focusedTable = nodes[0].id;
            return;
        }
        let best: { id: string; score: number } | null = null;
        for (const node of nodes) {
            if (node.id === focusedTable) continue;
            const dx = node.position.x - current.position.x;
            const dy = node.position.y - current.position.y;
            let primary: number;
            let aligned: boolean;
            if (direction === 'right') {
                primary = dx;
                aligned = dx > 0;
            } else if (direction === 'left') {
                primary = -dx;
                aligned = dx < 0;
            } else if (direction === 'down') {
                primary = dy;
                aligned = dy > 0;
            } else {
                primary = -dy;
                aligned = dy < 0;
            }
            if (!aligned) continue;
            const cross = direction === 'left' || direction === 'right' ? Math.abs(dy) : Math.abs(dx);
            const score = primary + cross * 2;
            if (!best || score < best.score) best = { id: node.id, score };
        }
        if (best) focusedTable = best.id;
    }
    function handleCanvasKeydown(event: KeyboardEvent) {
        if (event.key === 'ArrowUp' || event.key === 'ArrowDown' || event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
            if (focusedEdgeId) return; // arrow keys move between nodes only, not while an edge is focused
            event.preventDefault();
            moveFocus(event.key === 'ArrowUp' ? 'up' : event.key === 'ArrowDown' ? 'down' : event.key === 'ArrowLeft' ? 'left' : 'right');
            return;
        }
        if (event.key === 'Tab' && focusedTable && edgesForFocusedTable.length) {
            event.preventDefault();
            const currentIndex = edgesForFocusedTable.findIndex((edge) => edge.id === focusedEdgeId);
            const nextIndex = event.shiftKey
                ? currentIndex <= 0
                    ? edgesForFocusedTable.length - 1
                    : currentIndex - 1
                : currentIndex === edgesForFocusedTable.length - 1
                  ? -1
                  : currentIndex + 1;
            focusedEdgeId = nextIndex === -1 ? '' : edgesForFocusedTable[nextIndex].id;
            return;
        }
        if (event.key === 'Enter') {
            event.preventDefault();
            if (focusedEdgeId) {
                const edge = edges.find((candidate) => candidate.id === focusedEdgeId);
                const data = edge?.data as { sourceTable: string; fk: SchemaForeignKey } | undefined;
                if (data) openFkInspector(data.sourceTable, data.fk);
            } else if (focusedTable) {
                workspace.openDocument(session, 'table', focusedTable);
            }
            return;
        }
        if (event.key === 'Escape') {
            event.preventDefault();
            if (focusedEdgeId) {
                focusedEdgeId = '';
            } else {
                focusedTable = '';
                (event.currentTarget as HTMLElement).blur();
            }
        }
    }

    function exportPalette(): DiagramExportPalette {
        const styles = getComputedStyle(window.document.documentElement);
        const color = (name: string, fallback: string) => styles.getPropertyValue(name).trim() || fallback;
        return {
            surface: color('--surface', '#ffffff'),
            subtle: color('--subtle', '#f0f1f1'),
            card: color('--card', '#e6e8e8'),
            border: color('--border', '#d7dada'),
            borderSoft: color('--border-soft', '#e5e7e7'),
            text: color('--text', '#171818'),
            muted: color('--muted', '#6f7373'),
            accent: color('--accent', '#2f3434'),
            accentText: color('--accent-text', '#171818'),
        };
    }

    function renderExport() {
        const positions = Object.fromEntries(nodes.map((node) => [node.id, node.position]));
        const options: DiagramExportOptions = {
            tables: visibleTables,
            positions,
            collapsed: document.diagramCollapsed,
            relatedTable: document.diagramRelated,
            nodeDetail,
            nodeWidth,
            columnHeight,
            nodeColumnHeight,
            edgeLabels,
            edgeMarkers,
            edgeStyle,
            edgeWidth,
            relatedEdgeWidth,
            labelSize,
            background,
            gridGap,
            gridMarkSize,
            palette: exportPalette(),
        };
        return renderSchemaDiagramSVG(options);
    }

    function exportName(extension: 'svg' | 'png') {
        const base = document.title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'schema-diagram';
        return `${base}.${extension}`;
    }

    async function exportSVG() {
        if (!visibleTables.length) return;
        const output = renderExport();
        const blob = new Blob([output.svg], { type: 'image/svg+xml;charset=utf-8' });
        if (await workspace.saveBlobWithPicker(blob, exportName('svg'), 'SVG image', 'svg')) {
            workspace.notify('Exported schema diagram as SVG.');
        }
    }

    async function exportPNG() {
        if (!visibleTables.length || exportingPNG) return;
        exportingPNG = true;
        try {
            const output = renderExport();
            const blob = await svgToPNG(output.svg, output.width, output.height);
            if (await workspace.saveBlobWithPicker(blob, exportName('png'), 'PNG image', 'png')) {
                workspace.notify('Exported schema diagram as PNG.');
            }
        } catch (error) {
            workspace.notify(error instanceof Error ? error.message : 'Could not export the schema diagram as PNG.');
        } finally {
            exportingPNG = false;
        }
    }
</script>

<section class="schema-diagram" class:config-open={advancedOpen} style={diagramStyle}>
    <header class="diagram-toolbar">
        <div class="diagram-title">
            <Icon name="diagram" size={18} />
            <div>
                <strong>Schema diagram</strong>
                <span>
                    {visibleTables.length} tables · {relationshipCount} relationships{isFiltered ? ' · filtered' : ''}
                </span>
            </div>
        </div>
        <label class="search-field diagram-search">
            <Icon name="search" size={14} />
            <input aria-label="Search diagram" placeholder="Search tables or columns..." bind:value={document.diagramSearch} />
            {#if document.diagramSearch}<button aria-label="Clear diagram search" onclick={() => (document.diagramSearch = '')}>
                    <Icon name="close" size={12} />
                </button>{/if}
        </label>
        {#if schemas.length}<Select bind:value={document.diagramSchema} options={schemaOptions} placeholder="Filter by schema" compact />{/if}
        {#if document.diagramRelated}<Button variant="outline" icon="close" onclick={() => (document.diagramRelated = '')}>
                Related: {objectLabel(document.diagramRelated)}
            </Button>{/if}
        <div class="diagram-config-wrap">
            <Button icon="settings" label="Diagram view options" active={configOpen} onclick={() => (configOpen = !configOpen)}>View</Button>
            {#if configOpen}<div class="diagram-config-panel" role="dialog" aria-label="Diagram view options">
                    <div class="diagram-config-head">
                        <div>
                            <strong>View options</strong>
                            <span>{interactionSummary}</span>
                        </div>
                        <button aria-label="Close diagram view options" onclick={() => (configOpen = false)}>
                            <Icon name="close" size={13} />
                        </button>
                    </div>
                    <div class="diagram-config-row">
                        <span>Grid</span>
                        <div class="diagram-segment" role="group" aria-label="Diagram grid style">
                            {#each backgroundOptions as option}<button
                                    class:active={background === option.value}
                                    aria-pressed={background === option.value}
                                    onclick={() => (background = option.value)}
                                >
                                    {option.label}
                                </button>{/each}
                        </div>
                    </div>
                    <div class="diagram-config-row">
                        <span>Scroll</span>
                        <div class="diagram-segment" role="group" aria-label="Diagram scroll behavior">
                            {#each scrollOptions as option}<button
                                    class:active={scrollMode === option.value}
                                    aria-pressed={scrollMode === option.value}
                                    onclick={() => (scrollMode = option.value)}
                                >
                                    {option.label}
                                </button>{/each}
                        </div>
                    </div>
                    <div class="diagram-config-row">
                        <span>Render</span>
                        <div class="diagram-segment" role="group" aria-label="Diagram rendering mode">
                            {#each renderOptions as option}<button
                                    class:active={renderMode === option.value}
                                    aria-pressed={renderMode === option.value}
                                    onclick={() => (renderMode = option.value)}
                                >
                                    {option.label}
                                </button>{/each}
                        </div>
                    </div>
                    <div class="diagram-toggle-grid">
                        <Input type="checkbox" bind:checked={showMinimap} label="Mini map" />
                        <Input type="checkbox" bind:checked={edgeLabels} label="Edge labels" />
                        <Input type="checkbox" bind:checked={lockNodes} label="Lock nodes" />
                    </div>
                    <Button
                        variant="outline"
                        icon="panel"
                        active={advancedOpen}
                        onclick={() => {
                            advancedOpen = true;
                            configOpen = false;
                        }}
                    >
                        More
                    </Button>
                </div>{/if}
        </div>
        <Button icon="download" label="Export schema diagram as SVG" disabled={!visibleTables.length} onclick={exportSVG}>SVG</Button>
        <Button icon="download" label="Export schema diagram as PNG" disabled={!visibleTables.length || exportingPNG} onclick={exportPNG}>
            {exportingPNG ? 'Exporting…' : 'PNG'}
        </Button>
        <Button icon="refresh" label="Refresh schema" disabled={session.connectionState !== 'connected'} onclick={() => workspace.loadFullSchema(session, true)} />
    </header>
    <div class="diagram-workspace">
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Keyboard navigation moves focus between tables and relationships without a mouse.) -->
        <div
            class="diagram-canvas"
            tabindex="0"
            role="application"
            aria-label="Schema diagram. Arrow keys move between tables, Tab cycles a table's relationships, Enter opens the focused table or relationship, Escape clears focus."
            onkeydown={handleCanvasKeydown}
            onfocus={focusCanvas}
        >
            {#if session.schemaLoading || session.schemaFullLoading}<div class="diagram-empty">
                    <Icon name="refresh" size={30} />
                    <h3>Loading schema</h3>
                </div>{:else if !visibleTables.length}<div class="diagram-empty">
                    <Icon name="diagram" size={30} />
                    <h3>No tables to draw</h3>
                    <p>Adjust the search or schema filter.</p>
                </div>{:else}<SvelteFlow
                    id={`schema-diagram-${document.id}-${flowKey}`}
                    {nodes}
                    {edges}
                    {nodeTypes}
                    {defaultEdgeOptions}
                    {fitViewOptions}
                    {snapGrid}
                    fitView
                    {minZoom}
                    {maxZoom}
                    nodeDragThreshold={4}
                    nodeClickDistance={3}
                    paneClickDistance={3}
                    selectionKey={null}
                    deleteKey={null}
                    nodesConnectable={false}
                    nodesDraggable={!lockNodes}
                    clickConnect={false}
                    autoPanOnConnect={false}
                    autoPanOnNodeDrag={false}
                    autoPanOnSelection={false}
                    autoPanOnNodeFocus={false}
                    selectNodesOnDrag={false}
                    selectionOnDrag={false}
                    nodesFocusable={false}
                    edgesFocusable={false}
                    elevateEdgesOnSelect={false}
                    defaultMarkerColor={null}
                    {onlyRenderVisibleElements}
                    panOnDrag={scrollMode !== 'locked'}
                    panOnScroll={scrollMode === 'pan'}
                    panOnScrollSpeed={0.7}
                    panOnScrollMode={PanOnScrollMode.Free}
                    zoomOnDoubleClick={false}
                    zoomOnScroll={scrollMode === 'zoom'}
                    zoomOnPinch={scrollMode !== 'locked'}
                    {connectionLineType}
                    noDragClass="nodrag"
                    noPanClass="nopan"
                    onnodeclick={({ node }) => {
                        focusedTable = node.id;
                        focusedEdgeId = '';
                        document.diagramRelated = document.diagramRelated === node.id ? '' : node.id;
                    }}
                    onnodedragstop={({ targetNode }) => {
                        if (targetNode) workspace.updateDiagramPosition(document, targetNode.id, targetNode.position);
                    }}
                    onedgeclick={({ edge }) => {
                        const data = edge.data as { sourceTable: string; fk: SchemaForeignKey } | undefined;
                        if (data) {
                            focusedTable = data.sourceTable;
                            focusedEdgeId = edge.id;
                            openFkInspector(data.sourceTable, data.fk);
                        }
                    }}
                >
                    {#if showControls}<Controls position="bottom-left" showLock={false} orientation="horizontal" {fitViewOptions} />{/if}
                    {#if showMinimap}<MiniMap
                            position="bottom-right"
                            width={minimapWidth}
                            height={minimapHeight}
                            pannable={minimapInteractive}
                            zoomable={minimapInteractive}
                            nodeColor={(node) => (node.id === document.diagramRelated ? 'var(--accent)' : 'var(--diagram-minimap-node)')}
                            nodeStrokeColor={() => 'var(--diagram-edge)'}
                            nodeStrokeWidth={1}
                            nodeBorderRadius={3}
                            maskColor="var(--diagram-minimap-mask)"
                            maskStrokeColor="var(--accent)"
                            maskStrokeWidth={1}
                        />{/if}
                    {#if background !== 'none'}<Background
                            variant={backgroundVariant}
                            gap={background === 'lines' ? Math.max(24, gridGap + 6) : gridGap}
                            size={background === 'cross' ? gridMarkSize : Math.max(0.8, gridMarkSize - 0.6)}
                            lineWidth={background === 'dots' ? 0.9 : 0.5}
                            patternColor="color-mix(in srgb, var(--accent) 22%, var(--surface))"
                        />{/if}
                </SvelteFlow>{/if}
        </div>
        {#if advancedOpen}<aside class="diagram-inspector" aria-label="Diagram advanced options">
                <header>
                    <div>
                        <span class="eyebrow">DIAGRAM VIEW</span>
                        <h3>Customize</h3>
                        <small>{visibleTables.length} tables · {relationshipCount} relationships</small>
                    </div>
                    <Button icon="close" label="Close diagram options" onclick={() => (advancedOpen = false)} />
                </header>
                <div class="diagram-inspector-content">
                    <section>
                        <h4>Behavior</h4>
                        <div class="diagram-config-row">
                            <span>Scroll</span>
                            <div class="diagram-segment" role="group" aria-label="Advanced scroll behavior">
                                {#each scrollOptions as option}<button
                                        class:active={scrollMode === option.value}
                                        aria-pressed={scrollMode === option.value}
                                        onclick={() => (scrollMode = option.value)}
                                    >
                                        {option.label}
                                    </button>{/each}
                            </div>
                        </div>
                        <div class="diagram-config-row">
                            <span>Render</span>
                            <div class="diagram-segment" role="group" aria-label="Advanced render mode">
                                {#each renderOptions as option}<button
                                        class:active={renderMode === option.value}
                                        aria-pressed={renderMode === option.value}
                                        onclick={() => (renderMode = option.value)}
                                    >
                                        {option.label}
                                    </button>{/each}
                            </div>
                        </div>
                        <div class="diagram-config-row">
                            <span>Zoom</span>
                            <div class="diagram-segment" role="group" aria-label="Advanced zoom range">
                                {#each zoomOptions as option}<button
                                        class:active={zoomRange === option.value}
                                        aria-pressed={zoomRange === option.value}
                                        onclick={() => (zoomRange = option.value)}
                                    >
                                        {option.label}
                                    </button>{/each}
                            </div>
                        </div>
                        <Input type="range" min="8" max="32" step="1" bind:value={fitPadding} label="Fit padding" suffix="%" />
                        <div class="diagram-toggle-grid">
                            <Input type="checkbox" bind:checked={lockNodes} label="Lock nodes" />
                            <Input type="checkbox" bind:checked={showControls} label="Controls" />
                            <Input type="checkbox" bind:checked={showMinimap} label="Mini map" />
                            <Input type="checkbox" bind:checked={minimapInteractive} disabled={!showMinimap} label="Map nav" />
                        </div>
                    </section>
                    <section>
                        <h4>Nodes</h4>
                        <div class="diagram-config-row">
                            <span>Detail</span>
                            <div class="diagram-segment" role="group" aria-label="Advanced node detail">
                                {#each detailOptions as option}<button
                                        class:active={nodeDetail === option.value}
                                        aria-pressed={nodeDetail === option.value}
                                        onclick={() => (nodeDetail = option.value)}
                                    >
                                        {option.label}
                                    </button>{/each}
                            </div>
                        </div>
                        <Input type="range" min="220" max="380" step="2" bind:value={nodeWidth} label="Node width" suffix="px" />
                        <Input type="range" min="24" max="38" step="1" bind:value={columnHeight} label="Row height" suffix="px" />
                        <Input type="range" min="150" max="420" step="10" bind:value={nodeColumnHeight} label="Column window" suffix="px" />
                    </section>
                    <section>
                        <h4>Edges</h4>
                        <div class="diagram-config-row">
                            <span>Shape</span>
                            <div class="diagram-segment" role="group" aria-label="Advanced edge style">
                                {#each edgeOptions as option}<button
                                        class:active={edgeStyle === option.value}
                                        aria-pressed={edgeStyle === option.value}
                                        onclick={() => (edgeStyle = option.value)}
                                    >
                                        {option.label}
                                    </button>{/each}
                            </div>
                        </div>
                        <div class="diagram-toggle-grid">
                            <Input type="checkbox" bind:checked={edgeLabels} label="Labels" />
                            <Input type="checkbox" bind:checked={edgeMarkers} label="Arrows" />
                        </div>
                        <Input type="range" min="0.8" max="3" step="0.1" bind:value={edgeWidth} label="Line width" suffix="px" />
                        <Input type="range" min="1.2" max="4" step="0.1" bind:value={relatedEdgeWidth} label="Related width" suffix="px" />
                        <Input type="range" min="9" max="14" step="1" bind:value={labelSize} label="Label text" suffix="px" />
                    </section>
                    <section>
                        <h4>Canvas</h4>
                        <div class="diagram-config-row">
                            <span>Grid</span>
                            <div class="diagram-segment" role="group" aria-label="Advanced grid style">
                                {#each backgroundOptions as option}<button
                                        class:active={background === option.value}
                                        aria-pressed={background === option.value}
                                        onclick={() => (background = option.value)}
                                    >
                                        {option.label}
                                    </button>{/each}
                            </div>
                        </div>
                        <Input type="range" min="12" max="44" step="2" bind:value={gridGap} label="Grid gap" suffix="px" />
                        <Input type="range" min="1" max="5" step="0.2" bind:value={gridMarkSize} label="Grid mark" suffix="px" />
                        <Input type="range" min="120" max="260" step="10" bind:value={minimapWidth} label="Mini map width" suffix="px" />
                        <Input type="range" min="80" max="180" step="10" bind:value={minimapHeight} label="Mini map height" suffix="px" />
                    </section>
                </div>
                <footer>
                    <Button
                        variant="outline"
                        onclick={() => {
                            showMinimap = true;
                            showControls = true;
                            edgeLabels = true;
                            edgeMarkers = true;
                            lockNodes = false;
                            minimapInteractive = false;
                            background = 'cross';
                            scrollMode = 'pan';
                            renderMode = 'auto';
                            edgeStyle = 'smoothstep';
                            zoomRange = 'balanced';
                            nodeDetail = 'full';
                            nodeWidth = 278;
                            columnHeight = 28;
                            nodeColumnHeight = 260;
                            gridGap = 22;
                            gridMarkSize = 2;
                            edgeWidth = 1.5;
                            relatedEdgeWidth = 2.2;
                            labelSize = 11;
                            minimapWidth = 170;
                            minimapHeight = 110;
                            fitPadding = 18;
                        }}
                    >
                        Reset view
                    </Button>
                </footer>
            </aside>{/if}
    </div>
</section>
{#if inspectingFk}<Modal title={`${inspectingFk.sourceTable}.${inspectingFk.fk.column}`} subtitle="Foreign key" onclose={() => (inspectingFk = null)}>
        <div class="function-detail">
            <div class="function-detail-row">
                <strong>References</strong>
                <code>{inspectingFk.fk.refTable}.{inspectingFk.fk.refColumn}</code>
            </div>
            {#if inspectingFk.fk.onDelete || inspectingFk.fk.onUpdate}<div class="function-detail-row">
                    <strong>Referential actions</strong>
                    <code>ON DELETE {inspectingFk.fk.onDelete ?? 'NO ACTION'} · ON UPDATE {inspectingFk.fk.onUpdate ?? 'NO ACTION'}</code>
                </div>{/if}
            <div class="fk-inspector-actions">
                <Button
                    variant="outline"
                    onclick={() => {
                        if (inspectingFk) workspace.openDocument(session, 'table', inspectingFk.sourceTable);
                        inspectingFk = null;
                    }}
                >
                    Open {objectLabel(inspectingFk.sourceTable)}
                </Button>
                <Button
                    onclick={() => {
                        if (inspectingFk) workspace.openDocument(session, 'table', inspectingFk.fk.refTable);
                        inspectingFk = null;
                    }}
                >
                    Open {objectLabel(inspectingFk.fk.refTable)}
                </Button>
            </div>
        </div>
    </Modal>{/if}
