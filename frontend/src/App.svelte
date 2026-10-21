<script lang="ts">
    import { onMount, untrack } from 'svelte';
    import { Events } from '@wailsio/runtime';
    import { openExternal, setWindowBackground } from './lib/state/backend';
    import { effectiveRowHeight } from './lib/state/appearance';
    import { workspace } from './lib/state/workspace.svelte';
    import SessionBar from './lib/features/workspace/SessionBar.svelte';
    import Workspace from './lib/features/workspace/Workspace.svelte';
    import ConnectionManager from './lib/features/connections/ConnectionManager.svelte';
    import Settings from './lib/features/settings/Settings.svelte';
    import CommandPalette from './lib/features/workspace/CommandPalette.svelte';
    import Button from './lib/components/Button.svelte';
    import Icon from './lib/components/Icon.svelte';
    import type { Theme } from './lib/state/types';
    import './lib/styles/theme.css';
    import './lib/styles/app.css';
    const themeLabels: Partial<Record<Theme, string>> = {
        'monochrome-light': 'Monochrome Light',
        monochrome: 'Monochrome',
    };
    let systemDark = $state(window.matchMedia('(prefers-color-scheme: dark)').matches);
    let theme = $derived(workspace.preferences.theme === 'system' ? (systemDark ? 'monochrome' : 'monochrome-light') : workspace.preferences.theme);
    onMount(() => {
        workspace.loadProfiles();
        workspace.loadDrivers();
        workspace.loadDesktop();
        workspace.loadAppInfo();
        // macOS Quarro → Settings… (see internal/desktop/menu.go).
        const offOpenSettings = Events.On('quarro:open-settings', () => (workspace.settings = true));
        const narrow = window.matchMedia('(max-width: 1000px)');
        const collapse = () => {
            if (narrow.matches)
                for (const session of workspace.sessions) {
                    session.sidebar = false;
                    session.inspector = false;
                }
        };
        collapse();
        narrow.addEventListener('change', collapse);
        const media = window.matchMedia('(prefers-color-scheme: dark)');
        const change = (e: MediaQueryListEvent) => (systemDark = e.matches);
        media.addEventListener('change', change);
        const save = () => workspace.persist();
        window.addEventListener('pagehide', save);
        return () => {
            narrow.removeEventListener('change', collapse);
            media.removeEventListener('change', change);
            window.removeEventListener('pagehide', save);
            offOpenSettings();
        };
    });
    $effect(() => {
        const snapshot = JSON.stringify(workspace.persistenceSnapshot());
        const timer = setTimeout(() => {
            if (snapshot) workspace.persist();
        }, 250);
        return () => clearTimeout(timer);
    });
    $effect(() => {
        const p = workspace.preferences;
        const root = document.documentElement;
        root.dataset.theme = theme;
        root.dataset.density = p.density;
        root.dataset.motion = p.reducedMotion ? 'reduced' : 'normal';
        const font = (name: string, fallback: string) => `${JSON.stringify(name || fallback)}, ${fallback}`;
        const styles: Record<string, string> = {
            '--app-zoom': String(p.zoom / 100),
            '--ui-font': font(p.uiFont, 'sans-serif'),
            '--ui-size': `${p.uiSize}px`,
            '--editor-font': font(p.editorFont, 'monospace'),
            '--editor-size': `${p.editorSize}px`,
            '--editor-line-height': String(p.lineHeight),
            '--editor-ligatures': p.ligatures ? 'normal' : 'none',
            '--grid-font': font(p.gridFont, 'sans-serif'),
            '--grid-size': `${p.gridSize}px`,
            '--grid-row-height': `${effectiveRowHeight(p)}px`,
        };
        for (const [key, value] of Object.entries(styles)) root.style.setProperty(key, value);
    });
    $effect(() => {
        // Waits for the version, so a cached release equal to it never shows,
        // and re-runs when the Check for updates setting is toggled.
        void workspace.preferences.checkUpdates;
        if (!workspace.appInfo) return;
        untrack(() => workspace.checkForUpdates());
    });
    $effect(() => {
        const titleBar = workspace.preferences.titleBar;
        untrack(() => workspace.applyTitleBar(titleBar));
    });
    $effect(() => {
        const appIcon = workspace.preferences.appIcon;
        untrack(() => workspace.applyAppIcon(appIcon));
    });
    $effect(() => {
        // Runs after the effect above sets data-theme, so --bg is the new
        // theme's. Go paints it before the next launch's first frame.
        void theme;
        if (!workspace.desktop) return;
        const background = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim();
        if (/^#[0-9a-f]{6}$/i.test(background)) setWindowBackground(background).catch(() => {});
    });
    $effect(() => {
        const zoom = workspace.preferences.zoom;
        if (window.innerWidth / (zoom / 100) < 1000) {
            untrack(() => {
                for (const session of workspace.sessions) session.sidebar = false;
            });
        }
    });
    $effect(() => {
        if (workspace.notice) {
            const timer = setTimeout(() => (workspace.notice = ''), 4200);
            return () => clearTimeout(timer);
        }
    });
    function isEditableTarget(target: EventTarget | null) {
        if (!(target instanceof HTMLElement)) return false;
        return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable || !!target.closest('.cm-editor');
    }
    function shortcuts(e: KeyboardEvent) {
        // Discoverable, OS-convention shortcuts: ⌘/Ctrl on every entry below
        // (the modifier key itself already differs per OS at the browser
        // level), plus a bare "/" to jump into object search like most
        // developer tools, only outside editable fields so typing "/" in the
        // editor or a form still inserts the character.
        if (!(e.ctrlKey || e.metaKey)) {
            if (e.key === '/' && !isEditableTarget(e.target)) {
                e.preventDefault();
                window.dispatchEvent(new CustomEvent('quarro:focus-object-search'));
            }
            return;
        }
        if (e.key.toLowerCase() === 'k') {
            e.preventDefault();
            workspace.commandPalette = !workspace.commandPalette;
            return;
        }
        if (e.key === ',') {
            e.preventDefault();
            workspace.settings = true;
        }
        if (e.key === '0') {
            e.preventDefault();
            workspace.preferences.zoom = 100;
        }
        if (workspace.settings || workspace.connections || workspace.commandPalette) return;
        const session = workspace.session;
        const doc = workspace.document;
        if (e.key.toLowerCase() === 't' && session) {
            e.preventDefault();
            workspace.openDocument(session);
        }
        if (e.key.toLowerCase() === 'w' && session && doc) {
            e.preventDefault();
            workspace.closeDocument(session, doc);
        }
        if (e.key === 's' && session && doc?.kind === 'query') {
            e.preventDefault();
            workspace.save(session, doc);
        }
        if (e.key === '.' && doc?.status === 'running') {
            e.preventDefault();
            workspace.stop(doc);
        }
        if (e.key === 'Tab' && workspace.sessions.length > 1) {
            e.preventDefault();
            workspace.nextSession(e.shiftKey ? -1 : 1);
        }
    }
</script>

<svelte:window onkeydown={shortcuts} />
<div class="app-shell">
    <SessionBar />
    <div class="sessions-host">
        {#each workspace.sessions as session (session.id)}<div class="session-host" hidden={workspace.active !== session.id}>
                <Workspace {session} />
            </div>{/each}{#if !workspace.sessions.length}<main class="empty-state welcome">
                <span class="empty-icon"><Icon name="database" size={36} /></span>
                <span class="eyebrow">WELCOME TO QUARRO</span>
                <h1>Your data. A little clearer.</h1>
                <p>Open a connection to start exploring your data.</p>
                <Button variant="primary" icon="plus" onclick={() => (workspace.connections = true)}>Open a connection</Button>
            </main>{/if}
    </div>
    <footer class="status-bar">
        <span>
            <span class="status-dot" class:offline={workspace.session?.connectionState !== 'connected'}></span>
            {workspace.session
                ? workspace.session.connectionState === 'connected'
                    ? 'Connected'
                    : workspace.session.connectionState === 'connecting'
                      ? 'Connecting…'
                      : workspace.session.connectionState === 'error'
                        ? 'Connection error'
                        : 'Disconnected'
                : 'No session'}{#if workspace.session}<span class="status-separator">/</span>
                {workspace.session.profile.environment}{/if}
        </span>
        <span class="status-center">
            {#if workspace.session}<Icon name={workspace.session.profile.readOnly ? 'key' : 'database'} size={12} />
                <span class:read-only-session={workspace.session.profile.readOnly}>
                    {workspace.session.profile.readOnly ? 'Read-only session' : 'Writable session'}
                </span>
                <span class="status-separator">|</span>{/if}
            UTF-8
        </span>
        <span>
            <button onclick={() => (workspace.commandPalette = true)} title="Open the command palette">
                <Icon name="command" size={12} />Commands
                <kbd>⌘K</kbd>
            </button>
            <span class="status-separator">|</span>
            <Icon name="clock" size={12} />{workspace.session ? workspace.session.documents.filter((d) => d.status === 'running').length : 0} running{#if workspace.sessions.length > 1}
                <span class="background-jobs-hint">
                    ({workspace.sessions.flatMap((s) => s.documents).filter((d) => d.status === 'running').length} total)
                </span>{/if}
            <span class="status-separator">|</span>
            <button onclick={() => (workspace.settings = true)}>
                {themeLabels[theme] ?? 'System'} theme
            </button>
        </span>
    </footer>
</div>
{#if workspace.connections}<ConnectionManager />{/if}{#if workspace.settings}<Settings />{/if}
{#if workspace.commandPalette}<CommandPalette />{/if}
{#if workspace.notice}<div class="toast" role="status">
        <Icon name="check" />
        <span>{workspace.notice}</span>
        <Button icon="close" label="Dismiss notification" onclick={() => (workspace.notice = '')} />
    </div>{/if}
{#if workspace.update}<div class="update-notice" role="status">
        <Icon name="download" />
        <span>Quarro {workspace.update.version} is available.</span>
        <Button variant="outline" onclick={() => workspace.update && openExternal(workspace.update.url)}>View release</Button>
        <Button icon="close" label="Dismiss update notice" onclick={() => workspace.dismissUpdate()} />
    </div>{/if}
{#if workspace.storageError}<div class="storage-warning" role="alert">Workspace storage is unavailable or full. Changes may not survive restart.</div>{/if}
