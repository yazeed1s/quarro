<script lang="ts">
    import type { CellEdit, Row, SchemaColumn } from '../../state/types';
    import Input from '../../components/Input.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';

    const boolOptions: SelectOption[] = [
        { value: 'true', label: 'true' },
        { value: 'false', label: 'false' },
    ];
    let {
        row,
        columns,
        staged,
        disabled = false,
        focusColumn = '',
        onchange,
    }: {
        row: Row;
        columns: SchemaColumn[];
        staged: Record<string, CellEdit>;
        disabled?: boolean;
        focusColumn?: string;
        onchange: (column: string, value: Row[string]) => void;
    } = $props();
    const uid = $props.id();
    function focusField(element: HTMLElement, name: string) {
        if (name !== focusColumn) return;
        const target = element instanceof HTMLTextAreaElement ? element : element.querySelector<HTMLButtonElement>('.select-trigger');
        if (target && target.getClientRects().length) target.focus({ preventScroll: false });
    }
    function value(name: string) {
        return staged[name] ? (staged[name].mode === 'null' ? null : (staged[name].value ?? '')) : row[name];
    }
</script>

<div class="inspector-content row-editor">
    {#each columns as column (column.name)}
        {@const current = value(column.name)}
        {@const changed = column.name in staged}
        <div class="inspector-field editor-field" class:changed>
            <div class="editor-field-heading">
                <label class="field-name" for={`${uid}-${column.name}`}>{column.name}</label>
                <span class="field-type">{column.primaryKey ? 'PK · ' : ''}{column.type || 'value'}</span>
                <div class="editor-null-toggle">
                    <Input
                        type="checkbox"
                        bind:checked={() => current === null, (isNull) => onchange(column.name, isNull ? null : (row[column.name] ?? ''))}
                        disabled={disabled || (column.notNull && current !== null)}
                        label="NULL"
                    />
                </div>
            </div>
            {#if typeof row[column.name] === 'object' && row[column.name] !== null}
                <small>Binary and object values are read-only.</small>
            {:else}
                <div class="editor-value-control">
                    {#if typeof row[column.name] === 'boolean'}
                        <div use:focusField={column.name}>
                            <Select
                                bind:value={() => String(current), (value) => onchange(column.name, value === 'true')}
                                options={boolOptions}
                                disabled={disabled || current === null}
                                placeholder={column.name}
                            />
                        </div>
                    {:else}
                        <textarea
                            use:focusField={column.name}
                            id={`${uid}-${column.name}`}
                            aria-label={column.name}
                            rows={String(current ?? '').includes('\n') || String(current ?? '').length > 90 ? 3 : 1}
                            placeholder={current === null ? 'NULL' : 'Empty string'}
                            value={current === null ? '' : String(current)}
                            disabled={disabled || current === null}
                            oninput={(e) => onchange(column.name, e.currentTarget.value)}></textarea>
                    {/if}
                </div>
                {#if changed}<div class="editor-field-actions">
                        {#if changed}
                            <span class="editor-changed-label">Changed</span>
                            <button {disabled} onclick={() => onchange(column.name, row[column.name])}> Revert </button>
                        {/if}
                    </div>{/if}
                {#if changed}<small class="editor-original">
                        Original: {row[column.name] === null ? 'NULL' : String(row[column.name]) || 'Empty string'}
                    </small>{/if}
            {/if}
        </div>
    {/each}
</div>
