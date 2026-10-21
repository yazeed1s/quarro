import type { ExplainNode } from '../../state/types';

export interface SqlRange {
    from: number;
    to: number;
}

function escapeRegExp(value: string): string {
    return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// findRelationRange locates the first "FROM <name>"/"JOIN <name>" reference
// to relation inside sql, tolerant of the common identifier-quoting styles
// this app's three SQL engines use (double quotes, backticks, brackets).
// Matched only as a whole word, case-insensitively, since SQL keywords and
// unquoted identifiers both are. Not a general SQL parser: a relation name
// that also appears as a column or string literal earlier in the text
// could in principle mismatch, which is an accepted, honestly-scoped
// limitation rather than a reason to bring in a real parser for a jump-to
// convenience action.
export function findRelationRange(sql: string, relation: string): SqlRange | undefined {
    if (!relation) return undefined;
    const esc = escapeRegExp(relation);
    const re = new RegExp(`\\b(?:from|join)\\s+[\`"[]?(${esc})[\`"\\]]?\\b`, 'i');
    const m = re.exec(sql);
    if (!m) return undefined;
    const start = m.index + m[0].lastIndexOf(m[1]);
    return { from: start, to: start + m[1].length };
}

// findNodeRangesInDocument maps plan nodes matching filter onto their
// position in fullSql: it first locates statementSql (the exact substring
// Explain ran, resolved once at fork time) inside fullSql, then locates
// each matching node's own relation reference within that substring. Used
// both for "Show in editor" (a single selected node) and for hotspot
// gutter marks (every hotspot-badged node); one range-finding function,
// two callers, not two different lookups.
export function findNodeRangesInDocument(fullSql: string, statementSql: string, root: ExplainNode | null, filter: (node: ExplainNode) => boolean): SqlRange[] {
    if (!root) return [];
    const stmtStart = fullSql.indexOf(statementSql);
    if (stmtStart < 0) return [];
    const out: SqlRange[] = [];
    const visit = (node: ExplainNode) => {
        if (filter(node) && node.relation) {
            const rel = findRelationRange(statementSql, node.relation);
            if (rel) out.push({ from: stmtStart + rel.from, to: stmtStart + rel.to });
        }
        node.children.forEach(visit);
    };
    visit(root);
    return out;
}
