<script lang="ts">
    import { onMount } from 'svelte';
    import { Window } from '@wailsio/runtime';
    import Button from '../../components/Button.svelte';
    import { workspace } from '../../state/workspace.svelte';
    let maximised = $state(false);
    // Windows gets caption buttons drawn like the native ones; elsewhere
    // (Linux "Quarro" title bar) they match the rest of the toolbar.
    let windows = $derived(workspace.desktop?.platform === 'windows');
    onMount(() => {
        // Maximise can also happen from the WM (keyboard, double-click on the
        // drag region), so re-read the state on every resize.
        const sync = () =>
            Window.IsMaximised().then(
                (value) => (maximised = value),
                () => {},
            );
        sync();
        window.addEventListener('resize', sync);
        return () => window.removeEventListener('resize', sync);
    });
</script>

{#if windows}
    <!-- Segoe Fluent Icons (Windows 11) / Segoe MDL2 Assets (Windows 10) caption glyphs. -->
    <div class="window-controls caption-buttons">
        <button type="button" class="caption-button" title="Minimise" aria-label="Minimise" onclick={() => Window.Minimise()}>
            <span aria-hidden="true">&#xE921;</span>
        </button>
        <button
            type="button"
            class="caption-button"
            title={maximised ? 'Restore' : 'Maximise'}
            aria-label={maximised ? 'Restore' : 'Maximise'}
            onclick={() => Window.ToggleMaximise()}
        >
            <span aria-hidden="true"
                >{#if maximised}&#xE923;{:else}&#xE922;{/if}</span
            >
        </button>
        <button type="button" class="caption-button close" title="Close window" aria-label="Close window" onclick={() => Window.Close()}>
            <span aria-hidden="true">&#xE8BB;</span>
        </button>
    </div>
{:else}
    <div class="window-controls">
        <Button icon="minimise" label="Minimise" onclick={() => Window.Minimise()} />
        <Button icon={maximised ? 'restore' : 'maximise'} label={maximised ? 'Restore' : 'Maximise'} onclick={() => Window.ToggleMaximise()} />
        <Button icon="close" label="Close window" onclick={() => Window.Close()} />
    </div>
{/if}
