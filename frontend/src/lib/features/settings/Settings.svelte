<script lang="ts">
    import { workspace, defaultPreferences } from '../../state/workspace.svelte';
    import { fields, type SettingSection } from './fields';
    import Modal from '../../components/Modal.svelte';
    import Icon, { type IconName } from '../../components/Icon.svelte';
    import Button from '../../components/Button.svelte';
    import SettingField from './SettingField.svelte';
    import { display } from '../results/format';
    import type { Theme } from '../../state/types';
    import { stageBadge } from '../../state/stage';
    let section = $state<SettingSection>('Appearance');
    let search = $state('');
    const themes: { value: Theme; label: string; icon: IconName }[] = [
        { value: 'monochrome-light', label: 'Monochrome Light', icon: 'sun' },
        { value: 'monochrome', label: 'Monochrome', icon: 'moon' },
        { value: 'system', label: 'System', icon: 'monitor' },
    ];
    const sections: { title: SettingSection; icon: IconName }[] = [
        { title: 'Appearance', icon: 'sun' },
        { title: 'Editor', icon: 'code' },
        { title: 'Results', icon: 'table' },
        { title: 'Connections', icon: 'database' },
        { title: 'Shortcuts', icon: 'key' },
        { title: 'Privacy', icon: 'key' },
        { title: 'Advanced', icon: 'settings' },
    ];
    let visible = $derived(
        fields.filter(
            (f) => (f.available?.() ?? true) && (search ? `${f.label} ${f.description} ${f.section}`.toLowerCase().includes(search.toLowerCase()) : f.section === section),
        ),
    );
    function reset() {
        for (const field of fields.filter((f) => f.section === section)) Object.assign(workspace.preferences, { [field.key]: defaultPreferences[field.key] });
        workspace.notify(`${section} preferences reset.`);
    }
</script>

