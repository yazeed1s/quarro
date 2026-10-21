<script lang="ts">
    import { workspace } from '../../state/workspace.svelte';
    import { stageBadge } from '../../state/stage';
    import * as backend from '../../state/backend';
    import type { Profile, Engine } from '../../state/types';
    import Modal from '../../components/Modal.svelte';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import DbLogo from '../../components/DbLogo.svelte';
    import Input from '../../components/Input.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';
    import { closeOnOutsideClick } from '../../components/dropdown';

    const environmentOptions: SelectOption[] = [
        { value: 'Local', label: 'Local' },
        { value: 'Dev', label: 'Dev' },
        { value: 'Stage', label: 'Stage' },
        { value: 'Production', label: 'Production' },
    ];
    const tlsModeOptions: SelectOption[] = [
        { value: 'verify-full', label: 'Verify certificate and hostname' },
        { value: 'disable', label: 'Disabled (unencrypted)' },
    ];
    const sshAuthMethodOptions: SelectOption[] = [
        { value: 'password', label: 'Password' },
        { value: 'key', label: 'Private key' },
    ];
    let connectionURL = $state('');
    let importing = $state(false);
    async function importConnection() {
        importing = true;
        try {
            const result = await backend.importURL(connectionURL);
            draft = {
                ...draft,
                ...result.profile,
                id: draft.id,
                folder: draft.folder,
                environment: draft.environment,
                favorite: draft.favorite,
                name: draft.name || result.profile.database,
            };
            password = result.password;
            connectionURL = '';
            testResult = null;
        } catch (e) {
            testResult = { ok: false, message: String(e) };
        } finally {
            importing = false;
        }
    }
    let search = $state('');
    let filter = $state('all');
    let editing = $state(false);
    let draft = $state<Profile>(blank());
    let password = $state('');
    let sshPassword = $state('');
    let sshPassphrase = $state('');
    // A host key seen on a Test connection/Save attempt that has not yet
    // been trusted (draft.sshHostFingerprint is still empty or differs).
    // Never persisted until the user explicitly trusts it; see trustSSHKey.
    let pendingSSHFingerprint = $state('');
    let testing = $state(false);
    let testResult = $state<{ ok: boolean; message: string; sshHostFingerprint?: string } | null>(null);
    let saving = $state(false);
    function blank(): Profile {
        return {
            id: crypto.randomUUID(),
            name: '',
            engine: 'postgres',
            database: '',
            schema: 'public',
            environment: 'Dev',
            folder: 'Personal',
            favorite: false,
            host: 'localhost',
            port: '5432',
            username: '',
            path: '',
            readOnly: false,
            hasPassword: false,
            tlsMode: 'verify-full',
            socket: '',
            tlsCA: '',
            tlsCert: '',
            tlsKey: '',
            tlsServerName: '',
            sshEnabled: false,
            sshHost: '',
            sshPort: '22',
            sshUsername: '',
            sshAuthMethod: 'password',
            sshPrivateKeyPath: '',
            sshHasPassword: false,
            sshHasPassphrase: false,
            sshHostFingerprint: '',
        };
    }
    let matches = $derived(
        workspace.profiles.filter(
            (p) =>
                `${p.name} ${p.database} ${p.folder} ${workspace.engineDisplayName(p.engine)}`.toLowerCase().includes(search.toLowerCase()) &&
                (filter === 'all' || (filter === 'favorites' && p.favorite) || (filter === 'recent' && workspace.sessions.some((s) => s.profile.id === p.id))),
        ),
    );
    function edit(profile: Profile) {
        draft = { ...profile, tlsMode: profile.tlsMode || 'verify-full', sshAuthMethod: profile.sshAuthMethod || 'password' };
        password = '';
        sshPassword = '';
        sshPassphrase = '';
        pendingSSHFingerprint = '';
        testResult = null;
        editing = true;
    }
    // A test or save that tunneled through SSH reports back the host key it
    // saw. Trust is never implicit: a fingerprint the profile doesn't
    // already carry waits in pendingSSHFingerprint for trustSSHKey to
    // confirm, rather than being written to draft automatically.
    function notePossibleNewSSHKey(fingerprint: string | undefined) {
        pendingSSHFingerprint = fingerprint && fingerprint !== draft.sshHostFingerprint ? fingerprint : '';
    }
    function trustSSHKey() {
        draft.sshHostFingerprint = pendingSSHFingerprint;
        pendingSSHFingerprint = '';
    }
    function forgetSSHKey() {
        draft.sshHostFingerprint = '';
        pendingSSHFingerprint = '';
    }
    async function test() {
        testing = true;
        testResult = null;
        try {
            testResult = await backend.testConnection(draft, password || undefined, sshPassword || undefined, sshPassphrase || undefined);
            if (testResult.ok) notePossibleNewSSHKey(testResult.sshHostFingerprint);
        } catch {
            testResult = {
                ok: false,
                message: 'Backend unavailable. Run this app with wails3 dev or a packaged build to test real connections.',
            };
        } finally {
            testing = false;
        }
    }
    async function save() {
        if (draft.engine === 'sqlite') {
            draft.database = draft.path.split('/').pop() || 'database.db';
            draft.schema = 'main';
        }
        saving = true;
        try {
            const result = await backend.saveProfile(draft, password || undefined, sshPassword || undefined, sshPassphrase || undefined);
            const index = workspace.profiles.findIndex((p) => p.id === result.profile.id);
            if (index < 0) workspace.profiles.push(result.profile);
            else workspace.profiles[index] = result.profile;
            editing = false;
            password = '';
            sshPassword = '';
            sshPassphrase = '';
            workspace.notify(
                result.passwordNotPersistent ? 'Connection saved. No OS credential store is available, so the password will only be kept for this session.' : 'Connection saved.',
            );
        } catch (error) {
            testResult = {
                ok: false,
                message: String(error),
            };
        } finally {
            saving = false;
        }
    }
    async function remove(profile: Profile) {
        try {
            await backend.deleteProfile(profile.id);
        } catch {
            workspace.notify('Removing this connection failed.');
            return;
        }
        workspace.profiles = workspace.profiles.filter((p) => p.id !== profile.id);
    }
    async function toggleFavorite(profile: Profile) {
        const updated = { ...profile, favorite: !profile.favorite };
        try {
            const result = await backend.saveProfile(updated);
            const index = workspace.profiles.findIndex((p) => p.id === profile.id);
            if (index >= 0) workspace.profiles[index] = result.profile;
        } catch {
            workspace.notify('Updating this connection failed.');
        }
    }
    function engineChanged(engine: Engine) {
        draft.engine = engine;
        const schema = workspace.drivers.find((d) => d.name === engine)?.configSchema ?? [];
        draft.port = schema.find((f) => f.key === 'port')?.default ?? '';
        draft.schema = engine === 'postgres' ? 'public' : '';
        draft.readOnly = false;
        if (engine === 'mongodb' || engine === 'redis') {
            draft.socket = '';
            draft.tlsMode = 'disable';
        }
        if (engine !== 'postgres' && engine !== 'mysql') {
            draft.sshEnabled = false;
        }
        // Redis has no username, and repurposes "Database" as a numbered index
        // (0-15 by default) rather than a name.
        if (engine === 'redis') {
            draft.username = '';
            draft.database = draft.database || '0';
        }
    }
