import type { KeyDetail, KeyMember, KVString } from '../../state/types';

// memberIdentity is what makes two members "the same element". HSCAN and
// SSCAN may return an element on more than one page, so hash fields and
// set members are deduped by it; list, sorted set, and stream pages never
// overlap (they page by offset or ID) and are appended as-is.
function memberIdentity(type: string, m: KeyMember): string | undefined {
    const raw = (s: KVString | undefined) => (s ? (s.binary ? `b:${s.base64}` : `t:${s.text}`) : '');
    if (type === 'hash') return raw(m.field);
    if (type === 'set') return raw(m.value);
    return undefined;
}

// mergeKeyPage appends the next page of the same key to what is already
// loaded. The newer page's length, TTL, and cursor win, since the key may
// have changed between reads; the stream summary only comes with page one.
export function mergeKeyPage(loaded: KeyDetail, page: KeyDetail): KeyDetail {
    if (loaded.key !== page.key || loaded.type !== page.type) return page;
    const seen = new Set<string>();
    const members: KeyMember[] = [];
    for (const m of [...loaded.members, ...page.members]) {
        const identity = memberIdentity(page.type, m);
        if (identity !== undefined) {
            if (seen.has(identity)) continue;
            seen.add(identity);
        }
        members.push(m);
    }
    return { ...page, members, stream: page.stream ?? loaded.stream };
}
