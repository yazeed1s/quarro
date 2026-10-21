<script lang="ts">
    import { Handle, Position } from '@xyflow/svelte';
    import type { ExplainNode } from '../../state/types';
    import { severityTier } from './layout';
    import Icon from '../../components/Icon.svelte';

    export interface PlanNodeData {
        node: ExplainNode;
        maxExclusive: number;
        analyzed: boolean;
    }

    let { data, selected = false }: { data: PlanNodeData; selected?: boolean } = $props();
    let tier = $derived(severityTier(data.node, data.maxExclusive));
    let metric = $derived(
        data.analyzed
            ? data.node.exclusiveTimeMs !== undefined
                ? `${data.node.exclusiveTimeMs.toFixed(2)} ms`
                : ''
            : data.node.exclusiveCost !== undefined
              ? `cost ${data.node.exclusiveCost.toFixed(1)}`
              : '',
    );
</script>

<div class="explain-node tier-{tier}" class:selected>
    <Handle type="target" position={Position.Top} />
    <div class="explain-node-head">
        <strong>{data.node.operation}</strong>
        {#if metric}<span class="explain-node-metric">{metric}</span>{/if}
    </div>
    {#if data.node.relation || data.node.badges.length}<div class="explain-node-body">
            {#if data.node.relation}<div class="explain-node-relation">{data.node.relation}</div>{/if}
            {#if data.node.badges.length}<div class="explain-node-badges">
                    {#each data.node.badges as badge}<span class="explain-badge sev-{badge.severity}" title={badge.message}>
                            <Icon name={badge.severity === 'warning' ? 'alert' : 'info'} size={10} />
                            {badge.kind}
                        </span>{/each}
                </div>{/if}
        </div>{/if}
    <Handle type="source" position={Position.Bottom} />
</div>
