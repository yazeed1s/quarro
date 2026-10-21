<script lang="ts">
    import { workspace } from '../../state/workspace.svelte';
    import type { Session } from '../../state/types';
    import Button from '../../components/Button.svelte';
    import Icon from '../../components/Icon.svelte';
    import ResizeHandle from '../../components/ResizeHandle.svelte';
    import ObjectBrowser from './ObjectBrowser.svelte';
    import DocumentView from './DocumentView.svelte';
    import { closeOnOutsideClick } from '../../components/dropdown';
    let { session }: { session: Session } = $props();
</script>

<div class="workspace-layout" class:sidebar-hidden={!session.sidebar}>
    <div class="sidebar-wrapper" style:width={`${session.sidebarWidth}px`}>
        <ObjectBrowser {session} />
    </div>
    <div class="sidebar-resizer">
        <ResizeHandle value={session.sidebarWidth} min={190} max={360} label="Sidebar width" onchange={(v) => (session.sidebarWidth = v)} />
    </div>
    <main class="workspace-main">
        <div class="document-bar">
            <Button icon="panel" label="Toggle explorer" active={session.sidebar} onclick={() => (session.sidebar = !session.sidebar)} />
            <nav class="document-tabs" aria-label="Open documents">
                {#each session.documents as document (document.id)}<div class="document-tab" class:selected={session.activeDocument === document.id}>
                        <button
                            class="document-select"
                            aria-current={session.activeDocument === document.id ? 'page' : undefined}
                            onclick={() => (session.activeDocument = document.id)}
                        >
                            <Icon
                                name={document.kind === 'query'
                                    ? 'code'
                                    : document.kind === 'diagram'
                                      ? 'diagram'
                                      : document.kind === 'chart'
                                        ? 'chart'
                                        : document.kind === 'visualQuery'
                                          ? 'columns'
                                          : 'table'}
                                size={14}
                            />
                            <span>{document.title}</span>
                            {#if document.kind === 'query'}<span class="draft-dot" title="Local draft"></span>{/if}
                        </button>
                        <Button icon="close" label={`Close ${document.title}`} onclick={() => workspace.closeDocument(session, document)} />
                    </div>{/each}
            </nav>
            <Button icon="plus" label="New query" onclick={() => workspace.openDocument(session)} />
            <details class="dropdown" use:closeOnOutsideClick>
                <summary aria-label="Document and layout menu" title="Document and layout menu">
                    <Icon name="more" />
                </summary>
                <div class="menu">
                    {#each session.documents as doc}<button onclick={() => (session.activeDocument = doc.id)}>
                            {doc.title}{#if session.activeDocument === doc.id}<Icon name="check" />{/if}
                        </button>{/each}
                    <hr />
                    <button onclick={() => workspace.openDiagram(session)}>Open schema diagram</button>
                    <button onclick={() => workspace.openDocument(session, 'visualQuery')}> New visual query </button>
                    <button onclick={() => workspace.resetLayout()}>Reset workspace layout</button>
                    <button
                        disabled={session.connectionState === 'connecting'}
                        onclick={() => {
                            if (session.connectionState === 'connected') workspace.disconnect(session);
                            else workspace.reconnect(session);
                        }}
                    >
                        {session.connectionState === 'connected' ? 'Disconnect' : 'Reconnect'} session
                    </button>
                </div>
            </details>
        </div>
        {#each session.documents as document (document.id)}<div class="document-host" hidden={session.activeDocument !== document.id}>
                <DocumentView {session} {document} />
            </div>{/each}
        {#if !session.documents.length}<div class="empty-state">
                <Icon name="code" size={32} />
                <h2>A fresh start</h2>
                <p>Open a table from the explorer or write a new query.</p>
                <Button variant="primary" icon="plus" onclick={() => workspace.openDocument(session)}>New query</Button>
            </div>{/if}
    </main>
</div>
