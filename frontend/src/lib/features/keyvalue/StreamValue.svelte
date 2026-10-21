<script lang="ts">
    import type { KeyDetail } from '../../state/types';
    import Button from '../../components/Button.svelte';
    import { kvText } from './format';

    let { detail, loading, onmore }: { detail: KeyDetail; loading: boolean; onmore: () => void } = $props();
</script>

<div class="key-members">
    <div class="inspector-content">
        {#if detail.stream}<div class="stream-summary">
                <div class="inspector-field">
                    <span class="field-name">First entry</span>
                    <p><code>{detail.stream.firstId || '—'}</code></p>
                </div>
                <div class="inspector-field">
                    <span class="field-name">Last entry</span>
                    <p><code>{detail.stream.lastId || '—'}</code></p>
                </div>
            </div>
            {#if detail.stream.groups.length}<table class="metadata-table" aria-label="Consumer groups">
                    <thead>
                        <tr>
                            <th>Group</th>
                            <th>Consumers</th>
                            <th>Pending</th>
                            <th>Lag</th>
                        </tr>
                    </thead>
                    <tbody>
                        {#each detail.stream.groups as group}<tr>
                                <td title={`Last delivered ${group.lastDeliveredId}`}>{group.name}</td>
                                <td>{group.consumers}</td>
                                <td>{group.pending}</td>
                                <td>{group.lag ?? '—'}</td>
                            </tr>{/each}
                    </tbody>
                </table>{:else}<p class="field-hint">No consumer groups.</p>{/if}{/if}
        {#each detail.members as entry (entry.id)}<div class="inspector-field stream-entry">
                <span class="field-name"><code>{entry.id}</code></span>
                {#each Object.entries(entry.fields ?? {}) as [field, value]}<p>
                        <span class="stream-field">{field}</span>
                        <span class:key-binary={value.binary}>{kvText(value)}</span>
                    </p>{/each}
            </div>{:else}<p class="field-hint">No entries.</p>{/each}
    </div>
    <footer>
        <small>Loaded {detail.members.length.toLocaleString()} of {detail.length.toLocaleString()} entries</small>
        {#if detail.cursor}<Button variant="outline" disabled={loading} onclick={onmore}>{loading ? 'Loading…' : 'Load more'}</Button>{/if}
    </footer>
</div>
