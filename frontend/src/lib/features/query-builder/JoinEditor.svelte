<script lang="ts">
    import type { VisualQueryJoin, VisualQueryJoinType } from '../../state/types';
    import Button from '../../components/Button.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';

    const typeOptions: SelectOption[] = [
        { value: 'inner', label: 'Inner join. Only matching rows' },
        { value: 'left', label: 'Left join. Keep all left rows' },
        { value: 'right', label: 'Right join. Keep all right rows' },
    ];

    export interface JoinEditorTable {
        alias: string;
        label: string;
        columns: string[];
    }

    let {
        tables,
        initial,
        defaultLeftAlias,
        onsave,
        onremove,
        onclose,
    }: {
        tables: JoinEditorTable[];
        initial?: VisualQueryJoin;
        defaultLeftAlias?: string;
        onsave: (join: VisualQueryJoin) => void;
        onremove?: () => void;
        onclose: () => void;
    } = $props();

    // This component is remounted fresh each time it opens (see the {#if
    // joinEditor} in VisualQueryBuilder.svelte), so seeding local state once
    // from props below is intentional; it never needs to track later prop
    // changes.
    // svelte-ignore state_referenced_locally
    let leftAlias = $state(initial?.left.table ?? defaultLeftAlias ?? tables[0]?.alias ?? '');
    // svelte-ignore state_referenced_locally
    let rightAlias = $state(initial?.right.table ?? tables.find((t) => t.alias !== leftAlias)?.alias ?? '');
    // svelte-ignore state_referenced_locally
    let leftColumn = $state(initial?.left.column ?? '');
    // svelte-ignore state_referenced_locally
    let rightColumn = $state(initial?.right.column ?? '');
    // svelte-ignore state_referenced_locally
    let type = $state<VisualQueryJoinType>(initial?.type ?? 'inner');

    let leftTable = $derived(tables.find((t) => t.alias === leftAlias));
    let rightTable = $derived(tables.find((t) => t.alias === rightAlias));
    let tableOptions: SelectOption[] = $derived(tables.map((t) => ({ value: t.alias, label: t.label })));
    let leftColumnOptions: SelectOption[] = $derived([{ value: '', label: 'Choose a column…' }, ...(leftTable?.columns ?? []).map((c) => ({ value: c, label: c }))]);
    let rightColumnOptions: SelectOption[] = $derived([{ value: '', label: 'Choose a column…' }, ...(rightTable?.columns ?? []).map((c) => ({ value: c, label: c }))]);
    let valid = $derived(!!leftAlias && !!rightAlias && leftAlias !== rightAlias && !!leftColumn && !!rightColumn);

    function save() {
        if (!valid) return;
        onsave({
            type,
            left: { table: leftAlias, column: leftColumn },
            right: { table: rightAlias, column: rightColumn },
        });
        onclose();
    }
</script>

<aside class="row-inspector join-inspector">
    <header>
        <div>
            <span class="eyebrow">{initial ? 'EDIT JOIN' : 'ADD JOIN'}</span>
            <h3>
                {leftTable ? leftTable.label : 'Choose tables'}{#if rightTable}
                    → {rightTable.label}{/if}
            </h3>
        </div>
        <Button icon="close" label="Close join editor" onclick={onclose} />
    </header>
    <div class="inspector-content">
        <Select bind:value={leftAlias} options={tableOptions} label="Left table" />
        <Select bind:value={rightAlias} options={tableOptions} label="Right table" />
        <Select bind:value={leftColumn} options={leftColumnOptions} label="Left column" />
        <Select bind:value={rightColumn} options={rightColumnOptions} label="Right column" />
        <Select bind:value={type} options={typeOptions} label="Join type" />
    </div>
    <footer>
        <div class="join-inspector-actions">
            {#if onremove}<Button
                    variant="danger"
                    onclick={() => {
                        onremove?.();
                        onclose();
                    }}
                >
                    Remove join
                </Button>{/if}
            <Button variant="primary" disabled={!valid} onclick={save}>
                {initial ? 'Save changes' : 'Add join'}
            </Button>
        </div>
    </footer>
</aside>
