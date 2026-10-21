<script lang="ts">
    import sqlite from './icons/sqlite.svg?raw';
    import postgres from './icons/postgres.svg?raw';
    import mysql from './icons/mysql.svg?raw';
    import mongodb from './icons/mongodb.svg?raw';
    import redis from './icons/redis.svg?raw';
    import fallback from './icons/database.svg?raw';

    let { engine, size = 24 }: { engine: string; size?: number } = $props();

    const icons: Record<string, string> = { sqlite, postgres, mysql, mongodb, redis };
</script>

<div class="db-logo" style="width: {size}px; height: {size}px;">
    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
    {@html icons[engine] ?? fallback}
</div>
