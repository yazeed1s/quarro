import { describe, expect, test } from 'vitest';
import type { SchemaTable } from '../../state/types';
import { renderSchemaDiagramSVG, type DiagramExportOptions } from './export';

const tables: SchemaTable[] = [
    {
        schema: 'main',
        name: 'users',
        kind: 'table',
        columns: [
            { name: 'id', type: 'INTEGER', notNull: true, primaryKey: true, defaultValue: null },
            { name: 'email<&', type: 'TEXT', notNull: false, primaryKey: false, defaultValue: null },
        ],
        indexes: [{ name: 'users_email', columns: ['email<&'], unique: true }],
        foreignKeys: [],
    },
    {
        schema: 'main',
        name: 'orders',
        kind: 'table',
        columns: [
            { name: 'id', type: 'INTEGER', notNull: true, primaryKey: true, defaultValue: null },
            { name: 'user_id', type: 'INTEGER', notNull: true, primaryKey: false, defaultValue: null },
        ],
        indexes: [],
        foreignKeys: [{ column: 'user_id', refTable: 'users', refColumn: 'id' }],
    },
];

const options: DiagramExportOptions = {
    tables,
    positions: { users: { x: 80, y: 70 }, orders: { x: 410, y: 70 } },
    collapsed: [],
    relatedTable: 'users',
    nodeDetail: 'full',
    nodeWidth: 278,
    columnHeight: 28,
    nodeColumnHeight: 260,
    edgeLabels: true,
    edgeMarkers: true,
    edgeStyle: 'smoothstep',
    edgeWidth: 1.5,
    relatedEdgeWidth: 2.2,
    labelSize: 11,
    background: 'cross',
    gridGap: 22,
    gridMarkSize: 2,
    palette: {
        surface: '#fff',
        subtle: '#eee',
        card: '#fafafa',
        border: '#ccc',
        borderSoft: '#ddd',
        text: '#111',
        muted: '#666',
        accent: '#222',
        accentText: '#111',
    },
};

describe('schema diagram SVG export', () => {
    test('renders a standalone diagram with tables, keys, and relationships', () => {
        const result = renderSchemaDiagramSVG(options);
        expect(result.svg).toMatch(/^<svg xmlns="http:\/\/www\.w3\.org\/2000\/svg"/);
        expect(result.svg).toContain('users');
        expect(result.svg).toContain('orders');
        expect(result.svg).toContain('user_id → id');
        expect(result.svg).toContain('marker-end="url(#arrow)"');
        expect(result.svg).toContain('fill="#fff"');
        expect(result.width).toBeGreaterThan(700);
        expect(result.height).toBeGreaterThan(150);
    });

    test('escapes schema text and honors export visibility settings', () => {
        const result = renderSchemaDiagramSVG({ ...options, edgeLabels: false, edgeMarkers: false, background: 'none' });
        expect(result.svg).toContain('email&lt;&amp;');
        expect(result.svg).not.toContain('user_id → id');
        expect(result.svg).not.toContain('marker-end');
        expect(result.svg).not.toContain('id="grid"');
        expect(result.svg).toContain('fill="#fff"');
    });
});
