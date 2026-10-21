<script lang="ts">
    import { onMount } from 'svelte';
    import { objectLabel } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Icon from '../../components/Icon.svelte';
    import type { IconName } from '../../components/Icon.svelte';
    interface Command {
        id: string;
        label: string;
        hint?: string;
        icon: IconName;
        disabled?: boolean;
        run: () => void;
    }
    let query = $state('');
    let selected = $state(0);
    let input: HTMLInputElement;
    function close() {
        workspace.commandPalette = false;
    }
    onMount(() => input?.focus());
    function run(command: Command) {
        if (command.disabled) return;
        close();
        command.run();
    }
    let session = $derived(workspace.session);
    let document = $derived(workspace.document);
    let commands = $derived.by((): Command[] => {
        const list: Command[] = [];
        const s = session;
        const d = document;
        if (s) {
            list.push({
                id: 'new-query',
                label: 'New query',
                hint: '⌘T',
                icon: 'plus',
                run: () => workspace.openDocument(s),
            });
            if (d) {
                list.push({
                    id: 'close-document',
                    label: 'Close current document',
                    hint: '⌘W',
                    icon: 'close',
                    run: () => workspace.closeDocument(s, d),
                });
            }
            if (d?.kind === 'query') {
                const native = s.capabilities.nativeQueryExecutor && !s.capabilities.sqlExecutor;
                list.push(
                    {
                        id: 'run',
                        label: 'Run query',
                        hint: '⌘↵',
                        icon: 'play',
                        disabled: s.connectionState !== 'connected' || d.status === 'running',
                        run: () => (native ? workspace.runNativeQuery(s, d) : workspace.run(s, d, d.selectionFrom === d.selectionTo ? 'statement' : 'selection')),
                    },
                    {
                        id: 'stop',
                        label: 'Stop running query',
                        hint: '⌘.',
                        icon: 'stop',
                        disabled: d.status !== 'running',
                        run: () => workspace.stop(d),
                    },
                    {
                        id: 'save-query',
                        label: 'Save query',
                        hint: '⌘S',
                        icon: 'bookmark',
                        run: () => workspace.save(s, d),
                    },
                );
                if (!native)
                    list.push(
                        {
                            id: 'run-all',
                            label: 'Run all statements',
                            icon: 'play',
                            disabled: s.connectionState !== 'connected' || d.status === 'running',
                            run: () => workspace.run(s, d, 'all'),
                        },
                        {
                            id: 'format-sql',
                            label: 'Format SQL',
                            icon: 'wand',
                            run: () => workspace.formatQuery(s, d),
                        },
                        {
                            id: 'go-to-definition',
                            label: 'Go to definition',
                            hint: 'F12',
                            icon: 'table',
                            run: () => workspace.goToDefinition(s, d),
                        },
                    );
            }
            list.push(
                {
                    id: 'diagram',
                    label: 'Open schema diagram',
                    icon: 'diagram',
                    run: () => workspace.openDiagram(s),
                },
                {
                    id: 'toggle-sidebar',
                    label: s.sidebar ? 'Hide sidebar' : 'Show sidebar',
                    icon: 'panel',
                    run: () => (s.sidebar = !s.sidebar),
                },
                {
                    id: 'toggle-inspector',
                    label: s.inspector ? 'Hide row inspector' : 'Show row inspector',
                    icon: 'panel',
                    run: () => (s.inspector = !s.inspector),
                },
                {
                    id: 'reset-layout',
                    label: 'Reset panel layout',
                    icon: 'refresh',
                    run: () => workspace.resetLayout(),
                },
                {
                    id: 'focus-search',
                    label: 'Focus object search',
                    hint: '/',
                    icon: 'search',
                    run: () => window.dispatchEvent(new CustomEvent('quarro:focus-object-search')),
                },
                {
                    id: 'clear-history',
                    label: 'Clear query history for this connection',
                    icon: 'trash',
                    run: () => workspace.clearHistory(s.profile.id),
                },
                {
                    id: 'reconnect',
                    label: s.connectionState === 'connected' ? 'Disconnect session' : 'Reconnect session',
                    icon: 'refresh',
                    run: () => (s.connectionState === 'connected' ? workspace.disconnect(s) : workspace.reconnect(s)),
                },
            );
            if (workspace.sessions.length > 1) {
                list.push(
                    {
                        id: 'next-session',
                        label: 'Next session',
                        hint: '⌃⇥',
                        icon: 'arrow',
                        run: () => workspace.nextSession(1),
                    },
                    {
                        id: 'prev-session',
                        label: 'Previous session',
                        hint: '⌃⇧⇥',
                        icon: 'arrow',
                        run: () => workspace.nextSession(-1),
                    },
                );
            }
        }
        list.push(
            {
                id: 'settings',
                label: 'Open settings',
                hint: '⌘,',
                icon: 'settings',
                run: () => (workspace.settings = true),
            },
            {
                id: 'new-connection',
                label: 'New connection',
                icon: 'link',
                run: () => (workspace.connections = true),
            },
        );
        if (s) {
            for (const table of s.schema.filter((t) => t.kind === 'table')) {
                list.push({
                    id: `table:${table.name}`,
                    label: `Go to table: ${objectLabel(table.name)}`,
                    icon: 'table',
                    run: () => workspace.openDocument(s, 'table', table.name),
                });
            }
            for (const saved of workspace.saved.filter((q) => q.profileId === s.profile.id)) {
                list.push({
                    id: `saved:${saved.id}`,
                    label: `Open saved query: ${saved.title}`,
                    icon: 'bookmark',
                    run: () => {
                        const doc = workspace.openDocument(s);
                        doc.sql = saved.sql;
                        doc.title = saved.title;
                        if (saved.object) doc.object = saved.object;
                    },
                });
            }
        }
        return list;
    });
    let filtered = $derived(query ? commands.filter((c) => c.label.toLowerCase().includes(query.toLowerCase())) : commands);
    $effect(() => {
        if (selected >= filtered.length) selected = Math.max(0, filtered.length - 1);
    });
    function onkeydown(e: KeyboardEvent) {
        if (e.key === 'Escape') {
            e.preventDefault();
            close();
        } else if (e.key === 'ArrowDown') {
            e.preventDefault();
            selected = Math.min(filtered.length - 1, selected + 1);
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            selected = Math.max(0, selected - 1);
        } else if (e.key === 'Enter') {
            e.preventDefault();
            const command = filtered[selected];
            if (command) run(command);
        }
    }
