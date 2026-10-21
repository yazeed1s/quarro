<script lang="ts">
    import Icon from './Icon.svelte';

    export type SelectOption = {
        value: string;
        label: string;
        disabled?: boolean;
    };

    let {
        value = $bindable(''),
        options,
        label,
        disabled = false,
        placeholder = 'Select',
        compact = false,
        onchange,
    }: {
        value: string;
        options: SelectOption[];
        label?: string;
        disabled?: boolean;
        placeholder?: string;
        compact?: boolean;
        onchange?: (value: string) => void;
    } = $props();

    let open = $state(false);
    let root = $state<HTMLDivElement>();
    let trigger = $state<HTMLButtonElement>();
    let menu = $state<HTMLDivElement>();
    let menuStyle = $state('');
    let selected = $derived(options.find((option) => option.value === value));

    function choose(option: SelectOption) {
        if (disabled || option.disabled) return;
        value = option.value;
        onchange?.(option.value);
        open = false;
    }

    function updatePosition() {
        if (!trigger) return;
        const rect = trigger.getBoundingClientRect();
        const openUp = rect.bottom + 220 > window.innerHeight && rect.top > window.innerHeight - rect.bottom;
        menuStyle = openUp
            ? `left:${rect.left}px; bottom:${window.innerHeight - rect.top + 3}px; width:${rect.width}px;`
            : `left:${rect.left}px; top:${rect.bottom + 3}px; width:${rect.width}px;`;
    }

    function handleOutsidePointerDown(event: PointerEvent) {
        const target = event.target as Node;
        if (root?.contains(target) || menu?.contains(target)) return;
        open = false;
    }

    function portal(node: HTMLElement) {
        // A <dialog> (Modal.svelte) renders in the browser's top layer, above
        // all regular DOM regardless of z-index. Appending straight to <body>
        // put the menu below that layer whenever the select lives inside an
        // open dialog, hiding it behind the modal. Appending inside the
        // dialog itself keeps it in that same top layer.
        const dialog = root?.closest('dialog[open]');
        (dialog ?? document.body).appendChild(node);
        return {
            destroy() {
                node.remove();
            },
        };
    }

    $effect(() => {
        if (!open) return;
        updatePosition();
        document.addEventListener('pointerdown', handleOutsidePointerDown, true);
        window.addEventListener('scroll', updatePosition, true);
        window.addEventListener('resize', updatePosition);
        return () => {
            document.removeEventListener('pointerdown', handleOutsidePointerDown, true);
            window.removeEventListener('scroll', updatePosition, true);
            window.removeEventListener('resize', updatePosition);
        };
    });
</script>

<div bind:this={root} class="select-control" class:compact class:open class:disabled>
    {#if label}<span class="control-label">{label}</span>{/if}
    <button
        bind:this={trigger}
        type="button"
        class="select-trigger"
        {disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={label ?? placeholder}
        onclick={() => (open = !open)}
        onkeydown={(event) => {
            if (event.key === 'Escape') open = false;
        }}
    >
        <span>{selected?.label ?? placeholder}</span>
        <Icon name={open ? 'chevronUp' : 'chevronDown'} size={13} />
    </button>
    {#if open}<div
            bind:this={menu}
            use:portal
            class="select-menu"
            role="listbox"
            tabindex="-1"
            aria-label={label ?? placeholder}
            style={menuStyle}
            onkeydown={(event) => {
                if (event.key === 'Escape') open = false;
            }}
        >
            {#each options as option}<button
                    type="button"
                    role="option"
                    class:active={option.value === value}
                    disabled={option.disabled}
                    aria-selected={option.value === value}
                    onclick={() => choose(option)}
                >
                    <span>{option.label}</span>
                    {#if option.value === value}<Icon name="check" size={13} />{/if}
                </button>{/each}
        </div>{/if}
</div>
