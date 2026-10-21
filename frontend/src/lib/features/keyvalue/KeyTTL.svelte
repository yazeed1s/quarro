<script lang="ts">
    import type { KeyDetail, KeyEdit } from '../../state/types';
    import { closeOnOutsideClick } from '../../components/dropdown';
    import Button from '../../components/Button.svelte';
    import Input from '../../components/Input.svelte';
    import { formatTTL, parseDuration } from './format';

    let { detail, busy = false, onedit }: { detail: KeyDetail; busy?: boolean; onedit: (edit: KeyEdit) => Promise<boolean> } = $props();
    let panel = $state<HTMLDetailsElement>();
    let text = $state('');
    let ms = $derived(parseDuration(text));
    const presets = ['1h', '1d', '7d'];

    async function apply(edit: KeyEdit) {
        if (await onedit(edit)) {
            text = '';
            if (panel) panel.open = false;
        }
    }
</script>

<details class="dropdown key-ttl" bind:this={panel} use:closeOnOutsideClick>
    <summary aria-label="Edit expiry" title="Edit expiry">{formatTTL(detail.ttlMillis)}</summary>
    <form
        class="key-ttl-panel"
        aria-label="Expiry"
        onsubmit={(e) => {
            e.preventDefault();
            if (ms !== undefined && !busy) apply({ key: detail.key, op: 'expire', value: '', ttlMillis: ms });
        }}
    >
        <Input label="Expires in" placeholder="e.g. 30m, 2h, 7d" bind:value={text} />
        <div class="key-ttl-presets">
            {#each presets as preset}<Button variant="outline" onclick={() => (text = preset)}>{preset}</Button>{/each}
        </div>
        {#if text.trim() && ms === undefined}<p class="error-text">Use a duration above zero, like 30s, 15m, 2h, or 7d.</p>{/if}
        <div class="key-ttl-actions">
            {#if detail.ttlMillis}<Button variant="outline" disabled={busy} onclick={() => apply({ key: detail.key, op: 'persist', value: '' })}>Remove expiry</Button>{/if}
            <Button type="submit" variant="primary" icon="check" disabled={busy || ms === undefined}>Set expiry</Button>
        </div>
    </form>
</details>