</script>

<div class="palette-backdrop" onclick={close} onkeydown={(e) => e.key === 'Escape' && close()} role="button" tabindex="-1" aria-label="Close command palette">
    <div class="palette" role="dialog" aria-label="Command palette" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.key === 'Escape' && close()}>
        <label class="palette-search">
            <Icon name="command" size={16} />
            <input bind:this={input} bind:value={query} {onkeydown} placeholder="Type a command or search…" autocomplete="off" aria-label="Command palette search" />
        </label>
        <div class="palette-list" role="listbox">
            {#each filtered as command, i}<button
                    class="palette-item"
                    class:active={i === selected}
                    class:disabled={command.disabled}
                    role="option"
                    aria-selected={i === selected}
                    onmouseenter={() => (selected = i)}
                    onclick={() => run(command)}
                >
                    <Icon name={command.icon} size={15} />
                    <span class="palette-label">{command.label}</span>
                    {#if command.hint}<kbd>{command.hint}</kbd>{/if}
                </button>{:else}<div class="palette-empty">No matching commands.</div>{/each}
        </div>
    </div>
</div>

<style>
    .palette-backdrop {
        position: fixed;
        inset: 0;
        background: color-mix(in srgb, black 35%, transparent);
        display: flex;
        justify-content: center;
        align-items: flex-start;
        padding-top: max(12vh, env(safe-area-inset-top, 0px));
        z-index: 200;
    }
    .palette {
        width: min(560px, calc(100vw - 32px));
        max-height: 60vh;
        display: flex;
        flex-direction: column;
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: 10px;
        box-shadow: var(--shadow);
        overflow: hidden;
    }
    .palette-search {
        display: flex;
        align-items: center;
        gap: 10px;
        padding: 12px 14px;
        border-bottom: 1px solid var(--border);
        color: var(--muted);
    }
    .palette-search input {
        flex: 1;
        border: none;
        background: none;
        outline: none;
        font-size: 14px;
        color: var(--text);
    }
    .palette-list {
        overflow-y: auto;
        padding: 6px;
    }
    .palette-item {
        display: flex;
        align-items: center;
        gap: 10px;
        width: 100%;
        padding: 8px 10px;
        border: none;
        background: none;
        border-radius: 6px;
        text-align: left;
        color: var(--text);
        cursor: pointer;
    }
    .palette-item.active {
        background: var(--raised);
    }
    .palette-item.disabled {
        opacity: 0.45;
        cursor: default;
    }
    .palette-label {
        flex: 1;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .palette-item kbd {
        font-size: 11px;
        color: var(--muted);
    }
    .palette-empty {
        padding: 20px;
        text-align: center;
        color: var(--muted);
        font-size: 13px;
    }
</style>
