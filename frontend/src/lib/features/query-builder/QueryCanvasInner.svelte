<script lang="ts">
    import {
        Background,
        BackgroundVariant,
        Controls,
        ConnectionLineType,
        MarkerType,
        MiniMap,
        PanOnScrollMode,
        SvelteFlow,
        useSvelteFlow,
        type Connection,
        type DefaultEdgeOptions,
        type Edge,
        type FitViewOptions,
        type IsValidConnection,
        type Node,
        type NodeTypes,
    } from '@xyflow/svelte';
    import Icon from '../../components/Icon.svelte';
    import { TABLE_DRAG_MIME } from './dnd';

    let {
        documentId,
        hasTables,
        nodes,
        edges,
        nodeTypes,
        defaultEdgeOptions,
        fitViewOptions,
        snapGrid,
        onNodeDragStop,
        onEdgeClick,
        onDropTable,
        isValidConnection,
        onConnect,
    }: {
        documentId: string;
        hasTables: boolean;
        nodes: Node[];
        edges: Edge[];
        nodeTypes: NodeTypes;
        defaultEdgeOptions: DefaultEdgeOptions;
        fitViewOptions: FitViewOptions;
        snapGrid: [number, number];
        onNodeDragStop: (alias: string, position: { x: number; y: number }) => void;
        onEdgeClick: (edge: Edge) => void;
        onDropTable: (table: string, position: { x: number; y: number }) => void;
        isValidConnection: IsValidConnection<Edge>;
        onConnect: (connection: Connection) => void;
    } = $props();

    // Requires an ancestor <SvelteFlowProvider> (QueryCanvas.svelte) — the
    // hook can't be called from the same component that only renders the
    // provider, so this component exists to be that descendant.
    const { screenToFlowPosition } = useSvelteFlow();

    let dragOver = $state(false);

    function handleDragOver(event: DragEvent) {
        if (!event.dataTransfer?.types.includes(TABLE_DRAG_MIME)) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = 'copy';
    }
    function handleDragEnter(event: DragEvent) {
        if (!event.dataTransfer?.types.includes(TABLE_DRAG_MIME)) return;
        dragOver = true;
    }
    function handleDragLeave(event: DragEvent) {
        // Cast to `globalThis.Node`, not the bare name — `Node` is shadowed in
        // this file's type space by the imported xyflow `Node` (a flow graph
        // node), not the DOM node type `Element.contains` expects.
        const related = event.relatedTarget;
        if (related instanceof globalThis.Node && (event.currentTarget as HTMLElement).contains(related)) return;
        dragOver = false;
    }
    function handleDrop(event: DragEvent) {
        dragOver = false;
        const table = event.dataTransfer?.getData(TABLE_DRAG_MIME);
        if (!table) return;
        event.preventDefault();
        const position = screenToFlowPosition({ x: event.clientX, y: event.clientY });
        onDropTable(table, position);
    }
</script>

<div
    class="query-canvas"
    class:drag-active={dragOver}
    role="application"
    ondragover={handleDragOver}
    ondragenter={handleDragEnter}
    ondragleave={handleDragLeave}
    ondrop={handleDrop}
>
    {#if !hasTables}<div class="diagram-empty">
            <Icon name="columns" size={30} />
            <h3>Add tables to begin</h3>
            <p>Search for a table above, or drag one from the object browser, to place it on the canvas.</p>
        </div>{:else}<SvelteFlow
            id={`visual-query-${documentId}`}
            {nodes}
            {edges}
            {nodeTypes}
            {defaultEdgeOptions}
            {fitViewOptions}
            {snapGrid}
            fitView
            minZoom={0.45}
            maxZoom={1.2}
            nodeDragThreshold={4}
            selectionKey={null}
            deleteKey={null}
            nodesConnectable={true}
            clickConnect={false}
            {isValidConnection}
            onconnect={onConnect}
            autoPanOnConnect={false}
            autoPanOnNodeDrag={false}
            autoPanOnSelection={false}
            selectNodesOnDrag={false}
            selectionOnDrag={false}
            edgesFocusable={false}
            elevateEdgesOnSelect={false}
            defaultMarkerColor={null}
            panOnScroll
            panOnScrollSpeed={0.7}
            panOnScrollMode={PanOnScrollMode.Free}
            zoomOnDoubleClick={false}
            zoomOnScroll
            zoomOnPinch
            connectionLineType={ConnectionLineType.SmoothStep}
            noDragClass="nodrag"
            noPanClass="nopan"
            onnodedragstop={({ targetNode }) => {
                if (targetNode) onNodeDragStop(targetNode.id, targetNode.position);
            }}
            onedgeclick={({ edge }) => onEdgeClick(edge)}
        >
            <Controls position="bottom-left" showLock={false} orientation="horizontal" {fitViewOptions} />
            <MiniMap
                position="bottom-right"
                width={150}
                height={100}
                pannable={false}
                zoomable={false}
                nodeColor={() => 'var(--diagram-minimap-node)'}
                nodeStrokeColor={() => 'var(--diagram-edge)'}
                nodeStrokeWidth={1}
                nodeBorderRadius={3}
                maskColor="var(--diagram-minimap-mask)"
                maskStrokeColor="var(--accent)"
                maskStrokeWidth={1}
            />
            <Background variant={BackgroundVariant.Cross} gap={24} size={4} lineWidth={1} patternColor="color-mix(in srgb, var(--accent) 22%, var(--surface))" />
        </SvelteFlow>{/if}
</div>
