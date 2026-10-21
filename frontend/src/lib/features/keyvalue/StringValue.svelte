<script lang="ts">
    import { untrack } from 'svelte';
    import type { KeyDetail, KeyEdit } from '../../state/types';
    import Button from '../../components/Button.svelte';
    import { kvText, prettyJSON } from './format';

    let {
        detail,
        editing = false,
        busy = false,
        onedit,
    }: {
        detail: KeyDetail;
        editing?: boolean;
        busy?: boolean;
        onedit?: (edit: KeyEdit) => Promise<boolean>;
    } = $props();
    let formatted = $state(true);
    let raw = $derived(kvText(detail.value, Number.MAX_SAFE_INTEGER));
    let pretty = $derived(detail.value?.binary ? undefined : prettyJSON(raw));
    let truncated = $derived((detail.value?.size ?? 0) < detail.length);
    // Writing back a partial or hex-rendered value would corrupt the key.
    let locked = $derived(truncated || !!detail.value?.binary);
    // The parent keys this component by detail, so a reload reseeds it.
    let draft = $state(untrack(() => kvText(detail.value, Number.MAX_SAFE_INTEGER)));

    function apply() {
        onedit?.({ key: detail.key, op: 'set', value: draft, oldValue: detail.value?.text ?? '' });
    }
</script>

{#if editing}<div class="inspector-content">
        {#if locked}<p class="field-hint">
                {detail.value?.binary ? 'Binary values cannot be edited here.' : 'This value is too large to load in full, so it cannot be edited here.'}
            </p>{/if}
        <textarea class="document-editor-text key-string-editor" aria-label="String value" disabled={locked || busy} bind:value={draft} rows={12}></textarea>
    </div>
    <footer class="row-edit-footer">
        <small>Saving keeps the key's expiry.</small>
        <div>
            <Button variant="primary" icon="check" disabled={locked || busy || draft === raw} onclick={apply}>Apply</Button>
            <Button variant="outline" disabled={busy || draft === raw} onclick={() => (draft = raw)}>Revert</Button>
        </div>
    </footer>{:else}
    {#if pretty}<div class="segmented">
            <button class:active={formatted} onclick={() => (formatted = true)}>Formatted JSON</button>
            <button class:active={!formatted} onclick={() => (formatted = false)}>Raw</button>
        </div>{/if}
    <div class="inspector-content">
        {#if detail.value?.binary}<p class="field-hint">Binary value (not UTF-8), shown as hex.</p>{/if}
        {#if truncated}<p class="field-hint">
                Showing the first {(detail.value?.size ?? 0).toLocaleString()} of {detail.length.toLocaleString()} bytes.
            </p>{/if}
        {#if raw === ''}<p class="empty-cell">empty string</p>{:else}<pre class="key-string">{pretty && formatted ? pretty : raw}</pre>{/if}
    </div>
{/if}