<Modal title="Settings" subtitle="Make yourself at home." onclose={() => (workspace.settings = false)} wide>
    <div class="settings-layout">
        <aside class="settings-nav">
            <label class="search-field">
                <Icon name="search" size={14} />
                <input aria-label="Search settings" placeholder="Find a setting…" bind:value={search} />
            </label>
            {#each sections as item}<button
                    class:active={section === item.title && !search}
                    onclick={() => {
                        section = item.title;
                        search = '';
                    }}
                >
                    <Icon name={item.icon} />{item.title}
                </button>{/each}
            <div class="settings-nav-bottom">
                Quarro {workspace.appInfo?.version ?? ''}{#if stageBadge}<span class="preview-badge">{stageBadge}</span>{/if}
                <small>Preferences stay on this device.</small>
            </div>
        </aside>
        <div class="settings-content">
            <div class="settings-heading">
                <div>
                    <span class="eyebrow">YOUR WORKSPACE, YOUR WAY</span>
                    <h2>{search ? 'Search results' : section}</h2>
                </div>
                {#if !search && !['Shortcuts', 'Advanced'].includes(section)}<Button icon="refresh" label={`Reset ${section}`} onclick={reset}>Reset</Button>{/if}
            </div>
            {#if !search && section === 'Appearance'}<h3 class="setting-group-title">Color theme</h3>
                <p class="muted">A comfortable view, any time of day.</p>
                <div class="theme-options">
                    {#each themes as theme}<button
                            class="theme-option"
                            class:active={workspace.preferences.theme === theme.value}
                            aria-pressed={workspace.preferences.theme === theme.value}
                            onclick={() => (workspace.preferences.theme = theme.value)}
                        >
                            <div class="theme-mini {theme.value}">
                                <div class="mini-bar"></div>
                                <div class="mini-body">
                                    <div class="mini-sidebar"></div>
                                    <div class="mini-lines">
                                        <i></i>
                                        <i></i>
                                        <i></i>
                                    </div>
                                </div>
                            </div>
                            <span>
                                <Icon name={theme.icon as IconName} size={14} />{theme.label}{#if workspace.preferences.theme === theme.value}<Icon name="check" size={14} />{/if}
                            </span>
                        </button>{/each}
                </div>{/if}
            {#each visible as field}<SettingField {field} />{/each}
            {#if search && !visible.length}<div class="empty-state">
                    <Icon name="search" size={28} />
                    <h3>No matching settings</h3>
                    <p>Try “font”, “rows”, or “history”.</p>
                </div>{/if}
            {#if !search && ['Appearance', 'Editor', 'Results'].includes(section)}<div class="appearance-preview">
                    <span class="eyebrow">LIVE PREVIEW</span>
                    <pre class="sql-preview"><span>SELECT</span> id, name, last_login
<span>FROM</span> users <span>WHERE</span> name = <em>'Sofía'</em>;</pre>
                    <table>
                        <thead>
                            <tr>
                                <th>id</th>
                                <th>name</th>
                                <th>last_login</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td>42</td>
                                <td>Sofía García</td>
                                <td class="null-cell">{workspace.preferences.nullDisplay || 'NULL'}</td>
                            </tr>
                            <tr>
                                <td>43</td>
                                <td>Amir Saleh</td>
                                <td>{display('2026-09-14T09:00:00.000Z', workspace.preferences)}</td>
                            </tr>
                        </tbody>
                    </table>
                </div>{/if}
            {#if !search && section === 'Connections'}<div class="inline-note">
                    <Icon name="database" />Connections currently open local demo workspaces. Network timeouts and credential settings will arrive with real database providers.
                </div>{/if}
            {#if !search && section === 'Shortcuts'}<p class="muted">Use Ctrl on Windows/Linux or ⌘ on macOS.</p>
                {#each [['Run demo preview', 'Ctrl / ⌘ + Enter'], ['New query', 'Ctrl / ⌘ + Alt + N'], ['Save query', 'Ctrl / ⌘ + S'], ['Open settings', 'Ctrl / ⌘ + ,'], ['Reset zoom', 'Ctrl / ⌘ + 0'], ['Close dialog', 'Escape'], ['Editor undo / redo', 'Ctrl / ⌘ + Z / Shift + Z'], ['Editor completion', 'Ctrl + Space'], ['Grid navigation', 'Arrow keys'], ['Inspect selected row', 'Enter'], ['Copy selected cell', 'Ctrl / ⌘ + C']] as [label, key]}<div
                        class="shortcut-row"
                    >
                        <span>{label}</span>
                        <kbd>{key}</kbd>
                    </div>{/each}{/if}
            {#if !search && section === 'Privacy'}<div class="setting-row">
                    <div>
                        <span class="setting-label">Clear history</span>
                        <p>Remove recorded demo runs. Saved queries are kept.</p>
                    </div>
                    <Button
                        variant="outline"
                        onclick={() => {
                            workspace.history = [];
                            workspace.notify('Query history cleared.');
                        }}
                    >
                        Clear history
                    </Button>
                </div>
                <div class="inline-note">
                    No analytics are collected. The only request Quarro makes on its own is the optional daily update check above. Drafts and saved queries may contain sensitive
                    text; they are stored in this application's local browser storage.
                </div>{/if}
            {#if !search && section === 'Advanced'}<div class="setting-row">
                    <div>
                        <span class="setting-label">Reset workspace layout</span>
                        <p>Restore default panel sizes in the active session.</p>
                    </div>
                    <Button
                        variant="outline"
                        onclick={() => {
                            workspace.resetLayout();
                            workspace.notify('Workspace layout reset.');
                        }}
                    >
                        Reset layout
                    </Button>
                </div>
                <div class="setting-row">
                    <div>
                        <span class="setting-label">Reset all preferences</span>
                        <p>Restore appearance and behavior defaults.</p>
                    </div>
                    <Button
                        variant="outline"
                        onclick={() => {
                            workspace.preferences = { ...defaultPreferences };
                            workspace.notify('Preferences reset.');
                        }}
                    >
                        Reset preferences
                    </Button>
                </div>
                <div class="inline-note">This is the interface preview. Database execution, schema changes, and secure credential storage are not implemented yet.</div>{/if}
        </div>
    </div>
</Modal>
