// Custom MIME type for dragging a table/view name onto the Visual Query
// Builder canvas, from the object browser sidebar or the builder's own
// table search results. Using a custom type (rather than 'text/plain')
// lets the canvas's drop handler tell "a table was dropped" apart from an
// accidental text/file drop.
export const TABLE_DRAG_MIME = 'application/x-quarro-table';

// Custom MIME type for dragging one or several column rows off a
// QueryTableNode onto the Columns/Filters/Order By panels. The payload is
// JSON: { table: alias, columns: string[] } — one owning table alias per
// drag, since a node's columns all belong to the same table.
export const COLUMN_DRAG_MIME = 'application/x-quarro-columns';

export interface ColumnDragPayload {
    table: string;
    columns: string[];
}

// The embedded webview's native drag-image surface (WebKitGTK on Linux,
// WebView2 on Windows) sometimes never gets dismissed once the drop lands
// inside the query canvas's CSS-transformed (pan/zoom) surface, leaving a
// ghost of the drag-image bitmap stuck on screen; it survives regardless of
// what happens to the DOM element that was snapshotted, since the webview
// already copied it into its own native surface at drag start; removing
// that source element sooner, later, or on dragend makes no difference. The
// only reliable fix is giving it nothing to persist: a transparent pixel
// instead of a rendered label, so there's no visible bitmap to get stuck.
// A GIF's transparency is a palette index, not a real alpha channel, and
// this native compositor path doesn't reliably honor it, leaving a faint
// opaque speck. A true-alpha PNG (RGBA, alpha 0) removes that ambiguity.
const transparentDragImage =
    typeof Image !== 'undefined'
        ? Object.assign(new Image(), { src: 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR4nGNgAAIAAAUAAXpeqz8AAAAASUVORK5CYII=' })
        : null;
export function setTableDragImage(event: DragEvent, _label: string) {
    if (transparentDragImage) event.dataTransfer?.setDragImage(transparentDragImage, 0, 0);
}

export function parseColumnDrag(event: DragEvent): ColumnDragPayload | null {
    const raw = event.dataTransfer?.getData(COLUMN_DRAG_MIME);
    if (!raw) return null;
    try {
        const parsed = JSON.parse(raw);
        if (typeof parsed?.table === 'string' && Array.isArray(parsed?.columns)) {
            const columns = parsed.columns.filter((c: unknown): c is string => typeof c === 'string');
            if (columns.length) return { table: parsed.table, columns };
        }
    } catch {
        // Not a column drag payload — ignore.
    }
    return null;
}
