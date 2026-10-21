import type { Preferences, Row } from '../../state/types';
import { isBlobCell } from '../../state/types';
import { copyText } from '../../state/backend';
export function display(value: Row[string], preferences: Preferences): string {
    if (value === null) return preferences.nullDisplay || 'NULL';
    if (isBlobCell(value)) return `<binary, ${value.size} byte${value.size === 1 ? '' : 's'}>`;
    if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:/.test(value)) {
        const date = new Date(value);
        if (!Number.isFinite(date.getTime())) return value;
        if (preferences.dateFormat === 'locale')
            return date.toLocaleString(undefined, {
                timeZone: preferences.timezone === 'UTC' ? 'UTC' : undefined,
            });
        if (preferences.timezone === 'local') {
            const pad = (v: number) => String(v).padStart(2, '0');
            return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
        }
        return date.toISOString().replace('T', ' ').replace('Z', ' UTC');
    }
    if (typeof value === 'object') {
        const count = Array.isArray(value) ? value.length : Object.keys(value).length;
        return Array.isArray(value) ? `[${count} item${count === 1 ? '' : 's'}]` : `{${count} field${count === 1 ? '' : 's'}}`;
    }
    return String(value);
}
// Classifies a SQLite declared column type into a compact header glyph.
// SQLite's type affinity rules mean a declared type can be almost any
// string a table author chose, so this matches on substrings rather than
// an exact set (the same convention SQLite itself uses for affinity).
export function columnGlyph(declaredType: string): '#' | '◷' | 'T' {
    const t = declaredType.toUpperCase();
    if (t.includes('DATE') || t.includes('TIME')) return '◷';
    if (t.includes('INT') || t.includes('REAL') || t.includes('FLOA') || t.includes('DOUB') || t.includes('NUMERIC') || t.includes('DECIMAL')) return '#';
    return 'T';
}
export async function copy(text: string, notify: (text: string) => void) {
    try {
        // The native clipboard works without the user-gesture and permission
        // rules WebKitGTK/WKWebView apply to navigator.clipboard.
        if (!(await copyText(text))) await navigator.clipboard.writeText(text);
        notify('Copied to clipboard.');
    } catch {
        notify('Clipboard access is unavailable in this environment.');
    }
}
