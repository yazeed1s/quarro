import type { ExplainNode } from '../../state/types';

export interface ExplainLayout {
    positions: Map<string, { x: number; y: number }>;
    width: number;
    height: number;
}

const COL_WIDTH = 240;
const ROW_HEIGHT = 130;

// layoutPlan assigns each node a deterministic position: depth (parent
// chain length) becomes the row, and leaves are ordered left to right with
// every parent centered over its own children's span. The same "leaves
// first, then average up" tree-layout approach SchemaDiagram.svelte uses
// for its own (differently-shaped) relational graph.
export function layoutPlan(root: ExplainNode | null): ExplainLayout {
    const positions = new Map<string, { x: number; y: number }>();
    if (!root) return { positions, width: 0, height: 0 };
    let nextLeaf = 0;
    const place = (node: ExplainNode, depth: number): number => {
        if (!node.children.length) {
            const x = nextLeaf++;
            positions.set(node.id, { x: x * COL_WIDTH, y: depth * ROW_HEIGHT });
            return x;
        }
        const xs = node.children.map((child) => place(child, depth + 1));
        const x = (xs[0] + xs[xs.length - 1]) / 2;
        positions.set(node.id, { x: x * COL_WIDTH, y: depth * ROW_HEIGHT });
        return x;
    };
    place(root, 0);
    return {
        positions,
        width: Math.max(nextLeaf, 1) * COL_WIDTH,
        height: (maxDepth(root) + 1) * ROW_HEIGHT,
    };
}

function maxDepth(node: ExplainNode): number {
    return node.children.length ? 1 + Math.max(...node.children.map(maxDepth)) : 0;
}

// flattenPlan lists every node in the tree, for the node count (minimap
// threshold), edge list, and Table View's future row list alike.
export function flattenPlan(root: ExplainNode | null): ExplainNode[] {
    if (!root) return [];
    const out: ExplainNode[] = [];
    const visit = (node: ExplainNode) => {
        out.push(node);
        node.children.forEach(visit);
    };
    visit(root);
    return out;
}

// exclusiveMetric reads whichever exclusive value the plan actually
// carries for this node: time under EXPLAIN ANALYZE, cost otherwise (the
// same fallback internal/database/explain.Annotate's own badge math uses).
export function exclusiveMetric(node: ExplainNode): number {
    return node.exclusiveTimeMs ?? node.exclusiveCost ?? 0;
}

// severityTier buckets a node's exclusive share of the plan's most
// expensive node into three coarse tiers for color coding. A fixed
// three-way split (not a continuous gradient) keeps the mapping legible at
// a glance, the same reasoning this app's other severity indicators
// (running/failed/disconnected states) already follow.
export function severityTier(node: ExplainNode, maxExclusive: number): 'ok' | 'warn' | 'bad' {
    if (maxExclusive <= 0) return 'ok';
    const share = exclusiveMetric(node) / maxExclusive;
    if (share >= 0.66) return 'bad';
    if (share >= 0.25) return 'warn';
    return 'ok';
}

export interface ExplainRow {
    node: ExplainNode;
    depth: number;
}

// flattenVisible lists every node with its depth, skipping the subtree
// under any node whose id is in collapsed. Table View's own row list.
export function flattenVisible(root: ExplainNode | null, collapsed: Set<string>): ExplainRow[] {
    if (!root) return [];
    const out: ExplainRow[] = [];
    const visit = (node: ExplainNode, depth: number) => {
        out.push({ node, depth });
        if (collapsed.has(node.id)) return;
        node.children.forEach((child) => visit(child, depth + 1));
    };
    visit(root, 0);
    return out;
}

export type DiagramMetric = 'time' | 'rows' | 'cost' | 'buffers';

// bufferTotal sums every Extra value whose key names a buffer count
// (Postgres's own BUFFERS option: "Shared Hit Blocks", "Local Read
// Blocks", ...). Every other engine's Extra simply has no such key, so
// this quietly returns 0 there rather than fabricating a number; it is
// presentation-layer aggregation over real, already-annotated data, not a
// new normalized backend field, since "buffers" has no single engine-
// neutral shape the way rows/cost/time do.
export function bufferTotal(node: ExplainNode): number {
    let total = 0;
    for (const [key, value] of Object.entries(node.extra ?? {})) {
        if (typeof value === 'number' && key.includes('Block')) total += value;
    }
    return total;
}

// diagramMetricValue reads the one value Diagram View's chosen metric
// represents for a node, always the node's own total (inclusive), not its
// exclusive share (that distinction is Graph View's own job).
export function diagramMetricValue(node: ExplainNode, metric: DiagramMetric): number {
    switch (metric) {
        case 'time':
            return node.inclusiveTimeMs ?? 0;
        case 'rows':
            return node.actualRows ?? node.estimatedRows ?? 0;
        case 'cost':
            return node.estimatedCost ?? 0;
        case 'buffers':
            return bufferTotal(node);
    }
}
