<script lang="ts">
    import { onMount } from 'svelte';
    import { EditorState, Compartment, type Text } from '@codemirror/state';
    import {
        EditorView,
        keymap,
        lineNumbers,
        highlightActiveLine,
        highlightActiveLineGutter,
        drawSelection,
        Decoration,
        ViewPlugin,
        hoverTooltip,
        type DecorationSet,
        type ViewUpdate,
    } from '@codemirror/view';
    import type { SyntaxNode } from '@lezer/common';
    import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
    import { sql, PostgreSQL, MySQL, SQLite, type SQLDialect, type SQLNamespace } from '@codemirror/lang-sql';
    import { syntaxHighlighting, HighlightStyle, indentUnit, bracketMatching, foldGutter, foldKeymap, syntaxTree } from '@codemirror/language';
    import { tags } from '@lezer/highlight';
    import {
        autocompletion,
        completionKeymap,
        closeBrackets,
        closeBracketsKeymap,
        snippetCompletion,
        type Completion,
        type CompletionContext,
        type CompletionResult,
    } from '@codemirror/autocomplete';
    import { search, searchKeymap } from '@codemirror/search';
    import { linter, forceLinting, lintGutter, lintKeymap, type Diagnostic } from '@codemirror/lint';
    import type { Document, Engine, HistoryEntry, SchemaColumn, SchemaTable } from '../../state/types';
    import { workspace } from '../../state/workspace.svelte';
    import * as backend from '../../state/backend';
    import type { StatementRange } from '../../state/backend';
    import type { SqlRange } from '../explain/reveal';
    let {
        document,
        engine,
        schema,
        profileId,
        sessionId,
        hotspotRanges = [],
        onrun,
    }: {
        document: Document;
        engine: Engine;
        schema: SchemaTable[];
        profileId: string;
        sessionId: string;
        /** Sibling explain documents' hotspot-badged node ranges, in this
         * document's own SQL text (see DocumentView.svelte, which computes
         * these from every open 'explain' document forked from this one). */
        hotspotRanges?: SqlRange[];
        onrun: () => void;
    } = $props();
    function escapeRegExp(value: string): string {
        return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }
    // Recently-queried tables sort above the rest in completion lists. Scanned
    // from this profile's own history text rather than a structured "tables
    // touched" record, since history entries only ever store raw SQL.
    function tableBoosts(entries: HistoryEntry[], id: string, tables: SchemaTable[]): Map<string, number> {
        const boosts = new Map<string, number>();
        const names = [...new Set(tables.map((t) => (t.objectName ?? t.name).toLowerCase()))];
        const recent = entries.filter((h) => h.profileId === id).slice(-200);
        for (const entry of recent) {
            const text = entry.sql.toLowerCase();
            for (const name of names) {
                if (new RegExp(`\\b${escapeRegExp(name)}\\b`).test(text)) boosts.set(name, Math.min((boosts.get(name) ?? 0) + 1, 5));
            }
        }
        return boosts;
    }
    function columnCompletion(c: SchemaColumn): Completion {
        const bits = [c.type || 'unknown'];
        if (c.primaryKey) bits.push('PK');
        if (c.notNull) bits.push('not null');
        return {
            label: c.name,
            type: 'property',
            detail: bits.join(' · '),
            info: c.defaultValue != null ? `default ${c.defaultValue}` : undefined,
        };
    }
    function tableCompletion(t: SchemaTable, boost: number): Completion {
        return { label: t.objectName ?? t.name, type: 'type', detail: t.kind, boost };
    }
    function completionSchema(tables: SchemaTable[], boosts: Map<string, number>): Record<string, SQLNamespace> {
        const result: Record<string, SQLNamespace> = {};
        for (const t of tables) {
            const label = t.objectName ?? t.name;
            const entry: SQLNamespace = {
                self: tableCompletion(t, boosts.get(label.toLowerCase()) ?? 0),
                children: t.columns.map(columnCompletion),
            };
            if (t.schema && t.objectName) {
                const group = (result[t.schema] ??= {}) as Record<string, SQLNamespace>;
                group[t.objectName] = entry;
            } else result[t.name] = entry;
        }
        return result;
    }
    // Statement skeletons offered alongside the language's own schema-aware
    // completions (see EditorState.languageData below). CodeMirror's Tab/
    // Shift-Tab snippet fields (snippetKeymap) let the user jump between the
    // placeholders such as ${table} without typing them by hand.
    const sqlSnippets = [
        snippetCompletion('SELECT ${columns} FROM ${table} WHERE ${condition};', {
            label: 'select-where',
            type: 'keyword',
            detail: 'SELECT … FROM … WHERE …',
        }),
        snippetCompletion('SELECT ${columns} FROM ${table} ORDER BY ${column} LIMIT ${100};', {
            label: 'select-order-limit',
            type: 'keyword',
            detail: 'SELECT … ORDER BY … LIMIT …',
        }),
        snippetCompletion('INSERT INTO ${table} (${columns}) VALUES (${values});', {
            label: 'insert-into',
            type: 'keyword',
            detail: 'INSERT INTO … VALUES …',
        }),
        snippetCompletion('UPDATE ${table} SET ${column} = ${value} WHERE ${condition};', {
            label: 'update-set',
            type: 'keyword',
            detail: 'UPDATE … SET … WHERE …',
        }),
        snippetCompletion('DELETE FROM ${table} WHERE ${condition};', {
            label: 'delete-from',
            type: 'keyword',
            detail: 'DELETE FROM … WHERE …',
        }),
        snippetCompletion('CREATE TABLE ${table} (\n  ${id} INTEGER PRIMARY KEY,\n  ${column} TEXT\n);', { label: 'create-table', type: 'keyword', detail: 'CREATE TABLE …' }),
    ];
    function snippetSource(context: CompletionContext): CompletionResult | null {
        const word = context.matchBefore(/[\w-]*/);
        if (!word || (word.from === word.to && !context.explicit)) return null;
        return { from: word.from, options: sqlSnippets, validFor: /^[\w-]*$/ };
    }
    // Unclosed strings/parens and other parser-rejected syntax, surfaced as
    // the lezer SQL grammar's own error nodes rather than a hand-rolled
    // syntax check.
    function parseDiagnostics(state: EditorState): Diagnostic[] {
        const diagnostics: Diagnostic[] = [];
        syntaxTree(state).iterate({
            enter: (node) => {
                if (!node.type.isError || diagnostics.length >= 50) return;
                diagnostics.push({
                    from: node.from,
                    to: Math.max(node.to, node.from + 1),
                    severity: 'error',
                    message: 'Syntax error.',
                    source: 'sql-parser',
                });
            },
        });
        return diagnostics;
    }
    // Maps a failed execution back onto the statement that caused it.
    // `result.statement` (set by the backend from the exact same Split ranges
    // Run Current Statement already uses) is always a verbatim substring of
    // whatever SQL text was sent to run — searched forward from the previous
    // match so repeated identical statements in one script line up in order.
    // A statement that no longer appears (the user has since edited it) is
    // silently skipped rather than mis-highlighted; this is also what makes a
    // diagnostic clear itself on the next edit or successful re-run, with no
    // extra bookkeeping.
    function serverDiagnostics(state: EditorState): Diagnostic[] {
        const text = state.doc.toString();
        const diagnostics: Diagnostic[] = [];
        let cursor = 0;
        for (const result of document.results) {
            if (result.kind !== 'error' || !result.statement) continue;
            const from = text.indexOf(result.statement, cursor);
            if (from === -1) continue;
            const to = from + result.statement.length;
            diagnostics.push({
                from,
                to,
                severity: 'error',
                message: result.errorCode ? `${result.error} (${result.errorCode})` : (result.error ?? 'Query failed.'),
                source: 'sql-server',
            });
            cursor = to;
        }
        return diagnostics;
    }
    function lintSource(v: EditorView): Diagnostic[] {
        return [...parseDiagnostics(v.state), ...serverDiagnostics(v.state)];
    }
    // Full-width line decorations for whichever statement the cursor is
    // currently inside, so Mod-Enter/Run Current Statement's target is
    // visually obvious before it runs. Skipped entirely for a single-
    // statement document — every statement is "the current one" there, so a
    // permanent tint would just be noise.
    function statementDecorations(state: EditorState, range: StatementRange | undefined): DecorationSet {
        if (!range) return Decoration.none;
        const last = state.doc.length;
        const fromLine = state.doc.lineAt(Math.min(range.start, last)).number;
        const toLine = state.doc.lineAt(Math.min(range.end, last)).number;
        const marks = [];
        for (let n = fromLine; n <= toLine; n++) marks.push(Decoration.line({ class: 'cm-current-statement' }).range(state.doc.line(n).from));
        return Decoration.set(marks);
    }
    // A subtle left-edge mark on any line a sibling Explain document's
    // hotspot-badged node maps back to. Deliberately not an error/warning
    // color: a hotspot is where the plan spent its time, not necessarily
    // something wrong with the query.
    function hotspotDecorations(state: EditorState, ranges: SqlRange[]): DecorationSet {
        if (!ranges.length) return Decoration.none;
        const last = state.doc.length;
        const lines = new Set<number>();
        for (const r of ranges) lines.add(state.doc.lineAt(Math.min(r.from, last)).number);
        const marks = [...lines].sort((a, b) => a - b).map((n) => Decoration.line({ class: 'cm-hotspot-line' }).range(state.doc.line(n).from));
        return Decoration.set(marks);
    }
    function currentStatement(ranges: StatementRange[], cursor: number): StatementRange | undefined {
        if (ranges.length <= 1) return undefined;
        return ranges.find((r) => cursor >= r.start && cursor <= r.end) ?? ranges.find((r) => r.start >= cursor);
    }
    // Mirrors internal/database/dialect/params.go's scanParams (quote/comment/
    // dollar-quote skipping, "::" never a placeholder) for display only — the
    // backend remains the source of truth for what actually gets bound.
    function scanParamRanges(pg: boolean, text: string): { from: number; to: number }[] {
        const ranges: { from: number; to: number }[] = [];
        const n = text.length;
        const isIdentByte = (c: string) => /[A-Za-z0-9_$]/.test(c);
        let i = 0;
        while (i < n) {
            const c = text[i];
            if (c === "'" || c === '"' || (!pg && c === '`')) {
                const quote = c;
                const escape = !pg || (i > 0 && (text[i - 1] === 'e' || text[i - 1] === 'E'));
                i++;
                let closed = false;
                while (i < n) {
                    if (escape && text[i] === '\\') {
                        i += 2;
                        continue;
                    }
                    if (text[i] === quote) {
                        i++;
                        if (i < n && text[i] === quote) {
                            i++;
                            continue;
                        }
                        closed = true;
                        break;
                    }
                    i++;
                }
                if (!closed) break;
                continue;
            }
            if (text.startsWith('--', i) && (pg || i + 2 === n || text.charCodeAt(i + 2) <= 32)) {
                while (i < n && text[i] !== '\n') i++;
                continue;
            }
            if (!pg && c === '#') {
                while (i < n && text[i] !== '\n') i++;
                continue;
            }
            if (text.startsWith('/*', i)) {
                const end = text.indexOf('*/', i + 2);
                if (end < 0) break;
                i = end + 2;
                continue;
            }
            if (pg && c === '$' && (i === 0 || !isIdentByte(text[i - 1]))) {
                let j = i + 1;
                while (j < n && (text[j] === '_' || /[A-Za-z]/.test(text[j]) || (j > i + 1 && /[0-9]/.test(text[j])))) j++;
                if (j < n && text[j] === '$') {
                    const tag = text.slice(i, j + 1);
                    const end = text.indexOf(tag, j + 1);
                    if (end < 0) break;
                    i = end + tag.length;
                    continue;
                }
            }
            if (c === ':') {
                if (i + 1 < n && text[i + 1] === ':') {
                    i += 2;
                    continue;
                }
                let j = i + 1;
                while (j < n && /[A-Za-z0-9_]/.test(text[j])) j++;
                if (j > i + 1) {
                    ranges.push({ from: i, to: j });
                    i = j;
                    continue;
                }
            }
            i++;
        }
        return ranges;
    }
    function paramHighlighter(pg: boolean) {
        function build(view: EditorView): DecorationSet {
            const text = view.state.doc.toString();
            return Decoration.set(scanParamRanges(pg, text).map((r) => Decoration.mark({ class: 'cm-param' }).range(r.from, r.to)));
        }
        return ViewPlugin.fromClass(
            class {
                decorations: DecorationSet;
                constructor(view: EditorView) {
                    this.decorations = build(view);
                }
                update(update: ViewUpdate) {
                    if (update.docChanged) this.decorations = build(update.view);
                }
            },
            { decorations: (v) => v.decorations },
        );
    }
    // Finds `)` matching the `(` at openIdx, walking over nested parens and
    // skipping quotes/comments the same way scanParamRanges does — needed
    // because a CTE body can itself contain parenthesized subexpressions.
    function findMatchingParen(pg: boolean, text: string, openIdx: number): number {
        let i = openIdx + 1;
        let depth = 1;
        const n = text.length;
        while (i < n && depth > 0) {
            const c = text[i];
            if (c === "'" || c === '"' || (!pg && c === '`')) {
                const quote = c;
                const escape = !pg || (i > 0 && (text[i - 1] === 'e' || text[i - 1] === 'E'));
                i++;
                while (i < n) {
                    if (escape && text[i] === '\\') {
                        i += 2;
                        continue;
                    }
                    if (text[i] === quote) {
                        i++;
                        if (i < n && text[i] === quote) {
                            i++;
                            continue;
                        }
                        break;
                    }
                    i++;
                }
                continue;
            }
            if (text.startsWith('--', i)) {
                while (i < n && text[i] !== '\n') i++;
                continue;
            }
            if (!pg && c === '#') {
                while (i < n && text[i] !== '\n') i++;
                continue;
            }
            if (text.startsWith('/*', i)) {
                const end = text.indexOf('*/', i + 2);
                i = end < 0 ? n : end + 2;
                continue;
            }
            if (c === '(') depth++;
            else if (c === ')') depth--;
            i++;
        }
        return i - 1;
    }
    // depth-0 index of the first occurrence of any keyword in `words` at or
    // after `from`, or text.length when none is found — used to find a CTE
    // body's top-level FROM/WHERE/etc. boundary without matching one nested
    // inside a subquery.
    function topLevelKeyword(text: string, from: number, words: RegExp): number {
        let depth = 0;
        for (let j = from; j < text.length; j++) {
            const c = text[j];
            if (c === '(') depth++;
            else if (c === ')') depth--;
            else if (depth === 0) {
                words.lastIndex = j;
                const m = words.exec(text);
                if (m && m.index === j) return j;
            }
        }
        return text.length;
    }
    function splitTopLevel(text: string): string[] {
        const parts: string[] = [];
        let depth = 0;
        let start = 0;
        for (let j = 0; j < text.length; j++) {
            const c = text[j];
            if (c === '(') depth++;
            else if (c === ')') depth--;
            else if (c === ',' && depth === 0) {
                parts.push(text.slice(start, j));
                start = j + 1;
            }
        }
        parts.push(text.slice(start));
        return parts;
    }
    // Best-effort output column name for one SELECT-list expression: an
    // explicit "AS alias", or the expression's own trailing identifier
    // ("t.name" -> "name", a bare column -> itself). A wildcard or an
    // expression with no discernible trailing name is skipped rather than
    // guessed at.
    function exprColumnName(expr: string): string | null {
        const trimmed = expr.trim();
        if (!trimmed || trimmed === '*' || /\.\*$/.test(trimmed)) return null;
        const idPattern = '([A-Za-z_][A-Za-z0-9_]*|"[^"]+"|`[^`]+`)';
        const asMatch = new RegExp(`\\bas\\s+${idPattern}\\s*$`, 'i').exec(trimmed);
        const bareMatch = asMatch ?? new RegExp(`${idPattern}\\s*$`).exec(trimmed);
        return bareMatch ? bareMatch[1].replace(/^["`]|["`]$/g, '') : null;
    }
    const cteBoundaryKeywords = /\b(from|where|group|having|order|limit|offset|window|union|intersect|except)\b/gi;
    function cteBodyColumns(body: string): string[] {
        const selectMatch = /\bselect\b/i.exec(body);
        if (!selectMatch) return [];
        let i = selectMatch.index + selectMatch[0].length;
        const distinctMatch = /^\s*(distinct|all)\b/i.exec(body.slice(i));
        if (distinctMatch) i += distinctMatch[0].length;
        const end = topLevelKeyword(body, i, cteBoundaryKeywords);
        return splitTopLevel(body.slice(i, end))
            .map(exprColumnName)
            .filter((name): name is string => !!name);
    }
    interface ParsedCTE {
        name: string;
        columns: string[];
    }
    // A regex/text-based CTE finder rather than a lezer tree walk: this SQL
    // grammar is intentionally flat/lenient (see how lang-sql's own alias
    // tracking manually scans sibling tokens instead of relying on dedicated
    // Select/From nodes), so a careful paren-depth-aware text scan is no less
    // reliable here and far easier to get right without a WITH-clause AST to
    // walk. Best-effort by design: an expression this can't name is silently
    // skipped rather than guessed at (see exprColumnName).
    function parseCTEs(pg: boolean, text: string): ParsedCTE[] {
        const ctes: ParsedCTE[] = [];
        const withRe = /\bwith\b/gi;
        for (let m = withRe.exec(text); m; m = withRe.exec(text)) {
            let i = m.index + m[0].length;
            const recMatch = /^\s*recursive\b/i.exec(text.slice(i));
            if (recMatch) i += recMatch[0].length;
            for (;;) {
                const nameMatch = /^\s*([A-Za-z_][A-Za-z0-9_]*|"[^"]+"|`[^`]+`)/.exec(text.slice(i));
                if (!nameMatch) break;
                const name = nameMatch[1].replace(/^["`]|["`]$/g, '');
                i += nameMatch[0].length;
                const colListMatch = /^\s*\(([^()]*)\)/.exec(text.slice(i));
                let explicitCols: string[] | null = null;
                if (colListMatch) {
                    explicitCols = colListMatch[1]
                        .split(',')
                        .map((s) => s.trim().replace(/^["`]|["`]$/g, ''))
                        .filter(Boolean);
                    i += colListMatch[0].length;
                }
                const asMatch = /^\s*as\b/i.exec(text.slice(i));
                if (!asMatch) break;
                i += asMatch[0].length;
                const openMatch = /^\s*\(/.exec(text.slice(i));
                if (!openMatch) break;
                const openIdx = i + openMatch[0].length - 1;
                const closeIdx = findMatchingParen(pg, text, openIdx);
                const body = text.slice(openIdx + 1, closeIdx);
                const columns = explicitCols ?? cteBodyColumns(body);
                if (columns.length) ctes.push({ name, columns });
                i = closeIdx + 1;
                const commaMatch = /^\s*,/.exec(text.slice(i));
                if (!commaMatch) break;
                i += commaMatch[0].length;
            }
            withRe.lastIndex = i;
        }
        return ctes;
    }
    function cteNamespace(pg: boolean, text: string): Record<string, SQLNamespace> {
        const result: Record<string, SQLNamespace> = {};
        for (const cte of parseCTEs(pg, text)) {
            result[cte.name] = {
                self: { label: cte.name, type: 'type', detail: 'CTE' },
                children: cte.columns.map((name): Completion => ({
                    label: name,
                    type: 'property',
                })),
            };
        }
        return result;
    }
    // --- Hover metadata and go-to-definition -----------------------------
    // Ported from @codemirror/lang-sql's own (unexported) qualifier-path
    // resolution (getAliases/pathFor/parentsFor in its source), so hover can
    // answer "what does the identifier under the pointer refer to" the same
    // way its completion source already does internally.
    function idNameFrom(doc: Text, node: SyntaxNode): string {
        const text = doc.sliceString(node.from, node.to);
        const m = /^([`'"[])(.*)([`'"\]])$/.exec(text);
        return m ? m[2] : text;
    }
    function isPlainId(node: SyntaxNode | null): node is SyntaxNode {
        return !!node && (node.name === 'Identifier' || node.name === 'QuotedIdentifier');
    }
    function pathForNode(doc: Text, node: SyntaxNode): string[] {
        if (node.name === 'CompositeIdentifier') {
            const path: string[] = [];
            for (let c = node.firstChild; c; c = c.nextSibling) if (isPlainId(c)) path.push(idNameFrom(doc, c));
            return path;
        }
        return [idNameFrom(doc, node)];
    }
    function tokenBeforeNode(node: SyntaxNode | null): SyntaxNode | null {
        if (!node) return null;
        const cursor = node.cursor().moveTo(node.from, -1);
        while (/Comment/.test(cursor.name)) cursor.moveTo(cursor.from, -1);
        return cursor.node;
    }
    function parentsForNode(doc: Text, start: SyntaxNode | null): string[] {
        const path: string[] = [];
        let node = start;
        while (node && node.name === '.') {
            const name = tokenBeforeNode(node);
            if (!isPlainId(name)) break;
            path.unshift(idNameFrom(doc, name));
            node = tokenBeforeNode(name);
        }
        return path;
    }
    const endFromKeywords = new Set(['where', 'group', 'having', 'order', 'union', 'intersect', 'except', 'all', 'distinct', 'limit', 'offset', 'fetch', 'for']);
    function aliasesFor(doc: Text, at: SyntaxNode): Record<string, string[]> | null {
        let statement: SyntaxNode | null = at;
        while (statement && statement.name !== 'Statement') statement = statement.parent;
        if (!statement) return null;
        let aliases: Record<string, string[]> | null = null;
        let sawFrom = false;
        let prevID: SyntaxNode | null = null;
        for (let scan: SyntaxNode | null = statement.firstChild; scan; scan = scan.nextSibling) {
            const kw = scan.name === 'Keyword' ? doc.sliceString(scan.from, scan.to).toLowerCase() : null;
            let alias: string | null = null;
            if (!sawFrom) {
                sawFrom = kw === 'from';
            } else if (kw === 'as' && prevID && isPlainId(scan.nextSibling)) {
                alias = idNameFrom(doc, scan.nextSibling!);
            } else if (kw && endFromKeywords.has(kw)) {
                break;
            } else if (prevID && isPlainId(scan)) {
                alias = idNameFrom(doc, scan);
            }
            if (alias) {
                aliases ??= Object.create(null);
                aliases![alias.toLowerCase()] = pathForNode(doc, prevID!);
            }
            prevID = /Identifier$/.test(scan.name) ? scan : null;
        }
        return aliases;
    }
    function identifierContext(state: EditorState, pos: number): { name: string; parents: string[] } | null {
        const node = syntaxTree(state).resolveInner(pos, -1);
        if (node.name !== 'Identifier' && node.name !== 'QuotedIdentifier') return null;
        const doc = state.doc;
        const aliases = aliasesFor(doc, node);
        let parents = parentsForNode(doc, tokenBeforeNode(node));
        if (aliases && parents.length === 1) parents = aliases[parents[0].toLowerCase()] ?? parents;
        return { name: idNameFrom(doc, node), parents };
    }
    function findTableByPath(tables: SchemaTable[], path: string[]): SchemaTable | undefined {
        const lower = path.map((p) => p.toLowerCase());
        if (lower.length === 0) return undefined;
        if (lower.length >= 2) {
            return tables.find((t) => (t.schema ?? '').toLowerCase() === lower[lower.length - 2] && (t.objectName ?? t.name).toLowerCase() === lower[lower.length - 1]);
        }
        return tables.find((t) => !t.schema && t.name.toLowerCase() === lower[0]);
    }
    function findColumn(table: SchemaTable, name: string): SchemaColumn | undefined {
        return table.columns.find((c) => c.name.toLowerCase() === name.toLowerCase());
    }
    type HoverInfo =
        | { kind: 'table'; table: SchemaTable }
        | {
              kind: 'column';
              table: SchemaTable | undefined;
              name: string;
              column: SchemaColumn | undefined;
              ambiguous?: boolean;
          };
    function resolveHover(state: EditorState, pos: number): HoverInfo | null {
        const ctx = identifierContext(state, pos);
        if (!ctx || !ctx.name) return null;
        if (ctx.parents.length === 0) {
            const table = findTableByPath(schema, [ctx.name]);
            if (table) return { kind: 'table', table };
            const node = syntaxTree(state).resolveInner(pos, -1);
            const aliases = aliasesFor(state.doc, node) ?? {};
            const candidates = Object.values(aliases)
                .map((path) => findTableByPath(schema, path))
                .filter((t): t is SchemaTable => !!t);
            const pool = candidates.length ? candidates : schema;
            const matches = pool.filter((t) => findColumn(t, ctx.name));
            if (!matches.length) return null;
            return {
                kind: 'column',
                table: matches[0],
                name: ctx.name,
                column: findColumn(matches[0], ctx.name),
                ambiguous: matches.length > 1,
            };
        }
        const table = findTableByPath(schema, ctx.parents);
        if (table) return { kind: 'column', table, name: ctx.name, column: findColumn(table, ctx.name) };
        if (ctx.parents.length === 1 && schema.some((t) => (t.schema ?? '').toLowerCase() === ctx.parents[0].toLowerCase())) {
            const qualified = findTableByPath(schema, [...ctx.parents, ctx.name]);
            if (qualified) return { kind: 'table', table: qualified };
        }
        return null;
    }
    function renderHoverDom(info: HoverInfo): HTMLElement {
        const root = window.document.createElement('div');
        root.className = 'cm-hover-info';
        const title = window.document.createElement('div');
        title.className = 'cm-hover-title';
        if (info.kind === 'table') {
            title.textContent = (info.table.schema ? `${info.table.schema}.` : '') + (info.table.objectName ?? info.table.name) + `  ·  ${info.table.kind}`;
            root.appendChild(title);
            const list = window.document.createElement('div');
            list.className = 'cm-hover-columns';
            const shown = info.table.columns.slice(0, 20);
            for (const c of shown) {
                const row = window.document.createElement('div');
                const bits = [c.type || 'unknown'];
                if (c.primaryKey) bits.push('PK');
                if (c.notNull) bits.push('not null');
                row.textContent = `${c.name}  ${bits.join(' · ')}`;
                list.appendChild(row);
            }
            if (info.table.columns.length > shown.length) {
                const more = window.document.createElement('div');
                more.className = 'cm-hover-note';
                more.textContent = `+${info.table.columns.length - shown.length} more`;
                list.appendChild(more);
            }
            root.appendChild(list);
        } else {
            title.textContent = info.name;
            root.appendChild(title);
            const detail = window.document.createElement('div');
            if (info.column) {
                const bits = [info.column.type || 'unknown'];
                if (info.column.primaryKey) bits.push('PK');
                if (info.column.notNull) bits.push('not null');
                detail.textContent = `${bits.join(' · ')} — ${info.table ? (info.table.objectName ?? info.table.name) : 'unknown table'}`;
            } else {
                detail.textContent = info.table ? `column of ${info.table.objectName ?? info.table.name}` : 'column';
            }
            root.appendChild(detail);
            if (info.ambiguous) {
                const note = window.document.createElement('div');
                note.className = 'cm-hover-note';
                note.textContent = 'Ambiguous — matched the first joined table with this column.';
                root.appendChild(note);
            }
        }
        return root;
    }
    const schemaHover = hoverTooltip((v, pos) => {
        const info = resolveHover(v.state, pos);
        if (!info) return null;
        const node = syntaxTree(v.state).resolveInner(pos, -1);
        return {
            pos: node.from,
            end: node.to,
            above: true,
            create: () => ({ dom: renderHoverDom(info) }),
        };
    });
    function goToDefinitionAt(pos: number) {
        const session = workspace.sessions.find((s) => s.id === sessionId);
        if (session) workspace.goToDefinition(session, document, pos);
    }
    let host: HTMLDivElement;
    let view = $state<EditorView>();
    const appearance = new Compartment();
    const language = new Compartment();
    const statementHighlight = new Compartment();
    const hotspotHighlight = new Compartment();
    // Keyed by DriverMetadata.sqlDialect (backend truth, see
    // internal/database/driver), not the engine name directly: a plugin
    // driver naming a dialect this map has no entry for falls back to
    // SQLite's lenient grammar rather than crashing.
    const sqlDialects: Record<string, SQLDialect> = {
        postgres: PostgreSQL,
        mysql: MySQL,
        sqlite: SQLite,
    };
    // Takes every input explicitly (rather than reading engine/schema/boosts/
    // document.sql from component scope) so the debounced reconfigure effect
    // below can capture them synchronously — reads inside a setTimeout
    // callback don't count as $effect dependencies.
    function languageExtension(text: string, eng: Engine, tables: SchemaTable[], boostMap: Map<string, number>) {
        const pg = eng === 'postgres';
        const dialectName = workspace.drivers.find((d) => d.name === eng)?.sqlDialect;
        const dialect = (dialectName && sqlDialects[dialectName]) || SQLite;
        const namespace = { ...completionSchema(tables, boostMap), ...cteNamespace(pg, text) };
        return [sql({ dialect, schema: namespace }), paramHighlighter(pg)];
    }
    const highlight = HighlightStyle.define([
        { tag: tags.keyword, color: 'var(--syntax-keyword)' },
        { tag: tags.comment, color: 'var(--syntax-comment)' },
        { tag: tags.string, color: 'var(--success)' },
        { tag: tags.number, color: 'var(--syntax-number)' },
        { tag: tags.operator, color: 'var(--accent-text)' },
    ]);
    function options() {
        const p = workspace.preferences;
        return [
            p.lineNumbers ? lineNumbers() : [],
            // Folding a CTE/subquery is only useful alongside a gutter to click;
            // tied to the same preference the line-number gutter uses so hiding
            // gutters doesn't leave one behind.
            p.lineNumbers ? foldGutter() : [],
            p.lineNumbers ? lintGutter() : [],
            p.activeLine ? [highlightActiveLine(), highlightActiveLineGutter()] : [],
            p.wordWrap ? EditorView.lineWrapping : [],
            EditorState.tabSize.of(p.tabSize),
            indentUnit.of(p.indentTabs ? '\t' : ' '.repeat(p.tabSize)),
        ];
    }
    let boosts = $derived(tableBoosts(workspace.history, profileId, schema));
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
                    linter(lintSource),
                    keymap.of([
                        {
                            key: 'Mod-Enter',
                            run: () => {
                                onrun();
                                return true;
                            },
                        },
                        {
                            key: 'F12',
                            run: (v) => {
                                goToDefinitionAt(v.state.selection.main.head);
                                return true;
                            },
                        },
                        ...closeBracketsKeymap,
                        ...defaultKeymap,
                        ...historyKeymap,
                        ...foldKeymap,
                        ...completionKeymap,
                        ...searchKeymap,
                        ...lintKeymap,
                        indentWithTab,
                    ]),
                    EditorView.domEventHandlers({
                        mousedown: (event, v) => {
                            if (!(event.metaKey || event.ctrlKey)) return false;
                            const pos = v.posAtCoords({ x: event.clientX, y: event.clientY });
                            if (pos == null) return false;
                            goToDefinitionAt(pos);
                            return true;
                        },
                    }),
                    search(),
                    language.of(languageExtension(document.sql, engine, schema, boosts)),
                    statementHighlight.of([]),
                    hotspotHighlight.of([]),
                    schemaHover,
                    EditorState.languageData.of(() => [{ autocomplete: snippetSource }]),
                    autocompletion(),
                    syntaxHighlighting(highlight),
                    appearance.of(options()),
                    EditorView.contentAttributes.of({ 'aria-label': 'SQL query editor' }),
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
    $effect(() => {
        const extensions = options();
        view?.dispatch({ effects: appearance.reconfigure(extensions) });
    });
    // Debounced so CTE re-parsing (and the completion namespace rebuild it
    // feeds) doesn't run on every keystroke — schema/engine/boost changes are
    // infrequent enough that the same debounce costs them nothing noticeable.
    $effect(() => {
        const text = document.sql;
        const eng = engine;
        const tables = schema;
        const boostMap = boosts;
        const timer = setTimeout(() => {
            view?.dispatch({
                effects: language.reconfigure(languageExtension(text, eng, tables, boostMap)),
            });
        }, 300);
        return () => clearTimeout(timer);
    });
    // Re-split into statements whenever the text actually changes, debounced
    // so arrow-key/typing bursts don't each fire a backend round trip; the
    // per-keystroke part (which range the cursor is in right now) stays a
    // synchronous lookup in the effect below, using whatever was last split.
    let statementRanges = $state<StatementRange[]>([]);
    $effect(() => {
        const text = document.sql;
        const id = sessionId;
        let cancelled = false;
        const timer = setTimeout(async () => {
            if (!id) return;
            try {
                const ranges = await backend.splitStatements(id, text);
                if (!cancelled) statementRanges = ranges;
            } catch {
                if (!cancelled) statementRanges = [];
            }
        }, 250);
        return () => {
            cancelled = true;
            clearTimeout(timer);
        };
    });
    $effect(() => {
        const range = currentStatement(statementRanges, document.cursor);
        if (!view) return;
        view.dispatch({
            effects: statementHighlight.reconfigure(EditorView.decorations.of(statementDecorations(view.state, range))),
        });
    });
    // Reflect an externally-set document.sql (formatting, opening a saved
    // query, a parameter-prompt retry) back into the editor. The view's own
    // updateListener already keeps document.sql in sync while typing, so this
    // only fires, and only once, when something else changed it.
    $effect(() => {
        const sql = document.sql;
        if (view && view.state.doc.toString() !== sql) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: sql } });
    });
    // A fresh execution result changes what serverDiagnostics() computes
    // without any editor transaction happening (results arrive from a Run
    // action, not from typing), so the linter's own "re-run after an idle
    // period following a document change" trigger never fires on its own —
    // force a lint pass right away so a failed statement squiggles
    // immediately instead of only after the next keystroke's debounce.
    $effect(() => {
        document.results;
        if (view) forceLinting(view);
    });
    $effect(() => {
        const ranges = hotspotRanges;
        if (!view) return;
        view.dispatch({
            effects: hotspotHighlight.reconfigure(EditorView.decorations.of(hotspotDecorations(view.state, ranges))),
        });
    });
    // workspace.revealExplainNode() ("Show in editor" on a plan node) bumps
    // revealSeq and sets selectionFrom/selectionTo/cursor in the same update;
    // keyed on revealSeq alone (not on those fields directly) so this never
    // fires from the editor's own updateListener writing them back on a
    // normal user selection, only on an explicit external request.
    $effect(() => {
        // revealSeq starts at 0 for every document and this effect runs once
        // on mount regardless; skip that initial run so simply opening a query
        // document never steals focus or moves the caret on its own.
        if (!view || document.revealSeq === 0) return;
        const last = view.state.doc.length;
        const from = Math.max(0, Math.min(document.selectionFrom, last));
        const to = Math.max(0, Math.min(document.selectionTo, last));
        view.dispatch({
            selection: { anchor: from, head: to },
            effects: EditorView.scrollIntoView(from, { y: 'center' }),
        });
        view.focus();
    });
</script>

<div class="query-editor" bind:this={host}></div>
