<script lang="ts">
    import { untrack } from 'svelte';
    import type { CellEdit, EditPreview, Row, SchemaColumn } from '../../state/types';
    import Button from '../../components/Button.svelte';
    import RowInspector from './RowInspector.svelte';
    import RowEditor from './RowEditor.svelte';
    import DocumentEditor from './DocumentEditor.svelte';
    let {
        row,
        columns,
        staged,
        mode = $bindable('inspect'),
        editable,
        reason,
        busy,
        onclose,
        onchange,
        onapply,
        onpreview,
        ondiscard,
        pendingCount,
        previewVersion,
        rowNumber,
        focusColumn,
        documentEdit,
    }: {
        row: Row | undefined;
        columns: SchemaColumn[];
        staged: Record<string, CellEdit>;
        mode: 'inspect' | 'edit';
        editable: boolean;
        reason: string;
        busy: boolean;
        onclose: () => void;
        onchange: (column: string, value: Row[string]) => void;
        onapply: () => void;
        onpreview: () => Promise<EditPreview[]>;
        ondiscard: () => void;
        pendingCount: number;
        previewVersion: string;
        rowNumber: number;
        focusColumn: string;
        // Set only for a MongoDB collection document: Edit mode renders a
        // whole-document JSON editor instead of RowEditor's flat column
        // grid, which a schemaless, arbitrarily nested document does not
        // fit the same way a SQL table's fixed columns do.
        documentEdit?: { sessionId: string; documentId: string; collection: string; onapplied: () => void } | null;
    } = $props();

    let previewOpen = $state(false);
    let previewLoading = $state(false);
    let previewError = $state('');
    let previews = $state<EditPreview[]>([]);
    let previewRequest = 0;

    async function refreshPreview() {
        const request = ++previewRequest;
        previewLoading = true;
        previewError = '';
        try {
            const next = await onpreview();
            if (request === previewRequest) previews = next;
        } catch (error) {
            if (request === previewRequest) {
                previews = [];
                previewError = String(error);
            }
        } finally {
            if (request === previewRequest) previewLoading = false;
        }
    }

    function togglePreview() {
        previewOpen = !previewOpen;
    }

    $effect(() => {
        const version = previewVersion;
        if (!previewOpen) return;
        if (!pendingCount) {
            previewOpen = false;
            previews = [];
            return;
        }
        void version;
        untrack(() => void refreshPreview());
    });
</script>

<svelte:window
    onkeydown={(event) => {
        if (event.key === 'Escape') onclose();
    }}
/>

<aside class="row-inspector row-details" aria-label="Row details">
    <header>
        <div>
            <span class="eyebrow">ROW DETAILS</span>
            <h3>{row ? `Record ${rowNumber}` : 'No row selected'}</h3>
            <small>{row ? `${Object.keys(row).length} fields` : 'Select a row to get started'}</small>
        </div>
        <Button icon="close" label="Close row details" onclick={onclose} />
    </header>
    <div class="segmented row-detail-tabs" role="tablist" aria-label="Row detail mode">
        {#each ['inspect', 'edit'] as tab}
            <button
                role="tab"
                aria-selected={mode === tab}
                class:active={mode === tab}
                onclick={() => (mode = tab as 'inspect' | 'edit')}
                onkeydown={(event) => {
                    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
                        event.preventDefault();
                        mode = event.key === 'Home' ? 'inspect' : event.key === 'End' ? 'edit' : mode === 'inspect' ? 'edit' : 'inspect';
                        (event.currentTarget.parentElement?.querySelectorAll('button')[mode === 'inspect' ? 0 : 1] as HTMLButtonElement)?.focus();
                    }
                }}
            >
                {tab === 'inspect' ? 'Inspect' : 'Edit'}{#if tab === 'edit' && Object.keys(staged).length}<span class="count">
                        {Object.keys(staged).length}
                    </span>{/if}
            </button>
        {/each}
    </div>
    {#if mode === 'inspect'}
        <RowInspector {row} />
    {:else if row && editable && documentEdit}
        <DocumentEditor
            sessionId={documentEdit.sessionId}
            documentId={documentEdit.documentId}
            collection={documentEdit.collection}
            id={JSON.stringify(row._id)}
            disabled={busy}
            onclose={() => (mode = 'inspect')}
            onapplied={documentEdit.onapplied}
        />
    {:else if row && editable}
        <div class="row-edit-note">Changes stay staged until you apply them.</div>
        <RowEditor {row} {columns} {staged} {focusColumn} disabled={busy} {onchange} />
        {#if previewOpen}
            <section class="edit-preview" aria-label="Generated SQL preview" aria-live="polite">
                <div class="edit-preview-heading">
                    <div>
                        <span class="eyebrow">GENERATED SQL</span>
                        <small>Values stay bound separately from the statement.</small>
                    </div>
                    <Button icon="close" label="Close SQL preview" onclick={() => (previewOpen = false)} />
                </div>
                {#if previewLoading}
                    <p class="edit-preview-state">Generating preview…</p>
                {:else if previewError}
                    <p class="edit-preview-state error-text">{previewError}</p>
                {:else}
                    <div class="edit-preview-list">
                        {#each previews as preview, index (`${preview.kind}-${index}-${preview.sql}`)}
                            <article class="edit-preview-item">
                                <span class="edit-preview-kind">{preview.kind}</span>
                                <!-- svelte-ignore a11y_no_noninteractive_tabindex (Read-only generated SQL is focusable for keyboard selection and copying.) -->
                                <pre tabindex="0">{preview.sql}</pre>
                                {#if preview.args.length}
                                    <div class="edit-preview-args">
                                        <span>Bound values</span>
                                        <ol>
                                            {#each preview.args as arg}<li><code>{arg}</code></li>{/each}
                                        </ol>
                                    </div>
                                {/if}
                            </article>
                        {/each}
                    </div>
                {/if}
            </section>
        {/if}
        <footer class="row-edit-footer">
            <small>{pendingCount} staged row change{pendingCount === 1 ? '' : 's'} in this table</small>
            <div>
                <Button variant="outline" disabled={busy || !pendingCount} active={previewOpen} onclick={togglePreview}>Preview SQL</Button>
                <Button variant="primary" icon="check" disabled={busy || !pendingCount} onclick={onapply}>
                    Apply all ({pendingCount})
                </Button>
                <Button variant="outline" disabled={busy || !pendingCount} onclick={ondiscard}>Discard all</Button>
            </div>
        </footer>
    {:else}
        <div class="empty-state">
            <h3>Read-only record</h3>
            <p>{row ? reason : 'Select a row to edit it.'}</p>
        </div>
    {/if}
</aside>
