<script lang="ts">
    import { untrack } from 'svelte';
    import type { Document, KeyEdit, Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Modal from '../../components/Modal.svelte';
    import Button from '../../components/Button.svelte';
    import Input from '../../components/Input.svelte';
    import Select from '../../components/Select.svelte';
    import { editableTypes, typeLabel } from './format';

    let { session, document, onclose }: { session: Session; document: Document; onclose: () => void } = $props();

    const typeOptions = editableTypes.map((t) => ({ value: t, label: typeLabel(t) }));
    let type = $state('string');
    // Start inside the keyspace group being browsed, e.g. "user:".
    let key = $state(untrack(() => (document.object ? `${document.object}:` : '')));
    let field = $state('');
    let value = $state('');
    let score = $state('0');
    let error = $state('');
    let busy = $state(false);

    // Redis has no empty hash, list, set, or sorted set, so every new key
    // starts with one element.
    let valueLabel = $derived(type === 'string' ? 'Value' : type === 'hash' ? 'First value' : type === 'list' ? 'First element' : 'First member');
    let scoreValue = $derived(score.trim() !== '' && Number.isFinite(Number(score)) ? Number(score) : undefined);
    let valid = $derived(key !== '' && (type !== 'hash' || field !== '') && (type !== 'zset' || scoreValue !== undefined));

    async function create(e: SubmitEvent) {
        e.preventDefault();
        if (!valid || busy) return;
        busy = true;
        error = '';
        const edit: KeyEdit = { key, op: 'create', type, value, field: type === 'hash' ? field : undefined, score: type === 'zset' ? scoreValue : undefined };
        try {
            const result = await workspace.applyKeyEdit(session, document, edit);
            if (result?.conflict) error = result.message || 'This key already exists.';
            if (!result?.applied) return;
            session.inspector = true;
            onclose();
        } finally {
            busy = false;
        }
    }
</script>

<Modal title="New key" subtitle="Creates the key with its first value." {onclose}>
    <form class="new-key-form" onsubmit={create}>
        <Select label="Key type" options={typeOptions} bind:value={type} />
        <Input label="Key name" bind:value={key} required />
        {#if type === 'hash'}<Input label="First field" bind:value={field} required />{/if}
        {#if type === 'zset'}<Input label="Score" bind:value={score} inputmode="decimal" required />{/if}
        <Input label={valueLabel} bind:value />
        {#if error}<p class="error-text" role="alert">{error}</p>{/if}
        <div class="new-key-actions">
            <Button variant="outline" onclick={onclose}>Cancel</Button>
            <Button type="submit" variant="primary" icon="plus" disabled={!valid || busy}>Create key</Button>
        </div>
    </form>
</Modal>
