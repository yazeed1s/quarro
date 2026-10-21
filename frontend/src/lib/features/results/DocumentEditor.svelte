<script lang="ts">
    import * as backend from '../../state/backend';
    import Button from '../../components/Button.svelte';

    let {
        sessionId,
        documentId,
        collection,
        id,
        disabled = false,
        onclose,
        onapplied,
    }: {
        sessionId: string;
        documentId: string;
        collection: string;
        // null means "insert a new document" rather than editing an existing one.
        id: string | null;
        disabled?: boolean;
        onclose: () => void;
        onapplied: () => void;
    } = $props();

    let text = $state('');
    let loading = $state(true);
    let applying = $state(false);
    let error = $state('');

    $effect(() => {
        const currentId = id;
        if (currentId === null) {
            text = '{\n  \n}';
            loading = false;
            error = '';
            return;
        }
        loading = true;
        error = '';
        backend
            .fetchDocumentJSON(sessionId, documentId, collection, currentId)
            .then((json) => {
                if (currentId === id) text = json;
            })
            .catch((e) => {
                if (currentId === id) error = String(e);
            })
            .finally(() => {
                if (currentId === id) loading = false;
            });
    });

    async function apply() {
        applying = true;
        error = '';
        try {
            const result = await backend.applyDocumentEdit(sessionId, documentId, collection, id === null ? 'insert' : 'replace', id ?? '', text);
            if (!result.applied) {
                error = 'This document no longer exists — it may have been deleted or changed elsewhere.';
                return;
            }
            onapplied();
            onclose();
        } catch (e) {
            error = String(e);
        } finally {
            applying = false;
        }
    }
</script>

<div class="inspector-content document-editor">
    <div class="inspector-field editor-field">
        <div class="editor-field-heading">
            <span class="field-name">{id === null ? 'New document' : 'Document'}</span>
            <span class="field-type">Extended JSON</span>
        </div>
        {#if loading}
            <p class="document-editor-state">Loading…</p>
        {:else}
            <textarea
                class="document-editor-text"
                aria-label="Document JSON"
                disabled={disabled || applying}
                bind:value={text}
                rows={14}
            ></textarea>
        {/if}
        {#if error}<p class="document-editor-state error-text">{error}</p>{/if}
    </div>
</div>
<footer class="row-edit-footer">
    <small>{id === null ? 'Inserted as a new document.' : 'Replaces the whole document.'}</small>
    <div>
        <Button variant="primary" icon="check" disabled={disabled || applying || loading} onclick={apply}>Apply</Button>
        <Button variant="outline" disabled={applying} onclick={onclose}>Cancel</Button>
    </div>
</footer>
