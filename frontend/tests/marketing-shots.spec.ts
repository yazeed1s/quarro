import { expect, test, type Page } from '@playwright/test';

// Maintained screenshot tool for website/public/screenshots/ (see
// website/README.md's "Updating screenshots" section). Unlike its
// predecessor (_shots.spec.ts, a "scratch, delete after running" script),
// this is meant to be re-run whenever the UI changes enough to make the
// marketing site's screenshots stale.
//
// Prerequisites:
//   docker compose -f ../dev/docker-compose.yml up -d
// This starts the repo's own pre-seeded MongoDB (quarro_demo: customers,
// orders, products, reviews) and Redis (customer:*/order:*/product:*/
// tags:product:* keys) dev containers on their documented ports (27017,
// 6380) — no separate seeding step needed.
//
// Run:
//   npm run test:server   # in one terminal, or let Playwright's webServer start it
//   npm run test:marketing -- tests/marketing-shots.spec.ts
//
// Output lands in frontend/shots/*.png (gitignored). Copy the ones you want
// into website/public/screenshots/.

test.use({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2 });

const mongoPort = process.env.MONGO_PORT ?? '27017';
const redisPort = process.env.REDIS_PORT ?? '6380';

async function prepareTheme(page: Page) {
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'Monochrome', exact: true }).click();
    await page.getByLabel('Application zoom', { exact: true }).fill('110');
    await page.getByLabel('Application zoom', { exact: true }).blur();
    await page.keyboard.press('Escape');
}

async function prepareThemeLight(page: Page) {
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'Light', exact: true }).click();
    await page.getByLabel('Application zoom', { exact: true }).fill('110');
    await page.getByLabel('Application zoom', { exact: true }).blur();
    await page.keyboard.press('Escape');
}

async function newConnection(
    page: Page,
    opts: {
        name: string;
        engine: string;
        host?: string;
        port?: string;
        database?: string;
        username?: string;
        password?: string;
    },
) {
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill(opts.name);
    const engineLabels: Record<string, string> = {
        sqlite: 'SQLite',
        postgres: 'PostgreSQL',
        mysql: 'MySQL',
        mongodb: 'MongoDB',
        redis: 'Redis',
    };
    await dialog.getByRole('group', { name: 'Database engine' }).getByRole('button', { name: engineLabels[opts.engine] }).click();
    if (opts.host) await dialog.getByLabel('Host', { exact: true }).fill(opts.host);
    if (opts.port) await dialog.getByLabel('Port', { exact: true }).fill(opts.port);
    if (opts.database) {
        const dbLabel = opts.engine === 'redis' ? 'Database index' : 'Database';
        await dialog.getByLabel(dbLabel, { exact: true }).fill(opts.database);
    }
    if (opts.username) await dialog.getByLabel('Username', { exact: true }).fill(opts.username);
    if (opts.password) await dialog.getByLabel('Password', { exact: true }).fill(opts.password);
    if (opts.engine === 'postgres' || opts.engine === 'mysql') {
        await dialog.locator('.connection-transport select').selectOption('disable');
    }
    await dialog.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(dialog.locator('.inline-note').last()).toContainText('Connected successfully');
    await dialog.getByRole('button', { name: 'Save profile', exact: true }).click();
    await dialog.getByRole('button', { name: `Open ${opts.name}`, exact: true }).click();
    await page
        .getByRole('button', { name: 'Dismiss notification' })
        .click({ timeout: 1000 })
        .catch(() => {});
}

async function runAll(doc: ReturnType<Page['locator']>, page: Page) {
    const runAllBtn = doc.getByRole('button', { name: 'Run All', exact: true });
    const wasOpen = await runAllBtn.isVisible();
    if (!wasOpen) {
        await doc.locator('.run-group summary').click();
    }
    await doc.getByRole('button', { name: 'Run All', exact: true }).click();
    // Native <details>/<summary> — a click inside doesn't auto-collapse it.
    if (!wasOpen) {
        await doc.locator('.run-group summary').click();
    }
}

