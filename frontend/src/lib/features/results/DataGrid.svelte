<script lang="ts">
    import type { Column, Document, Row } from '../../state/types';
    import type { GridLayout } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import { effectiveRowHeight } from '../../state/appearance';
    import { display, copy, columnGlyph } from './format';
    import Icon from '../../components/Icon.svelte';
    let {
        rows,
        resultColumns,
        document,
        layout,
        oninspect,
        onsort,
        onedit,
    }: {
        rows: Row[];
        resultColumns: Column[];
        document: Document;
        layout: GridLayout;
        oninspect: () => void;
        onsort?: () => void;
        onedit?: (row: Row, column: string) => void;
    } = $props();
    let scroller: HTMLDivElement;
    let scrollTop = $state(0);
    let viewport = $state(350);
    let scrollLeft = $state(0);
    let viewportWidth = $state(1000);
    let cellColumn = $state(0);
    let liveColumns = $derived(resultColumns.length ? resultColumns.map((c) => c.name) : Object.keys(rows[0] ?? {}));
    let allColumnsOrdered = $derived.by(() => {
        if (!layout.order.length) return liveColumns;
        const known = new Set(liveColumns);
        const ordered = layout.order.filter((c) => known.has(c));
        const extra = liveColumns.filter((c) => !layout.order.includes(c));
        return [...ordered, ...extra];
    });
    let columns = $derived(allColumnsOrdered.filter((c) => !layout.hidden.includes(c)));
    function columnWidth(column: string) {
        return layout.widths[column] || (column === 'id' ? 72 : column === 'email' ? 245 : column.includes('_at') || column === 'last_login' ? 210 : 164);
    }
    let positions = $derived.by(() => {
        let x = 44;
        return columns.map((name) => {
            const left = x;
            x += columnWidth(name);
            return { name, left, width: columnWidth(name) };
        });
    });
    let firstColumn = $derived(
        Math.max(
            0,
            positions.findIndex((p) => p.left + p.width >= scrollLeft - 200),
        ),
    );
    let lastColumn = $derived.by(() => {
        const end = positions.findIndex((p) => p.left > scrollLeft + viewportWidth + 200);
        return end < 0 ? columns.length : end;
    });
    let visibleColumns = $derived(columns.slice(firstColumn, lastColumn));
    let totalWidth = $derived(positions.length ? positions[positions.length - 1].left + positions[positions.length - 1].width : 44);
    let beforeWidth = $derived(firstColumn > 0 ? positions[firstColumn].left - 44 : 0);
    let afterWidth = $derived(lastColumn < positions.length ? totalWidth - positions[lastColumn].left : 0);
    let declaredType = $derived(Object.fromEntries(resultColumns.map((c) => [c.name, c.declaredType])));
    let rowHeight = $derived(effectiveRowHeight(workspace.preferences));
    let start = $derived(Math.max(0, Math.floor(scrollTop / rowHeight) - 4));
    let end = $derived(Math.min(rows.length, start + Math.ceil(viewport / rowHeight) + 10));
    $effect(() => {
        const count = rows.length;
        if (scroller && scrollTop > Math.max(0, count * rowHeight - viewport)) {
            scroller.scrollTop = Math.max(0, count * rowHeight - viewport);
            scrollTop = scroller.scrollTop;
        }
    });
    function sort(column: string) {
        document.descending = document.sort === column ? !document.descending : false;
        document.sort = column;
        document.selected = -1;
        scrollTop = 0;
        if (scroller) scroller.scrollTop = 0;
        onsort?.();
    }
    function resize(event: PointerEvent, column: string) {
        event.preventDefault();
        event.stopPropagation();
        const target = event.currentTarget as HTMLElement;
        target.setPointerCapture(event.pointerId);
        const initial = layout.widths[column] || (column === 'id' ? 72 : column === 'email' ? 245 : column.includes('_at') || column === 'last_login' ? 210 : 164);
        const x = event.clientX;
        const move = (e: PointerEvent) => (layout.widths[column] = Math.max(72, Math.min(600, initial + e.clientX - x)));
        const done = () => {
            target.removeEventListener('pointermove', move);
            target.removeEventListener('pointerup', done);
        };
        target.addEventListener('pointermove', move);
        target.addEventListener('pointerup', done);
    }
    function navigate(e: KeyboardEvent) {
        if (e.target !== e.currentTarget) return;
        const moves: Record<string, number> = {
            ArrowDown: 1,
            ArrowUp: -1,
            PageDown: 10,
            PageUp: -10,
        };
        if (e.key in moves) {
            e.preventDefault();
            document.selected = Math.max(0, Math.min(rows.length - 1, document.selected + moves[e.key]));
            const top = document.selected * rowHeight;
            if (top < scroller.scrollTop || top > scroller.scrollTop + viewport - rowHeight * 2) scroller.scrollTop = top;
        } else if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
            e.preventDefault();
            cellColumn = Math.max(0, Math.min(columns.length - 1, cellColumn + (e.key === 'ArrowRight' ? 1 : -1)));
            document.selectedColumn = columns[cellColumn] ?? '';
            const position = positions[cellColumn];
            if (position && position.left < scroller.scrollLeft) scroller.scrollLeft = position.left;
            else if (position && position.left + position.width > scroller.scrollLeft + viewportWidth) scroller.scrollLeft = position.left + position.width - viewportWidth;
        } else if (e.key === 'Enter') oninspect();
        else if ((e.ctrlKey || e.metaKey) && e.key === 'c' && rows[document.selected]) {
            e.preventDefault();
            void copy(display(rows[document.selected][columns[cellColumn]] ?? '', workspace.preferences), (s) => workspace.notify(s));
        }
    }
