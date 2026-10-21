import { describe, expect, test } from 'vitest';
import { bufferTotal, diagramMetricValue, exclusiveMetric, flattenPlan, flattenVisible, layoutPlan, severityTier } from './layout';
import type { ExplainNode } from '../../state/types';

function node(partial: Partial<ExplainNode>): ExplainNode {
    return {
        id: '1',
        operation: 'Seq Scan',
        loops: 1,
        badges: [],
        children: [],
        ...partial,
    };
}

describe('layoutPlan', () => {
    test('returns an empty layout for no plan', () => {
        const layout = layoutPlan(null);
        expect(layout.positions.size).toBe(0);
        expect(layout.width).toBe(0);
    });

    test('centers a parent over its two leaf children', () => {
        const left = node({ id: 'a' });
        const right = node({ id: 'b' });
        const root = node({ id: 'root', children: [left, right] });
        const layout = layoutPlan(root);
        const posA = layout.positions.get('a')!;
        const posB = layout.positions.get('b')!;
        const posRoot = layout.positions.get('root')!;
        expect(posA.y).toBeGreaterThan(posRoot.y); // children sit one row below their parent
        expect(posB.y).toBe(posA.y);
        expect(posRoot.x).toBe((posA.x + posB.x) / 2);
    });
});

describe('flattenPlan', () => {
    test('lists every node in a stable pre-order', () => {
        const child = node({ id: 'c' });
        const root = node({ id: 'root', children: [child] });
        expect(flattenPlan(root).map((n) => n.id)).toEqual(['root', 'c']);
    });
});

describe('flattenVisible', () => {
    test('skips children under a collapsed node', () => {
        const grandchild = node({ id: 'gc' });
        const child = node({ id: 'c', children: [grandchild] });
        const root = node({ id: 'root', children: [child] });
        const all = flattenVisible(root, new Set());
        expect(all.map((r) => r.node.id)).toEqual(['root', 'c', 'gc']);
        const collapsed = flattenVisible(root, new Set(['c']));
        expect(collapsed.map((r) => r.node.id)).toEqual(['root', 'c']);
    });

    test('tracks depth', () => {
        const child = node({ id: 'c', children: [node({ id: 'gc' })] });
        const root = node({ id: 'root', children: [child] });
        const rows = flattenVisible(root, new Set());
        expect(rows.map((r) => r.depth)).toEqual([0, 1, 2]);
    });
});

describe('exclusiveMetric / severityTier', () => {
    test('falls back to exclusive cost when no time is present', () => {
        expect(exclusiveMetric(node({ exclusiveCost: 4 }))).toBe(4);
        expect(exclusiveMetric(node({ exclusiveTimeMs: 2, exclusiveCost: 4 }))).toBe(2);
    });

    test('buckets by share of the plan maximum', () => {
        expect(severityTier(node({ exclusiveTimeMs: 90 }), 100)).toBe('bad');
        expect(severityTier(node({ exclusiveTimeMs: 30 }), 100)).toBe('warn');
        expect(severityTier(node({ exclusiveTimeMs: 5 }), 100)).toBe('ok');
        expect(severityTier(node({ exclusiveTimeMs: 5 }), 0)).toBe('ok');
    });
});

describe('bufferTotal / diagramMetricValue', () => {
    test('sums only Extra keys naming a buffer count', () => {
        const n = node({
            extra: { 'Shared Hit Blocks': 12, 'Shared Read Blocks': 3, 'Index Name': 'x' },
        });
        expect(bufferTotal(n)).toBe(15);
        expect(bufferTotal(node({}))).toBe(0);
    });

    test('diagramMetricValue reads the right field per metric, actual rows over estimated', () => {
        const n = node({
            inclusiveTimeMs: 1.5,
            estimatedRows: 10,
            actualRows: 8,
            estimatedCost: 2.5,
        });
        expect(diagramMetricValue(n, 'time')).toBe(1.5);
        expect(diagramMetricValue(n, 'rows')).toBe(8);
        expect(diagramMetricValue(n, 'cost')).toBe(2.5);
        expect(diagramMetricValue(node({ estimatedRows: 10 }), 'rows')).toBe(10);
    });
});
