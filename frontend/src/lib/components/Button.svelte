<script lang="ts">
    import type { Snippet } from 'svelte';
    import Icon, { type IconName } from './Icon.svelte';
    let {
        children,
        icon,
        label,
        onclick,
        variant = 'ghost',
        disabled = false,
        type = 'button',
        active = false,
    }: {
        children?: Snippet;
        icon?: IconName;
        label?: string;
        onclick?: (event: MouseEvent) => void;
        variant?: 'ghost' | 'primary' | 'outline' | 'danger';
        disabled?: boolean;
        type?: 'button' | 'submit';
        active?: boolean;
    } = $props();
</script>

<button class="button {variant}" class:icon-only={!children} class:active {type} {disabled} {onclick} title={label} aria-label={label}>
    {#if icon}<Icon name={icon} />{/if}{@render children?.()}
</button>
