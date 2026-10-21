<script lang="ts">
    let {
        value,
        min,
        max,
        orientation = 'vertical',
        label,
        reverse = false,
        onchange,
    }: {
        value: number;
        min: number;
        max: number;
        orientation?: 'vertical' | 'horizontal';
        label: string;
        reverse?: boolean;
        onchange: (value: number) => void;
    } = $props();
    function clamp(v: number) {
        return Math.min(max, Math.max(min, v));
    }
    function start(e: PointerEvent) {
        const el = e.currentTarget as HTMLElement;
        el.setPointerCapture(e.pointerId);
        const origin = orientation === 'vertical' ? e.clientX : e.clientY;
        const initial = value;
        const move = (event: PointerEvent) => onchange(clamp(initial + ((orientation === 'vertical' ? event.clientX : event.clientY) - origin) * (reverse ? -1 : 1)));
        const end = () => {
            el.removeEventListener('pointermove', move);
            el.removeEventListener('pointerup', end);
            el.removeEventListener('lostpointercapture', end);
        };
        el.addEventListener('pointermove', move);
        el.addEventListener('pointerup', end);
        el.addEventListener('lostpointercapture', end);
    }
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Focusable separators implement the keyboard window-splitter pattern.) -->
<div
    class="resize-handle {orientation}"
    role="separator"
    tabindex="0"
    aria-label={label}
    aria-orientation={orientation}
    aria-valuenow={Math.round(value)}
    aria-valuemin={min}
    aria-valuemax={max}
    onpointerdown={start}
    onkeydown={(e) => {
        if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) {
            e.preventDefault();
            onchange(clamp(value + (['ArrowRight', 'ArrowDown'].includes(e.key) ? 16 : -16) * (reverse ? -1 : 1)));
        }
    }}
></div>
