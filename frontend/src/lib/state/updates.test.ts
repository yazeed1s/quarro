import { describe, expect, it } from 'vitest';
import { emptyUpdateCache, parseUpdateCache, shouldCheck, updateCheckInterval, visibleUpdate } from './updates';

const release = { available: true, version: '0.2.0', url: 'https://github.com/yazeed1s/quarro/releases/tag/v0.2.0' };

describe('update cache', () => {
    it('checks at most once per interval', () => {
        const now = 10 * updateCheckInterval;
        expect(shouldCheck(emptyUpdateCache, now)).toBe(true);
        expect(shouldCheck({ ...emptyUpdateCache, checkedAt: now - 1000 }, now)).toBe(false);
        expect(shouldCheck({ ...emptyUpdateCache, checkedAt: now - updateCheckInterval }, now)).toBe(true);
        // A clock set backwards must not suppress checks for days.
        expect(shouldCheck({ ...emptyUpdateCache, checkedAt: now + 1000 }, now)).toBe(true);
    });

    it('shows a found release until it is dismissed or installed', () => {
        expect(visibleUpdate({ ...emptyUpdateCache, latest: release }, '0.1.0')).toEqual(release);
        expect(visibleUpdate({ ...emptyUpdateCache, latest: release, dismissed: '0.2.0' }, '0.1.0')).toBeNull();
        expect(visibleUpdate({ ...emptyUpdateCache, latest: release }, '0.2.0')).toBeNull();
        expect(visibleUpdate(emptyUpdateCache, '0.1.0')).toBeNull();
    });

    it('parses stored values defensively', () => {
        expect(parseUpdateCache(null)).toEqual(emptyUpdateCache);
        expect(parseUpdateCache('{')).toEqual(emptyUpdateCache);
        expect(parseUpdateCache('"text"')).toEqual(emptyUpdateCache);
        expect(parseUpdateCache(JSON.stringify({ checkedAt: 5, latest: release, dismissed: '0.1.9' }))).toEqual({ checkedAt: 5, latest: release, dismissed: '0.1.9' });
        expect(parseUpdateCache(JSON.stringify({ checkedAt: 'x', latest: { version: 1 }, dismissed: 3 }))).toEqual(emptyUpdateCache);
    });
});
