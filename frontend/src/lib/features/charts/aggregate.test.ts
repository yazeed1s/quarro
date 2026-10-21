import { describe, expect, test } from 'vitest';
import { aggregateRows, combineBarLineSeries, suggestChartConfig, type BarLineChart } from './aggregate';
import type { Column, Row } from '../../state/types';

describe('aggregateRows', () => {
    test('returns an empty bar chart for empty rows', () => {
        const chart = aggregateRows([], {
            chartType: 'bar',
            role: { x: 'status', y: 'total' },
            aggregate: 'sum',
            pieLimit: 12,
            binCount: 10,
        });
        expect(chart).toEqual({ chartType: 'bar', categories: [], series: [] });
    });

    test('groups all-null x values under a visible NULL label', () => {
        const rows: Row[] = [
            { status: null, total: '10' },
            { status: null, total: '15' },
        ];
        const chart = aggregateRows(rows, {
            chartType: 'bar',
            role: { x: 'status', y: 'total' },
            aggregate: 'count',
            pieLimit: 12,
            binCount: 10,
        });
        expect(chart).toEqual({
            chartType: 'bar',
            categories: ['NULL'],
            series: [{ name: 'Series', data: [2] }],
        });
    });

    test('ignores non-numeric values for numeric aggregates', () => {
        const rows: Row[] = [
            { status: 'paid', total: '12.5' },
            { status: 'paid', total: 'not a number' },
            { status: 'paid', total: 7.5 },
        ];
        const chart = aggregateRows(rows, {
            chartType: 'bar',
            role: { x: 'status', y: 'total' },
            aggregate: 'sum',
            pieLimit: 12,
            binCount: 10,
        });
        expect(chart).toEqual({
            chartType: 'bar',
            categories: ['paid'],
            series: [{ name: 'Series', data: [20] }],
        });
    });

    test('collapses pie slices over the cap into Other', () => {
        const rows: Row[] = Array.from({ length: 5 }, (_, i) => ({
            label: `category ${i + 1}`,
            total: String(10 - i),
        }));
        const chart = aggregateRows(rows, {
            chartType: 'pie',
            role: { x: 'label', y: 'total' },
            aggregate: 'sum',
            pieLimit: 3,
            binCount: 10,
        });
        expect(chart).toEqual({
            chartType: 'pie',
            slices: [
                { name: 'category 1', value: 10 },
                { name: 'category 2', value: 9 },
                { name: 'category 3', value: 8 },
                { name: 'Other', value: 13 },
            ],
        });
    });

    test('returns no bins for empty rows', () => {
        const chart = aggregateRows([], {
            chartType: 'histogram',
            role: { x: 'amount', y: 'amount' },
            aggregate: 'count',
            pieLimit: 12,
            binCount: 5,
        });
        expect(chart).toEqual({ chartType: 'histogram', bins: [] });
    });

    test('bins numeric values into the requested bin count, covering the full range', () => {
        const rows: Row[] = [0, 1, 2, 8, 9, 10].map((amount) => ({ amount }));
        const chart = aggregateRows(rows, {
            chartType: 'histogram',
            role: { x: 'amount', y: 'amount' },
            aggregate: 'count',
            pieLimit: 12,
            binCount: 5,
        });
        expect(chart.chartType).toBe('histogram');
        if (chart.chartType !== 'histogram') throw new Error('unreachable');
        expect(chart.bins).toHaveLength(5);
        expect(chart.bins.reduce((sum, bin) => sum + bin.count, 0)).toBe(rows.length);
        // The max value (10) falls in the last bin, inclusive of the upper edge.
        expect(chart.bins.at(-1)?.count).toBeGreaterThan(0);
    });

    test('a single distinct value collapses into one bin instead of dividing by a zero-width range', () => {
        const rows: Row[] = [5, 5, 5].map((amount) => ({ amount }));
        const chart = aggregateRows(rows, {
            chartType: 'histogram',
            role: { x: 'amount', y: 'amount' },
            aggregate: 'count',
            pieLimit: 12,
            binCount: 10,
        });
        expect(chart).toEqual({ chartType: 'histogram', bins: [{ label: '5', count: 3 }] });
    });
});