</script>

<Modal title="Your connections" subtitle="A home for every database you work with." onclose={() => (workspace.connections = false)} wide>
    <div class="connection-manager">
        <aside class="connection-nav">
            <span class="eyebrow">WORKSPACE</span>
            {#each [['all', 'All connections', 'database'], ['favorites', 'Favorites', 'bookmark'], ['recent', 'Open sessions', 'clock']] as [value, label, icon]}<button
                    class:active={filter === value}
                    onclick={() => {
                        filter = value;
                        editing = false;
                    }}
                >
                    <Icon name={icon as 'database'} />{label}
                </button>{/each}
            {#if stageBadge}<div class="connection-nav-bottom">
                    <span class="preview-badge">{stageBadge}</span>
                    <p>Sample data is available in this build. All connections execute real SQL.</p>
                </div>{/if}
        </aside>
        <div class="connection-content">
            {#if editing}<form
                    onsubmit={(e) => {
                        e.preventDefault();
                        save();
                    }}
                >
                    <div class="form-heading">
                        <h3>
                            {workspace.profiles.some((p) => p.id === draft.id) ? 'Edit connection' : 'New connection'}
                        </h3>
                        <Button label="Back to connections" onclick={() => (editing = false)}>Back</Button>
                    </div>
                    <details class="connection-import">
                        <summary><Icon name="chevron" size={13} />Import connection URL</summary>
                        <label class="field">
                            Connection URL
                            <input type="password" autocomplete="off" placeholder="postgres:// or mysql://" bind:value={connectionURL} />
                        </label>
                        <Button disabled={importing || !connectionURL} onclick={importConnection}>Import URL</Button>
                        <p class="field-hint">The URL is cleared after import. Its password goes to the credential store when saved.</p>
                    </details>
                    <label class="field">
                        Connection name
                        <input required maxlength="80" placeholder="e.g. Acme development" bind:value={draft.name} />
                    </label>
                    <div class="field">
                        <span>Database engine</span>
                        <div class="engine-selector" role="group" aria-label="Database engine">
                            {#each workspace.drivers as d (d.name)}
                                <button
                                    type="button"
                                    class="engine-option"
                                    class:active={draft.engine === d.name}
                                    onclick={() => engineChanged(d.name)}
                                    aria-pressed={draft.engine === d.name}
                                >
                                    <DbLogo engine={d.name} size={28} />
                                    <span>{d.displayName}</span>
                                </button>
                            {/each}
                        </div>
                    </div>
                    <Select bind:value={draft.environment} options={environmentOptions} label="Environment" />
                    {#if draft.engine === 'sqlite'}<label class="field">
                            Database file path
                            <input required placeholder="/path/to/database.db" bind:value={draft.path} />
                        </label>
                        <p class="field-hint">The file is created automatically if it does not exist.</p>{:else}<div class="form-grid host-port">
                            <label class="field">
                                Host
                                <input required={!draft.socket} bind:value={draft.host} />
                            </label>
                            <label class="field">
                                Port
                                <input inputmode="numeric" pattern={'[0-9]{1,5}'} bind:value={draft.port} />
                            </label>
                        </div>
                        <div class="form-grid">
                            {#if draft.engine !== 'redis'}<label class="field">
                                    Username
                                    <input bind:value={draft.username} autocomplete="off" />
                                </label>{/if}
                            <label class="field">
                                {draft.engine === 'redis' ? 'Database index' : 'Database'}
                                <input required={draft.engine !== 'redis'} placeholder={draft.engine === 'redis' ? '0' : 'database_name'} bind:value={draft.database} />
                            </label>
                        </div>
                        <label class="field">
                            Password
                            <input type="password" autocomplete="off" placeholder={draft.hasPassword ? 'Saved. Leave blank to keep it' : ''} bind:value={password} />
                        </label>
                        {#if draft.engine === 'postgres'}<label class="field">
                                Schema
                                <input bind:value={draft.schema} />
                            </label>{/if}{/if}
                    <Input type="checkbox" bind:checked={draft.readOnly} label="Open this connection as read-only" />
                    <p class="field-hint">
                        {draft.readOnly
                            ? draft.engine === 'sqlite'
                                ? 'The file must already exist. Quarro and SQLite will refuse changes in this session.'
                                : 'Quarro will hide mutation controls and the backend will refuse changes for this session.'
                            : 'Queries and editing features can change data.'}
                    </p>
                    <label class="field">
                        Folder
                        <input required bind:value={draft.folder} />
                    </label>
                    <div class="inline-note">
                        <Icon name="database" />Passwords are stored in your OS credential store, never in this app's own files.
                    </div>
                    {#if draft.engine !== 'sqlite' && draft.engine !== 'mongodb' && draft.engine !== 'redis'}
                        <details class="connection-transport" open>
                            <summary><Icon name="chevron" size={13} />Transport &amp; security</summary>
                            <label class="field">
                                Unix socket
                                <input
                                    placeholder={draft.engine === 'postgres' ? '/var/run/postgresql (directory)' : '/var/run/mysqld/mysqld.sock (file)'}
                                    bind:value={draft.socket}
                                />
                            </label>
                            <Select
                                bind:value={() => draft.tlsMode ?? 'verify-full', (v) => (draft.tlsMode = v)}
                                options={tlsModeOptions}
                                label="TLS mode"
                                onchange={(value) => {
                                    if (value === 'disable') {
                                        draft.tlsCA = '';
                                        draft.tlsCert = '';
                                        draft.tlsKey = '';
                                        draft.tlsServerName = '';
                                    }
                                }}
                            />
                            {#if draft.tlsMode !== 'disable'}
                                <label class="field">
                                    CA certificate path
                                    <input bind:value={draft.tlsCA} placeholder="System trust store by default" />
                                </label>
                                <label class="field">
                                    TLS server name
                                    <input bind:value={draft.tlsServerName} placeholder="Connection host by default" />
                                </label>
                                <div class="form-grid">
                                    <label class="field">
                                        Client certificate path
                                        <input bind:value={draft.tlsCert} />
                                    </label>
                                    <label class="field">
                                        Client key path
                                        <input bind:value={draft.tlsKey} />
                                    </label>
                                </div>
                            {:else}<p class="field-hint">Traffic is unencrypted. Use this only for trusted local connections or Unix sockets.</p>{/if}
                        </details>
                    {/if}
                    {#if draft.engine === 'postgres' || draft.engine === 'mysql'}
                        <details class="connection-ssh" open={draft.sshEnabled}>
                            <summary><Icon name="chevron" size={13} />SSH tunnel</summary>
                            <Input type="checkbox" bind:checked={draft.sshEnabled} label="Connect through an SSH tunnel" />
                            {#if draft.sshEnabled}
                                <div class="form-grid host-port">
                                    <label class="field">
                                        SSH host
                                        <input required bind:value={draft.sshHost} placeholder="jump.example.com" />
                                    </label>
                                    <label class="field">
                                        SSH port
                                        <input inputmode="numeric" pattern={'[0-9]{1,5}'} bind:value={draft.sshPort} placeholder="22" />
                                    </label>
                                </div>
                                <label class="field">
                                    SSH username
                                    <input required bind:value={draft.sshUsername} autocomplete="off" />
                                </label>
                                <Select
                                    bind:value={() => draft.sshAuthMethod ?? 'password', (v) => (draft.sshAuthMethod = v as 'password' | 'key')}
                                    options={sshAuthMethodOptions}
                                    label="SSH authentication"
                                />
                                {#if draft.sshAuthMethod === 'key'}
                                    <label class="field">
                                        Private key path
                                        <input required bind:value={draft.sshPrivateKeyPath} placeholder="/home/you/.ssh/id_ed25519" />
                                    </label>
                                    <label class="field">
                                        Key passphrase
                                        <input
                                            type="password"
                                            autocomplete="off"
                                            placeholder={draft.sshHasPassphrase ? 'Saved. Leave blank to keep it' : 'Leave blank if the key has none'}
                                            bind:value={sshPassphrase}
                                        />
                                    </label>
                                {:else}
                                    <label class="field">
                                        SSH password
                                        <input
                                            type="password"
                                            autocomplete="off"
                                            placeholder={draft.sshHasPassword ? 'Saved. Leave blank to keep it' : ''}
                                            bind:value={sshPassword}
                                        />
                                    </label>
                                {/if}
                                {#if pendingSSHFingerprint}
                                    <div class="inline-note">
                                        <Icon name="database" />New SSH host key: <code>{pendingSSHFingerprint}</code>
                                        <Button onclick={trustSSHKey}>Trust this key</Button>
                                    </div>
                                {:else if draft.sshHostFingerprint}
                                    <div class="inline-note">
                                        <Icon name="check" />Trusted host key: <code>{draft.sshHostFingerprint}</code>
                                        <Button onclick={forgetSSHKey}>Forget trusted key</Button>
                                    </div>
                                {:else}
                                    <p class="field-hint">Trust is established the first time Test connection or Save succeeds; you will be asked to confirm the host key then.</p>
                                {/if}
                            {/if}
                        </details>
                    {/if}
                    {#if testResult}<div class="inline-note" class:error-text={!testResult.ok}>
                            <Icon name={testResult.ok ? 'check' : 'disconnect'} />{testResult.message}
                        </div>{/if}
                    <div class="form-actions">
                        <Button onclick={() => (editing = false)}>Cancel</Button><Button disabled={testing} onclick={test}>
                            {testing ? 'Testing…' : 'Test connection'}
                        </Button><Button type="submit" variant="primary" disabled={saving}>
                            {saving ? 'Saving…' : 'Save profile'}
                        </Button>
                    </div>
                </form>{:else}<div class="connections-toolbar">
                    <label class="search-field">
                        <Icon name="search" />
                        <input aria-label="Search connections" placeholder="Find a connection…" bind:value={search} />
                    </label>
                    <Button
                        icon="plus"
                        variant="primary"
                        onclick={() => {
                            draft = blank();
                            password = '';
                            testResult = null;
                            editing = true;
                        }}
                    >
                        New connection
                    </Button>
                </div>
                {#if workspace.profilesLoading}<div class="empty-state">
                        <Icon name="database" size={32} />
                        <h3>Loading connections…</h3>
                    </div>{:else}
                    {#each [...new Set(matches.map((p) => p.folder))] as folder}<div class="folder-label">
                            <Icon name="folder" size={14} />{folder}
                        </div>
                        {#each matches.filter((p) => p.folder === folder) as profile}<div class="connection-card">
                                <span class="connection-engine {profile.engine}">
                                    <DbLogo engine={profile.engine} size={22} />
                                </span>
                                <button class="connection-card-main" onclick={() => workspace.openProfile(profile)}>
                                    <strong>{profile.name}</strong>
                                    <small>
                                        {workspace.engineDisplayName(profile.engine)}
                                        <span>·</span>
                                        {profile.database || profile.path}
                                    </small>
                                </button>
                                {#if profile.readOnly}<span class="environment read-only-profile">READ ONLY</span>{/if}
                                <span class="environment">{profile.environment}</span>
                                <Button
                                    icon="bookmark"
                                    label={`${profile.favorite ? 'Remove' : 'Add'} favorite ${profile.name}`}
                                    active={profile.favorite}
                                    onclick={() => toggleFavorite(profile)}
                                />
                                <details class="dropdown" use:closeOnOutsideClick>
                                    <summary aria-label={`Options for ${profile.name}`}><Icon name="more" /></summary>
                                    <div class="menu">
                                        <button onclick={() => edit(profile)}>Edit profile</button>
                                        <button
                                            onclick={() =>
                                                edit({
                                                    ...profile,
                                                    id: crypto.randomUUID(),
                                                    name: `${profile.name} copy`,
                                                    hasPassword: false,
                                                    tlsMode: 'verify-full',
                                                    socket: '',
                                                    tlsCA: '',
                                                    tlsCert: '',
                                                    tlsKey: '',
                                                    tlsServerName: '',
                                                    sshHasPassword: false,
                                                    sshHasPassphrase: false,
                                                    sshHostFingerprint: '',
                                                })}
                                        >
                                            Duplicate profile
                                        </button>
                                        <button class="error-text" onclick={() => remove(profile)}> Remove saved profile </button>
                                    </div>
                                </details>
                                <Button icon="arrow" label={`Open ${profile.name}`} onclick={() => workspace.openProfile(profile)} />
                            </div>{/each}{/each}{#if !matches.length}<div class="empty-state">
                            <Icon name="database" size={32} />
                            <h3>No connections here yet</h3>
                            <p>Create a connection or change your search.</p>
                        </div>{/if}{/if}{/if}
        </div>
    </div>
</Modal>
