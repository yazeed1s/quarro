import { objectLabel, type SchemaTable } from '../../state/types';

export type DiagramExportBackground = 'cross' | 'dots' | 'lines' | 'none';
export type DiagramExportEdgeStyle = 'smoothstep' | 'step' | 'straight' | 'bezier';
export type DiagramExportNodeDetail = 'full' | 'names' | 'keys';

export interface DiagramExportPalette {
    surface: string;
    subtle: string;
    card: string;
    border: string;
    borderSoft: string;
    text: string;
    muted: string;
    accent: string;
    accentText: string;
}

export interface DiagramExportOptions {
    tables: SchemaTable[];
    positions: Record<string, { x: number; y: number }>;
    collapsed: string[];
    relatedTable: string;
    nodeDetail: DiagramExportNodeDetail;
    nodeWidth: number;
    columnHeight: number;
    nodeColumnHeight: number;
    edgeLabels: boolean;
    edgeMarkers: boolean;
    edgeStyle: DiagramExportEdgeStyle;
    edgeWidth: number;
    relatedEdgeWidth: number;
    labelSize: number;
    background: DiagramExportBackground;
    gridGap: number;
    gridMarkSize: number;
    palette: DiagramExportPalette;
}

export interface DiagramSVG {
    svg: string;
    width: number;
    height: number;
}

interface ExportNode {
    table: SchemaTable;
    x: number;
    y: number;
    height: number;
    collapsed: boolean;
    rows: ExportRow[];
    hiddenCount: number;
}

interface ExportRow {
    name: string;
    type: string;
    nullable: boolean;
    primaryKey: boolean;
    foreignKey: boolean;
    unique: boolean;
}

const padding = 48;
const headerHeight = 54;
const footerHeight = 36;
const minNodeHeight = headerHeight + footerHeight;

