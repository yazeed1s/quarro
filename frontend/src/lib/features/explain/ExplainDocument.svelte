<script lang="ts">
    import type { Document, ExplainMode, ExplainNode, ExplainView, Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import { copy } from '../results/format';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import Modal from '../../components/Modal.svelte';
    import ResizeHandle from '../../components/ResizeHandle.svelte';
    import ExplainGraph from './ExplainGraph.svelte';
    import { bufferTotal, diagramMetricValue, flattenVisible, type DiagramMetric, type ExplainRow } from './layout';

    let { session, document }: { session: Session; document: Document } = $props();

    let panelWidth = $state(320);
    let confirmAnalyze = $state(false);
    let tableCollapsed = $state<Set<string>>(new Set());
    let diagramMetric = $state<DiagramMetric>('time');
    let noAnalyzeSupport = $derived(!session.capabilities.supportsAnalyze);
    let plan = $derived(document.explainPlan);
    let selected = $derived(plan ? findNode(plan.root, document.explainSelectedNodeId) : undefined);

    function findNode(node: ExplainNode | null, id: string): ExplainNode | undefined {
        if (!node || !id) return undefined;
        if (node.id === id) return node;
        for (const child of node.children) {
            const found: ExplainNode | undefined = findNode(child, id);
            if (found) return found;
        }
        return undefined;
    }

    function selectNode(id: string) {
        document.explainSelectedNodeId = id === document.explainSelectedNodeId ? '' : id;
    }

    function toggleCollapse(id: string) {
        const next = new Set(tableCollapsed);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        tableCollapsed = next;
    }

    function requestMode(mode: ExplainMode) {
        if (mode === document.explainMode) return;
        if (mode === 'analyze') {
            if (noAnalyzeSupport) return;
            if (workspace.looksMutating(document.explainSql)) {
                confirmAnalyze = true;
                return;
            }
        }
        document.explainMode = mode;
        workspace.runExplain(session, document);
    }

    function confirmRunAnalyze() {
        confirmAnalyze = false;
        document.explainMode = 'analyze';
        workspace.runExplain(session, document);
    }

    const viewOptions: { value: ExplainView; label: string }[] = [
        { value: 'graph', label: 'Graph' },
        { value: 'table', label: 'Table' },
        { value: 'diagram', label: 'Diagram' },
        { value: 'stats', label: 'Stats' },
        { value: 'raw', label: 'Raw' },
    ];
    const metricOptions: { value: DiagramMetric; label: string }[] = [
        { value: 'time', label: 'Time' },
        { value: 'rows', label: 'Rows' },
        { value: 'cost', label: 'Cost' },
        { value: 'buffers', label: 'Buffers' },
    ];
    function metricLabel(m: DiagramMetric): string {
        return metricOptions.find((o) => o.value === m)?.label ?? m;
    }
    function metricValueLabel(node: ExplainNode, m: DiagramMetric): string {
        const v = diagramMetricValue(node, m);
        if (m === 'time') return `${v.toFixed(2)} ms`;
        if (m === 'cost') return v.toFixed(2);
        if (m === 'buffers') return `${v} blocks`;
        return String(v);
    }

    // Table View: every row, respecting per-row collapse state.
    let tableRows = $derived<ExplainRow[]>(flattenVisible(plan?.root ?? null, tableCollapsed));
    // Diagram View: every row, never collapsible (a flat, always-complete
    // flame-graph-lite list, per the plan's own "one horizontal bar per
    // node" design), still depth-indented to preserve parent context.
    let diagramRows = $derived<ExplainRow[]>(flattenVisible(plan?.root ?? null, new Set()));
    let diagramMax = $derived(diagramRows.length ? Math.max(0, ...diagramRows.map((r) => diagramMetricValue(r.node, diagramMetric))) : 0);

    let operationEntries = $derived(Object.entries(plan?.stats.operationCounts ?? {}).sort((a, b) => b[1] - a[1]));
    let maxOpCount = $derived(Math.max(1, ...operationEntries.map(([, c]) => c)));
</script>

<div class="explain-document">
    <div class="explain-toolbar">
        <div class="segmented explain-mode-toggle" role="group" aria-label="Explain mode">
            <button class:active={document.explainMode === 'estimate'} onclick={() => requestMode('estimate')}> Estimate </button>
            <button
                class:active={document.explainMode === 'analyze'}
                disabled={noAnalyzeSupport}
                title={noAnalyzeSupport ? 'This connection has no execution-analysis mode.' : ''}
                onclick={() => requestMode('analyze')}
            >
                Analyze
            </button>
        </div>
        <code class="explain-sql" title={document.explainSql}>{document.explainSql}</code>
        <Button icon="refresh" label="Re-run explain" disabled={document.status === 'running'} onclick={() => workspace.runExplain(session, document)}>Re-run</Button>
    </div>
    <div class="explain-subtoolbar">
        <div class="segmented explain-view-toggle" role="group" aria-label="Explain view">
            {#each viewOptions as opt (opt.value)}
                <button class:active={document.explainView === opt.value} onclick={() => (document.explainView = opt.value)}>
                    {opt.label}
                </button>
            {/each}
        </div>
    </div>
    {#if document.status === 'running'}
        <div class="explain-status">
            <Icon name="refresh" size={14} />Running…
        </div>
    {:else if document.status === 'error'}
        <div class="explain-status explain-status-error">
            <Icon name="alert" size={14} />{document.message}
        </div>
    {:else if plan?.warnings.length}
        <div class="explain-status">
            <Icon name="info" size={14} />{plan.warnings.join(' ')}
        </div>
    {/if}
    <div class="explain-body">
        {#if document.explainView === 'graph'}
            <div class="explain-canvas">
                <ExplainGraph root={plan?.root ?? null} analyzed={plan?.analyzed ?? false} selectedNodeId={document.explainSelectedNodeId} onselect={selectNode} />
            </div>
        {:else if document.explainView === 'table'}
            <div class="explain-view-scroll">
                {#if !plan?.root}
                    <div class="explain-empty"><p>No plan yet.</p></div>
                {:else}
                    <table class="metadata-table explain-plan-table">
                        <thead>
                            <tr>
                                <th></th>
                                <th>Operation</th>
                                <th>Relation</th>
                                <th>Rows (est → actual)</th>
                                <th>Cost</th>
                                <th>Time (ms)</th>
                                <th>Loops</th>
                            </tr>
                        </thead>
                        <tbody>
                            {#each tableRows as row (row.node.id)}
                                <tr class:explain-row-selected={row.node.id === document.explainSelectedNodeId} onclick={() => selectNode(row.node.id)}>
                                    <td class="explain-plan-table-toggle" style:padding-left={`${row.depth * 16}px`}>
                                        {#if row.node.children.length}<button
                                                class="explain-collapse-toggle"
                                                aria-label={tableCollapsed.has(row.node.id) ? 'Expand children' : 'Collapse children'}
                                                onclick={(e) => {
                                                    e.stopPropagation();
                                                    toggleCollapse(row.node.id);
                                                }}
                                            >
                                                <Icon name={tableCollapsed.has(row.node.id) ? 'chevronDown' : 'chevronUp'} size={12} />
                                            </button>{/if}
                                    </td>
                                    <td>{row.node.operation}</td>
                                    <td>{row.node.relation ?? '-'}</td>
                                    <td>
                                        {row.node.estimatedRows ?? '-'}{row.node.actualRows !== undefined ? ` → ${row.node.actualRows}` : ''}
                                    </td>
                                    <td>{row.node.estimatedCost?.toFixed(2) ?? '-'}</td>
                                    <td>{row.node.inclusiveTimeMs?.toFixed(3) ?? '-'}</td>
                                    <td>{row.node.loops}</td>
                                </tr>
                            {/each}
                        </tbody>
                    </table>
                {/if}
            </div>
        {:else if document.explainView === 'diagram'}
            <div class="explain-view-scroll explain-diagram">
                {#if !plan?.root}
                    <div class="explain-empty"><p>No plan yet.</p></div>
                {:else}
                    <div class="segmented explain-diagram-metric" role="group" aria-label="Diagram metric">
                        {#each metricOptions as opt (opt.value)}
                            <button class:active={diagramMetric === opt.value} onclick={() => (diagramMetric = opt.value)}>
                                {opt.label}
                            </button>
                        {/each}
                    </div>
                    {#if diagramMax <= 0}
                        <p class="explain-empty-note">
                            No {metricLabel(diagramMetric).toLowerCase()} data in this plan.
                        </p>
                    {/if}
                    <div class="explain-diagram-rows">
                        {#each diagramRows as row (row.node.id)}
                            <button
                                class="explain-diagram-row"
                                class:explain-row-selected={row.node.id === document.explainSelectedNodeId}
                                style:padding-left={`${row.depth * 14 + 10}px`}
                                onclick={() => selectNode(row.node.id)}
                            >
                                <span class="explain-diagram-label">
                                    {row.node.operation}{row.node.relation ? ` · ${row.node.relation}` : ''}
                                </span>
                                <span class="explain-diagram-bar-track">
                                    <span class="explain-diagram-bar" style:width={`${diagramMax > 0 ? (diagramMetricValue(row.node, diagramMetric) / diagramMax) * 100 : 0}%`}
                                    ></span>
                                </span>
                                <span class="explain-diagram-value">
                                    {metricValueLabel(row.node, diagramMetric)}
                                </span>
                            </button>
                        {/each}
                    </div>
                {/if}
            </div>
        {:else if document.explainView === 'stats'}
            <div class="explain-view-scroll explain-stats">
                {#if !plan}
                    <div class="explain-empty"><p>No plan yet.</p></div>
                {:else}
                    <div class="explain-stats-grid">
                        <div class="explain-stat-card">
                            <span class="explain-stat-label">Nodes</span>
                            <strong>{plan.stats.nodeCount}</strong>
                        </div>
                        {#if plan.stats.totalCost !== undefined}
                            <div class="explain-stat-card">
                                <span class="explain-stat-label">Total cost</span>
                                <strong>{plan.stats.totalCost.toFixed(2)}</strong>
                            </div>
                        {/if}
                        {#if plan.stats.totalTimeMs !== undefined}
                            <div class="explain-stat-card">
                                <span class="explain-stat-label">Total time</span>
                                <strong>{plan.stats.totalTimeMs.toFixed(2)} ms</strong>
                            </div>
                        {/if}
                    </div>
                    <section>
                        <h4>Operations</h4>
                        <div class="explain-stat-bars">
                            {#each operationEntries as [op, count] (op)}
                                <div class="explain-stat-bar-row">
                                    <span class="explain-stat-bar-name">{op}</span>
                                    <span class="explain-stat-bar-track">
                                        <span class="explain-stat-bar" style:width={`${(count / maxOpCount) * 100}%`}></span>
                                    </span>
                                    <span>{count}</span>
                                </div>
                            {/each}
                        </div>
                    </section>
                    <section>
                        <h4>
                            Relations scanned <span class="count">{plan.stats.relationsScanned.length}</span>
                        </h4>
                        <div class="explain-tag-list">
                            {#each plan.stats.relationsScanned as r (r)}<span class="explain-tag">
                                    {r}
                                </span>{/each}
                            {#if !plan.stats.relationsScanned.length}<p class="explain-empty-note">None recorded.</p>{/if}
                        </div>
                    </section>
                    <section>
                        <h4>
                            Indexes used <span class="count">{plan.stats.indexesUsed.length}</span>
                        </h4>
                        <div class="explain-tag-list">
                            {#each plan.stats.indexesUsed as ix (ix)}<span class="explain-tag">{ix}</span>{/each}
                            {#if !plan.stats.indexesUsed.length}<p class="explain-empty-note">None recorded.</p>{/if}
                        </div>
                    </section>
                {/if}
            </div>
        {:else if document.explainView === 'raw'}
            <div class="explain-view-scroll explain-raw">
                {#if !plan}
                    <div class="explain-empty"><p>No plan yet.</p></div>
                {:else}
                    <div class="explain-raw-meta">
                        <span>
                            {plan.rawFormat.toUpperCase()}{plan.engineVersion ? ` · ${plan.engine} ${plan.engineVersion}` : ` · ${plan.engine}`}
                        </span>
                        <Button icon="copy" label="Copy raw output" onclick={() => copy(plan!.rawOutput, (s) => workspace.notify(s))}>Copy</Button>
                    </div>
                    <pre class="explain-raw-pre">{plan.rawOutput}</pre>
                {/if}
            </div>
        {/if}
        {#if selected && document.explainView !== 'stats' && document.explainView !== 'raw'}
            <ResizeHandle value={panelWidth} min={240} max={520} orientation="vertical" reverse label="Node detail panel width" onchange={(v) => (panelWidth = v)} />
            <aside class="explain-detail" style:width={`${panelWidth}px`} aria-label="Plan node details">
                <header>
                    <div>
                        <span class="eyebrow">NODE DETAIL</span>
                        <h3>{selected.operation}</h3>
                    </div>
                    <div class="explain-detail-actions">
                        {#if selected.relation}<Button icon="arrow" label="Show in editor" onclick={() => workspace.revealExplainNode(session, document, selected)}>
                                Show in editor
                            </Button>{/if}
                        <Button icon="close" label="Close" onclick={() => (document.explainSelectedNodeId = '')} />
                    </div>
                </header>
                <dl>
                    {#if selected.relation}<dt>Relation</dt>
                        <dd>{selected.relation}</dd>{/if}
                    {#if selected.alias}<dt>Alias</dt>
                        <dd>{selected.alias}</dd>{/if}
                    {#if selected.filter}<dt>Filter</dt>
                        <dd><code>{selected.filter}</code></dd>{/if}
                    {#if selected.indexCondition}<dt>Index condition</dt>
                        <dd><code>{selected.indexCondition}</code></dd>{/if}
                    <dt>Rows</dt>
                    <dd>
                        {selected.estimatedRows ?? '-'} estimated
                        {#if selected.actualRows !== undefined}· {selected.actualRows} actual{/if}
                    </dd>
                    <dt>Loops</dt>
                    <dd>{selected.loops}</dd>
                    {#if selected.estimatedCost !== undefined}<dt>Cost</dt>
                        <dd>
                            {selected.estimatedCost.toFixed(2)} total
                            {#if selected.exclusiveCost !== undefined}· {selected.exclusiveCost.toFixed(2)} exclusive{/if}
                        </dd>{/if}
                    {#if selected.inclusiveTimeMs !== undefined}<dt>Time</dt>
                        <dd>
                            {selected.inclusiveTimeMs.toFixed(3)} ms inclusive
                            {#if selected.exclusiveTimeMs !== undefined}· {selected.exclusiveTimeMs.toFixed(3)} ms exclusive{/if}
                        </dd>{/if}
                    {#if bufferTotal(selected) > 0}<dt>Buffers</dt>
                        <dd>{bufferTotal(selected)} blocks</dd>{/if}
                    {#each Object.entries(selected.extra ?? {}) as [key, value] (key)}<dt>{key}</dt>
                        <dd>
                            {typeof value === 'object' ? JSON.stringify(value) : String(value)}
                        </dd>{/each}
                </dl>
                {#if selected.badges.length}<div class="explain-detail-badges">
                        {#each selected.badges as badge}<div class="explain-detail-badge sev-{badge.severity}">
                                <Icon name={badge.severity === 'warning' ? 'alert' : 'info'} size={12} />
                                <span>{badge.message}</span>
                            </div>{/each}
                    </div>{/if}
            </aside>
        {/if}
    </div>
</div>

{#if confirmAnalyze}
    <Modal title="Run this statement for real?" onclose={() => (confirmAnalyze = false)}>
        <p class="explain-confirm-text">This does not look like a read-only statement. EXPLAIN ANALYZE actually executes it, including its effects. This is not a preview.</p>
        <pre class="explain-confirm-sql">{document.explainSql}</pre>
        <div class="explain-confirm-actions">
            <Button label="Cancel" onclick={() => (confirmAnalyze = false)}>Cancel</Button>
            <Button variant="primary" label="Run it" onclick={confirmRunAnalyze}>Run it</Button>
        </div>
    </Modal>
{/if}
