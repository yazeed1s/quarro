<script lang="ts">
    import { onMount } from 'svelte';
    import { objectLabel } from '../../state/types';
    import * as backend from '../../state/backend';
    import type { Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Icon from '../../components/Icon.svelte';
    import Button from '../../components/Button.svelte';
    import { TABLE_DRAG_MIME, setTableDragImage } from '../query-builder/dnd';
    import Modal from '../../components/Modal.svelte';
    import type { SchemaFunction } from '../../state/types';
    let { session }: { session: Session } = $props();
    let databases = $state<string[]>([]);
    let loadingDatabases = $state(false);
    let schemaFilter = $state('');
    let searchInput: HTMLInputElement;
    onMount(() => {
        // Global "/" shortcut and the command palette's "Focus object search"
        // both dispatch this; only the visible session's browser should react.
        const focus = () => {
            if (workspace.active === session.id) {
                page = 'objects';
                searchInput?.focus();
            }
        };
        window.addEventListener('quarro:focus-object-search', focus);
        return () => window.removeEventListener('quarro:focus-object-search', focus);
    });
    const schemas = $derived([...new Set(session.schema.map((t) => t.schema).filter((s): s is string => !!s))]);
    async function loadDatabases() {
        loadingDatabases = true;
        try {
            databases = await backend.listDatabases(session.id);
        } catch (e) {
            workspace.notify(String(e));
        } finally {
            loadingDatabases = false;
        }
    }
    $effect(() => {
        if (page === 'connection' && !databases.length && !loadingDatabases && session.profile.engine !== 'sqlite' && session.connectionState === 'connected') {
            loadDatabases();
        }
    });
    let draggingObject = $state<string | null>(null);
    onMount(() => {
        // WebKitGTK sometimes never fires `dragend` on the source row once a
        // drop lands inside the query canvas's transformed (pan/zoom)
        // surface, leaving this row stuck in its dragging visual state.
        // `drop` reliably fires regardless, so use it as a backstop.
        const clear = () => (draggingObject = null);
        window.addEventListener('drop', clear);
        window.addEventListener('dragend', clear);
        return () => {
            window.removeEventListener('drop', clear);
            window.removeEventListener('dragend', clear);
        };
    });
    let search = $state('');
    let page = $state('objects');
    let expanded = $state(true);
    let views = $state(false);
    // Neither MongoDB collections nor Redis keyspaces have SQL views or
    // foreign-key relationships to show in a diagram: SupportsJoins is false
    // for exactly the engines with no relational join concept.
    let hideRelational = $derived(!session.capabilities.supportsJoins);
    let tables = $derived(session.schema.filter((t) => (t.kind === 'table' || t.kind === 'collection' || t.kind === 'keyspace') && (!schemaFilter || t.schema === schemaFilter)));
    // The section heading follows what kind of object this session's schema
    // actually contains, not the engine's name.
    let objectKindLabel = $derived(tables[0]?.kind === 'collection' ? 'Collections' : tables[0]?.kind === 'keyspace' ? 'Keyspaces' : 'Tables');
    let viewObjects = $derived(session.schema.filter((t) => t.kind === 'view' && (!schemaFilter || t.schema === schemaFilter)));
    let matchingTables = $derived(tables.filter((t) => t.name.toLowerCase().includes(search.toLowerCase())));
    let matchingViews = $derived(viewObjects.filter((t) => t.name.toLowerCase().includes(search.toLowerCase())));
    let functionObjects = $derived(session.routines.filter((f) => !schemaFilter || f.schema === schemaFilter));
    let matchingFunctions = $derived(functionObjects.filter((f) => f.name.toLowerCase().includes(search.toLowerCase())));
    let functions = $state(false);
    let selectedFunction = $state<SchemaFunction | null>(null);
    let savedList = $derived(
        workspace.saved
            .filter((q) => q.profileId === session.profile.id && `${q.title} ${q.sql} ${q.folder}`.toLowerCase().includes(search.toLowerCase()))
            .sort((a, b) => Number(b.favorite) - Number(a.favorite) || b.at - a.at),
    );
    let historyList = $derived(workspace.history.filter((q) => q.profileId === session.profile.id && `${q.title} ${q.sql}`.toLowerCase().includes(search.toLowerCase())));
    function openQuery(sql: string, title: string, object?: string) {
        const doc = workspace.openDocument(session);
        doc.sql = sql;
        doc.title = title;
        if (object) doc.object = object;
    }
</script>

<aside class="object-browser">
    <div class="browser-heading">
        <span class="eyebrow">EXPLORER</span>
        <Button icon="refresh" label="Refresh schema" onclick={() => workspace.loadSchema(session)} />
    </div>
    <label class="search-field">
        <Icon name="search" size={14} />
        <input bind:this={searchInput} aria-label="Search objects" placeholder="Search objects…" bind:value={search} />
        <kbd>/</kbd>
    </label>
    <div class="browser-database">
        <span class="engine-mark {session.profile.engine}"><Icon name="database" /></span>
        <div>
            <strong>{session.profile.database}</strong>
            <small>
                {workspace.engineDisplayName(session.profile.engine)}
                <span>· {session.profile.schema || 'database'}</span>
            </small>
        </div>
        <span class="environment" class:stage={session.profile.environment === 'Stage'}>
            {session.profile.environment}
        </span>
    </div>
    <div class="browser-content">
        {#if page === 'objects'}
            {#if session.schemaLoading}<div class="sidebar-empty">Loading schema…</div>{:else}
                {#if !hideRelational}<button class="tree-object diagram-object" onclick={() => workspace.openDiagram(session)}>
                        <Icon name="diagram" size={14} />
                        <span>Schema diagram</span>
                        <span class="object-count">{tables.length}</span>
                    </button>{/if}
                <button class="tree-group" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>
                    <span class:rotate={expanded}><Icon name="chevron" size={12} /></span>
                    <Icon name="folder" size={15} />
                    <span>{objectKindLabel}</span>
                    <span class="count">{tables.length}</span>
                </button>
                {#if expanded}{#each matchingTables as table}<div class="tree-object-row">
                            <button
                                class="tree-object"
                                class:active={session.documents.find((d) => d.id === session.activeDocument)?.object === table.name}
                                class:dragging={draggingObject === table.name}
                                draggable="true"
                                ondragstart={(e) => {
                                    e.dataTransfer?.setData(TABLE_DRAG_MIME, table.name);
                                    setTableDragImage(e, objectLabel(table.name));
                                    draggingObject = table.name;
                                }}
                                ondragend={() => (draggingObject = null)}
                                onclick={() => workspace.openDocument(session, 'table', table.name)}
                            >
                                <Icon name="table" size={14} />
                                <span>{objectLabel(table.name)}</span>
                                {#if table.kind !== 'keyspace'}<span class="object-count">
                                        {table.columns.length}
                                    </span>{/if}
                            </button>
                            <button
                                class="tree-inline-action"
                                aria-label={`Visualize ${objectLabel(table.name)}`}
                                title={`Visualize ${objectLabel(table.name)}`}
                                onclick={() => workspace.visualizeTable(session, table.name)}
                            >
                                <Icon name="chart" size={12} />
                            </button>
                            {#if !hideRelational}<button
                                    class="tree-inline-action"
                                    aria-label="Show in diagram"
                                    title={`Show ${objectLabel(table.name)} in diagram`}
                                    onclick={() => workspace.openDiagram(session, table.name)}
                                >
                                    <Icon name="diagram" size={12} />
                                </button>{/if}
                        </div>{/each}{/if}
                {#if !hideRelational}<button class="tree-group" aria-expanded={views} onclick={() => (views = !views)}>
                        <span class:rotate={views}><Icon name="chevron" size={12} /></span>
                        <Icon name="table" size={15} />
                        <span>Views</span>
                        <span class="count">{viewObjects.length}</span>
                    </button>
                    {#if views}{#each matchingViews as view}<div class="tree-object-row">
                                <button
                                    class="tree-object"
                                    class:dragging={draggingObject === view.name}
                                    draggable="true"
                                    ondragstart={(e) => {
                                        e.dataTransfer?.setData(TABLE_DRAG_MIME, view.name);
                                        setTableDragImage(e, objectLabel(view.name));
                                        draggingObject = view.name;
                                    }}
                                    ondragend={() => (draggingObject = null)}
                                    onclick={() => workspace.openDocument(session, 'table', view.name)}
                                >
                                    <Icon name="table" size={14} />
                                    <span>{objectLabel(view.name)}</span>
                                </button>
                                <button
                                    class="tree-inline-action"
                                    aria-label={`Visualize ${objectLabel(view.name)}`}
                                    title={`Visualize ${objectLabel(view.name)}`}
                                    onclick={() => workspace.visualizeTable(session, view.name)}
                                >
                                    <Icon name="chart" size={12} />
                                </button>
                            </div>{/each}{/if}{/if}
                {#if session.capabilities.routineDiscoverer}<button class="tree-group" aria-expanded={functions} onclick={() => (functions = !functions)}>
                        <span class:rotate={functions}><Icon name="chevron" size={12} /></span>
                        <Icon name="code" size={15} />
                        <span>Functions</span>
                        <span class="count">{functionObjects.length}</span>
                    </button>
                    {#if functions}{#each matchingFunctions as fn}<div class="tree-object-row">
                                <button class="tree-object" onclick={() => (selectedFunction = fn)}>
                                    <Icon name="code" size={14} />
                                    <span>{objectLabel(fn.name)}</span>
                                    <span class="object-count">{fn.kind === 'procedure' ? 'proc' : 'fn'}</span>
                                </button>
                            </div>{/each}{/if}{/if}
                {#if search && !matchingTables.length && !matchingViews.length && !matchingFunctions.length}<div class="sidebar-empty">
                        No objects match "{search}".
                        <button class="text-button" onclick={() => (search = '')}>Clear search</button>
                    </div>{/if}
                <div class="schema-note">{session.schema.length} objects</div>
            {/if}
        {:else if page === 'connection'}
            <div class="list-heading connection-heading">
                <span class="eyebrow">DATABASES</span>
                <Button icon="refresh" label="Refresh databases" disabled={loadingDatabases || session.connectionState !== 'connected'} onclick={loadDatabases} />
            </div>
            <div class="connection-page">
                {#if loadingDatabases && !databases.length}<div class="sidebar-empty compact">
                        <span class="spinner"></span>
                        Loading databases…
                    </div>
                {:else if !databases.length}<div class="sidebar-empty">
                        {session.connectionState === 'connected' ? 'No databases found on this connection.' : 'Connect to see available databases.'}
                    </div>
                {:else}{#each databases as database}
                        <button class="tree-object" class:active={database === session.profile.database} onclick={() => workspace.openDatabase(session, database)}>
                            <Icon name="database" size={14} />
                            <span>{database}</span>
                        </button>
                        {#if database === session.profile.database && session.profile.engine === 'postgres' && schemas.length}<div class="schema-picker">
                                <div class="schema-chips">
                                    <button class="schema-chip" class:active={!schemaFilter} onclick={() => (schemaFilter = '')}> All schemas </button>
                                    {#each schemas as schema}<button
                                            class="schema-chip"
                                            class:active={schemaFilter === schema}
                                            onclick={() => (schemaFilter = schemaFilter === schema ? '' : schema)}
                                        >
                                            {schema}
                                        </button>{/each}
                                </div>
                            </div>{/if}
                    {/each}{/if}
            </div>
        {:else if page === 'saved'}
            <div class="eyebrow list-heading">SAVED QUERIES</div>
            {#each savedList as query}<div class="saved-item-row">
                    <button class="saved-item" onclick={() => openQuery(query.sql, query.title, query.object)}>
                        <Icon name="code" />
                        <span>
                            <input
                                class="saved-title-input"
                                value={query.title}
                                onchange={(e) => workspace.renameSaved(query, e.currentTarget.value)}
                                onclick={(e) => e.stopPropagation()}
                                aria-label={`Rename ${query.title}`}
                            />
                            <small>
                                <input
                                    class="saved-folder-input"
                                    value={query.folder}
                                    placeholder="No folder"
                                    onchange={(e) => workspace.setSavedFolder(query, e.currentTarget.value)}
                                    onclick={(e) => e.stopPropagation()}
                                    aria-label={`Folder for ${query.title}`}
                                />
                            </small>
                        </span>
                    </button>
                    <button
                        class="tree-inline-action"
                        class:active={query.favorite}
                        aria-label={query.favorite ? 'Remove from favorites' : 'Mark as favorite'}
                        title={query.favorite ? 'Remove from favorites' : 'Mark as favorite'}
                        onclick={() => workspace.toggleSavedFavorite(query)}
                    >
                        <Icon name="star" size={12} />
                    </button>
                    <button class="tree-inline-action" aria-label={`Delete ${query.title}`} title="Delete saved query" onclick={() => workspace.deleteSaved(query)}>
                        <Icon name="trash" size={12} />
                    </button>
                </div>{:else}<div class="sidebar-empty">
                    Your useful queries, close at hand.
                    <small>Save an open query to get started.</small>
                </div>{/each}
        {:else}
            <div class="eyebrow list-heading history-heading">
                <span>RECENT QUERIES</span>
                {#if historyList.length}<button class="text-button" onclick={() => workspace.clearHistory(session.profile.id)}> Clear </button>{/if}
            </div>
            {#if !workspace.preferences.rememberHistory}<div class="sidebar-empty compact">History is turned off in Settings → Advanced.</div>{/if}
            {#each historyList as query}<div class="saved-item-row">
                    <button class="saved-item" onclick={() => openQuery(query.sql, query.title, query.object)}>
                        <Icon name={query.outcome === 'error' ? 'disconnect' : 'code'} />
                        <span>
                            {query.title}
                            <small>
                                {new Date(query.at).toLocaleString()} · {query.durationMs} ms
                                {#if query.outcome === 'error'}· failed{/if}
                            </small>
                        </span>
                    </button>
                    <button
                        class="tree-inline-action"
                        aria-label={`Remove ${query.title} from history`}
                        title="Remove from history"
                        onclick={() => workspace.deleteHistoryEntry(query)}
                    >
                        <Icon name="trash" size={12} />
                    </button>
                </div>{:else}<div class="sidebar-empty">
                    Your query runs will appear here.
                    <small>Run a query from the editor.</small>
                </div>{/each}
        {/if}
    </div>
    <nav class="browser-nav" aria-label="Explorer sections">
        <button class:active={page === 'objects'} onclick={() => (page = 'objects')}>
            <Icon name="database" />Database objects
        </button>
        <button class:active={page === 'saved'} onclick={() => (page = 'saved')}>
            <Icon name="bookmark" />Saved queries
            <span class="count">
                {workspace.saved.filter((q) => q.profileId === session.profile.id).length}
            </span>
        </button>
        <button class:active={page === 'history'} onclick={() => (page = 'history')}>
            <Icon name="clock" />Query history
        </button>
        {#if session.profile.engine !== 'sqlite'}<button class:active={page === 'connection'} onclick={() => (page = 'connection')}>
                <Icon name="link" />Connection
                {#if databases.length}<span class="count">{databases.length}</span>{/if}
            </button>{/if}
    </nav>
    <div class="browser-footer">
        <Button icon="plus" variant="outline" onclick={() => (workspace.connections = true)}>New connection</Button>
        <small>One workspace. A clearer picture.</small>
    </div>
</aside>
{#if selectedFunction}<Modal
        title={objectLabel(selectedFunction.name)}
        subtitle={selectedFunction.kind === 'procedure' ? 'Procedure' : 'Function'}
        onclose={() => (selectedFunction = null)}
    >
        <div class="function-detail">
            {#if selectedFunction.parameters || selectedFunction.returnType}<div class="function-detail-row">
                    <strong>Signature</strong>
                    <code>({selectedFunction.parameters}){selectedFunction.returnType ? ` → ${selectedFunction.returnType}` : ''}</code>
                </div>{/if}
            {#if selectedFunction.language}<div class="function-detail-row">
                    <strong>Language</strong>
                    <span>{selectedFunction.language}</span>
                </div>{/if}
            {#if selectedFunction.definition}<div class="function-detail-row">
                    <strong>Definition</strong>
                    <pre class="function-detail-body">{selectedFunction.definition}</pre>
                </div>
            {:else}<p class="function-detail-empty">No source body is available for this routine.</p>{/if}
        </div>
    </Modal>{/if}
