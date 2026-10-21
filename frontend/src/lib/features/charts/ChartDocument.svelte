<script lang="ts">
    import { onDestroy, onMount } from 'svelte';
    import type { EChartsOption, EChartsType } from 'echarts';
    import type { ChartAggregate, ChartSeriesConfig, ChartSource, ChartType, Document, Row, Session, VisualQuery, VisualQueryAggregate } from '../../state/types';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import Input from '../../components/Input.svelte';
    import Modal from '../../components/Modal.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';
    import * as backend from '../../state/backend';
    import { aggregateRows, combineBarLineSeries, defaultChartSeries, label, numericValue, type AggregatedChart, type BarLineChart } from './aggregate';
    import { workspace } from '../../state/workspace.svelte';

    let { session, document }: { session: Session; document: Document } = $props();

    type RenderMode = 'snapshot' | 'pushdown' | 'loadedRows' | 'combined' | 'empty' | 'error';
    type RenderState = {
        chart?: AggregatedChart;
        mode: RenderMode;
        label: string;
        detail: string;
        rows: number;
    };
    type SeriesOutcome =
        | {
              status: 'ok';
              mode: RenderMode;
              label: string;
              detail: string;
              rows: number;
              chart: AggregatedChart;
          }
        | { status: 'empty'; label: string; detail: string }
        | { status: 'error'; detail: string };
    type ChartPaletteMode = 'theme' | 'cool' | 'warm' | 'mono';
    type ChartLegendPosition = 'top' | 'right' | 'bottom' | 'left';
    type ChartExportBackground = 'white' | 'surface' | 'transparent';

    let chartEl = $state<HTMLDivElement>();
    let chart = $state<EChartsType>();
    let renderTimer: ReturnType<typeof setTimeout> | undefined;
    let resizeObserver: ResizeObserver | undefined;
    let themeObserver: MutationObserver | undefined;
    let themeKey = $state('');
    let renderState = $state<RenderState>({
        mode: 'empty',
        label: 'No data',
        detail: 'Pick fields to render this chart.',
        rows: 0,
    });
    let renderSeq = 0;
    const pendingJobs = new Set<string>();

    let seriesList = $derived(document.chartSeries);
    let chartType = $derived(seriesList[0]?.chartType ?? 'bar');
    // "Add series" only ever offers bar/line: pie,
    // scatter, and histogram are inherently single-series, and
    // combineBarLineSeries only knows how to merge the bar/line shape.
    let canAddSeries = $derived(chartType === 'bar' || chartType === 'line');
    let showViewInspector = $state(false);
    let showLegend = $state(true);
    let showTooltip = $state(true);
    let showLabels = $state(false);
    let showAxisLabels = $state(true);
    let showGridLines = $state(true);
    let showDataZoom = $state(false);
    let animationEnabled = $state(true);
    let lineSmooth = $state(true);
    let legendPosition = $state<ChartLegendPosition>('top');
    let paletteMode = $state<ChartPaletteMode>('theme');
    let exportBackground = $state<ChartExportBackground>('white');
    let canvasPadding = $state(18);
    let canvasMinHeight = $state(340);
    let gridLeft = $state(44);
    let gridRight = $state(18);
    let gridTop = $state(36);
    let gridBottom = $state(40);
    let labelFontSize = $state(12);
    let axisFontSize = $state(11);
    let axisRotate = $state(0);
    let barWidth = $state(72);
    let lineWidth = $state(2);
    let pointSize = $state(8);
    let pieInnerRadius = $state(42);
    let pieOuterRadius = $state(72);
    let animationDuration = $state(450);
    let chartViewStyle = $derived(`--chart-canvas-padding: ${canvasPadding}px; --chart-canvas-min-height: ${canvasMinHeight}px;`);
    let chartViewKey = $derived(
        JSON.stringify({
            showLegend,
            showTooltip,
            showLabels,
            showAxisLabels,
            showGridLines,
            showDataZoom,
            animationEnabled,
            lineSmooth,
            legendPosition,
            paletteMode,
            exportBackground,
            canvasPadding,
            canvasMinHeight,
            gridLeft,
            gridRight,
            gridTop,
            gridBottom,
            labelFontSize,
            axisFontSize,
            axisRotate,
            barWidth,
            lineWidth,
            pointSize,
            pieInnerRadius,
            pieOuterRadius,
            animationDuration,
        }),
    );

    // activeSeries falls back to the first series synchronously (not via an
    // $effect) so it — and the chart canvas <div> gated on it below — are
    // available on the very first render. onMount only runs once; if the
    // canvas div weren't in the DOM yet when it fires (as it wouldn't be with
    // an effect-deferred default), the chart would never initialize at all.
    let activeSeriesId = $state('');
    let activeSeries = $derived(seriesList.find((s) => s.id === activeSeriesId) ?? seriesList[0]);
    let activeSource = $derived(activeSeries?.source);
    let activeOwner = $derived(activeSource ? ownerSession(activeSource) : session);
    let activeSourceDoc = $derived(
        activeSource?.kind === 'table' && activeSource.sourceDocumentId ? activeOwner.documents.find((item) => item.id === activeSource.sourceDocumentId) : undefined,
    );
    let activeTableLoadedRows = $derived(
        activeSourceDoc?.kind === 'table' && activeSourceDoc.results[activeSourceDoc.activeResult]?.kind === 'rows'
            ? (activeSourceDoc.results[activeSourceDoc.activeResult].rows ?? [])
            : activeSource?.kind === 'table'
              ? activeSource.rows
              : [],
    );
    let activeRows = $derived(activeSource?.kind === 'snapshot' ? activeSource.rows : activeTableLoadedRows);
    let activeColumns = $derived(activeSource?.columns ?? []);
    let activeColumnNames = $derived(activeColumns.map((c) => c.name));
    let columnOptions = $derived(activeColumnNames.map((column) => ({ value: column, label: column })));
    let optionalColumnOptions = $derived([{ value: '', label: 'Single series' }, ...columnOptions]);

    let aggregateOptions: { value: ChartAggregate; label: string }[] = [
        { value: 'count', label: 'Count rows' },
        { value: 'sum', label: 'Sum' },
        { value: 'avg', label: 'Average' },
        { value: 'min', label: 'Minimum' },
        { value: 'max', label: 'Maximum' },
        { value: 'countDistinct', label: 'Count distinct' },
    ];
    let chartTypeOptions: { value: ChartType; label: string; disabled: boolean }[] = $derived([
        { value: 'bar', label: 'Bar', disabled: false },
        { value: 'line', label: 'Line', disabled: false },
        { value: 'pie', label: 'Pie', disabled: seriesList.length > 1 },
        { value: 'scatter', label: 'Scatter', disabled: seriesList.length > 1 },
        { value: 'histogram', label: 'Histogram', disabled: seriesList.length > 1 },
    ]);
    let paletteOptions: SelectOption[] = [
        { value: 'theme', label: 'Theme' },
        { value: 'cool', label: 'Cool' },
        { value: 'warm', label: 'Warm' },
        { value: 'mono', label: 'Mono' },
    ];
    let legendPositionOptions: SelectOption[] = [
        { value: 'top', label: 'Top' },
        { value: 'right', label: 'Right' },
        { value: 'bottom', label: 'Bottom' },
        { value: 'left', label: 'Left' },
    ];
    let exportBackgroundOptions: SelectOption[] = [
        { value: 'white', label: 'White' },
        { value: 'surface', label: 'Surface' },
        { value: 'transparent', label: 'Transparent' },
    ];

    // "Add series" modal state.
    let showAddSeries = $state(false);
    let addSeriesSessionId = $state('');
    let addSeriesSession = $derived(workspace.sessions.find((s) => s.id === addSeriesSessionId) ?? session);
    let addSeriesQueries = $derived(
        addSeriesSession.documents.filter((d) => d.kind === 'query' && d.results[d.activeResult]?.kind === 'rows' && (d.results[d.activeResult].rows?.length ?? 0) > 0),
    );
    let addSeriesTables = $derived(addSeriesSession.schema.filter((t) => t.kind === 'table' || t.kind === 'view' || t.kind === 'collection'));

    function ownerSession(source: ChartSource): Session {
        return workspace.sessions.find((s) => s.id === source.sessionId) ?? session;
    }

    function sourceRows(source: ChartSource, owner: Session): Row[] {
        if (source.kind === 'snapshot') return source.rows;
        const doc = source.sourceDocumentId ? owner.documents.find((item) => item.id === source.sourceDocumentId) : undefined;
        if (doc?.kind === 'table' && doc.results[doc.activeResult]?.kind === 'rows') {
            return doc.results[doc.activeResult].rows ?? [];
        }
        return source.rows;
    }

    $effect(() => {
        const needsFill = document.chartSeries.some((s) => !(s.chartType && s.role?.x && s.role?.y));
        if (!needsFill) return;
        document.chartSeries = document.chartSeries.map((s) => {
            if (s.chartType && s.role?.x && s.role?.y) return s;
            const owner = ownerSession(s.source);
            return {
                ...defaultChartSeries(s.id || crypto.randomUUID(), s.label || document.title, {
                    ...s.source,
                    rows: sourceRows(s.source, owner),
                }),
                ...s,
            };
        });
    });

    $effect(() => {
        const seriesKey = document.chartSeries.map((s) => {
            const owner = ownerSession(s.source);
            return {
                id: s.id,
                sessionId: s.source.sessionId,
                sourceKind: s.source.kind,
                sourceObject: s.source.kind === 'table' ? s.source.object : undefined,
                rows: sourceRows(s.source, owner).length,
                capabilities: owner.capabilities,
                connectionState: owner.connectionState,
                columns: s.source.columns.map((c) => c.name),
                chartType: s.chartType,
                role: s.role,
                aggregate: s.aggregate,
                pieLimit: s.pieLimit,
                binCount: s.binCount,
            };
        });
        const key = JSON.stringify({ themeKey, seriesKey });
        if (!key || !chart || !seriesList.length) return;
        clearTimeout(renderTimer);
        renderTimer = setTimeout(() => refreshChart(), 120);
    });

    $effect(() => {
        const key = `${chartViewKey}|${chartViewStyle}`;
        if (!key || !chart || !renderState.chart) return;
        chart.setOption(toOption(renderState.chart), true);
        chart.resize();
    });

    onMount(async () => {
        if (!chartEl) return;
        const echarts = await import('echarts');
        chart = echarts.init(chartEl, undefined, { renderer: 'canvas' });
        resizeObserver = new ResizeObserver(() => chart?.resize());
        resizeObserver.observe(chartEl);
        const updateTheme = () => {
            const html = window.document.documentElement;
            themeKey = `${html.dataset.theme ?? ''}|${html.dataset.density ?? ''}`;
            chart?.resize();
        };
        updateTheme();
        themeObserver = new MutationObserver(updateTheme);
        themeObserver.observe(window.document.documentElement, {
            attributes: true,
            attributeFilter: ['data-theme', 'data-density'],
        });
    });

    onDestroy(() => {
        clearTimeout(renderTimer);
        themeObserver?.disconnect();
        resizeObserver?.disconnect();
        for (const jobId of pendingJobs) backend.cancel(jobId).catch(() => {});
        pendingJobs.clear();
        chart?.dispose();
    });

    function updateSeries(patch: Partial<ChartSeriesConfig>) {
        if (!activeSeries) return;
        const id = activeSeries.id;
        document.chartSeries = document.chartSeries.map((s) => (s.id === id ? { ...s, ...patch } : s));
    }

    function updateRole(name: 'x' | 'y' | 'series' | 'color', value: string) {
        if (!activeSeries) return;
        const role = { ...activeSeries.role, [name]: value || undefined };
        if (name === 'x' || name === 'y') role[name] = value;
        updateSeries({ role });
    }

    function openAddSeries() {
        addSeriesSessionId = session.id;
        showAddSeries = true;
    }

    function addSeriesFromQuery(doc: Document) {
        workspace.addChartSeriesFromQuery(document, addSeriesSession, doc);
        showAddSeries = false;
    }

    function addSeriesFromTable(tableName: string) {
        workspace.addChartSeriesFromTable(document, addSeriesSession, tableName);
        showAddSeries = false;
    }

    async function resolveSeries(config: ChartSeriesConfig): Promise<SeriesOutcome> {
        const source = config.source;
        const owner = ownerSession(source);
        const rows = sourceRows(source, owner);
        // Histogram never has a useful GROUP BY (it bins raw values of one
        // field, not aggregated groups), so it never pushes down — same
        // reasoning that already excludes scatter and countDistinct.
        const canPushdown =
            source.kind === 'table' &&
            owner.connectionState === 'connected' &&
            owner.capabilities.aggregator &&
            config.chartType !== 'scatter' &&
            config.chartType !== 'histogram' &&
            config.aggregate !== 'countDistinct';

        if (canPushdown && source.kind === 'table') {
            const jobId = crypto.randomUUID();
            pendingJobs.add(jobId);
            document.jobId = jobId;
            try {
                const result = await backend.aggregate(
                    owner.id,
                    document.id,
                    jobId,
                    tableVisualQuery(source.object, config),
                    workspace.preferences.resultLimit,
                    workspace.preferences.timeout,
                );
                const first = result.results[0];
                if (first?.kind === 'error') {
                    return { status: 'error', detail: first.error ?? 'Aggregation failed.' };
                }
                return {
                    status: 'ok',
                    mode: 'pushdown',
                    label: 'Server pushdown',
                    detail: 'Aggregated by the database; the table grid was not loaded.',
                    rows: first?.rows?.length ?? 0,
                    chart: fromPushdownRows(first?.rows ?? [], config),
                };
            } catch (error) {
                return { status: 'error', detail: String(error) };
            } finally {
                pendingJobs.delete(jobId);
            }
        }

        // A 'snapshot' source is only ever created with rows already in hand
        // (Visualize refuses to fork a chart from an empty result); an empty one
        // here can only mean its rows were stripped from persistence across a
        // restart (a chart snapshot can run into megabytes and localStorage has
        // no room for several of them). Say so plainly instead of rendering a
        // misleadingly-empty chart, and point at
        // the explicit per-series Refresh action that re-resolves it.
        if (source.kind === 'snapshot' && !rows.length) {
            return {
                status: 'empty',
                label: 'Needs refresh',
                detail: 'This snapshot was not kept across restart. Use Refresh to re-run its source query.',
            };
        }

        if (source.kind === 'table' && !rows.length) {
            return {
                status: 'empty',
                label: 'No loaded rows fallback',
                detail:
                    config.chartType === 'scatter' || config.chartType === 'histogram' || config.aggregate === 'countDistinct'
                        ? 'This chart type needs loaded rows because it cannot use server aggregation yet.'
                        : 'Reconnect this session to use server aggregation.',
            };
        }

        return {
            status: 'ok',
            mode: source.kind === 'snapshot' ? 'snapshot' : 'loadedRows',
            label: source.kind === 'snapshot' ? 'Snapshot rows' : 'Loaded rows fallback',
            detail: source.kind === 'snapshot' ? 'Charted from the query result snapshot.' : 'Charted from rows already loaded in the table grid.',
            rows: rows.length,
            chart: aggregateRows(rows, {
                chartType: config.chartType,
                role: config.role,
                aggregate: config.aggregate,
                pieLimit: config.pieLimit,
                binCount: config.binCount,
            }),
        };
    }

    // refreshSeries is the one explicit, per-series way to re-resolve a
    // series right now, never automatic. A
    // 'table' source's pushdown/loaded-rows resolution is already always
    // fresh on every render (resolveSeries re-fetches it live), so bumping
    // the series' object identity is enough to make refreshChart's debounce
    // effect re-run it immediately. A 'snapshot' source has nothing to
    // re-fetch on its own — the only honest way to refresh it is to re-run
    // the query document it was snapshotted from and take its new rows,
    // which needs that document to still be open.
    async function refreshSeries(config: ChartSeriesConfig) {
        const source = config.source;
        if (source.kind === 'snapshot') {
            const owner = ownerSession(source);
            const sourceDoc = source.sourceDocumentId ? owner.documents.find((item) => item.id === source.sourceDocumentId) : undefined;
            if (!sourceDoc || sourceDoc.kind !== 'query') {
                workspace.notify('The original query document is no longer open. Re-run it and Visualize again to refresh this series.');
                return;
            }
            if (owner.connectionState !== 'connected') {
                workspace.notify('Reconnect this session before refreshing this series.');
                return;
            }
            await workspace.run(owner, sourceDoc, 'all');
            const result = sourceDoc.results[sourceDoc.activeResult];
            if (result?.kind !== 'rows' || !result.rows?.length) {
                workspace.notify('Refreshing this series did not return any rows.');
                return;
            }
            document.chartSeries = document.chartSeries.map((s) =>
                s.id === config.id
                    ? {
                          ...s,
                          source: {
                              ...s.source,
                              columns: result.columns ?? [],
                              rows: result.rows ?? [],
                          },
                      }
                    : s,
            );
            return;
        }
        document.chartSeries = document.chartSeries.map((s) => (s.id === config.id ? { ...s } : s));
    }

    async function refreshChart() {
        if (!chart || !seriesList.length) return;
        const seq = ++renderSeq;
        document.status = 'running';
        document.message = seriesList.length > 1 ? 'Resolving series…' : 'Aggregating…';
        const configs = seriesList;
        const outcomes = await Promise.all(configs.map((s) => resolveSeries(s)));
        if (seq !== renderSeq) return;

        const failed = outcomes.find((o) => o.status === 'error');
        if (failed && failed.status === 'error') {
            document.status = 'error';
            document.message = failed.detail;
            applyState({
                mode: 'error',
                label: configs.length > 1 ? 'A series failed to resolve' : 'Server aggregation failed',
                detail: failed.detail,
                rows: 0,
            });
            return;
        }

        const ok = outcomes
            .map((o, i) => ({ outcome: o, seriesLabel: configs[i].label }))
            .filter(
                (
                    entry,
                ): entry is {
                    outcome: Extract<SeriesOutcome, { status: 'ok' }>;
                    seriesLabel: string;
                } => entry.outcome.status === 'ok',
            );
        if (!ok.length) {
            const first = outcomes[0];
            document.status = 'done';
            document.message = first?.status === 'empty' ? first.detail : 'No data.';
            applyState({
                mode: 'empty',
                label: first?.status === 'empty' ? first.label : 'No data',
                detail: first?.status === 'empty' ? first.detail : 'Pick fields to render this chart.',
                rows: 0,
            });
            return;
        }

        document.status = 'done';
        const totalRows = ok.reduce((sum, entry) => sum + entry.outcome.rows, 0);
        const single = ok.length === 1;
        const combinedLabel = single ? ok[0].outcome.label : `${ok.length} series combined`;
        const combinedDetail = single ? ok[0].outcome.detail : ok.map((entry) => `${entry.seriesLabel}: ${entry.outcome.label.toLowerCase()}`).join(' · ');
        document.message = combinedDetail;

        const first = ok[0].outcome.chart;
        if (first.chartType === 'pie' || first.chartType === 'scatter' || first.chartType === 'histogram') {
            applyState({
                chart: first,
                mode: single ? ok[0].outcome.mode : 'combined',
                label: combinedLabel,
                detail: combinedDetail,
                rows: totalRows,
            });
            return;
        }
        const combined = combineBarLineSeries(
            ok.map((entry) => ({
                label: entry.seriesLabel,
                chart: entry.outcome.chart as BarLineChart,
            })),
        );
        applyState({
            chart: combined,
            mode: single ? ok[0].outcome.mode : 'combined',
            label: combinedLabel,
            detail: combinedDetail,
            rows: totalRows,
        });
    }

    function applyState(next: RenderState) {
        renderState = next;
        if (next.chart) chart?.setOption(toOption(next.chart), true);
        else chart?.clear();
    }

    function tableVisualQuery(object: string, config: ChartSeriesConfig): VisualQuery {
        const aggregate = config.aggregate as Exclude<VisualQueryAggregate, ''>;
        const columns = [
            { table: 't1', column: config.role.x, alias: '__x' },
            ...(config.role.series ? [{ table: 't1', column: config.role.series, alias: '__series' }] : []),
            { table: 't1', column: config.role.y, aggregate, alias: '__value' },
        ];
        const groupBy = [{ table: 't1', column: config.role.x }, ...(config.role.series ? [{ table: 't1', column: config.role.series }] : [])];
        return {
            tables: [{ object, alias: 't1' }],
            joins: [],
            columns,
            filters: [],
            groupBy,
            having: [],
            orderBy: [{ table: 't1', column: config.role.x, descending: false }],
            limit: 0,
        };
    }

    function fromPushdownRows(resultRows: Row[], config: ChartSeriesConfig): AggregatedChart {
        if (config.chartType === 'pie') {
            const slices = resultRows.map((row) => ({ name: label(row.__x), value: numericValue(row.__value) ?? 0 })).sort((a, b) => b.value - a.value);
            return { chartType: 'pie', slices: slices.slice(0, config.pieLimit) };
        }
        const categories = Array.from(new Set(resultRows.map((row) => label(row.__x))));
        const grouped = new Map<string, Map<string, number>>();
        for (const row of resultRows) {
            const name = config.role.series ? label(row.__series) : 'Series';
            const values = grouped.get(name) ?? new Map<string, number>();
            values.set(label(row.__x), numericValue(row.__value) ?? 0);
            grouped.set(name, values);
        }
        return {
            chartType: config.chartType === 'line' ? 'line' : 'bar',
            categories,
            series: Array.from(grouped.entries()).map(([name, values]) => ({
                name,
                data: categories.map((category) => values.get(category) ?? null),
            })),
        };
    }

    function paletteFor(styles: CSSStyleDeclaration) {
        const accent = styles.getPropertyValue('--accent').trim();
        const success = styles.getPropertyValue('--success').trim();
        const error = styles.getPropertyValue('--error').trim();
        const muted = styles.getPropertyValue('--muted').trim();
        const raised = styles.getPropertyValue('--raised').trim();
        if (paletteMode === 'cool') return [accent, '#6f9fd8', '#7aa891', '#8f85c9', muted, raised];
        if (paletteMode === 'warm') return [accent, '#d48a57', '#c7a45d', '#b86f71', error, raised];
        if (paletteMode === 'mono') return [accent, muted, raised, '#7f8794', '#9ba3ad', '#606975'];
        return [accent, success, '#7f8fb3', '#b78364', error, '#8aa078', '#b68ab8', raised];
    }

    function exportBackgroundColor(styles: CSSStyleDeclaration) {
        if (exportBackground === 'transparent') return 'transparent';
        if (exportBackground === 'surface') return styles.getPropertyValue('--surface').trim();
        return '#ffffff';
    }

    function legendOption(muted: string) {
        const base = {
            show: showLegend,
            textStyle: { color: muted, fontSize: labelFontSize },
            type: 'scroll' as const,
        };
        if (!showLegend) return base;
        if (legendPosition === 'right') {
            return { ...base, orient: 'vertical' as const, right: 10, top: 36, bottom: 20 };
        }
        if (legendPosition === 'left') {
            return { ...base, orient: 'vertical' as const, left: 10, top: 36, bottom: 20 };
        }
        if (legendPosition === 'bottom') return { ...base, left: 'center' as const, bottom: 8 };
        return { ...base, left: 'center' as const, top: 6 };
    }

    function cartesianGrid() {
        return {
            left: gridLeft,
            right: showLegend && (legendPosition === 'right' || legendPosition === 'left') ? Math.max(gridRight, 92) : gridRight,
            top: showLegend && legendPosition === 'top' ? Math.max(gridTop, 44) : gridTop,
            bottom: showDataZoom ? Math.max(gridBottom, 62) : gridBottom,
            containLabel: true,
        };
    }

    function cartesianZoom() {
        if (!showDataZoom) return undefined;
        return [{ type: 'inside' as const }, { type: 'slider' as const, height: 18, bottom: 8 }];
    }

    function axisLabelOption(muted: string) {
        return {
            show: showAxisLabels,
            color: muted,
            fontSize: axisFontSize,
            rotate: axisRotate,
        };
    }

    function toOption(aggregated: AggregatedChart): EChartsOption {
        const styles = getComputedStyle(window.document.documentElement);
        const text = styles.getPropertyValue('--text').trim();
        const muted = styles.getPropertyValue('--muted').trim();
        const border = styles.getPropertyValue('--border').trim();
        const palette = paletteFor(styles);
        const trigger: 'item' | 'axis' = aggregated.chartType === 'pie' ? 'item' : 'axis';
        const base = {
            animation: animationEnabled,
            animationDuration,
            backgroundColor: 'transparent',
            color: palette,
            textStyle: { color: text, fontFamily: 'var(--ui-font)' },
            tooltip: {
                show: showTooltip,
                trigger,
                backgroundColor: styles.getPropertyValue('--surface').trim(),
                borderColor: border,
                textStyle: { color: text },
            },
            legend: legendOption(muted),
        };
        if (aggregated.chartType === 'pie') {
            return {
                ...base,
                series: [
                    {
                        name: activeSeries?.label ?? seriesList[0]?.label ?? 'Series',
                        type: 'pie',
                        radius: [`${pieInnerRadius}%`, `${pieOuterRadius}%`],
                        center: ['50%', legendPosition === 'bottom' ? '46%' : '52%'],
                        data: aggregated.slices,
                        label: { show: showLabels, color: text, fontSize: labelFontSize },
                    },
                ],
            };
        }
        if (aggregated.chartType === 'scatter') {
            return {
                ...base,
                dataZoom: cartesianZoom(),
                grid: cartesianGrid(),
                xAxis: {
                    type: 'value',
                    axisLabel: axisLabelOption(muted),
                    splitLine: { show: showGridLines, lineStyle: { color: border } },
                },
                yAxis: {
                    type: 'value',
                    axisLabel: axisLabelOption(muted),
                    splitLine: { show: showGridLines, lineStyle: { color: border } },
                },
                series: aggregated.series.map((item) => ({
                    name: item.name,
                    type: 'scatter',
                    symbolSize: pointSize,
                    label: { show: showLabels, color: text, fontSize: labelFontSize },
                    data: item.data,
                })),
            };
        }
        if (aggregated.chartType === 'histogram') {
            return {
                ...base,
                dataZoom: cartesianZoom(),
                grid: cartesianGrid(),
                xAxis: {
                    type: 'category',
                    data: aggregated.bins.map((bin) => bin.label),
                    axisLabel: axisLabelOption(muted),
                    axisLine: { lineStyle: { color: border } },
                },
                yAxis: {
                    type: 'value',
                    axisLabel: axisLabelOption(muted),
                    splitLine: { show: showGridLines, lineStyle: { color: border } },
                },
                series: [
                    {
                        name: activeSeries?.label ?? seriesList[0]?.label ?? 'Series',
                        type: 'bar',
                        barWidth: `${barWidth}%`,
                        label: { show: showLabels, color: text, fontSize: labelFontSize },
                        data: aggregated.bins.map((bin) => bin.count),
                    },
                ],
            };
        }
        return {
            ...base,
            dataZoom: cartesianZoom(),
            grid: cartesianGrid(),
            xAxis: {
                type: 'category',
                data: aggregated.categories,
                axisLabel: axisLabelOption(muted),
                axisLine: { lineStyle: { color: border } },
            },
            yAxis: {
                type: 'value',
                axisLabel: axisLabelOption(muted),
                splitLine: { show: showGridLines, lineStyle: { color: border } },
            },
            series: aggregated.series.map((item) => ({
                name: item.name,
                type: aggregated.chartType,
                barWidth: aggregated.chartType === 'bar' ? `${barWidth}%` : undefined,
                smooth: aggregated.chartType === 'line' ? lineSmooth : undefined,
                symbolSize: pointSize,
                lineStyle: { width: lineWidth },
                label: { show: showLabels, color: text, fontSize: labelFontSize },
                data: item.data,
            })),
        };
    }

    function chartFileName(extension: string) {
        return `${document.title.toLowerCase().replace(/[^a-z0-9]+/g, '-') || 'chart'}.${extension}`;
    }

    async function exportPNG() {
        if (!chart) return;
        const styles = getComputedStyle(window.document.documentElement);
        const url = chart.getDataURL({
            type: 'png',
            pixelRatio: 2,
            backgroundColor: exportBackgroundColor(styles),
        });
        if (await workspace.saveBlobWithPicker(dataURLToBlob(url), chartFileName('png'), 'PNG image', 'png')) {
            workspace.notify('Exported chart as PNG.');
        }
    }

    // ECharts' PNG data URL (getDataURL) is always base64; its SVG one
    // (getSvgDataURL, via zrender's SVGPainter.toDataURL(base64) called with
    // no argument) is percent-encoded text instead — decoding either one the
    // other's way corrupts the file, so branch on which encoding is present.
    function dataURLToBlob(url: string) {
        const [meta, data] = url.split(',');
        const mime = meta.match(/data:(.*?);/)?.[1] ?? 'image/png';
        if (meta.includes(';base64')) {
            const binary = atob(data);
            const bytes = new Uint8Array(binary.length);
            for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
            return new Blob([bytes], { type: mime });
        }
        return new Blob([decodeURIComponent(data)], { type: mime });
    }

    // exportSVG renders the current option through a throwaway, off-screen
    // SVG-mode ECharts instance rather than switching the visible chart's own
    // renderer: getSvgDataURL() only produces real SVG markup for a chart
    // whose zrender painter is already the SVG one (a canvas-mode instance's
    // getSvgDataURL() would not return SVG at all), and the visible chart
    // stays canvas-rendered for its normal interactive performance.
    async function exportSVG() {
        if (!renderState.chart || !chartEl) return;
        const echarts = await import('echarts');
        const hidden = window.document.createElement('div');
        hidden.style.position = 'fixed';
        hidden.style.left = '-9999px';
        hidden.style.width = `${chartEl.clientWidth}px`;
        hidden.style.height = `${chartEl.clientHeight}px`;
        window.document.body.appendChild(hidden);
        const svgChart = echarts.init(hidden, undefined, { renderer: 'svg' });
        svgChart.setOption(toOption(renderState.chart));
        const url = svgChart.getSvgDataURL();
        svgChart.dispose();
        hidden.remove();
        if (await workspace.saveBlobWithPicker(dataURLToBlob(url), chartFileName('svg'), 'SVG image', 'svg')) {
            workspace.notify('Exported chart as SVG.');
        }
    }
