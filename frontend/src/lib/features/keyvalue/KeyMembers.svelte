<script lang="ts">
    import type { KeyDetail, KeyEdit, KeyMember } from '../../state/types';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import Select from '../../components/Select.svelte';
    import { kvText } from './format';

    let {
        detail,
        loading,
        onmore,
        editing = false,
        busy = false,
        onedit,
    }: {
        detail: KeyDetail;
        loading: boolean;
        onmore: () => void;
        editing?: boolean;
        busy?: boolean;
        onedit?: (edit: KeyEdit) => Promise<boolean>;
    } = $props();
    let filter = $state('');

    // One table for hashes, lists, sets, and sorted sets: they differ only
    // in which columns a member carries.
    let columns = $derived(detail.type === 'hash' ? ['Field', 'Value'] : detail.type === 'list' ? ['#', 'Value'] : detail.type === 'zset' ? ['Member', 'Score'] : ['Member']);
    let shown = $derived.by(() => {
        const q = filter.trim().toLowerCase();
        if (!q) return detail.members;
        return detail.members.filter((m) => kvText(m.value).toLowerCase().includes(q) || kvText(m.field).toLowerCase().includes(q));
    });

    function cells(m: KeyMember): { text: string; binary?: boolean }[] {
        const value = { text: kvText(m.value), binary: m.value.binary };
        if (detail.type === 'hash') return [{ text: kvText(m.field), binary: m.field?.binary }, value];
        if (detail.type === 'list') return [{ text: String(m.index ?? '') }, value];
        if (detail.type === 'zset') return [value, { text: String(m.score ?? '') }];
        return [value];
    }
    // Binary values are shown as hex, so editing them would write the hex
    // text back over the real bytes.
    const binary = (m: KeyMember) => !!(m.value.binary || m.field?.binary);

    // The row being edited, by member identity: detail.members is replaced
    // on reload, which also ends the edit.
    let editingMember = $state<KeyMember | null>(null);
    let draftField = $state('');
    let draftValue = $state('');
    let draftScore = $state('');

    let addField = $state('');
    let addValue = $state('');
    let addScore = $state('');
    let addEnd = $state('tail');
    const endOptions = [
        { value: 'tail', label: 'Append to tail' },
        { value: 'head', label: 'Prepend to head' },
    ];

    const score = (s: string) => (s.trim() !== '' && Number.isFinite(Number(s)) ? Number(s) : undefined);

    function startEdit(m: KeyMember) {
        editingMember = m;
        draftField = m.field?.text ?? '';
        draftValue = m.value.text;
        draftScore = String(m.score ?? '');
    }
    async function save(m: KeyMember) {
        const base = { key: detail.key, value: draftValue, oldValue: m.value.text };
        const edit: KeyEdit =
            detail.type === 'hash'
                ? { ...base, op: 'hset', field: draftField, oldField: m.field?.text }
                : detail.type === 'list'
                  ? { ...base, op: 'lset', index: m.index }
                  : detail.type === 'zset'
                    ? { ...base, op: 'zadd', score: score(draftScore), oldScore: m.score }
                    : { ...base, op: 'sadd' };
        if (await onedit?.(edit)) editingMember = null;
    }
    async function remove(m: KeyMember) {
        const label = detail.type === 'hash' ? `field "${m.field?.text}"` : detail.type === 'list' ? `element ${m.index}` : `member "${m.value.text}"`;
        const last = detail.length === 1 ? ' It is the last one, so the key will be deleted too.' : '';
        if (!window.confirm(`Remove ${label}?${last}`)) return;
        const edit: KeyEdit =
            detail.type === 'hash'
                ? { key: detail.key, op: 'hdel', field: m.field?.text, value: '', oldValue: m.value.text }
                : detail.type === 'list'
                  ? { key: detail.key, op: 'lrem', index: m.index, value: '', oldValue: m.value.text }
                  : detail.type === 'zset'
                    ? { key: detail.key, op: 'zrem', value: m.value.text, oldScore: m.score }
                    : { key: detail.key, op: 'srem', value: m.value.text };
        await onedit?.(edit);
    }
    async function add() {
        const edit: KeyEdit =
            detail.type === 'hash'
                ? { key: detail.key, op: 'hset', field: addField, value: addValue }
                : detail.type === 'list'
                  ? { key: detail.key, op: addEnd === 'head' ? 'lpush' : 'rpush', value: addValue }
                  : detail.type === 'zset'
                    ? { key: detail.key, op: 'zadd', value: addValue, score: score(addScore) }
                    : { key: detail.key, op: 'sadd', value: addValue };
        if (await onedit?.(edit)) {
            addField = '';
            addValue = '';
            addScore = '';
        }
    }
    let canAdd = $derived((detail.type !== 'hash' || addField !== '') && (detail.type !== 'zset' || score(addScore) !== undefined));
    let canSave = $derived((detail.type !== 'hash' || draftField !== '') && (detail.type !== 'zset' || score(draftScore) !== undefined));
