import type { CellValue, ChartAggregate, ChartFieldRole, ChartSeriesConfig, ChartType, Column, Row } from '../../state/types';
import { isBlobCell } from '../../state/types';

export interface ChartConfig {
    chartType: ChartType;
    role: ChartFieldRole;
    aggregate: ChartAggregate;
    pieLimit: number;
    binCount: number;
}

export interface XYSeries {
    name: string;
    data: (number | null)[];
}

export interface PieSlice {
    name: string;
    value: number;
}

export interface ScatterSeries {
    name: string;
    data: [number, number][];
}

export interface HistogramBin {
    label: string;
    count: number;
}

export type AggregatedChart =
    | { chartType: 'bar' | 'line'; categories: string[]; series: XYSeries[] }
    | { chartType: 'pie'; slices: PieSlice[] }
    | { chartType: 'scatter'; series: ScatterSeries[] }
    | { chartType: 'histogram'; bins: HistogramBin[] };

// columnKind classifies one column as the shape suggestChartConfig's rule
// table keys off: a declared type wins when it says something (numeric or
// temporal SQL/driver type names); otherwise fall back to sniffing a sample
// of actual values, which is the only signal a schemaless column (MongoDB's
// declared type is always "mixed") ever has.
function columnKind(column: Column, sample: Row[]): 'numeric' | 'temporal' | 'categorical' {
    if (isTemporalType(column.declaredType) || sample.some((row) => isTemporalValue(row[column.name]))) {
        return 'temporal';
    }
    if (isNumericType(column.declaredType) || sample.some((row) => typeof numericValue(row[column.name]) === 'number')) {
        return 'numeric';
    }
    return 'categorical';
}

export function defaultRole(columns: Column[], rows: Row[]): ChartFieldRole {
    return suggestChartConfig(columns, rows).role;
}

// suggestChartConfig applies a fixed rule table: a chart type + field
// mapping guessed from the shape of a source's
// columns, applied once when a chart document is first created and always
// overridable afterward through the config panel. A temporal+numeric pair
// takes priority over a categorical+numeric pair when both are present,
// since a time series is normally the more useful zero-configuration
// default for a table that has a timestamp column at all.
export function suggestChartConfig(columns: Column[], rows: Row[]): { chartType: ChartType; role: ChartFieldRole } {
    const sample = rows.slice(0, 50);
    const kinds = columns.map((column) => ({ column, kind: columnKind(column, sample) }));
    const temporal = kinds.filter((k) => k.kind === 'temporal');
    const numeric = kinds.filter((k) => k.kind === 'numeric');
    const categorical = kinds.filter((k) => k.kind === 'categorical');

    if (temporal.length && numeric.length) {
        return {
            chartType: 'line',
            role: { x: temporal[0].column.name, y: numeric[0].column.name },
        };
    }
    if (categorical.length && numeric.length) {
        return {
            chartType: 'bar',
            role: { x: categorical[0].column.name, y: numeric[0].column.name },
        };
    }
    if (numeric.length >= 2) {
        return {
            chartType: 'scatter',
            role: { x: numeric[0].column.name, y: numeric[1].column.name },
        };
    }
    if (numeric.length === 1) {
        return {
            chartType: 'histogram',
            role: { x: numeric[0].column.name, y: numeric[0].column.name },
        };
    }
    // Degenerate fallback (no numeric column at all, or no columns): the best
    // a bar chart can do is pick the first two fields it has.
    const names = columns.map((c) => c.name);
    return { chartType: 'bar', role: { x: names[0] ?? '', y: names[1] ?? names[0] ?? '' } };
}

export function defaultChartSeries(id: string, label: string, source: ChartSeriesConfig['source']): ChartSeriesConfig {
    const { chartType, role } = suggestChartConfig(source.columns, source.rows);
    return {
        id,
        label,
        source,
        chartType,
        role,
        aggregate: 'count',
        pieLimit: 12,
        binCount: 10,
    };
}

