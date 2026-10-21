<script lang="ts">
    import type { Document, KeyEdit, Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Button from '../../components/Button.svelte';
    import { copy } from '../results/format';
    import { editableTypes, formatLength, formatTTL, typeLabel } from './format';
    import StringValue from './StringValue.svelte';
    import KeyMembers from './KeyMembers.svelte';
    import StreamValue from './StreamValue.svelte';
    import KeyTTL from './KeyTTL.svelte';

    let { session, document, onclose }: { session: Session; document: Document; onclose: () => void } = $props();
    let detail = $derived(document.keyDetail);
    let more = () => detail && workspace.inspectKey(session, document, detail.key, true);
    let summary = $derived(detail?.exists ? [typeLabel(detail.type), formatLength(detail.type, detail.length)].filter(Boolean).join(' · ') : '');

    let mode = $state<'inspect' | 'edit'>('inspect');
    let canEdit = $derived(session.capabilities.keyValueEditor && !session.profile.readOnly && !!detail?.exists && editableTypes.includes(detail.type));
    let editing = $derived(canEdit && mode === 'edit');
    // Any existing key's expiry can change, including a stream's, whose
    // content is read-only here.
    let canEditTTL = $derived(session.capabilities.keyValueEditor && !session.profile.readOnly && !!detail?.exists);
    // A conflict belongs to the key it happened on, so opening another key
    // never shows a stale one.
    let conflict = $state<{ key: string; text: string } | null>(null);
    let conflictText = $derived(conflict && conflict.key === detail?.key ? conflict.text : '');
    let busy = $state(false);

    // edit applies one change and reports whether it went through. A
    // conflict is shown here, with a reload action, rather than as a
    // notification, since the user needs to look at the key again.
    async function edit(e: KeyEdit): Promise<boolean> {
        busy = true;
        conflict = null;
        try {
            const result = await workspace.applyKeyEdit(session, document, e);
            if (result?.conflict) conflict = { key: e.key, text: result.message || 'This key changed since it was loaded.' };
            return !!result?.applied;
        } finally {
            busy = false;
        }
    }
    function reload() {
        if (!detail) return;
        conflict = null;
        workspace.inspectKey(session, document, detail.key);
    }
    async function deleteKey() {
        if (!detail || !window.confirm(`Delete key "${detail.key}"? This cannot be undone.`)) return;
        await edit({ key: detail.key, op: 'del', value: '' });
    }
</script>

<aside class="row-inspector row-details key-inspector" aria-label="Key details">
    <header>
        <div>
            <span class="eyebrow">KEY</span>
            <h3 title={detail?.key}>{detail?.key ?? (document.keyLoading ? 'Loading…' : 'No key selected')}</h3>
            {#if summary && detail}<small class="key-summary">
                    {summary} ·
                    {#if canEditTTL}<KeyTTL {detail} {busy} onedit={edit} />{:else}{formatTTL(detail.ttlMillis)}{/if}
                </small>{:else if !detail}<small>Select a key to inspect it</small>{/if}
        </div>
        <div class="key-inspector-actions">
            {#if detail}<Button icon="copy" label="Copy key name" onclick={() => copy(detail.key, (s) => workspace.notify(s))} />
                <Button icon="refresh" label="Reload key" disabled={document.keyLoading} onclick={reload} />{/if}
            {#if canEdit}<Button icon="trash" label="Delete key" disabled={busy} onclick={deleteKey} />{/if}
            <Button icon="close" label="Close key details" onclick={onclose} />
        </div>
    </header>
    {#if canEdit}<div class="segmented row-detail-tabs" role="tablist" aria-label="Key detail mode">
            <button role="tab" aria-selected={mode === 'inspect'} class:active={mode === 'inspect'} onclick={() => (mode = 'inspect')}>Inspect</button>
            <button role="tab" aria-selected={mode === 'edit'} class:active={mode === 'edit'} onclick={() => (mode = 'edit')}>Edit</button>
        </div>{/if}
    {#if conflictText}<div class="key-conflict" role="alert">
            <p class="error-text">{conflictText}</p>
            <Button variant="outline" icon="refresh" onclick={reload}>Reload key</Button>
        </div>{/if}
    {#if document.keyError}<div class="inspector-content">
            <p class="error-text">{document.keyError}</p>
        </div>{:else if detail && !detail.exists}<div class="empty-state">
            <h3>Key not found</h3>
            <p>It may have expired or been deleted since the list was loaded.</p>
        </div>{:else if detail?.type === 'string'}{#key detail}<StringValue {detail} {editing} {busy} onedit={edit} />{/key}{:else if detail?.type === 'stream'}<StreamValue
            {detail}
            loading={document.keyLoading}
            onmore={more}
        />{:else if detail && ['hash', 'list', 'set', 'zset'].includes(detail.type)}{#key detail.key}<KeyMembers
                {detail}
                loading={document.keyLoading}
                onmore={more}
                {editing}
                {busy}
                onedit={edit}
            />{/key}{:else if detail}<div class="empty-state">
            <h3>{detail.type}</h3>
            <p>This type isn't viewable here yet.</p>
        </div>{:else if !document.keyLoading}<div class="empty-state"><p>Select a key to look a little closer.</p></div>{/if}
</aside>