</script>

<div class="key-members">
    <label class="search-field filter-field">
        <Icon name="search" size={14} />
        <input aria-label="Search loaded members" placeholder="Search loaded…" bind:value={filter} />
    </label>
    <div class="inspector-content">
        {#if shown.length}<table class="metadata-table key-member-table">
                <thead>
                    <tr>
                        {#each columns as column}<th>{column}</th>{/each}
                        {#if editing}<th aria-label="Actions"></th>{/if}
                    </tr>
                </thead>
                <tbody>
                    {#each shown as member}<tr>
                            {#if editing && editingMember === member}
                                {#if detail.type === 'hash'}<td><input class="key-edit-input" aria-label="Field" bind:value={draftField} /></td>{/if}
                                {#if detail.type === 'list'}<td>{member.index}</td>{/if}
                                <td><input class="key-edit-input" aria-label={detail.type === 'hash' || detail.type === 'list' ? 'Value' : 'Member'} bind:value={draftValue} /></td>
                                {#if detail.type === 'zset'}<td><input class="key-edit-input" aria-label="Score" inputmode="decimal" bind:value={draftScore} /></td>{/if}
                                <td class="key-row-actions">
                                    <Button icon="check" label="Save" disabled={busy || !canSave} onclick={() => save(member)} />
                                    <Button icon="close" label="Cancel edit" disabled={busy} onclick={() => (editingMember = null)} />
                                </td>
                            {:else}
                                {#each cells(member) as cell}<td class:key-binary={cell.binary} title={cell.binary ? 'Binary value, shown as hex' : undefined}>
                                        {#if cell.text === ''}<span class="empty-cell">empty</span>{:else}{cell.text}{/if}
                                    </td>{/each}
                                {#if editing}<td class="key-row-actions">
                                        {#if binary(member)}<span class="field-hint" title="Binary values cannot be edited here">binary</span>{:else}<Button
                                                icon="wand"
                                                label="Edit"
                                                disabled={busy}
                                                onclick={() => startEdit(member)}
                                            />
                                            <Button icon="trash" label="Remove" disabled={busy} onclick={() => remove(member)} />{/if}
                                    </td>{/if}
                            {/if}
                        </tr>{/each}
                </tbody>
            </table>{:else}<p class="field-hint">{filter ? 'Nothing loaded matches this search.' : 'No members.'}</p>{/if}
    </div>
    {#if editing}<form
            class="key-add-form"
            aria-label="Add member"
            onsubmit={(e) => {
                e.preventDefault();
                if (canAdd && !busy) add();
            }}
        >
            {#if detail.type === 'hash'}<input class="key-edit-input" aria-label="New field" placeholder="Field" bind:value={addField} />{/if}
            <input
                class="key-edit-input"
                aria-label={detail.type === 'hash' || detail.type === 'list' ? 'New value' : 'New member'}
                placeholder={detail.type === 'hash' || detail.type === 'list' ? 'Value' : 'Member'}
                bind:value={addValue}
            />
            {#if detail.type === 'zset'}<input class="key-edit-input" aria-label="New score" placeholder="Score" inputmode="decimal" bind:value={addScore} />{/if}
            {#if detail.type === 'list'}<Select label="Insert at" compact options={endOptions} bind:value={addEnd} />{/if}
            <Button type="submit" variant="outline" icon="plus" disabled={busy || !canAdd}>Add</Button>
        </form>{/if}
    <footer>
        <small>
            Loaded {detail.members.length.toLocaleString()} of {detail.length.toLocaleString()}{filter ? ` · ${shown.length.toLocaleString()} shown` : ''}
        </small>
        {#if detail.cursor}<Button variant="outline" disabled={loading} onclick={onmore}>{loading ? 'Loading…' : 'Load more'}</Button>{/if}
    </footer>
</div>
