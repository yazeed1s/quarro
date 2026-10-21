// Applied to a <details class="dropdown"> menu (Workspace, DocumentView,
// SessionBar, ConnectionManager toolbars). <details> only toggles on its own
// summary click; without this it stays open until the user clicks that
// summary again, even after clicking elsewhere on the page.
export function closeOnOutsideClick(node: HTMLDetailsElement) {
    function handlePointerDown(event: PointerEvent) {
        if (node.open && !node.contains(event.target as Node)) node.open = false;
    }
    document.addEventListener('pointerdown', handlePointerDown, true);
    return {
        destroy() {
            document.removeEventListener('pointerdown', handlePointerDown, true);
        },
    };
}
