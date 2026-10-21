<script lang="ts">
    import { onMount, type Snippet } from 'svelte';
    import Button from './Button.svelte';
    let {
        title,
        subtitle = '',
        onclose,
        children,
        wide = false,
    }: {
        title: string;
        subtitle?: string;
        onclose: () => void;
        children: Snippet;
        wide?: boolean;
    } = $props();
    let dialog: HTMLDialogElement;
    onMount(() => {
        dialog.showModal();
        return () => dialog.close();
    });
</script>

<dialog bind:this={dialog} class:wide oncancel={onclose} aria-label={title}>
    <header class="modal-header">
        <div>
            <h2>{title}</h2>
            {#if subtitle}<p>{subtitle}</p>{/if}
        </div>
        <Button icon="close" label="Close dialog" onclick={onclose} />
    </header>
    {@render children()}
</dialog>