export function aggregateRows(rows: Row[], config: ChartConfig): AggregatedChart {
    if (config.chartType === 'scatter') return aggregateScatter(rows, config.role);
    if (config.chartType === 'histogram') return aggregateHistogram(rows, config.role, config.binCount);
    const groups = groupedValues(rows, config.role, config.aggregate);
    if (config.chartType === 'pie') {
        return { chartType: 'pie', slices: pieSlices(groups.get('Series') ?? [], config.pieLimit) };
    }
    const categories = Array.from(new Set(Array.from(groups.values()).flatMap((items) => items.map((item) => item.x))));
    return {
        chartType: config.chartType,
        categories,
        series: Array.from(groups.entries()).map(([name, items]) => {
            const byX = new Map(items.map((item) => [item.x, item.y]));
            return { name, data: categories.map((category) => byX.get(category) ?? null) };
        }),
    };
}

function groupedValues(rows: Row[], role: ChartFieldRole, aggregate: ChartAggregate) {
    const buckets = new Map<string, Map<string, CellValue[]>>();
    for (const row of rows) {
        const series = role.series ? label(row[role.series]) : 'Series';
        const x = label(row[role.x]);
        const values = buckets.get(series) ?? new Map<string, CellValue[]>();
        values.set(x, [...(values.get(x) ?? []), row[role.y]]);
        buckets.set(series, values);
    }
    const result = new Map<string, { x: string; y: number }[]>();
    for (const [series, values] of buckets) {
        result.set(
            series,
            Array.from(values.entries()).map(([x, bucket]) => ({
                x,
                y: aggregateBucket(bucket, aggregate),
            })),
        );
    }
    return result;
}

function aggregateBucket(values: CellValue[], aggregate: ChartAggregate) {
    if (aggregate === 'count') return values.length;
    if (aggregate === 'countDistinct') return new Set(values.map((value) => label(value))).size;
    const numbers = values.map((value) => numericValue(value)).filter((value): value is number => typeof value === 'number');
    if (!numbers.length) return 0;
    if (aggregate === 'sum') return numbers.reduce((sum, value) => sum + value, 0);
    if (aggregate === 'avg') return numbers.reduce((sum, value) => sum + value, 0) / Math.max(1, numbers.length);
    if (aggregate === 'min') return Math.min(...numbers);
    return Math.max(...numbers);
}

function pieSlices(items: { x: string; y: number }[], limit: number) {
    const sorted = [...items].sort((a, b) => b.y - a.y);
    const cap = Math.max(1, limit || 12);
    if (sorted.length <= cap) return sorted.map(({ x, y }) => ({ name: x, value: y }));
    const visible = sorted.slice(0, cap);
    const other = sorted.slice(cap).reduce((sum, item) => sum + item.y, 0);
    return [...visible.map(({ x, y }) => ({ name: x, value: y })), { name: 'Other', value: other }];
}

function aggregateScatter(rows: Row[], role: ChartFieldRole): AggregatedChart {
    const groups = new Map<string, [number, number][]>();
    for (const row of rows) {
        const x = numericValue(row[role.x]);
        const y = numericValue(row[role.y]);
        if (typeof x !== 'number' || typeof y !== 'number') continue;
        const name = role.color ? label(row[role.color]) : 'Series';
        groups.set(name, [...(groups.get(name) ?? []), [x, y]]);
    }
    return {
        chartType: 'scatter',
        series: Array.from(groups.entries()).map(([name, data]) => ({ name, data })),
    };
}

// aggregateHistogram bins one numeric field client-side — a presentation
// choice, not an aggregation, so it stays client-side even for a pushdown
// source (ChartDocument.svelte never routes a
// histogram series through server-side Aggregate in the first place, since
// there's no useful GROUP BY for "give me every raw value to bin").
function aggregateHistogram(rows: Row[], role: ChartFieldRole, binCount: number): AggregatedChart {
    const values = rows.map((row) => numericValue(row[role.x])).filter((value): value is number => typeof value === 'number');
    if (!values.length) return { chartType: 'histogram', bins: [] };
    const min = Math.min(...values);
    const max = Math.max(...values);
    if (min === max) {
        return {
            chartType: 'histogram',
            bins: [{ label: formatBinBound(min), count: values.length }],
        };
    }
    const count = Math.max(1, Math.min(100, Math.round(binCount) || 10));
    const width = (max - min) / count;
    const counts = new Array(count).fill(0);
    for (const value of values) {
        const idx = Math.min(count - 1, Math.max(0, Math.floor((value - min) / width)));
        counts[idx] += 1;
    }
    const bins = counts.map((bucketCount, i) => {
        const lo = min + i * width;
        const hi = i === count - 1 ? max : min + (i + 1) * width;
        return { label: `${formatBinBound(lo)}–${formatBinBound(hi)}`, count: bucketCount };
    });
    return { chartType: 'histogram', bins };
}

