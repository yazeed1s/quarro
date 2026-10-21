<script lang="ts">
    import type { Row } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Button from '../../components/Button.svelte';
    import { display, copy } from './format';
    import JsonTree from './JsonTree.svelte';
    let { row }: { row: Row | undefined } = $props();
    let mode = $state('fields');
    function isNode(value: Row[string]) {
        return typeof value === 'object' && value !== null && !('blob' in value);
    }
</script>

<div class="segmented">
    <button class:active={mode === 'fields'} onclick={() => (mode = 'fields')}>Fields</button>
    <button class:active={mode === 'json'} onclick={() => (mode = 'json')}>JSON</button>
</div>
{#if row}<div class="inspector-content">
        {#if mode === 'json'}<pre>{JSON.stringify(row, null, 2)}</pre>{:else}{#each Object.entries(row) as [key, value]}<div class="inspector-field">
                    <span class="field-name">{key}</span>
                    {#if isNode(value)}<JsonTree {value} />{:else}<p class:null-cell={value === null} class:empty-cell={value === ''}>
                            {value === '' ? 'empty string' : display(value, workspace.preferences)}
                        </p>{/if}
                </div>{/each}{/if}
    </div>
    <footer>
        <Button icon="copy" variant="outline" onclick={() => copy(JSON.stringify(row, null, 2), (s) => workspace.notify(s))}>Copy as JSON</Button>
    </footer>{:else}<div class="empty-state"><p>Select a row to look a little closer.</p></div>{/if}
