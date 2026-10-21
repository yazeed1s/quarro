import type { UpdateInfo } from './types';

/** How often the startup update check may hit GitHub. */
export const updateCheckInterval = 24 * 60 * 60 * 1000;

export interface UpdateCache {
    /** When GitHub was last asked (ms since epoch); 0 = never. */
    checkedAt: number;
    /** The newer release found by that check, if any. */
    latest: UpdateInfo | null;
    /** A version the user dismissed; its notice never comes back. */
    dismissed: string;
}

export const emptyUpdateCache: UpdateCache = { checkedAt: 0, latest: null, dismissed: '' };

export function shouldCheck(cache: UpdateCache, now: number): boolean {
    // A clock moved backwards also counts as stale.
    return now - cache.checkedAt >= updateCheckInterval || now < cache.checkedAt;
}

export function visibleUpdate(cache: UpdateCache, currentVersion: string): UpdateInfo | null {
    const latest = cache.latest;
    if (!latest?.available || latest.version === cache.dismissed || latest.version === currentVersion) return null;
    return latest;
}

/** Parses stored JSON, falling back to an empty cache on anything unexpected. */
export function parseUpdateCache(raw: string | null): UpdateCache {
    try {
        const value = JSON.parse(raw || 'null');
        if (!value || typeof value !== 'object') return { ...emptyUpdateCache };
        const latest = value.latest;
        return {
            checkedAt: Number.isFinite(value.checkedAt) ? value.checkedAt : 0,
            latest:
                latest && typeof latest.version === 'string' && typeof latest.url === 'string'
                    ? { available: latest.available === true, version: latest.version, url: latest.url }
                    : null,
            dismissed: typeof value.dismissed === 'string' ? value.dismissed : '',
        };
    } catch {
        return { ...emptyUpdateCache };
    }
}