function formatBinBound(value: number): string {
    return Number.isInteger(value) ? String(value) : value.toFixed(2);
}

export function numericValue(value: CellValue | undefined): number | undefined {
    if (typeof value === 'number' && Number.isFinite(value)) return value;
    if (typeof value !== 'string' || !value.trim()) return undefined;
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
}

export function label(value: CellValue | undefined): string {
    if (value === null || value === undefined) return 'NULL';
    if (isBlobCell(value)) return `BLOB ${value.size} bytes`;
    if (typeof value === 'object') return Array.isArray(value) ? `[${value.length} items]` : `{${Object.keys(value).length} fields}`;
    return String(value);
}

// isTemporalValue only classifies an actual date/time-*looking* string —
// numericValue is checked first so a plain integer (e.g. an id or an epoch
// number, both of which Date.parse can misinterpret) is never misclassified
// as temporal just because it happens to parse.
function isTemporalValue(value: CellValue | undefined): boolean {
    if (typeof value !== 'string' || !value.trim()) return false;
    if (numericValue(value) !== undefined) return false;
    return Number.isFinite(Date.parse(value));
}

function isTemporalType(type: string | undefined): boolean {
    return /date|time|timestamp/i.test(type ?? '');
}

function isNumericType(type: string | undefined): boolean {
    return /int|real|float|double|decimal|numeric|number|serial/i.test(type ?? '');
}

export type BarLineChart = Extract<AggregatedChart, { chartType: 'bar' | 'line' }>;

// combineBarLineSeries merges several independently-resolved bar/line charts
// (each one series config's own snapshot, pushdown, or loaded-rows result)
// into one chart: the union of every x
// category across all of them, each series' values aligned to that union
// with a gap (null, never a fabricated zero) where it has none. Nothing here
// knows or cares whether the inputs came from the same table, a different
// table, or a different engine entirely — each was already fully resolved
// before it got here.
export function combineBarLineSeries(resolved: { label: string; chart: BarLineChart }[]): BarLineChart {
    const chartType = resolved[0]?.chart.chartType ?? 'bar';
    const categories = mergeCategories(resolved.map((r) => r.chart.categories));
    const series: XYSeries[] = [];
    for (const r of resolved) {
        for (const line of r.chart.series) {
            const values = new Map(r.chart.categories.map((category, i) => [category, line.data[i] ?? null]));
            const name = resolved.length === 1 ? line.name : line.name === 'Series' ? r.label : `${r.label} · ${line.name}`;
            series.push({ name, data: categories.map((category) => values.get(category) ?? null) });
        }
    }
    return { chartType, categories, series };
}

// mergeCategories unions x categories across independently-resolved series,
// sorted numerically or by date when every category parses that way (a
// month-by-month or id-by-id chart should read left-to-right in order, not
// in whatever order each series happened to encounter its own values),
// falling back to first-seen insertion order otherwise.
function mergeCategories(lists: string[][]): string[] {
    const seen = new Set<string>();
    const union: string[] = [];
    for (const list of lists) {
        for (const category of list) {
            if (!seen.has(category)) {
                seen.add(category);
                union.push(category);
            }
        }
    }
    if (union.every((c) => Number.isFinite(Number(c)) && c.trim() !== '')) {
        return [...union].sort((a, b) => Number(a) - Number(b));
    }
    if (union.every((c) => Number.isFinite(Date.parse(c)))) {
        return [...union].sort((a, b) => Date.parse(a) - Date.parse(b));
    }
    return union;
}
