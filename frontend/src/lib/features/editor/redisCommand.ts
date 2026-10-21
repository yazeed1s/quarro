import { StreamLanguage } from '@codemirror/language';
import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete';

// commandLines returns the command lines to run for a cursor (from === to:
// the line under it) or a selection (every line it touches). Blank lines
// and "#" comment lines are skipped, so a scratchpad can hold notes. A
// selection that ends at the very start of a line does not include that
// line, matching how a drag-selected block of lines usually ends.
export function commandLines(text: string, from: number, to: number): string[] {
    let start = Math.min(from, to);
    let end = Math.max(from, to);
    if (end > start && text[end - 1] === '\n') end--;
    start = text.lastIndexOf('\n', start - 1) + 1;
    const lineEnd = text.indexOf('\n', end);
    end = lineEnd === -1 ? text.length : lineEnd;
    return text
        .slice(start, end)
        .split('\n')
        .map((line) => line.trim())
        .filter((line) => line !== '' && !line.startsWith('#'));
}

// redisCommandLanguage highlights a command line: the first word is the
// command, quoted arguments are strings, bare numbers are numbers, and a
// line starting with "#" is a comment.
export const redisCommandLanguage = StreamLanguage.define<{ first: boolean }>({
    name: 'redis',
    startState: () => ({ first: true }),
    token(stream, state) {
        if (stream.sol()) state.first = true;
        if (stream.eatSpace()) return null;
        if (state.first && stream.peek() === '#') {
            stream.skipToEnd();
            return 'comment';
        }
        const quote = stream.peek();
        if (quote === '"' || quote === "'") {
            stream.next();
            let escaped = false;
            while (!stream.eol()) {
                const ch = stream.next();
                if (ch === quote && !escaped) break;
                escaped = !escaped && ch === '\\';
            }
            state.first = false;
            return 'string';
        }
        stream.eatWhile(/[^\s"']/);
        if (state.first) {
            state.first = false;
            return 'keyword';
        }
        return /^-?\d+(\.\d+)?$/.test(stream.current()) ? 'number' : null;
    },
});

// commandCompletion completes the first word of a line from the server's
// own command list. names is read on every request, since the list
// arrives after the editor mounts.
export function commandCompletion(names: () => string[]) {
    return (ctx: CompletionContext): CompletionResult | null => {
        const line = ctx.state.doc.lineAt(ctx.pos);
        const word = /^\s*(\S*)$/.exec(line.text.slice(0, ctx.pos - line.from));
        if (!word || (!word[1] && !ctx.explicit) || word[1].startsWith('#')) return null;
        const options = names().map((name) => ({ label: name.toUpperCase(), type: 'keyword' }));
        if (!options.length) return null;
        return { from: ctx.pos - word[1].length, options, validFor: /^\S*$/ };
    };
}

// commandNamesFrom reads command names out of a COMMAND LIST reply (one
// name per row) or, on Redis before 7, a COMMAND reply (one nested entry
// per row whose first element is the name).
export function commandNamesFrom(rows: Record<string, unknown>[]): string[] {
    const names = rows
        .map((row) => (Array.isArray(row.value) ? row.value[0] : row.value))
        .filter((name): name is string => typeof name === 'string' && name !== '')
        .map((name) => name.toLowerCase());
    return [...new Set(names)].sort();
}
