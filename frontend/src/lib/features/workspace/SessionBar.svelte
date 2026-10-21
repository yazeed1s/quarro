<script lang="ts">
    import { workspace } from '../../state/workspace.svelte';
    import { stageBadge } from '../../state/stage';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import { closeOnOutsideClick } from '../../components/dropdown';
    import WindowControls from './WindowControls.svelte';
    import { Window } from '@wailsio/runtime';
    // brand/generate_icons.py renders each mark at 1x/2x/3x for the compact 20px slot.
    import darkMark from '../../../../../brand/ui/header-dark-20.png';
    import darkMark2x from '../../../../../brand/ui/header-dark-20@2x.png';
    import darkMark3x from '../../../../../brand/ui/header-dark-20@3x.png';
    import lightMark from '../../../../../brand/ui/header-light-20.png';
    import lightMark2x from '../../../../../brand/ui/header-light-20@2x.png';
    import lightMark3x from '../../../../../brand/ui/header-light-20@3x.png';
    // The session bar is the window's title bar whenever the OS isn't drawing
    // one above it: frameless windows, and macOS's hidden-inset title bar.
    let titlebar = $derived(Boolean(workspace.desktop?.frameless || workspace.desktop?.trafficLights));
    // Windows' caption buttons sit flush against the window's top-right
    // corner, outside the padded actions group.
    let captionButtons = $derived(Boolean(workspace.desktop?.frameless && workspace.desktop.platform === 'windows'));
    // macOS hides the traffic lights in fullscreen, so their space is freed.
    let fullscreen = $state(false);
    $effect(() => {
        if (!workspace.desktop?.trafficLights) return;
        const sync = () =>
            Window.IsFullscreen().then(
                (value) => (fullscreen = value),
                () => {},
            );
        sync();
        window.addEventListener('resize', sync);
        return () => window.removeEventListener('resize', sync);
    });
    function maximiseFromBar(e: MouseEvent) {
        // Wails' runtime already turns this into the native title-bar
        // double-click on macOS; frameless windows have to do it themselves.
        if (!workspace.desktop?.frameless || !(e.target instanceof Element)) return;
        if (getComputedStyle(e.target).getPropertyValue('--wails-draggable').trim() !== 'drag') return;
        Window.ToggleMaximise();
    }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions (Double-click to maximise is a pointer shortcut; the maximise button and OS shortcuts cover keyboard use.) -->
<header
    class="session-bar"
    class:titlebar
    class:caption-buttons={captionButtons}
    class:traffic-lights={workspace.desktop?.trafficLights && !fullscreen}
    ondblclick={maximiseFromBar}
>
    <button class="brand" onclick={() => (workspace.connections = true)} aria-label="Quarro connection manager">
        <span class="brand-icon">
            <img class="brand-mark brand-mark-dark-asset" src={darkMark} srcset="{darkMark2x} 2x, {darkMark3x} 3x" width="20" height="20" alt="" />
            <img class="brand-mark brand-mark-light-asset" src={lightMark} srcset="{lightMark2x} 2x, {lightMark3x} 3x" width="20" height="20" alt="" />
        </span>
    </button>
    <nav class="session-tabs" aria-label="Database sessions">
        {#each workspace.sessions as session (session.id)}
            <div class="session-tab" class:selected={workspace.active === session.id}>
                <button
                    class="session-select"
                    aria-current={workspace.active === session.id ? 'page' : undefined}
                    onclick={() => (workspace.active = session.id)}
                    title={`${workspace.engineDisplayName(session.profile.engine)} · ${session.profile.name}`}
                >
                    <span class="engine-mark {session.profile.engine}">
                        <Icon name="database" size={14} />
                    </span>
                    <span class="truncate">{session.profile.name}</span>
                    <span class="status-dot" class:offline={session.connectionState !== 'connected'} class:connecting={session.connectionState === 'connecting'}></span>
                </button>
                <Button icon="close" label={`Close session ${session.profile.name}`} onclick={() => workspace.closeSession(session)} />
            </div>
        {/each}
    </nav>
    <Button icon="plus" label="New session" onclick={() => (workspace.connections = true)} />
    <details class="dropdown session-overflow" use:closeOnOutsideClick>
        <summary title="All sessions" aria-label="All sessions"><Icon name="more" /></summary>
        <div class="menu">
            {#each workspace.sessions as session}<button onclick={() => (workspace.active = session.id)}>
                    {session.profile.name}{#if session.id === workspace.active}<Icon name="check" />{/if}
                </button>{/each}
            <button onclick={() => (workspace.connections = true)}>Manage connections</button>
        </div>
    </details>
    <div class="session-actions">
        {#if stageBadge}<span class="preview-badge">{stageBadge}</span>{/if}
        <Button icon="settings" label="Settings" onclick={() => (workspace.settings = true)} />
        {#if workspace.desktop?.frameless && !captionButtons}<WindowControls />{/if}
    </div>
    {#if captionButtons}<WindowControls />{/if}
</header>
