<script lang="ts">
    import type { Setting } from './fields';
    import { workspace, defaultPreferences } from '../../state/workspace.svelte';
    import Input from '../../components/Input.svelte';
    import Select, { type SelectOption } from '../../components/Select.svelte';
    let { field }: { field: Setting } = $props();
    function update(value: string | number | boolean) {
        Object.assign(workspace.preferences, { [field.key]: value });
    }
</script>

<div class="setting-row">
    <div>
        <label for={`setting-${field.key}`}>{field.label}</label>
        <p>{field.description}</p>
    </div>
    {#if field.type === 'checkbox'}<Input type="checkbox" ariaLabel={field.label} bind:checked={() => Boolean(workspace.preferences[field.key]), (v) => update(v)} />
    {:else if field.type === 'select'}<Select
            bind:value={() => String(workspace.preferences[field.key]), (v) => update(v)}
            options={field.options ?? []}
            placeholder={field.label}
        />
    {:else if field.type === 'number'}<input
            id={`setting-${field.key}`}
            class="number-input"
            type="number"
            min={field.min}
            max={field.max}
            step={field.step ?? 1}
            value={Number(workspace.preferences[field.key])}
            onchange={(e) => {
                const value = Number(e.currentTarget.value);
                update(Math.min(field.max ?? Infinity, Math.max(field.min ?? 0, Number.isFinite(value) && e.currentTarget.value ? value : Number(defaultPreferences[field.key]))));
                e.currentTarget.value = String(workspace.preferences[field.key]);
            }}
        />
    {:else}<input id={`setting-${field.key}`} type="text" maxlength="80" value={String(workspace.preferences[field.key])} oninput={(e) => update(e.currentTarget.value)} />{/if}
</div>
