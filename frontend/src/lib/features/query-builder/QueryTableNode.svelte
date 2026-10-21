<script lang="ts">
    import { Handle, Position } from '@xyflow/svelte';
    import { objectLabel, type SchemaTable } from '../../state/types';
    import Icon from '../../components/Icon.svelte';
    import Input from '../../components/Input.svelte';
    import { COLUMN_DRAG_MIME } from './dnd';

    export interface QueryTableNodeData {
        table: SchemaTable;
        alias: string;
        selectedColumns: Set<string>;
        onToggleColumn: (column: string) => void;
        // Absent when the session's engine has no relational join concept
        // (see VisualQueryBuilder.svelte's supportsJoins).
        onJoinTo?: () => void;
        onRemove: () => void;
    }

    let { data, selected = false }: { data: QueryTableNodeData; selected?: boolean } = $props();
    let fkColumns = $derived(new Set(data.table.foreignKeys.map((fk) => fk.column)));

    // UI-only row selection for the drag gesture — separate from
    // `data.selectedColumns` (the actual SELECT list), since rows can be
    // selected purely to drag them onto Filters/Order By, not the columns
    // panel. Local per-node state, not persisted.
    let selectedRows = $state<Set<string>>(new Set());
    let anchorIndex = $state<number | null>(null);
    let columnNames = $derived(data.table.columns.map((c) => c.name));

    // Drag-preview state: every row actually being dragged (the whole
    // multi-selection, if the drag started on a selected row) gets a
    // `.dragging` class while the gesture is in flight.
    let draggingColumns = $state<Set<string> | null>(null);

    // Per-row `draggable` toggle for the row-drag gesture (Phase B). A
    // `dragstart` event's `target` is always the nearest `draggable="true"`
    // ancestor — here, the row — never the original element the gesture
    // started on, so `dragstart` alone can't tell a row-drag apart from a
    // join-drawing drag on a nested connector handle. The browser decides
    // whether to start a native drag by reading this attribute at mousedown
    // time, so flipping it off there (before the browser's own gesture
    // detection runs) and back on shortly after reliably exempts the handles
    // without disabling row dragging from anywhere else in the row.
    let rowDraggable = $state(true);
    function handleRowMouseDown(event: MouseEvent) {
        if (!(event.target as HTMLElement).closest('.query-handle')) return;
        rowDraggable = false;
        const restore = () => {
            rowDraggable = true;
            window.removeEventListener('mouseup', restore);
            window.removeEventListener('dragend', restore);
        };
        window.addEventListener('mouseup', restore);
        window.addEventListener('dragend', restore);
    }

    function handleRowClick(event: MouseEvent, column: string, index: number) {
        if ((event.target as HTMLElement).closest('.input-control')) return;
        if (event.shiftKey && anchorIndex !== null) {
            const [start, end] = [Math.min(anchorIndex, index), Math.max(anchorIndex, index)];
            selectedRows = new Set(columnNames.slice(start, end + 1));
        } else if (event.metaKey || event.ctrlKey) {
            const next = new Set(selectedRows);
            if (next.has(column)) next.delete(column);
            else next.add(column);
            selectedRows = next;
            anchorIndex = index;
        } else {
            selectedRows = new Set([column]);
            anchorIndex = index;
        }
    }
    function dragColumns(column: string): string[] {
        if (!selectedRows.has(column)) return [column];
        const ordered = columnNames.filter((name) => selectedRows.has(name));
        return ordered.length ? ordered : [column];
    }
    function handleRowDragStart(event: DragEvent, column: string) {
        // A gesture that starts on a connector dot is a join-drawing drag, not
        // a row drag — the row is still the nearest `draggable="true"`
        // ancestor, so the browser would otherwise hijack it into a native
        // HTML5 drag and starve xyflow's own pointer-based connection gesture
        // of the mousemove events it needs. Cancel the native drag in that case
        // and let the Handle's own mousedown handler take over instead.
        if ((event.target as HTMLElement).closest('.query-handle')) {
            event.preventDefault();
            return;
        }
        const columns = dragColumns(column);
        event.dataTransfer?.setData(COLUMN_DRAG_MIME, JSON.stringify({ table: data.alias, columns }));
        if (event.dataTransfer) event.dataTransfer.effectAllowed = 'copy';
        draggingColumns = new Set(columns);
    }
    function handleRowDragEnd() {
        draggingColumns = null;
    }
</script>

<div class="query-node" class:selected>
    <header>
        <div>
            <span>{data.alias}</span>
            <strong>{objectLabel(data.table.name)}</strong>
        </div>
        <button
            class="node-action nodrag nopan"
            aria-label={`Remove ${objectLabel(data.table.name)} from the query`}
            onclick={(event) => {
                event.stopPropagation();
                data.onRemove();
            }}
        >
            <Icon name="close" size={13} />
        </button>
    </header>
    <div class="query-node-columns">
        {#each data.table.columns as column, index}<div
                class="query-column nodrag nopan"
                class:row-selected={selectedRows.has(column.name)}
                class:dragging={draggingColumns?.has(column.name)}
                title={`${column.name} · ${column.type || 'value'}`}
                role="option"
                aria-label={`${column.name} row, drag to add to the query`}
                aria-selected={selectedRows.has(column.name)}
                tabindex="0"
                draggable={rowDraggable}
                onmousedown={handleRowMouseDown}
                ondragstart={(event) => handleRowDragStart(event, column.name)}
                ondragend={handleRowDragEnd}
                onclick={(event) => handleRowClick(event, column.name, index)}
                onkeydown={(event) => {
                    if (event.key !== 'Enter' && event.key !== ' ') return;
                    if ((event.target as HTMLElement).closest('.input-control')) return;
                    event.preventDefault();
                    handleRowClick(event as unknown as MouseEvent, column.name, index);
                }}
            >
                <Input type="checkbox" bind:checked={() => data.selectedColumns.has(column.name), () => data.onToggleColumn(column.name)} />
                <span class="column-markers">
                    {#if column.primaryKey}<Icon name="key" size={11} />{:else}<span></span>{/if}
                    {#if fkColumns.has(column.name)}<Icon name="link" size={11} />{/if}
                </span>
                <span class="column-name">{column.name}</span>
                <span class="column-type">{column.type || 'value'}</span>
                <Handle
                    class="query-handle"
                    type="target"
                    position={Position.Left}
                    id={column.name}
                    draggable={false}
                    title={`Drag from another column here to join on ${column.name}`}
                />
                <Handle
                    class="query-handle"
                    type="source"
                    position={Position.Right}
                    id={column.name}
                    draggable={false}
                    title={`Drag from ${column.name} to another table's column to join`}
                />
            </div>{/each}
    </div>
    {#if data.onJoinTo}<footer>
            <button
                class="nodrag nopan"
                onclick={(event) => {
                    event.stopPropagation();
                    data.onJoinTo?.();
                }}
            >
                <Icon name="link" size={12} />Join to…
            </button>
        </footer>{/if}
</div>
