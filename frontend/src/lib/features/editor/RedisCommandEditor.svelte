<script lang="ts">
    import { onMount } from 'svelte';
    import { EditorState } from '@codemirror/state';
    import { EditorView, drawSelection, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers, placeholder } from '@codemirror/view';
    import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
    import { defaultHighlightStyle, indentUnit, syntaxHighlighting } from '@codemirror/language';
    import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap } from '@codemirror/autocomplete';
    import { search, searchKeymap } from '@codemirror/search';
    import type { Document, Session } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import { commandCompletion, redisCommandLanguage } from './redisCommand';

    let {
        document,
        session,
        onrun,
    }: {
        document: Document;
        session: Session;
        onrun: () => void;
    } = $props();

    let host: HTMLDivElement;
    let view: EditorView | undefined;
    // Filled once the server answers COMMAND LIST; completion reads it live.
    let names: string[] = [];

    function editorOptions() {
        const p = workspace.preferences;
        return [
            p.lineNumbers ? lineNumbers() : [],
            p.activeLine ? [highlightActiveLine(), highlightActiveLineGutter()] : [],
            p.wordWrap ? EditorView.lineWrapping : [],
            EditorState.tabSize.of(p.tabSize),
            indentUnit.of(p.indentTabs ? '\t' : ' '.repeat(p.tabSize)),
        ];
    }

    onMount(() => {
        view = new EditorView({
            parent: host,
            state: EditorState.create({
                doc: document.sql,
                extensions: [
                    history(),
                    drawSelection(),
                    closeBrackets(),
                    redisCommandLanguage,
                    autocompletion({ override: [commandCompletion(() => names)] }),
                    search(),
                    syntaxHighlighting(defaultHighlightStyle),
                    placeholder('One command per line, e.g. HGETALL user:1. Mod-Enter runs the line under the cursor, or every selected line.'),
                    editorOptions(),
                    keymap.of([
                        {
                            key: 'Mod-Enter',
                            run: () => {
                                onrun();
                                return true;
                            },
                        },
                        ...closeBracketsKeymap,
                        ...completionKeymap,
                        ...defaultKeymap,
                        ...historyKeymap,
                        ...searchKeymap,
                        indentWithTab,
                    ]),
                    EditorView.contentAttributes.of({ 'aria-label': 'Redis command editor' }),
                    EditorView.updateListener.of((update) => {
                        if (update.docChanged) document.sql = update.state.doc.toString();
                        if (update.selectionSet) {
                            const range = update.state.selection.main;
                            document.selectionFrom = range.from;
                            document.selectionTo = range.to;
                            document.cursor = range.head;
                        }
                    }),
                ],
            }),
        });
        return () => view?.destroy();
    });

    // Fetch the command list once the session is connected (it is cached
    // per session, so other Redis query documents reuse it).
    $effect(() => {
        if (session.connectionState !== 'connected') return;
        workspace.redisCommandNames(session, document).then((list) => (names = list));
    });

    // Saved-query/history navigation can replace the text after mount.
    $effect(() => {
        const text = document.sql;
        if (view && view.state.doc.toString() !== text) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
    });
</script>

<div class="native-query-editor">
    <div class="query-editor native-query-code" bind:this={host}></div>
</div>
