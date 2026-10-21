<script lang="ts">
    import { onMount } from 'svelte';
    import { EditorState } from '@codemirror/state';
    import { EditorView, drawSelection, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from '@codemirror/view';
    import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
    import { bracketMatching, defaultHighlightStyle, foldGutter, foldKeymap, indentUnit, syntaxHighlighting } from '@codemirror/language';
    import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
    import { search, searchKeymap } from '@codemirror/search';
    import { json } from '@codemirror/lang-json';
    import type { Document, Session } from '../../state/types';
    import { objectLabel } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';

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
    let collectionOptions: SelectOption[] = $derived(session.schema.filter((t) => t.kind === 'collection').map((t) => ({ value: t.name, label: objectLabel(t.name) })));

    // New query documents start with the generic Document default object
    // ("users"). Choose the first real collection once discovery completes;
    // keep a restored/saved collection when it still exists.
    $effect(() => {
        const options = collectionOptions;
        if (options.length && !options.some((option) => option.value === document.object)) document.object = options[0].value;
    });

    function editorOptions() {
        const p = workspace.preferences;
        return [
            p.lineNumbers ? [lineNumbers(), foldGutter()] : [],
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
                    bracketMatching(),
                    closeBrackets(),
                    json(),
                    search(),
                    syntaxHighlighting(defaultHighlightStyle),
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
                        ...defaultKeymap,
                        ...historyKeymap,
                        ...foldKeymap,
                        ...searchKeymap,
                        indentWithTab,
                    ]),
                    EditorView.contentAttributes.of({ 'aria-label': 'Aggregation pipeline editor' }),
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

    // Saved-query/history navigation can replace the pipeline after mount.
    $effect(() => {
        const pipeline = document.sql;
        if (view && view.state.doc.toString() !== pipeline) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: pipeline } });
    });
</script>

<div class="native-query-editor">
    <div class="native-query-collection">
        <span class="field-name">Collection</span>
        <Select bind:value={document.object} options={collectionOptions} placeholder="Choose a collection" />
    </div>
    <div class="query-editor native-query-code" bind:this={host}></div>
</div>
