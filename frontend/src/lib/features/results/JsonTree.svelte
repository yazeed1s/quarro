<script lang="ts">
    import type { CellValue } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Button from '../../components/Button.svelte';
    import { display, copy } from './format';
    import JsonTree from './JsonTree.svelte';

    let { value, label, depth = 0 }: { value: CellValue; label?: string; depth?: number } = $props();

    let isNode = $derived(typeof value === 'object' && value !== null && !('blob' in value));
    let entries = $derived.by(() => {
        if (!isNode) return [];
        return Array.isArray(value) ? value.map((v, i) => [String(i), v] as const) : Object.entries(value as Record<string, CellValue>);
    });
    let summary = $derived(isNode ? display(value, workspace.preferences) : '');
</script>

{#if isNode}
    <details class="json-node" open={depth < 2}>
        <summary>
            {#if label !== undefined}<span class="json-key">{label}</span>{/if}
            <span class="json-summary">{summary}</span>
            <span class="json-copy">
                <Button
                    icon="copy"
                    label="Copy value"
                    onclick={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        void copy(JSON.stringify(value, null, 2), (text) => workspace.notify(text));
                    }}
                />
            </span>
        </summary>
        <div class="json-children">
            {#each entries as [key, child]}<div class="json-row">
                    <JsonTree value={child} label={key} depth={depth + 1} />
                </div>{/each}
        </div>
    </details>
{:else}
    <div class="json-leaf">
        {#if label !== undefined}<span class="json-key">{label}</span>{/if}
        <span class:null-cell={value === null} class:empty-cell={value === ''}>
            {value === '' ? 'empty string' : display(value, workspace.preferences)}
        </span>
    </div>
{/if}
