<script lang="ts">
    import type { Document, ResultSet, Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Icon from '../../components/Icon.svelte';
    import Button from '../../components/Button.svelte';
    import NewKeyDialog from './NewKeyDialog.svelte';
    import { formatLength, formatTTL, typeLabel } from './format';

    let { session, document, result }: { session: Session; document: Document; result: ResultSet } = $props();
    let entries = $derived(result.entries ?? []);
    let openKey = $derived(document.keyDetail?.key ?? '');
    let canEdit = $derived(session.capabilities.keyValueEditor && !session.profile.readOnly);
    let creating = $state(false);

    function open(key: string) {
        session.inspector = true;
        workspace.inspectKey(session, document, key);
    }
    function scan(prefix: string) {
        document.tableFilter = prefix;
        workspace.closeKey(document);
        workspace.runTableQuery(session, document);
    }
</script>

<div class="grid-toolbar">
    <label class="search-field filter-field">
        <Icon name="filter" size={14} />
        <input aria-label="Filter keys by prefix" placeholder={`Prefix, e.g. ${document.object}:`} value={document.tableFilter} onchange={(e) => scan(e.currentTarget.value)} />
        {#if document.tableFilter}<button aria-label="Clear key filter" onclick={() => scan('')}>
                <Icon name="close" size={12} />
            </button>{/if}
    </label>
    {#if canEdit}<Button icon="plus" variant="outline" onclick={() => (creating = true)}>New key</Button>{/if}
    <span class="grid-toolbar-note">
        {entries.length.toLocaleString()}{result.truncated ? '+' : ''} key{entries.length === 1 ? '' : 's'}
    </span>
</div>
{#if entries.length}<div class="key-list">
        <table class="metadata-table" aria-label="Keys">
            <thead>
                <tr>
                    <th>Key</th>
                    <th>Type</th>
                    <th>Size</th>
                    <th>TTL</th>
                </tr>
            </thead>
            <tbody>
                {#each entries as entry (entry.key)}<tr class:active={entry.key === openKey} aria-selected={entry.key === openKey}>
                        <td>
                            <button type="button" class="key-name" onclick={() => open(entry.key)}><code>{entry.key}</code></button>
                        </td>
                        <td><span class="key-type" data-type={entry.type}>{typeLabel(entry.type)}</span></td>
                        <td>{formatLength(entry.type, entry.length)}</td>
                        <td>{formatTTL(entry.ttlMillis)}</td>
                    </tr>{/each}
            </tbody>
        </table>
        {#if result.truncated}<p class="field-hint">More keys matched than fit this scan; narrow the prefix to see the rest.</p>{/if}
    </div>{:else}<div class="empty-state">
        <Icon name="key" size={28} />
        <h3>No keys found</h3>
        <p>Nothing matched this prefix.</p>
    </div>{/if}
{#if creating}<NewKeyDialog {session} {document} onclose={() => (creating = false)} />{/if}
