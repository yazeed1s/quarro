import { describe, expect, test } from 'vitest';
import { commandLines, commandNamesFrom } from './redisCommand';

const text = 'GET a\n\n# note\nHGETALL user:1\n  LRANGE l 0 -1  \nPING';
const at = (s: string) => text.indexOf(s);

describe('commandLines', () => {
    test('runs the line under the cursor', () => {
        expect(commandLines(text, at('user:1'), at('user:1'))).toEqual(['HGETALL user:1']);
        expect(commandLines(text, 0, 0)).toEqual(['GET a']);
        expect(commandLines(text, text.length, text.length)).toEqual(['PING']);
    });

    test('a cursor on a blank or comment line runs nothing', () => {
        expect(commandLines(text, at('\n\n') + 1, at('\n\n') + 1)).toEqual([]);
        expect(commandLines(text, at('note'), at('note'))).toEqual([]);
    });

    test('a selection runs every line it touches, skipping blanks and comments', () => {
        expect(commandLines(text, at('a\n'), at('LRANGE') + 2)).toEqual(['GET a', 'HGETALL user:1', 'LRANGE l 0 -1']);
    });

    test('a selection ending at the start of a line leaves that line out', () => {
        expect(commandLines(text, at('HGETALL'), at('  LRANGE'))).toEqual(['HGETALL user:1']);
    });

    test('a backwards selection is the same as a forwards one', () => {
        expect(commandLines(text, at('PING') + 2, at('LRANGE'))).toEqual(['LRANGE l 0 -1', 'PING']);
    });
});

test('commandNamesFrom reads COMMAND LIST and older COMMAND replies', () => {
    expect(commandNamesFrom([{ value: 'GET' }, { value: 'set' }, { value: 'get' }])).toEqual(['get', 'set']);
    expect(commandNamesFrom([{ value: ['hgetall', -2, ['readonly']] }, { value: null }])).toEqual(['hgetall']);
});
