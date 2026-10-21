<script lang="ts">
    import {
        SvelteFlowProvider,
        type Connection,
        type DefaultEdgeOptions,
        type Edge,
        type FitViewOptions,
        type IsValidConnection,
        type Node,
        type NodeTypes,
    } from '@xyflow/svelte';
    import QueryCanvasInner from './QueryCanvasInner.svelte';

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
</script>

<SvelteFlowProvider>
    <QueryCanvasInner
        {documentId}
        {hasTables}
        {nodes}
        {edges}
        {nodeTypes}
        {defaultEdgeOptions}
        {fitViewOptions}
        {snapGrid}
        {onNodeDragStop}
        {onEdgeClick}
        {onDropTable}
        {isValidConnection}
        {onConnect}
    />
</SvelteFlowProvider>
