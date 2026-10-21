<script lang="ts">
    import type { Document, Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Modal from '../../components/Modal.svelte';
    import Button from '../../components/Button.svelte';
    import Input from '../../components/Input.svelte';
    let { session, document }: { session: Session; document: Document } = $props();
    // The prompt mounts only after paramPrompt is populated. Seed bindings
    // before the first render so Input never receives an undefined checkbox.
    // svelte-ignore state_referenced_locally -- each prompt mounts with its own initial values
    let values = $state<Record<string, string>>(Object.fromEntries(document.paramPrompt.map((name) => [name, document.paramValues[name] ?? ''])));
    // svelte-ignore state_referenced_locally -- the checkbox state belongs to this prompt instance
    let nulls = $state<Record<string, boolean>>(Object.fromEntries(document.paramPrompt.map((name) => [name, document.paramValues[name] === null])));
    function submit(e: SubmitEvent) {
        e.preventDefault();
        const result: Record<string, string | null> = {};
        for (const name of document.paramPrompt) result[name] = nulls[name] ? null : values[name];
        workspace.submitParams(session, document, result);
    }
</script>

<Modal
    title="Run with parameters"
    subtitle={`This statement uses ${document.paramPrompt.length === 1 ? 'a named parameter' : 'named parameters'}. Provide a value for each before running.`}
    onclose={() => workspace.cancelParamPrompt(document)}
>
    <form class="param-prompt" onsubmit={submit}>
        {#each document.paramPrompt as name}<div class="param-row">
                <span class="param-name"><code>:{name}</code></span>
                <input type="text" bind:value={values[name]} disabled={nulls[name]} placeholder="Value" autocomplete="off" aria-label={`Value for :${name}`} />
                <Input type="checkbox" bind:checked={nulls[name]} ariaLabel={`Set :${name} to NULL`} label="NULL" />
            </div>{/each}
        <p class="param-hint">Values are bound as query parameters, not inserted into the SQL text. Plain numbers are sent as numbers; anything else is sent as text.</p>
        <div class="param-actions">
            <Button type="button" variant="outline" onclick={() => workspace.cancelParamPrompt(document)}>Cancel</Button>
            <Button type="submit" variant="primary" icon="play">Run</Button>
        </div>
    </form>
</Modal>

<style>
    .param-prompt {
        display: flex;
        flex-direction: column;
        gap: 12px;
        padding: 4px 20px 20px;
    }
    .param-row {
        display: flex;
        align-items: center;
        gap: 10px;
    }
    .param-name {
        flex: 0 0 auto;
        min-width: 90px;
        font-weight: 600;
    }
    .param-row input[type='text'] {
        flex: 1;
    }
    .param-hint {
        margin: 0;
        color: var(--muted);
        font-size: 12px;
    }
    .param-actions {
        display: flex;
        justify-content: flex-end;
        gap: 8px;
    }
</style>