test('1. hero-editor — query editor with a joined result set', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await doc.locator('.cm-content').click();
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await page.keyboard.type('SELECT u.name, u.email, o.status, o.total\nFROM users u\nJOIN orders o ON o.user_id = u.id\nORDER BY o.created_at DESC\nLIMIT 20;');
    await runAll(doc, page);
    await expect(doc.locator('.data-grid')).toBeVisible();
    await page.waitForTimeout(300);
    await page.screenshot({ path: 'shots/hero-editor.png' });
});

test('2. schema-diagram — users/orders foreign key', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page
        .locator('.object-browser')
        .getByRole('button', { name: /^Schema diagram\b/ })
        .click();
    const diagram = page.locator('.session-host:not([hidden]) .diagram-canvas');
    await expect(diagram).toBeVisible();
    await page.waitForTimeout(800);
    // The SvelteFlow minimap floats bottom-right and the default fit-view
    // lands the last table node right under it — pan the canvas left/up so
    // the minimap sits over empty space instead of covering a table.
    const box = await diagram.boundingBox();
    if (box) {
        await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
        await page.mouse.down();
        await page.mouse.move(box.x + box.width / 2 - 150, box.y + box.height / 2 - 30, {
            steps: 10,
        });
        await page.mouse.up();
        await page.mouse.move(box.x + 40, box.y + 40);
    }
    await page.waitForTimeout(300);
    await page.screenshot({ path: 'shots/schema-diagram.png' });

    // "Related" highlight: light up the foreign-key connection between two
    // tables (dims everything else) — a much clearer "how these connect"
    // shot than the plain overview above.
    const ordersNode = page.locator('.schema-node').filter({ hasText: 'orders' });
    await ordersNode.getByRole('button', { name: 'Related', exact: true }).click();
    await page.getByRole('button', { name: /fit view/i }).click();
    await page.waitForTimeout(400);

    // Drag "users" further from "orders" so the connecting edge/label has
    // visible breathing room instead of the two cards touching.
    const usersNode = page.locator('.schema-node').filter({ hasText: 'users' });
    const usersHeaderBefore = await usersNode.locator('header').boundingBox();
    if (usersHeaderBefore) {
        const sx = usersHeaderBefore.x + usersHeaderBefore.width / 2;
        const sy = usersHeaderBefore.y + usersHeaderBefore.height / 2;
        await page.mouse.move(sx, sy);
        await page.mouse.down();
        await page.mouse.move(sx + 110, sy + 20, { steps: 12 });
        await page.mouse.up();
    }
    await page.getByRole('button', { name: /fit view/i }).click();
    await page.waitForTimeout(500);
    await page.screenshot({ path: 'shots/schema-diagram-related.png' });

    const oBox = await ordersNode.boundingBox();
    const uBox = await usersNode.boundingBox();
    if (oBox && uBox) {
        const x = Math.min(oBox.x, uBox.x) - 32;
        const y = Math.min(oBox.y, uBox.y) - 32;
        const right = Math.max(oBox.x + oBox.width, uBox.x + uBox.width) + 32;
        const bottom = Math.max(oBox.y + oBox.height, uBox.y + uBox.height) + 32;
        await page.screenshot({
            path: 'shots/schema-diagram-detail.png',
            clip: { x, y, width: right - x, height: bottom - y },
        });
    }
});

