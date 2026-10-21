import type { Preferences } from './types';

// Shared by the grid virtualizer and the settings preview.
export function effectiveRowHeight(p: Preferences): number {
    const compact = p.density === 'compact';
    return Math.max(p.rowHeight - (compact ? 6 : 0), p.gridSize * 1.5 + (compact ? 6 : 10), compact ? 24 : 28);
}