</script>

<section class="chart-document" aria-label="Chart document" style={chartViewStyle}>
    {#if activeSource && activeSeries}<div class="chart-preview" class:view-open={showViewInspector}>
            <div class="chart-canvas-panel">
                <div bind:this={chartEl} class="chart-canvas" role="img" aria-label={`${activeSeries.chartType} chart: ${renderState.label} — ${renderState.detail}`}></div>
            </div>
            <aside class="chart-config-panel">
                <span class="eyebrow">SERIES</span>
                <div class="chart-series-tabs" role="group" aria-label="Chart series">
                    {#each seriesList as item (item.id)}
                        <div class="chart-series-tab" class:active={item.id === activeSeries?.id}>
                            <button
                                type="button"
                                class="chart-series-select"
                                aria-current={item.id === activeSeries?.id}
                                onclick={() => (activeSeriesId = item.id)}
                                title={item.source.sourceTitle}
                            >
                                {item.label}
                            </button>
                            <Button icon="refresh" label={`Refresh series ${item.label}`} onclick={() => refreshSeries(item)} />
                            {#if seriesList.length > 1}<Button
                                    icon="close"
                                    label={`Remove series ${item.label}`}
                                    onclick={() => workspace.removeChartSeries(document, item.id)}
                                />{/if}
                        </div>
                    {/each}
                    {#if canAddSeries}<Button icon="plus" variant="outline" label="Add series" onclick={openAddSeries}>Add series</Button>{/if}
                </div>
                <h3>{activeSeries.label}</h3>
                <div class="chart-mode" class:pushdown={renderState.mode === 'pushdown'} role="status" aria-live="polite">
                    <strong>{renderState.label}</strong>
                    <span>{renderState.detail}</span>
                </div>
                <div class="chart-view-wrap">
                    <Button icon="settings" variant="outline" label="Chart view options" active={showViewInspector} onclick={() => (showViewInspector = !showViewInspector)}>
                        View
                    </Button>
                </div>
                <dl>
                    <div>
                        <dt>Source</dt>
                        <dd>{activeSource.sourceTitle}</dd>
                    </div>
                    <div>
                        <dt>{renderState.mode === 'pushdown' ? 'Groups' : 'Rows'}</dt>
                        <dd>{renderState.rows}</dd>
                    </div>
                    <div>
                        <dt>Columns</dt>
                        <dd>{activeSource.columns.length}</dd>
                    </div>
                </dl>
                <Select value={chartType} options={chartTypeOptions} label="Chart type" onchange={(value) => workspace.setChartType(document, value as ChartType)} />
                <Select
                    value={activeSeries.role.x}
                    options={columnOptions}
                    label={activeSeries.chartType === 'pie' ? 'Label field' : activeSeries.chartType === 'histogram' ? 'Value field' : 'X field'}
                    onchange={(value) => updateRole('x', value)}
                />
                {#if activeSeries.chartType !== 'histogram'}<Select
                        value={activeSeries.role.y}
                        options={columnOptions}
                        label={activeSeries.chartType === 'pie' ? 'Value field' : 'Y field'}
                        onchange={(value) => updateRole('y', value)}
                    />{/if}
                {#if activeSeries.chartType === 'histogram'}<!-- A histogram bins one field's raw values; it has no aggregate/series/color role. -->
                {:else if activeSeries.chartType === 'scatter'}<Select
                        value={activeSeries.role.color ?? ''}
                        options={optionalColumnOptions}
                        label="Color field"
                        onchange={(value) => updateRole('color', value)}
                    />{:else}<Select
                        value={activeSeries.aggregate}
                        options={aggregateOptions}
                        label="Aggregate"
                        onchange={(value) => updateSeries({ aggregate: value as ChartAggregate })}
                    />
                    <Select value={activeSeries.role.series ?? ''} options={optionalColumnOptions} label="Series field" onchange={(value) => updateRole('series', value)} />{/if}
                {#if activeSeries.chartType === 'pie'}<label>
                        <span>Slice cap</span>
                        <input
                            aria-label="Pie slice cap"
                            type="number"
                            min="1"
                            max="30"
                            value={activeSeries.pieLimit}
                            onchange={(event) => updateSeries({ pieLimit: Number(event.currentTarget.value) || 12 })}
                        />
                    </label>{/if}
                {#if activeSeries.chartType === 'histogram'}<label>
                        <span>Bin count</span>
                        <input
                            aria-label="Bin count"
                            type="number"
                            min="1"
                            max="100"
                            value={activeSeries.binCount}
                            onchange={(event) => updateSeries({ binCount: Number(event.currentTarget.value) || 10 })}
                        />
                    </label>{/if}
                <div class="chart-export-row">
                    <Button icon="download" variant="outline" onclick={exportPNG} label="Export chart as PNG">Export PNG</Button>
                    <Button icon="download" variant="outline" onclick={exportSVG} label="Export chart as SVG">Export SVG</Button>
                </div>
            </aside>
            {#if showViewInspector}<aside class="chart-view-inspector" aria-label="Chart view inspector">
                    <div class="chart-view-inspector-head">
                        <div>
                            <span class="eyebrow">VIEW</span>
                            <h3>Chart appearance</h3>
                        </div>
                        <Button icon="close" label="Close chart view inspector" onclick={() => (showViewInspector = false)} />
                    </div>
                    <div class="chart-view-section">
                        <h4>Behavior</h4>
                        <Input type="checkbox" bind:checked={animationEnabled} label="Animation" />
                        <Input type="checkbox" bind:checked={showTooltip} label="Tooltip" />
                        <Input type="checkbox" bind:checked={showDataZoom} disabled={activeSeries.chartType === 'pie'} label="Data zoom" />
                        <Input type="range" min="0" max="1200" step="50" bind:value={animationDuration} label="Animation speed" suffix="ms" />
                    </div>
                    <div class="chart-view-section">
                        <h4>Canvas</h4>
                        <Input type="range" min="6" max="40" bind:value={canvasPadding} label="Padding" suffix="px" />
                        <Input type="range" min="240" max="720" step="20" bind:value={canvasMinHeight} label="Min height" suffix="px" />
                        <Select bind:value={exportBackground} options={exportBackgroundOptions} label="PNG background" />
                    </div>
                    <div class="chart-view-section">
                        <h4>Legend</h4>
                        <Input type="checkbox" bind:checked={showLegend} label="Show legend" />
                        <Select bind:value={legendPosition} options={legendPositionOptions} label="Position" />
                        <Select bind:value={paletteMode} options={paletteOptions} label="Palette" />
                    </div>
                    <div class="chart-view-section">
                        <h4>Grid and axes</h4>
                        <Input type="checkbox" bind:checked={showAxisLabels} disabled={activeSeries.chartType === 'pie'} label="Axis labels" />
                        <Input type="checkbox" bind:checked={showGridLines} disabled={activeSeries.chartType === 'pie'} label="Grid lines" />
                        <Input type="range" min="20" max="120" bind:value={gridLeft} disabled={activeSeries.chartType === 'pie'} label="Left" suffix="px" />
                        <Input type="range" min="8" max="120" bind:value={gridRight} disabled={activeSeries.chartType === 'pie'} label="Right" suffix="px" />
                        <Input type="range" min="18" max="120" bind:value={gridTop} disabled={activeSeries.chartType === 'pie'} label="Top" suffix="px" />
                        <Input type="range" min="24" max="120" bind:value={gridBottom} disabled={activeSeries.chartType === 'pie'} label="Bottom" suffix="px" />
                        <Input type="range" min="9" max="16" bind:value={axisFontSize} disabled={activeSeries.chartType === 'pie'} label="Axis text" suffix="px" />
                        <Input type="range" min="-60" max="60" step="5" bind:value={axisRotate} disabled={activeSeries.chartType === 'pie'} label="Axis rotate" suffix="deg" />
                    </div>
                    <div class="chart-view-section">
                        <h4>Series</h4>
                        <Input type="checkbox" bind:checked={showLabels} label="Value labels" />
                        <Input type="range" min="9" max="18" bind:value={labelFontSize} label="Label text" suffix="px" />
                        <Input
                            type="range"
                            min="18"
                            max="100"
                            bind:value={barWidth}
                            disabled={activeSeries.chartType === 'line' || activeSeries.chartType === 'scatter' || activeSeries.chartType === 'pie'}
                            label="Bar width"
                            suffix="%"
                        />
                        <Input type="checkbox" bind:checked={lineSmooth} disabled={activeSeries.chartType !== 'line'} label="Smooth line" />
                        <Input type="range" min="1" max="8" bind:value={lineWidth} disabled={activeSeries.chartType !== 'line'} label="Line width" suffix="px" />
                        <Input
                            type="range"
                            min="2"
                            max="24"
                            bind:value={pointSize}
                            disabled={activeSeries.chartType === 'bar' || activeSeries.chartType === 'histogram' || activeSeries.chartType === 'pie'}
                            label="Point size"
                            suffix="px"
                        />
                        <Input type="range" min="0" max="70" bind:value={pieInnerRadius} disabled={activeSeries.chartType !== 'pie'} label="Pie hole" suffix="%" />
                        <Input type="range" min="45" max="92" bind:value={pieOuterRadius} disabled={activeSeries.chartType !== 'pie'} label="Pie size" suffix="%" />
                    </div>
                </aside>{/if}
        </div>{:else}<div class="empty-state">
            <span class="empty-icon"><Icon name="chart" size={28} /></span>
            <h3>No chart source</h3>
            <p>Visualize a query result or table to seed this chart document.</p>
        </div>{/if}
</section>

{#if showAddSeries}<Modal
        title="Add series"
        subtitle="Pick a table, collection, or query result from any connected session. It resolves independently and is combined by the x-axis when the chart renders."
        onclose={() => (showAddSeries = false)}
    >
        <div class="add-series-body">
            <Select
                bind:value={addSeriesSessionId}
                options={workspace.sessions.filter((s) => s.connectionState === 'connected').map((candidate) => ({ value: candidate.id, label: candidate.profile.name }))}
                label="Session"
            />
            {#if addSeriesTables.length}<div class="add-series-group">
                    <span class="eyebrow">TABLES &amp; COLLECTIONS</span>
                    <ul>
                        {#each addSeriesTables as table}<li>
                                <span>{table.name}</span>
                                <Button icon="plus" variant="outline" label={`Add ${table.name} as a series`} onclick={() => addSeriesFromTable(table.name)}>Add</Button>
                            </li>{/each}
                    </ul>
                </div>{/if}
            {#if addSeriesQueries.length}<div class="add-series-group">
                    <span class="eyebrow">OPEN QUERY RESULTS</span>
                    <ul>
                        {#each addSeriesQueries as doc}<li>
                                <span>{doc.title}</span>
                                <Button icon="plus" variant="outline" label={`Add ${doc.title} as a series`} onclick={() => addSeriesFromQuery(doc)}>Add</Button>
                            </li>{/each}
                    </ul>
                </div>{/if}
            {#if !addSeriesTables.length && !addSeriesQueries.length}<p class="add-series-empty">
                    No chartable tables, collections, or loaded query results in this session yet.
                </p>{/if}
        </div>
    </Modal>{/if}