test('2b. schema-diagram-many — a hub table lit up against a richer schema', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await newConnection(page, {
        name: 'Storefront Postgres',
        engine: 'postgres',
        host: '127.0.0.1',
        port: '5433',
        database: 'quarro_demo',
        username: 'postgres',
        password: 'quarro',
    });
    await page
        .locator('.object-browser')
        .getByRole('button', { name: /^Schema diagram\b/ })
        .click();
    const diagram = page.locator('.session-host:not([hidden]) .diagram-canvas');
    await expect(diagram).toBeVisible();
    await page.waitForTimeout(700);
    // order_items sits at the hub of this schema (FKs to both orders and
    // products), so lighting it up connects three tables, not just two.
    const hubNode = page.locator('.schema-node').filter({ hasText: 'order_items' });
    await hubNode.getByRole('button', { name: 'Related', exact: true }).click();
    await page.getByRole('button', { name: /fit view/i }).click();
    await page.waitForTimeout(400);

    // Default layout puts all three in a row; drag order_items down into a
    // triangle against orders/products so both connecting edges are visibly
    // distinct instead of overlapping a single horizontal line.
    const hubHeader = await hubNode.locator('header').boundingBox();
    if (hubHeader) {
        const sx = hubHeader.x + hubHeader.width / 2;
        const sy = hubHeader.y + hubHeader.height / 2;
        await page.mouse.move(sx, sy);
        await page.mouse.down();
        await page.mouse.move(sx + 260, sy + 260, { steps: 14 });
        await page.mouse.up();
    }
    await page.getByRole('button', { name: /fit view/i }).click();
    await page.waitForTimeout(600);
    await page.screenshot({ path: 'shots/schema-diagram-many.png' });
});

test('3. visual-query-builder — a joined query with generated SQL', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page.locator('.document-bar details.dropdown summary').click();
    await page.getByRole('button', { name: 'New visual query', exact: true }).click();
    await page.locator('.document-bar details.dropdown summary').click(); // collapse; native <details> doesn't auto-close
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(doc.locator('.visual-query-builder')).toBeVisible();

    const addTable = doc.getByLabel('Add a table to the query', { exact: true });
    await addTable.fill('orders');
    await doc.getByRole('option', { name: 'orders', exact: true }).click();
    await addTable.fill('users');
    await doc.getByRole('option', { name: 'users', exact: true }).click();

    const ordersNode = doc.locator('.query-node').filter({ hasText: 'orders' });
    const usersNode = doc.locator('.query-node').filter({ hasText: 'users' });
    await ordersNode.getByRole('button', { name: 'Join to…', exact: true }).click();
    const joinPanel = doc.locator('.join-inspector');
    // Left table defaults to the node "Join to…" was opened from (orders);
    // right table auto-defaults to the only other table on the canvas
    // (users) — no need to touch either select.
    await joinPanel.locator('.inspector-field').filter({ hasText: 'Left column' }).locator('select').selectOption('user_id');
    await joinPanel.locator('.inspector-field').filter({ hasText: 'Right column' }).locator('select').selectOption('id');
    await joinPanel.getByRole('button', { name: 'Add join', exact: true }).click();

    for (const name of ['status', 'total', 'created_at']) {
        await ordersNode.locator('.query-column').filter({ hasText: name }).locator('input').check();
    }
    for (const name of ['name', 'email']) {
        await usersNode.locator('.query-column').filter({ hasText: name }).locator('input').check();
    }
    await expect(doc.locator('pre')).not.toContainText('Add tables');
    await doc.locator('.query-canvas').click({ position: { x: 20, y: 20 } });
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
    await page.screenshot({ path: 'shots/visual-query-builder.png' });
});

test('4. chart-pushdown-sql — cumulative revenue, a line that actually grows', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await newConnection(page, {
        name: 'Storefront Postgres',
        engine: 'postgres',
        host: '127.0.0.1',
        port: '5433',
        database: 'quarro_demo',
        username: 'postgres',
        password: 'quarro',
    });
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await doc.locator('.cm-content').click();
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await page.keyboard.type(
        'SELECT o.order_date,\n  SUM(SUM(oi.quantity * oi.unit_price)) OVER (ORDER BY o.order_date) AS cumulative_revenue\nFROM orders o\nJOIN order_items oi ON oi.order_id = o.id\nGROUP BY o.order_date\nORDER BY o.order_date;',
    );
    await runAll(doc, page);
    await expect(doc.locator('.results-footer')).toContainText('loaded rows');
    await doc.getByRole('button', { name: 'Visualize query results', exact: true }).click();
    const chart = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(chart.locator('.chart-canvas canvas')).toBeVisible();
    await chart.getByLabel('Chart type', { exact: true }).selectOption('line');
    await chart.getByLabel('X field', { exact: true }).selectOption('order_date');
    await chart.getByLabel('Y field', { exact: true }).selectOption('cumulative_revenue');
    await chart.getByLabel('Aggregate', { exact: true }).selectOption('sum');
    await page.waitForTimeout(1500);
    await page.screenshot({ path: 'shots/chart-pushdown-sql.png' });
});