describe('suggestChartConfig', () => {
    test('one categorical and one numeric column suggests bar', () => {
        const columns: Column[] = [
            { name: 'status', declaredType: 'text' },
            { name: 'total', declaredType: 'integer' },
        ];
        const rows: Row[] = [{ status: 'paid', total: 5 }];
        expect(suggestChartConfig(columns, rows)).toEqual({
            chartType: 'bar',
            role: { x: 'status', y: 'total' },
        });
    });

    test('a temporal and a numeric column suggests line, even alongside a categorical column', () => {
        const columns: Column[] = [
            { name: 'created_at', declaredType: 'timestamp' },
            { name: 'status', declaredType: 'text' },
            { name: 'total', declaredType: 'numeric' },
        ];
        const rows: Row[] = [{ created_at: '2026-01-01', status: 'paid', total: 5 }];
        expect(suggestChartConfig(columns, rows)).toEqual({
            chartType: 'line',
            role: { x: 'created_at', y: 'total' },
        });
    });

    test('two numeric columns suggest scatter', () => {
        const columns: Column[] = [
            { name: 'height', declaredType: 'real' },
            { name: 'weight', declaredType: 'real' },
        ];
        expect(suggestChartConfig(columns, [])).toEqual({
            chartType: 'scatter',
            role: { x: 'height', y: 'weight' },
        });
    });

    test('one numeric column alone suggests histogram', () => {
        const columns: Column[] = [{ name: 'amount', declaredType: 'real' }];
        expect(suggestChartConfig(columns, [])).toEqual({
            chartType: 'histogram',
            role: { x: 'amount', y: 'amount' },
        });
    });

    test('a schemaless column classifies from sampled values, not just a declared type', () => {
        // MongoDB's declared type is always "mixed"; the only signal available
        // is what the sampled values look like.
        const columns: Column[] = [
            { name: 'status', declaredType: 'mixed' },
            { name: 'total', declaredType: 'mixed' },
        ];
        const rows: Row[] = [
            { status: 'active', total: 5 },
            { status: 'retired', total: 3 },
        ];
        expect(suggestChartConfig(columns, rows)).toEqual({
            chartType: 'bar',
            role: { x: 'status', y: 'total' },
        });
    });
});

describe('combineBarLineSeries', () => {
    function bar(categories: string[], data: (number | null)[], name = 'Series'): BarLineChart {
        return { chartType: 'bar', categories, series: [{ name, data }] };
    }

    test('a single resolved series passes through unprefixed, matching the pre-multi-source shape', () => {
        const combined = combineBarLineSeries([{ label: 'Orders', chart: bar(['active', 'retired'], [3, 1]) }]);
        expect(combined).toEqual({
            chartType: 'bar',
            categories: ['active', 'retired'],
            series: [{ name: 'Series', data: [3, 1] }],
        });
    });

    test('unions categories across independent series and leaves a gap, not a zero, where one has none', () => {
        const combined = combineBarLineSeries([
            { label: 'PostgreSQL orders', chart: bar(['Jan', 'Feb'], [10, 20]) },
            { label: 'MongoDB tickets', chart: bar(['Feb', 'Mar'], [5, 8]) },
        ]);
        expect(combined.categories).toEqual(['Jan', 'Feb', 'Mar']);
        expect(combined.series).toEqual([
            { name: 'PostgreSQL orders', data: [10, 20, null] },
            { name: 'MongoDB tickets', data: [null, 5, 8] },
        ]);
    });

    test('prefixes a sub-series name with its own series label once more than one series is combined', () => {
        const combined = combineBarLineSeries([
            { label: 'Orders', chart: bar(['Jan', 'Feb'], [10, 20], 'active') },
            { label: 'Tickets', chart: bar(['Jan', 'Feb'], [1, 2], 'open') },
        ]);
        expect(combined.series).toEqual([
            { name: 'Orders · active', data: [10, 20] },
            { name: 'Tickets · open', data: [1, 2] },
        ]);
    });

    test('sorts purely numeric categories numerically rather than lexicographically', () => {
        const combined = combineBarLineSeries([{ label: 'A', chart: bar(['2', '10', '1'], [2, 10, 1]) }]);
        expect(combined.categories).toEqual(['1', '2', '10']);
    });
});
