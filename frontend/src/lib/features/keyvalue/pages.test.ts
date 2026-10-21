import { describe, expect, test } from 'vitest';
import type { KeyDetail, KeyMember } from '../../state/types';
import { mergeKeyPage } from './pages';
import { formatTTL, kvText, parseDuration } from './format';

const text = (t: string) => ({ text: t, size: t.length });
const page = (type: string, members: KeyMember[], cursor = ''): KeyDetail => ({ key: 'k', type, exists: true, length: 3, members, cursor });

describe('mergeKeyPage', () => {
    test('drops a hash field HSCAN returned on both pages', () => {
        const first = page(
            'hash',
            [
                { field: text('a'), value: text('1') },
                { field: text('b'), value: text('2') },
            ],
            '7',
        );
        const next = page('hash', [
            { field: text('b'), value: text('2') },
            { field: text('c'), value: text('3') },
        ]);
        const merged = mergeKeyPage(first, next);
        expect(merged.members.map((m) => m.field?.text)).toEqual(['a', 'b', 'c']);
        expect(merged.cursor).toBe('');
    });

    test('keeps a repeated list element, since lists page by offset', () => {
        const merged = mergeKeyPage(page('list', [{ index: 0, value: text('x') }], '1'), page('list', [{ index: 1, value: text('x') }]));
        expect(merged.members).toHaveLength(2);
    });

    test('tells a binary member apart from text that looks like its base64', () => {
        const bin = { text: '', binary: true, base64: 'AA==', size: 1 };
        const merged = mergeKeyPage(page('set', [{ value: bin }], '3'), page('set', [{ value: text('AA==') }]));
        expect(merged.members).toHaveLength(2);
    });

    test('keeps the stream summary from the first page', () => {
        const first = { ...page('stream', [{ id: '1-1', value: text('') }], '1-1'), stream: { firstId: '1-1', lastId: '1-2', groups: [] } };
        const merged = mergeKeyPage(first, page('stream', [{ id: '1-2', value: text('') }]));
        expect(merged.stream?.lastId).toBe('1-2');
        expect(merged.members.map((m) => m.id)).toEqual(['1-1', '1-2']);
    });

    test('replaces instead of merging when the key changed type', () => {
        const merged = mergeKeyPage(page('hash', [{ field: text('a'), value: text('1') }], '7'), page('set', [{ value: text('z') }]));
        expect(merged.type).toBe('set');
        expect(merged.members).toHaveLength(1);
    });
});

describe('formatTTL', () => {
    test.each([
        [undefined, 'No expiry'],
        [400, '<1s'],
        [59_000, '59s'],
        [3_725_000, '1h 2m'],
        [2 * 86_400_000 + 3_600_000 + 5000, '2d 1h'],
    ])('%s ms reads %s', (ms, want) => {
        expect(formatTTL(ms)).toBe(want);
    });
});

test('kvText shows binary bytes as hex', () => {
    expect(kvText({ text: '', binary: true, base64: btoa('\xff\x00'), size: 2 })).toBe('ff 00');
});

describe('parseDuration', () => {
    test.each([
        ['30s', 30_000],
        ['15m', 900_000],
        ['2h', 7_200_000],
        ['7d', 604_800_000],
        ['1h 30m', 5_400_000],
        ['1h30m', 5_400_000],
        ['90', 90_000],
        ['1.5', 1500],
        ['250ms', 250],
        [' 2H ', 7_200_000],
    ])('%s is %d ms', (text, want) => {
        expect(parseDuration(text)).toBe(want);
    });

    // Zero would make PEXPIRE delete the key, so it is not a duration.
    test.each(['', '0', '0s', '-5', '5x', 'h', '1h abc', '1e3', '0.0001'])('%j is rejected', (text) => {
        expect(parseDuration(text)).toBeUndefined();
    });
});