test('5. chart-pushdown-mongo — a MongoDB collection, aggregated by the database', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await newConnection(page, {
        name: 'Storefront Mongo',
        engine: 'mongodb',
        host: '127.0.0.1',
        port: mongoPort,
        database: 'quarro_demo',
    });
    await page.locator('.object-browser').getByRole('button', { name: 'Visualize orders', exact: true }).click();
    const chart = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(chart.locator('.chart-canvas canvas')).toBeVisible();
    await chart.getByLabel('X field', { exact: true }).selectOption('status');
    await chart.getByLabel('Aggregate', { exact: true }).selectOption('count');
    await expect(chart.locator('.chart-mode')).toHaveClass(/pushdown/, { timeout: 10_000 });
    await page.waitForTimeout(500);
    await page.screenshot({ path: 'shots/chart-pushdown-mongo.png' });
});

test('6. row-inspector-drawer — staged edit, original vs. new value', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page
        .locator('.object-browser')
        .getByRole('button', { name: /^users\b/ })
        .click();
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(doc.locator('.data-grid')).toBeVisible();
    await doc.locator('.data-grid tbody tr').nth(0).locator('td button.cell').nth(3).dblclick();
    const drawer = doc.locator('.row-inspector.row-details');
    await expect(drawer).toBeVisible();
    const roleField = drawer.getByLabel('role', { exact: true });
    await roleField.fill('editor');
    await roleField.blur();
    await page.waitForTimeout(300);
    await doc.screenshot({ path: 'shots/row-inspector-drawer.png' });
});

test('7. command-palette — everything reachable from the keyboard', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page.keyboard.press('Control+k');
    await expect(page.locator('.palette')).toBeVisible();
    await page.waitForTimeout(300);
    await page.screenshot({ path: 'shots/command-palette.png' });
});

test('8. redis-keyspace — key, value, TTL', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await newConnection(page, {
        name: 'Storefront Redis',
        engine: 'redis',
        host: '127.0.0.1',
        port: redisPort,
    });
    await page
        .locator('.object-browser')
        .getByRole('button', { name: /^customer\b/ })
        .click();
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(doc.locator('table.metadata-table')).toBeVisible();
    await page.waitForTimeout(300);
    await doc.screenshot({ path: 'shots/redis-keyspace.png' });
});

test('9. connection-manager — five engines, one dialog', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('Storefront');
    await dialog.getByRole('group', { name: 'Database engine' }).getByRole('button', { name: 'MongoDB' }).click();
    await page.waitForTimeout(200);
    await dialog.screenshot({ path: 'shots/connection-manager.png' });
});

test('10. import-export-panel — an explicit export scope', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await doc.locator('.cm-content').click();
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await page.keyboard.type('SELECT * FROM products;');
    await runAll(doc, page);
    await expect(doc.locator('.results-footer')).toContainText('loaded rows');
    await doc.getByRole('button', { name: 'Export results', exact: true }).click();
    await expect(doc.locator('.transfer-panel')).toBeVisible();
    await page.waitForTimeout(300);
    await doc.screenshot({ path: 'shots/import-export-panel.png' });
});

test('11. hero-editor-light — the same query, in Light theme', async ({ page }) => {
    await page.goto('/');
    await prepareThemeLight(page);
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await doc.locator('.cm-content').click();
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await page.keyboard.type('SELECT u.name, u.email, o.status, o.total\nFROM users u\nJOIN orders o ON o.user_id = u.id\nORDER BY o.created_at DESC\nLIMIT 20;');
    await runAll(doc, page);
    await expect(doc.locator('.data-grid')).toBeVisible();
    await page.waitForTimeout(300);
    await page.screenshot({ path: 'shots/hero-editor-light.png' });
});

test('12. settings-open — the Settings window over the workspace', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
    await page.waitForTimeout(300);
    await page.screenshot({ path: 'shots/settings-open.png' });
});
