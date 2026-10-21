import type { KVString } from '../../state/types';

// formatTTL renders a remaining expiry. Under a second reads "<1s" rather
// than "0s", which would look like "expires now".
export function formatTTL(ms: number | undefined): string {
    if (ms == null || ms <= 0) return 'No expiry';
    if (ms < 1000) return '<1s';
    let rest = Math.floor(ms / 1000);
    const units: [string, number][] = [
        ['d', 86400],
        ['h', 3600],
        ['m', 60],
        ['s', 1],
    ];
    const parts: string[] = [];
    for (const [label, size] of units) {
        if (rest >= size) {
            parts.push(`${Math.floor(rest / size)}${label}`);
            rest %= size;
        }
        if (parts.length === 2) break;
    }
    return parts.join(' ');
}

const durationUnits: Record<string, number> = { ms: 1, s: 1000, m: 60_000, h: 3_600_000, d: 86_400_000 };

// parseDuration reads a TTL typed as "30s", "15m", "2h", "7d", a
// combination like "1h 30m", or a bare number of seconds, and returns
// milliseconds. Anything else, and anything not above zero, is undefined:
// a zero TTL would delete the key rather than set an expiry.
export function parseDuration(text: string): number | undefined {
    const t = text.trim().toLowerCase();
    if (!t) return undefined;
    let total = 0;
    if (/^\d+(\.\d+)?$/.test(t)) {
        total = Number(t) * 1000;
    } else {
        const part = /(\d+(?:\.\d+)?)\s*(ms|s|m|h|d)\s*/y;
        let end = 0;
        for (let m = part.exec(t); m; m = part.exec(t)) {
            total += Number(m[1]) * durationUnits[m[2]];
            end = part.lastIndex;
        }
        if (end !== t.length) return undefined;
    }
    const ms = Math.round(total);
    return ms > 0 && Number.isSafeInteger(ms) ? ms : undefined;
}

const typeLabels: Record<string, string> = {
    string: 'String',
    hash: 'Hash',
    list: 'List',
    set: 'Set',
    zset: 'Sorted set',
    stream: 'Stream',
};

// editableTypes are the types the key inspector can edit; streams are an
// append log, read-only here.
export const editableTypes = ['string', 'hash', 'list', 'set', 'zset'];

export function typeLabel(type: string | undefined): string {
    return type ? (typeLabels[type] ?? type) : '';
}

// formatLength describes a key's size the way its type counts it.
export function formatLength(type: string | undefined, length: number): string {
    if (type === 'string') return length === 1 ? '1 byte' : `${length.toLocaleString()} bytes`;
    if (type === 'hash') return length === 1 ? '1 field' : `${length.toLocaleString()} fields`;
    if (type === 'stream') return length === 1 ? '1 entry' : `${length.toLocaleString()} entries`;
    if (type && type in typeLabels) return length === 1 ? '1 item' : `${length.toLocaleString()} items`;
    return '';
}

// kvText is a value's display text. Binary values show as hex (capped,
// since this is for reading, not copying) instead of mangled text.
export function kvText(s: KVString | undefined, maxBytes = 512): string {
    if (!s) return '';
    if (!s.binary) return s.text;
    const bytes = Uint8Array.from(atob(s.base64 ?? ''), (c) => c.charCodeAt(0));
    const hex = Array.from(bytes.subarray(0, maxBytes), (b) => b.toString(16).padStart(2, '0')).join(' ');
    return bytes.length > maxBytes ? `${hex} …` : hex;
}

// prettyJSON returns text reformatted as indented JSON when it is a JSON
// object or array, a common way to store structured data in a Redis string.
export function prettyJSON(text: string): string | undefined {
    const t = text.trim();
    if (!(t.startsWith('{') || t.startsWith('['))) return undefined;
    try {
        return JSON.stringify(JSON.parse(t), null, 2);
    } catch {
        return undefined;
    }
}
