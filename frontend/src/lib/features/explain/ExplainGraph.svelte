<script lang="ts">
    import { Background, BackgroundVariant, Controls, MiniMap, SvelteFlow } from '@xyflow/svelte';
    import type { Edge, Node, NodeTypes } from '@xyflow/svelte';
    import type { ExplainNode } from '../../state/types';
    import { flattenPlan, layoutPlan } from './layout';
    import ExplainPlanNode from './ExplainPlanNode.svelte';

    let {
        root,
        analyzed,
        selectedNodeId,
        onselect,
    }: {
        root: ExplainNode | null;
        analyzed: boolean;
        selectedNodeId: string;
        onselect: (id: string) => void;
    } = $props();

    const nodeTypes: NodeTypes = { plan: ExplainPlanNode };
    const fitViewOptions = { padding: 0.2 };
    const defaultEdgeOptions: Edge = { type: 'smoothstep' } as Edge;

    let flatNodes = $derived(flattenPlan(root));
    let maxExclusive = $derived(flatNodes.reduce((max, n) => Math.max(max, n.exclusiveTimeMs ?? n.exclusiveCost ?? 0), 0));
    let nodes = $derived.by((): Node[] => {
        const layout = layoutPlan(root);
        return flatNodes.map((n) => ({
            id: n.id,
            type: 'plan',
            position: layout.positions.get(n.id) ?? { x: 0, y: 0 },
            data: { node: n, maxExclusive, analyzed },
            selected: n.id === selectedNodeId,
        }));
    });
    let edges = $derived.by((): Edge[] =>
        flatNodes
            .filter((n) => n.parentId)
            .map((n) => ({
                id: `${n.parentId}-${n.id}`,
                source: n.parentId!,
                target: n.id,
                type: 'smoothstep',
            })),
    );
    let showMinimap = $derived(flatNodes.length > 10);
</script>

{#if !root}
    <div class="explain-empty">
        <p>No plan yet.</p>
    </div>
{:else}
    <SvelteFlow
        {nodes}
        {edges}
        {nodeTypes}
        {defaultEdgeOptions}
        {fitViewOptions}
        fitView
        minZoom={0.2}
        maxZoom={2}
        nodesDraggable={false}
        nodesConnectable={false}
        clickConnect={false}
        elementsSelectable={true}
        selectionKey={null}
        deleteKey={null}
        nodesFocusable={false}
        edgesFocusable={false}
        defaultMarkerColor={null}
        onnodeclick={({ node }) => onselect(node.id === selectedNodeId ? '' : node.id)}
        onpaneclick={() => onselect('')}
    >
        <Controls position="bottom-left" showLock={false} orientation="horizontal" {fitViewOptions} />
        {#if showMinimap}<MiniMap
                position="bottom-right"
                pannable
                zoomable
                nodeColor={() => 'var(--diagram-minimap-node)'}
                nodeStrokeColor={() => 'var(--diagram-edge)'}
                nodeStrokeWidth={1}
                nodeBorderRadius={3}
                maskColor="var(--diagram-minimap-mask)"
                maskStrokeColor="var(--accent)"
                maskStrokeWidth={1}
            />{/if}
        <Background variant={BackgroundVariant.Cross} gap={22} size={2} lineWidth={0.5} patternColor="color-mix(in srgb, var(--accent) 22%, var(--surface))" />
    </SvelteFlow>
{/if}
