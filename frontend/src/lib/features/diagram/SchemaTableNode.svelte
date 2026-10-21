<script lang="ts">
    import { Handle, Position } from '@xyflow/svelte';
    import { objectLabel, type SchemaForeignKey, type SchemaTable } from '../../state/types';
    import Icon from '../../components/Icon.svelte';

    export interface TableNodeData {
        table: SchemaTable;
        collapsed: boolean;
        detail: 'full' | 'names' | 'keys';
        related: boolean;
        dimmed: boolean;
        selfReferences: SchemaForeignKey[];
        kbdFocused: boolean;
        onOpen: (table: string) => void;
        onToggle: (table: string) => void;
        onFocusRelated: (table: string) => void;
    }

    let { data, selected = false }: { data: TableNodeData; selected?: boolean } = $props();
    const visibleColumnLimit = 42;
    let pkCount = $derived(data.table.columns.filter((column) => column.primaryKey).length);
    let fkColumns = $derived(new Set(data.table.foreignKeys.map((fk) => fk.column)));
    // A composite unique index does not make any single column individually
    // unique, so only single-column unique indexes map to a per-column marker;
    // a composite one stays visible only in the table document's Indexes tab.
    let uniqueColumns = $derived(new Set(data.table.indexes.filter((index) => index.unique && index.columns.length === 1).map((index) => index.columns[0])));
    let visibleColumns = $derived(data.table.columns.slice(0, visibleColumnLimit));
    let displayedColumns = $derived(data.detail === 'keys' ? visibleColumns.filter((column) => column.primaryKey || fkColumns.has(column.name)) : visibleColumns);
    let hiddenColumnCount = $derived(Math.max(0, data.table.columns.length - visibleColumnLimit));
    let hiddenKeyCount = $derived(data.detail === 'keys' && !displayedColumns.length ? data.table.columns.length : 0);
</script>

<div class="schema-node" class:selected class:related={data.related} class:dimmed={data.dimmed} class:kbd-focused={data.kbdFocused}>
    <Handle type="target" position={Position.Left} />
    <Handle type="source" position={Position.Right} />
    <header>
        <div>
            <span>{data.table.schema || 'main'}</span>
            <strong>{objectLabel(data.table.name)}</strong>
        </div>
        <button
            class="node-action nodrag nopan"
            aria-label={`Collapse ${objectLabel(data.table.name)}`}
            onclick={(event) => {
                event.stopPropagation();
                data.onToggle(data.table.name);
            }}
        >
            {data.collapsed ? '+' : '-'}
        </button>
    </header>
    {#if !data.collapsed}<div class="schema-node-columns" class:names-only={data.detail === 'names'}>
            {#each displayedColumns as column}<button
                    class="schema-column nodrag nopan"
                    class:names-only={data.detail === 'names'}
                    title={`${column.name} · ${column.type || 'value'}${uniqueColumns.has(column.name) ? ' · unique' : ''}`}
                    onclick={(event) => {
                        event.stopPropagation();
                        data.onOpen(data.table.name);
                    }}
                >
                    <span class="column-markers">
                        {#if column.primaryKey}<Icon name="key" size={11} />{:else}<span></span>{/if}
                        {#if fkColumns.has(column.name)}<Icon name="link" size={11} />{/if}
                        {#if uniqueColumns.has(column.name)}<Icon name="unique" size={11} />{/if}
                    </span>
                    <span class="column-name">{column.name}</span>
                    {#if data.detail === 'full'}<span class="column-type">{column.type || 'value'}</span>
                        {#if !column.notNull}<span class="column-null">NULL</span>{/if}{/if}
                </button>{/each}
            {#if hiddenKeyCount}<button
                    class="schema-column schema-column-more nodrag nopan"
                    title="No primary or foreign key columns"
                    onclick={(event) => {
                        event.stopPropagation();
                        data.onOpen(data.table.name);
                    }}
                >
                    <span class="column-markers"></span>
                    <span class="column-name">No key columns</span>
                    {#if data.detail === 'full'}<span class="column-type">open table</span>{/if}
                </button>{/if}
            {#if data.selfReferences.length}<div class="schema-self-refs">
                    {#each data.selfReferences as fk}<div class="schema-self-ref" title={`${fk.column} references ${fk.refColumn}`}>
                            <Icon name="link" size={11} />
                            <span>Self ref</span>
                            <strong>{fk.column} → {fk.refColumn}</strong>
                        </div>{/each}
                </div>{/if}
            {#if hiddenColumnCount}<button
                    class="schema-column schema-column-more nodrag nopan"
                    title={`${hiddenColumnCount} more columns`}
                    onclick={(event) => {
                        event.stopPropagation();
                        data.onOpen(data.table.name);
                    }}
                >
                    <span class="column-markers"></span>
                    <span class="column-name">+ {hiddenColumnCount} more columns</span>
                    <span class="column-type">open table</span>
                </button>{/if}
        </div>{/if}
    <footer>
        <button
            class="nodrag nopan"
            onclick={(event) => {
                event.stopPropagation();
                data.onOpen(data.table.name);
            }}
        >
            Open table
        </button>
        <button
            class="nodrag nopan"
            onclick={(event) => {
                event.stopPropagation();
                data.onFocusRelated(data.table.name);
            }}
        >
            Related
        </button>
        <span>{pkCount ? `${pkCount} PK` : 'No PK'}</span>
    </footer>
</div>