function xml(value: string) {
    return value.replace(/[&<>"']/g, (character) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' })[character] ?? character);
}

function truncate(value: string, length: number) {
    return value.length > length ? `${value.slice(0, Math.max(1, length - 1))}…` : value;
}

function shownRows(table: SchemaTable, detail: DiagramExportNodeDetail, rowLimit: number) {
    const foreignKeys = new Set(table.foreignKeys.map((foreignKey) => foreignKey.column));
    const uniqueColumns = new Set(table.indexes.filter((index) => index.unique && index.columns.length === 1).map((index) => index.columns[0]));
    const candidates = detail === 'keys' ? table.columns.filter((column) => column.primaryKey || foreignKeys.has(column.name)) : table.columns;
    const visible = candidates.slice(0, rowLimit).map((column) => ({
        name: column.name,
        type: column.type || 'value',
        nullable: !column.notNull,
        primaryKey: column.primaryKey,
        foreignKey: foreignKeys.has(column.name),
        unique: uniqueColumns.has(column.name),
    }));
    if (detail === 'keys' && candidates.length === 0) {
        return { rows: [{ name: 'No key columns', type: '', nullable: false, primaryKey: false, foreignKey: false, unique: false }], hiddenCount: 0 };
    }
    return { rows: visible, hiddenCount: Math.max(0, candidates.length - visible.length) };
}

function edgePath(style: DiagramExportEdgeStyle, sourceX: number, sourceY: number, targetX: number, targetY: number) {
    if (style === 'straight') return `M ${sourceX} ${sourceY} L ${targetX} ${targetY}`;
    const middleX = (sourceX + targetX) / 2;
    if (style === 'step') return `M ${sourceX} ${sourceY} L ${middleX} ${sourceY} L ${middleX} ${targetY} L ${targetX} ${targetY}`;
    const distance = Math.max(36, Math.abs(targetX - sourceX) * 0.45);
    return `M ${sourceX} ${sourceY} C ${sourceX + distance} ${sourceY}, ${targetX - distance} ${targetY}, ${targetX} ${targetY}`;
}

function gridDefinition(options: DiagramExportOptions) {
    if (options.background === 'none') return { definition: '', fill: options.palette.surface };
    const gap = options.background === 'lines' ? Math.max(24, options.gridGap + 6) : options.gridGap;
    const color = options.palette.border;
    if (options.background === 'dots') {
        return {
            definition: `<pattern id="grid" width="${gap}" height="${gap}" patternUnits="userSpaceOnUse"><circle cx="${gap / 2}" cy="${gap / 2}" r="${Math.max(0.8, options.gridMarkSize - 0.6)}" fill="${xml(color)}" opacity="0.55"/></pattern>`,
            fill: 'url(#grid)',
        };
    }
    if (options.background === 'lines') {
        return {
            definition: `<pattern id="grid" width="${gap}" height="${gap}" patternUnits="userSpaceOnUse"><path d="M ${gap} 0 L 0 0 0 ${gap}" fill="none" stroke="${xml(color)}" stroke-width="0.5" opacity="0.5"/></pattern>`,
            fill: 'url(#grid)',
        };
    }
    const size = options.gridMarkSize;
    return {
        definition: `<pattern id="grid" width="${gap}" height="${gap}" patternUnits="userSpaceOnUse"><path d="M ${gap / 2 - size} ${gap / 2} H ${gap / 2 + size} M ${gap / 2} ${gap / 2 - size} V ${gap / 2 + size}" fill="none" stroke="${xml(color)}" stroke-width="0.7" opacity="0.55"/></pattern>`,
        fill: 'url(#grid)',
    };
}

export function renderSchemaDiagramSVG(options: DiagramExportOptions): DiagramSVG {
    const collapsed = new Set(options.collapsed);
    const rowLimit = Math.max(1, Math.min(42, Math.floor(options.nodeColumnHeight / options.columnHeight)));
    const rawNodes = options.tables.map((table) => {
        const position = options.positions[table.name] ?? { x: 0, y: 0 };
        const rows = shownRows(table, options.nodeDetail, rowLimit);
        const isCollapsed = collapsed.has(table.name);
        const visibleRows = rows.rows.length + (rows.hiddenCount ? 1 : 0);
        const columnAreaHeight = isCollapsed ? 0 : Math.min(options.nodeColumnHeight, visibleRows * options.columnHeight);
        return {
            table,
            x: position.x,
            y: position.y,
            height: isCollapsed ? minNodeHeight : headerHeight + columnAreaHeight + footerHeight,
            collapsed: isCollapsed,
            rows: rows.rows,
            hiddenCount: rows.hiddenCount,
        };
    });
    const minX = Math.min(...rawNodes.map((node) => node.x), 0);
    const minY = Math.min(...rawNodes.map((node) => node.y), 0);
    const nodes: ExportNode[] = rawNodes.map((node) => ({ ...node, x: node.x - minX + padding, y: node.y - minY + padding }));
    const width = Math.ceil(Math.max(...nodes.map((node) => node.x + options.nodeWidth), padding) + padding);
    const height = Math.ceil(Math.max(...nodes.map((node) => node.y + node.height), padding) + padding);
    const byName = new Map(nodes.map((node) => [node.table.name, node]));
    const grid = gridDefinition(options);
    const marker = options.edgeMarkers
        ? `<marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="context-stroke"/></marker>`
        : '';

    const edges = nodes.flatMap((source) =>
        source.table.foreignKeys.flatMap((foreignKey) => {
            if (foreignKey.refTable === source.table.name) return [];
            const target = byName.get(foreignKey.refTable);
            if (!target) return [];
            const sourceOnLeft = source.x <= target.x;
            const sourceX = sourceOnLeft ? source.x + options.nodeWidth : source.x;
            const targetX = sourceOnLeft ? target.x : target.x + options.nodeWidth;
            const sourceY = source.y + source.height / 2;
            const targetY = target.y + target.height / 2;
            const related = options.relatedTable === source.table.name || options.relatedTable === target.table.name;
            const color = related ? options.palette.accent : options.palette.muted;
            const lineWidth = related ? options.relatedEdgeWidth : options.edgeWidth;
            const path = edgePath(options.edgeStyle, sourceX, sourceY, targetX, targetY);
            const labelX = (sourceX + targetX) / 2;
            const labelY = (sourceY + targetY) / 2;
            const label = `${foreignKey.column} → ${foreignKey.refColumn}`;
            return [
                `<g class="relationship"><path d="${path}" fill="none" stroke="${xml(color)}" stroke-width="${lineWidth}"${options.edgeMarkers ? ' marker-end="url(#arrow)"' : ''}/>${
                    options.edgeLabels
                        ? `<rect x="${labelX - Math.max(28, label.length * options.labelSize * 0.3)}" y="${labelY - options.labelSize}" width="${Math.max(56, label.length * options.labelSize * 0.6)}" height="${options.labelSize + 8}" rx="4" fill="${xml(options.palette.card)}" stroke="${xml(color)}"/><text x="${labelX}" y="${labelY + 3}" text-anchor="middle" fill="${xml(options.palette.text)}" font-size="${options.labelSize}" font-weight="700">${xml(label)}</text>`
                        : ''
                }</g>`,
            ];
        }),
    );

    const nodeMarkup = nodes.map((node) => {
        const related = node.table.name === options.relatedTable;
        const border = related ? options.palette.accent : options.palette.border;
        const name = truncate(objectLabel(node.table.name), Math.max(12, Math.floor(options.nodeWidth / 8)));
        const schema = truncate(node.table.schema || 'main', Math.max(12, Math.floor(options.nodeWidth / 7)));
        const primaryKeyCount = node.table.columns.filter((column) => column.primaryKey).length;
        const rows = node.collapsed
            ? ''
            : node.rows
                  .map((row, index) => {
                      const y = node.y + headerHeight + index * options.columnHeight;
                      const markers = [row.primaryKey ? 'PK' : '', row.foreignKey ? 'FK' : '', row.unique ? 'UQ' : ''].filter(Boolean).join(' ');
                      const typeText = options.nodeDetail === 'full' ? truncate(row.type, 10) : '';
                      const nullText = options.nodeDetail === 'full' && row.nullable ? 'NULL' : '';
                      return `<g><rect x="${node.x}" y="${y}" width="${options.nodeWidth}" height="${options.columnHeight}" fill="${xml(options.palette.card)}"/><line x1="${node.x}" y1="${y + options.columnHeight}" x2="${node.x + options.nodeWidth}" y2="${y + options.columnHeight}" stroke="${xml(options.palette.borderSoft)}"/><text x="${node.x + 10}" y="${y + options.columnHeight / 2 + 4}" fill="${xml(options.palette.accentText)}" font-size="9" font-weight="700">${markers}</text><text x="${node.x + 54}" y="${y + options.columnHeight / 2 + 4}" fill="${xml(options.palette.text)}" font-size="12" font-weight="600">${xml(truncate(row.name, options.nodeDetail === 'full' ? 20 : 30))}</text>${
                          options.nodeDetail === 'full'
                              ? `<text x="${node.x + options.nodeWidth - 48}" y="${y + options.columnHeight / 2 + 4}" text-anchor="end" fill="${xml(options.palette.muted)}" font-size="10">${xml(typeText)}</text><text x="${node.x + options.nodeWidth - 8}" y="${y + options.columnHeight / 2 + 4}" text-anchor="end" fill="${xml(options.palette.muted)}" font-size="9" font-style="italic">${nullText}</text>`
                              : ''
                      }</g>`;
                  })
                  .join('');
        const hiddenRowIndex = node.rows.length;
        const hiddenRow =
            !node.collapsed && node.hiddenCount
                ? `<g><rect x="${node.x}" y="${node.y + headerHeight + hiddenRowIndex * options.columnHeight}" width="${options.nodeWidth}" height="${options.columnHeight}" fill="${xml(options.palette.card)}"/><text x="${node.x + 54}" y="${node.y + headerHeight + hiddenRowIndex * options.columnHeight + options.columnHeight / 2 + 4}" fill="${xml(options.palette.accentText)}" font-size="11" font-weight="700">+ ${node.hiddenCount} more columns</text></g>`
                : '';
        const footerY = node.y + node.height - footerHeight;
        return `<g class="table-node"><rect x="${node.x}" y="${node.y}" width="${options.nodeWidth}" height="${node.height}" rx="6" fill="${xml(options.palette.card)}" stroke="${xml(border)}" stroke-width="${related ? 2 : 1}"/><path d="M ${node.x + 6} ${node.y} H ${node.x + options.nodeWidth - 6} Q ${node.x + options.nodeWidth} ${node.y} ${node.x + options.nodeWidth} ${node.y + 6} V ${node.y + headerHeight} H ${node.x} V ${node.y + 6} Q ${node.x} ${node.y} ${node.x + 6} ${node.y}" fill="${xml(options.palette.subtle)}"/><text x="${node.x + 11}" y="${node.y + 19}" fill="${xml(options.palette.muted)}" font-size="9" font-weight="700" letter-spacing="0.6">${xml(schema.toUpperCase())}</text><text x="${node.x + 11}" y="${node.y + 39}" fill="${xml(options.palette.text)}" font-size="13" font-weight="700">${xml(name)}</text>${rows}${hiddenRow}<rect x="${node.x}" y="${footerY}" width="${options.nodeWidth}" height="${footerHeight}" fill="${xml(options.palette.subtle)}"/><line x1="${node.x}" y1="${footerY}" x2="${node.x + options.nodeWidth}" y2="${footerY}" stroke="${xml(options.palette.border)}"/><text x="${node.x + 10}" y="${footerY + 23}" fill="${xml(options.palette.muted)}" font-size="10" font-weight="700">${node.table.columns.length} columns</text><text x="${node.x + options.nodeWidth - 10}" y="${footerY + 23}" text-anchor="end" fill="${xml(options.palette.muted)}" font-size="10" font-weight="700">${primaryKeyCount ? `${primaryKeyCount} PK` : 'No PK'}</text></g>`;
    });

    const gridOverlay = grid.definition ? `<rect width="100%" height="100%" fill="${grid.fill}"/>` : '';
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-label="Schema diagram"><defs>${grid.definition}${marker}</defs><rect width="100%" height="100%" fill="${xml(options.palette.surface)}"/>${gridOverlay}<g font-family="Inter, ui-sans-serif, system-ui, sans-serif">${edges.join('')}${nodeMarkup.join('')}</g></svg>`;
    return { svg, width, height };
}

export async function svgToPNG(svg: string, width: number, height: number) {
    const maxCanvasDimension = 8192;
    const scale = Math.min(2, maxCanvasDimension / width, maxCanvasDimension / height);
    const canvas = window.document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(width * scale));
    canvas.height = Math.max(1, Math.round(height * scale));
    const context = canvas.getContext('2d');
    if (!context) throw new Error('PNG export is not supported by this browser.');
    const source = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml;charset=utf-8' }));
    try {
        const image = new Image();
        await new Promise<void>((resolve, reject) => {
            image.onload = () => resolve();
            image.onerror = () => reject(new Error('Could not render the schema diagram for PNG export.'));
            image.src = source;
        });
        context.drawImage(image, 0, 0, canvas.width, canvas.height);
        return await new Promise<Blob>((resolve, reject) => {
            canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('Could not encode the schema diagram as PNG.'))), 'image/png');
        });
    } finally {
        URL.revokeObjectURL(source);
    }
}
