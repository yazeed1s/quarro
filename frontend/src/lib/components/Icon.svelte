<script module lang="ts">
    import Database from '@lucide/svelte/icons/database';
    import Table2 from '@lucide/svelte/icons/table-2';
    import Code from '@lucide/svelte/icons/code';
    import Plus from '@lucide/svelte/icons/plus';
    import X from '@lucide/svelte/icons/x';
    import ChevronRight from '@lucide/svelte/icons/chevron-right';
    import ChevronUp from '@lucide/svelte/icons/chevron-up';
    import ChevronDown from '@lucide/svelte/icons/chevron-down';
    import Search from '@lucide/svelte/icons/search';
    import Settings from '@lucide/svelte/icons/settings';
    import Play from '@lucide/svelte/icons/play';
    import Square from '@lucide/svelte/icons/square';
    import Download from '@lucide/svelte/icons/download';
    import PanelLeft from '@lucide/svelte/icons/panel-left';
    import RefreshCw from '@lucide/svelte/icons/refresh-cw';
    import Clock from '@lucide/svelte/icons/clock';
    import Bookmark from '@lucide/svelte/icons/bookmark';
    import Check from '@lucide/svelte/icons/check';
    import Sun from '@lucide/svelte/icons/sun';
    import Moon from '@lucide/svelte/icons/moon';
    import Monitor from '@lucide/svelte/icons/monitor';
    import Folder from '@lucide/svelte/icons/folder';
    import Key from '@lucide/svelte/icons/key';
    import Link from '@lucide/svelte/icons/link';
    import Ellipsis from '@lucide/svelte/icons/ellipsis';
    import ArrowRight from '@lucide/svelte/icons/arrow-right';
    import Copy from '@lucide/svelte/icons/copy';
    import Funnel from '@lucide/svelte/icons/funnel';
    import Unplug from '@lucide/svelte/icons/unplug';
    import Columns3 from '@lucide/svelte/icons/columns-3';
    import Network from '@lucide/svelte/icons/network';
    import Wand2 from '@lucide/svelte/icons/wand-2';
    import Star from '@lucide/svelte/icons/star';
    import Terminal from '@lucide/svelte/icons/terminal';
    import Trash2 from '@lucide/svelte/icons/trash-2';
    import ChartColumn from '@lucide/svelte/icons/chart-column';
    import Activity from '@lucide/svelte/icons/activity';
    import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
    import Info from '@lucide/svelte/icons/info';
    import Asterisk from '@lucide/svelte/icons/asterisk';
    import Minus from '@lucide/svelte/icons/minus';
    import SquareStack from '@lucide/svelte/icons/square-stack';

    export type IconName =
        | 'database'
        | 'table'
        | 'code'
        | 'plus'
        | 'close'
        | 'chevron'
        | 'chevronUp'
        | 'chevronDown'
        | 'search'
        | 'settings'
        | 'play'
        | 'stop'
        | 'download'
        | 'panel'
        | 'refresh'
        | 'clock'
        | 'bookmark'
        | 'check'
        | 'sun'
        | 'moon'
        | 'monitor'
        | 'folder'
        | 'key'
        | 'link'
        | 'more'
        | 'arrow'
        | 'copy'
        | 'filter'
        | 'disconnect'
        | 'columns'
        | 'diagram'
        | 'wand'
        | 'star'
        | 'command'
        | 'trash'
        | 'chart'
        | 'explain'
        | 'alert'
        | 'info'
        | 'unique'
        | 'minimise'
        | 'maximise'
        | 'restore';

    // Maps this app's stable icon-name vocabulary to lucide-svelte components,
    // so call sites (`<Icon name="database" />`) never need to know which
    // underlying icon set or exact icon name is in use.
    const icons = {
        database: Database,
        table: Table2,
        code: Code,
        plus: Plus,
        close: X,
        chevron: ChevronRight,
        chevronUp: ChevronUp,
        chevronDown: ChevronDown,
        search: Search,
        settings: Settings,
        play: Play,
        stop: Square,
        download: Download,
        panel: PanelLeft,
        refresh: RefreshCw,
        clock: Clock,
        bookmark: Bookmark,
        check: Check,
        sun: Sun,
        moon: Moon,
        monitor: Monitor,
        folder: Folder,
        key: Key,
        link: Link,
        more: Ellipsis,
        arrow: ArrowRight,
        copy: Copy,
        filter: Funnel,
        disconnect: Unplug,
        columns: Columns3,
        diagram: Network,
        wand: Wand2,
        star: Star,
        command: Terminal,
        trash: Trash2,
        chart: ChartColumn,
        explain: Activity,
        alert: TriangleAlert,
        info: Info,
        unique: Asterisk,
        minimise: Minus,
        maximise: Square,
        restore: SquareStack,
    } as const;
</script>

<script lang="ts">
    let { name, size = 16 }: { name: IconName; size?: number } = $props();
    let IconComponent = $derived(icons[name]);
</script>

<IconComponent width={size} height={size} strokeWidth={1.65} aria-hidden="true" />
