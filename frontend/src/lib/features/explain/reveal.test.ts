import { describe, expect, test } from 'vitest';
import { findNodeRangesInDocument, findRelationRange } from './reveal';
import type { ExplainNode } from '../../state/types';

function node(partial: Partial<ExplainNode>): ExplainNode {
    return { id: '1', operation: 'Seq Scan', loops: 1, badges: [], children: [], ...partial };
}

describe('findRelationRange', () => {
    test('finds a bare relation after FROM', () => {
        const sql = 'SELECT * FROM orders o WHERE o.id = 1';
        const r = findRelationRange(sql, 'orders')!;
        expect(sql.slice(r.from, r.to)).toBe('orders');
    });

    test('finds a relation after JOIN, quoted', () => {
        const sql = 'SELECT * FROM o JOIN "users" u ON u.id = o.user_id';
        const r = findRelationRange(sql, 'users')!;
        expect(sql.slice(r.from, r.to)).toBe('users');
    });

    test('finds a backtick-quoted MySQL relation', () => {
        const sql = 'SELECT * FROM `orders`';
        const r = findRelationRange(sql, 'orders')!;
        expect(sql.slice(r.from, r.to)).toBe('orders');
    });

    test('returns undefined for an empty relation or no match', () => {
        expect(findRelationRange('SELECT * FROM orders', '')).toBeUndefined();
        expect(findRelationRange('SELECT 1', 'orders')).toBeUndefined();
    });
});

describe('findNodeRangesInDocument', () => {
    test('offsets the statement match into the full document', () => {
        const full = '-- draft\nSELECT * FROM orders WHERE id = 1;\n';
        const statement = 'SELECT * FROM orders WHERE id = 1';
        const root = node({
            id: 'scan',
            relation: 'orders',
            badges: [{ kind: 'hotspot', severity: 'warning', message: 'x' }],
        });
        const ranges = findNodeRangesInDocument(full, statement, root, (n) => n.id === 'scan');
        expect(ranges).toHaveLength(1);
        expect(full.slice(ranges[0].from, ranges[0].to)).toBe('orders');
    });

    test('returns nothing when the statement text is no longer present', () => {
        const full = 'SELECT * FROM users';
        const statement = 'SELECT * FROM orders';
        const root = node({ relation: 'orders' });
        expect(findNodeRangesInDocument(full, statement, root, () => true)).toEqual([]);
    });

    test('collects every node matching filter across the tree', () => {
        const full = 'SELECT * FROM orders o JOIN users u ON u.id = o.user_id';
        const statement = full;
        const scan = node({
            id: 'a',
            relation: 'orders',
            badges: [{ kind: 'hotspot', severity: 'warning', message: 'x' }],
        });
        const lookup = node({
            id: 'b',
            relation: 'users',
            badges: [{ kind: 'hotspot', severity: 'warning', message: 'y' }],
        });
        const root = node({ id: 'root', operation: 'Nested Loop', children: [scan, lookup] });
        const ranges = findNodeRangesInDocument(full, statement, root, (n) => n.badges.some((b) => b.kind === 'hotspot'));
        expect(ranges.map((r) => full.slice(r.from, r.to))).toEqual(['orders', 'users']);
    });
});
