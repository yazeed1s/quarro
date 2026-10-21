<script lang="ts">
    import type { HTMLInputAttributes } from 'svelte/elements';

    type InputType = 'text' | 'password' | 'number' | 'range' | 'checkbox';
    type InputMode = 'none' | 'text' | 'tel' | 'url' | 'email' | 'numeric' | 'decimal' | 'search';

    let {
        type = 'text',
        value = $bindable(''),
        checked = $bindable(false),
        label,
        suffix = '',
        min = 0,
        max = 100,
        step = 1,
        placeholder = '',
        disabled = false,
        required = false,
        autocomplete,
        inputmode,
        pattern,
        ariaLabel,
        onchange,
        oninput,
    }: {
        type?: InputType;
        value?: string | number;
        checked?: boolean;
        label?: string;
        suffix?: string;
        min?: string | number;
        max?: string | number;
        step?: string | number;
        placeholder?: string;
        disabled?: boolean;
        required?: boolean;
        autocomplete?: HTMLInputAttributes['autocomplete'];
        inputmode?: InputMode;
        pattern?: string;
        ariaLabel?: string;
        onchange?: (event: Event) => void;
        oninput?: (event: Event) => void;
    } = $props();

    let minValue = $derived(Number(min));
    let maxValue = $derived(Number(max));
    let stepValue = $derived(Number(step) || 1);
    let numericValue = $derived(Number(value));
    let rangePercent = $derived(maxValue === minValue ? 0 : Math.min(100, Math.max(0, ((numericValue - minValue) / (maxValue - minValue)) * 100)));

    function emitInput() {
        oninput?.(new Event('input'));
    }

    function emitChange() {
        onchange?.(new Event('change'));
    }

    function updateRange(next: number, commit = false) {
        const bounded = Math.min(maxValue, Math.max(minValue, next));
        const stepped = Math.round((bounded - minValue) / stepValue) * stepValue + minValue;
        const decimals = String(step).includes('.') ? String(step).split('.')[1].length : 0;
        value = Number(stepped.toFixed(decimals));
        emitInput();
        if (commit) emitChange();
    }

    function updateFromPointer(event: PointerEvent, commit = false) {
        if (disabled) return;
        const target = event.currentTarget as HTMLElement;
        target.setPointerCapture?.(event.pointerId);
        const rect = target.getBoundingClientRect();
        const ratio = rect.width ? (event.clientX - rect.left) / rect.width : 0;
        updateRange(minValue + ratio * (maxValue - minValue), commit);
    }

    function rangeKey(event: KeyboardEvent) {
        if (disabled) return;
        const largeStep = stepValue * 10;
        if (event.key === 'ArrowLeft' || event.key === 'ArrowDown') {
            event.preventDefault();
            updateRange(numericValue - stepValue, true);
        } else if (event.key === 'ArrowRight' || event.key === 'ArrowUp') {
            event.preventDefault();
            updateRange(numericValue + stepValue, true);
        } else if (event.key === 'PageDown') {
            event.preventDefault();
            updateRange(numericValue - largeStep, true);
        } else if (event.key === 'PageUp') {
            event.preventDefault();
            updateRange(numericValue + largeStep, true);
        } else if (event.key === 'Home') {
            event.preventDefault();
            updateRange(minValue, true);
        } else if (event.key === 'End') {
            event.preventDefault();
            updateRange(maxValue, true);
        }
    }

    function toggleChecked() {
        if (disabled) return;
        checked = !checked;
        emitChange();
    }
</script>

{#if type === 'checkbox'}
    <button type="button" class="input-control checkbox" class:checked class:disabled {disabled} aria-pressed={checked} aria-label={ariaLabel ?? label} onclick={toggleChecked}>
        <input type="checkbox" tabindex="-1" aria-hidden="true" bind:checked {disabled} />
        <span class="input-check" aria-hidden="true"></span>
        {#if label}<span>{label}</span>{/if}
    </button>
{:else if type === 'range'}
    <label class="input-control range" class:disabled>
        {#if label}<span class="control-label">{label}</span>{/if}
        <input class="input-native-hidden" type="range" tabindex="-1" aria-hidden="true" bind:value {min} {max} {step} {disabled} />
        <span
            class="input-range-shell"
            role="slider"
            tabindex={disabled ? -1 : 0}
            aria-label={ariaLabel ?? label}
            aria-valuemin={minValue}
            aria-valuemax={maxValue}
            aria-valuenow={numericValue}
            aria-disabled={disabled}
            onpointerdown={(event) => updateFromPointer(event)}
            onpointermove={(event) => {
                if (event.buttons === 1) updateFromPointer(event);
            }}
            onpointerup={(event) => updateFromPointer(event, true)}
            onkeydown={rangeKey}
        >
            <span class="input-range-track" aria-hidden="true">
                <span class="input-range-fill" style:width={`${rangePercent}%`}></span>
                <span class="input-range-thumb" style:left={`${rangePercent}%`}></span>
            </span>
        </span>
        {#if suffix}<output>{value}{suffix}</output>{/if}
    </label>
{:else}
    <label class="input-control field" class:disabled>
        {#if label}<span class="control-label">{label}</span>{/if}
        <input
            {type}
            bind:value
            {min}
            {max}
            {step}
            {placeholder}
            {disabled}
            {required}
            autocomplete={autocomplete ?? undefined}
            inputmode={inputmode ?? undefined}
            {pattern}
            aria-label={ariaLabel ?? label}
            {oninput}
            {onchange}
        />
        {#if suffix}<output>{value}{suffix}</output>{/if}
    </label>
{/if}