</script>

<div class="grid-wrap">
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (The scroll region supports keyboard cell navigation and copy.) -->
    <div
        class="grid-scroll"
        bind:this={scroller}
        bind:clientHeight={viewport}
        bind:clientWidth={viewportWidth}
        onscroll={() => {
            scrollTop = scroller.scrollTop;
            scrollLeft = scroller.scrollLeft;
        }}
        tabindex="0"
        role="region"
        aria-label="Query result grid. Arrow keys select cells; Enter opens inspector; Control or Command C copies a cell."
        onkeydown={navigate}
    >
        <table
            class="data-grid"
            class:stripes={workspace.preferences.stripes}
            style:--row-height={`${rowHeight}px`}
            aria-rowcount={rows.length + 1}
            aria-colcount={columns.length + 1}
            style:width={`${totalWidth}px`}
        >
            <colgroup>
                <col style="width:44px" />
                {#if beforeWidth}<col style:width={`${beforeWidth}px`} />{/if}
                {#each visibleColumns as column}<col style:width={`${columnWidth(column)}px`} />{/each}
                {#if afterWidth}<col style:width={`${afterWidth}px`} />{/if}
            </colgroup>
            <thead>
                <tr>
                    <th class="row-number">#</th>
                    {#if beforeWidth}<th aria-hidden="true" class="column-spacer"></th>{/if}
                    {#each visibleColumns as column}<th aria-sort={document.sort === column ? (document.descending ? 'descending' : 'ascending') : 'none'}>
                            <button class="column-label" onclick={() => sort(column)}>
                                {#if column === 'id'}<Icon name="key" size={12} />{:else}<span class="type-mark">
                                        {columnGlyph(declaredType[column] ?? '')}
                                    </span>{/if}
                                <span>{column}</span>
                                {#if document.sort === column}<span>{document.descending ? '↓' : '↑'}</span>{/if}
                            </button>
                            <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Column separators are keyboard resizable.) -->
                            <span
                                role="separator"
                                aria-label={`Resize ${column}`}
                                aria-orientation="vertical"
                                aria-valuenow={layout.widths[column] || 164}
                                tabindex="0"
                                class="column-resize"
                                onpointerdown={(e) => resize(e, column)}
                                onkeydown={(e) => {
                                    if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
                                        e.preventDefault();
                                        layout.widths[column] = Math.max(72, (layout.widths[column] || 164) + (e.key === 'ArrowRight' ? 20 : -20));
                                    }
                                }}
                            ></span>
                        </th>{/each}
                    {#if afterWidth}<th aria-hidden="true" class="column-spacer"></th>{/if}
                </tr>
            </thead>
            <tbody>
                {#if start > 0}<tr aria-hidden="true" class="spacer">
                        <td colspan={visibleColumns.length + 1 + (beforeWidth ? 1 : 0) + (afterWidth ? 1 : 0)} style:height={`${start * rowHeight}px`}></td>
                    </tr>{/if}
                {#each rows.slice(start, end) as row, index}
                    <tr class:selected={document.selected === start + index} aria-rowindex={start + index + 2}>
                        <td class="row-number">{start + index + 1}</td>
                        {#if beforeWidth}<td aria-hidden="true" class="column-spacer"></td>{/if}
                        {#each visibleColumns as column, c}<td
                                class:null-cell={row[column] === null}
                                class:empty-cell={row[column] === ''}
                                class:numeric={columnGlyph(declaredType[column] ?? '') === '#'}
                                class:focused-cell={document.selected === start + index && cellColumn === firstColumn + c}
                            >
                                <button
                                    class="cell"
                                    tabindex="-1"
                                    title={row[column] === '' ? 'Empty string' : display(row[column], workspace.preferences)}
                                    onclick={() => {
                                        document.selected = start + index;
                                        cellColumn = firstColumn + c;
                                        document.selectedColumn = column;
                                        scroller.focus();
                                    }}
                                    ondblclick={() => (onedit ? onedit(row, column) : oninspect())}
                                >
                                    {#if row[column] === ''}<span class="empty-cell"> empty string </span>{:else if column === 'status'}<span
                                            class="cell-status"
                                            class:positive={row[column] === 'active' || row[column] === 'completed'}
                                        >
                                            <span></span>
                                            {display(row[column], workspace.preferences)}
                                        </span>{:else if column === 'role'}<span class="role-value">
                                            {display(row[column], workspace.preferences)}
                                        </span>{:else}{display(row[column], workspace.preferences)}{/if}
                                </button>
                            </td>{/each}
                        {#if afterWidth}<td aria-hidden="true" class="column-spacer"></td>{/if}
                    </tr>
                {/each}
                {#if end < rows.length}<tr aria-hidden="true" class="spacer">
                        <td colspan={visibleColumns.length + 1 + (beforeWidth ? 1 : 0) + (afterWidth ? 1 : 0)} style:height={`${(rows.length - end) * rowHeight}px`}></td>
                    </tr>{/if}
            </tbody>
        </table>
    </div>
</div>
