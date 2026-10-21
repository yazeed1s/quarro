import { expect, test } from '@playwright/test';
import { readFileSync } from 'node:fs';

const activeDocument = (page: import('@playwright/test').Page) => page.locator('.session-host:not([hidden]) .document-host:not([hidden])');

const objectButton = (page: import('@playwright/test').Page, name: string) =>
    page.locator('.session-host:not([hidden]) .object-browser button.tree-object').filter({ has: page.getByText(name, { exact: true }) });

async function chooseOption(root: import('@playwright/test').Locator | import('@playwright/test').Page, label: string, option: string) {
    await root.getByRole('button', { name: label, exact: true }).click();
    // Select menus are portaled to the page (or the open dialog's top layer),
    // so a component-scoped locator cannot see the listbox after opening it.
    const page = 'page' in root ? root.page() : root;
    await page.getByRole('listbox', { name: label, exact: true }).getByRole('option', { name: option, exact: true }).click();
}

async function runQuery(doc: import('@playwright/test').Locator) {
    await doc.getByRole('button', { name: 'Run query', exact: true }).click();
}

test('creating a real connection, running a query, and session isolation', async ({ page }) => {
    await page.goto('/');
    // First run auto-opens one session against the seeded demo database.
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(1);

    // Create a second, distinctly-named real SQLite connection so the two
    // sessions can be told apart by tab name.
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'New connection', exact: true }).click();
    const sessionName = `Second session ${Date.now()}`;
    await page.getByLabel('Connection name', { exact: true }).fill(sessionName);
    await page
        .getByRole('group', { name: 'Database engine' })
        .getByRole('button', { name: /SQLite/ })
        .click();
    await page.getByLabel('Database file path', { exact: true }).fill(`/tmp/quarro-e2e-second-${Date.now()}.db`);
    await page.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(page.getByRole('dialog').locator('.inline-note').last()).toContainText('Connected successfully');
    await page.getByRole('button', { name: 'Save profile', exact: true }).click();
    await page.getByRole('button', { name: `Open ${sessionName}`, exact: true }).click();
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(2);

    const doc = activeDocument(page);
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+End');
    await page.keyboard.type('\n-- keep this draft');
    await expect(editor).toContainText('keep this draft');

    await page.getByRole('button', { name: 'Sample database', exact: true }).click();
    await expect(activeDocument(page).locator('.cm-content')).not.toContainText('keep this draft');
    await runQuery(activeDocument(page));
    await expect(activeDocument(page).locator('.results-footer')).toContainText('of 125 loaded rows');

    await page.getByRole('button', { name: sessionName, exact: true }).click();
    await expect(editor).toContainText('keep this draft');
    await editor.click();
    await page.keyboard.press('Control+z');
    await expect(editor).not.toContainText('keep this draft');
});

test('visualizing a query result opens a restorable chart document and exports PNG', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    await runQuery(doc);
    await expect(doc.locator('.results-footer')).toContainText('of 125 loaded rows');

    await doc.getByRole('button', { name: 'Visualize query results', exact: true }).click();
    const chart = activeDocument(page);
    await expect(chart.locator('.chart-document')).toBeVisible();
    await expect(chart.locator('.chart-config-panel')).toContainText('Query 1');
    await expect(chart.locator('.chart-config-panel')).toContainText('125');
    await expect(chart.locator('.chart-config-panel')).toContainText('Columns');
    await expect(chart.locator('.chart-canvas canvas')).toBeVisible();
    await chooseOption(chart, 'X field', 'status');
    await chooseOption(chart, 'Y field', 'id');
    await chooseOption(chart, 'Aggregate', 'Count rows');
    for (const type of ['Line', 'Pie', 'Scatter', 'Bar']) {
        await chooseOption(chart, 'Chart type', type);
        await expect(chart.locator('.chart-canvas canvas')).toBeVisible();
    }
    const [pngDownload] = await Promise.all([page.waitForEvent('download'), chart.getByRole('button', { name: 'Export chart as PNG', exact: true }).click()]);
    expect(pngDownload.suggestedFilename()).toBe('chart-1.png');
    const [svgDownload] = await Promise.all([page.waitForEvent('download'), chart.getByRole('button', { name: 'Export chart as SVG', exact: true }).click()]);
    expect(svgDownload.suggestedFilename()).toBe('chart-1.svg');

    // A restart never keeps a snapshot series' fetched rows (localStorage has
    // no room for that at scale): the series config survives, but it comes
    // back needing an explicit Refresh.
    await page.reload();
    const restored = activeDocument(page);
    await expect(restored.locator('.connection-warning')).toContainText('disconnected');
    await expect(restored.locator('.chart-document')).toBeVisible();
    await expect(restored.locator('.chart-config-panel')).toContainText('Query 1');
    await expect(restored.locator('.chart-mode')).toContainText('Needs refresh');
    await expect(restored.locator('.chart-config-panel')).toContainText('0');

    await restored.getByRole('button', { name: 'Reconnect', exact: true }).click();
    await expect(restored.locator('.connection-warning')).toHaveCount(0);
    await restored.getByRole('button', { name: 'Refresh series Query 1', exact: true }).click();
    await expect(restored.locator('.chart-mode')).toContainText('Snapshot rows');
    await expect(restored.locator('.chart-config-panel')).toContainText('125');
});

test('visualizing a large table from the object browser uses server pushdown', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await page.keyboard.type(`CREATE TABLE big_orders (id INTEGER PRIMARY KEY, status TEXT);
WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 15050)
INSERT INTO big_orders SELECT n, CASE WHEN n % 2 = 0 THEN 'paid' ELSE 'open' END FROM seq;`);
    await expect(editor).toContainText('big_orders');
    await doc.locator('.run-group summary').click();
    await doc.getByRole('button', { name: 'Run All', exact: true }).click();
    await expect(doc.getByRole('button', { name: 'Run query', exact: true })).toBeEnabled();

    const browser = page.locator('.session-host:not([hidden]) .object-browser');
    await browser.getByRole('button', { name: 'Refresh schema', exact: true }).click();
    await expect(browser.getByRole('button', { name: 'Visualize big_orders', exact: true })).toBeVisible();
    await browser.getByRole('button', { name: 'Visualize big_orders', exact: true }).click();

    const chart = activeDocument(page);
    await expect(chart.locator('.chart-document')).toBeVisible();
    await expect(chart.locator('.chart-mode')).toContainText('Server pushdown');
    await expect(chart.locator('.chart-mode')).toContainText('the table grid was not loaded');
    await expect(chart.locator('.chart-config-panel')).toContainText('Groups');
    await expect(chart.locator('.chart-config-panel')).toContainText('2');
    await expect(chart.locator('.chart-canvas canvas')).toBeVisible();
});

async function runSQL(doc: import('@playwright/test').Locator, page: import('@playwright/test').Page, sql: string) {
    await doc.locator('.cm-content').click();
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await page.keyboard.type(sql);
    // "Run query" resolves the statement at the cursor, whose position can
    // race the freshly-typed text; "Run All" (used the same way by the
    // big_orders pushdown test above) always runs the whole editor and isn't
    // sensitive to that race. The dropdown is a native <details> that stays
    // open across calls once opened, so only open it when it's closed.
    const runAll = doc.getByRole('button', { name: 'Run All', exact: true });
    if (!(await runAll.isVisible())) {
        await doc.locator('.run-group summary').click();
    }
    await runAll.click();
    await expect(doc.locator('.results-footer')).toContainText('loaded rows');
}

test('opening Visualize suggests a chart type from the shape of the fields, and histogram binning renders', async ({ page }) => {
    await page.goto('/');
    const query = activeDocument(page);

    // Two numeric-looking columns (products.price is stored as TEXT but every
    // value parses as a number) and no categorical/temporal column at all:
    // the rule table suggests scatter with zero configuration.
    await runSQL(query, page, 'SELECT price, stock FROM products;');
    await query.getByRole('button', { name: 'Visualize query results', exact: true }).click();
    const scatterChart = activeDocument(page);
    await expect(scatterChart.locator('.chart-document')).toBeVisible();
    await expect(scatterChart.getByRole('button', { name: 'Chart type', exact: true })).toContainText('Scatter');
    await expect(scatterChart.locator('.chart-canvas canvas')).toBeVisible();

    // Switching to histogram replaces Y field/aggregate with a single value
    // field and a bin-count control, and still renders.
    await chooseOption(scatterChart, 'Chart type', 'Histogram');
    await expect(scatterChart.locator('.chart-config-panel')).toContainText('Value field');
    await expect(scatterChart.getByRole('button', { name: 'Value field', exact: true })).toBeVisible();
    await expect(scatterChart.getByRole('button', { name: 'Y field', exact: true })).toHaveCount(0);
    await expect(scatterChart.getByRole('button', { name: 'Aggregate', exact: true })).toHaveCount(0);
    await expect(scatterChart.getByLabel('Bin count', { exact: true })).toHaveValue('10');
    await scatterChart.getByLabel('Bin count', { exact: true }).fill('4');
    await scatterChart.getByLabel('Bin count', { exact: true }).press('Tab');
    await expect(scatterChart.locator('.chart-canvas canvas')).toBeVisible();

    // Back to the query tab: one numeric column alone suggests histogram
    // directly, with zero configuration.
    await page.locator('.document-tabs').getByRole('button', { name: 'Query 1 Local draft', exact: true }).click();
    await runSQL(query, page, 'SELECT price FROM products;');
    await query.getByRole('button', { name: 'Visualize query results', exact: true }).click();
    const histChart = activeDocument(page);
    await expect(histChart.getByRole('button', { name: 'Chart type', exact: true })).toContainText('Histogram');
    await expect(histChart.locator('.chart-canvas canvas')).toBeVisible();

    // Back again: a categorical column plus a numeric column (no temporal
    // column present) suggests bar.
    await page.locator('.document-tabs').getByRole('button', { name: 'Query 1 Local draft', exact: true }).click();
    await runSQL(query, page, 'SELECT status, total FROM orders;');
    await query.getByRole('button', { name: 'Visualize query results', exact: true }).click();
    const barChart = activeDocument(page);
    await expect(barChart.getByRole('button', { name: 'Chart type', exact: true })).toContainText('Bar');
    await expect(barChart.locator('.chart-canvas canvas')).toBeVisible();
});

test('themes and validated appearance settings persist after reload', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'Monochrome', exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'monochrome');
    await page.getByRole('button', { name: 'Monochrome Light', exact: true }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'monochrome-light');
    await page.getByRole('button', { name: 'Monochrome', exact: true }).click();
    await page.getByLabel('Interface size', { exact: true }).fill('17');
    await page.getByLabel('Interface size', { exact: true }).press('Tab');
    expect(await page.getByRole('button', { name: 'Editor', exact: true }).evaluate((el) => parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThan(14);
    await page.getByRole('button', { name: 'Editor', exact: true }).click();
    await page.getByRole('button', { name: 'Word wrap', exact: true }).click();
    await page.getByRole('button', { name: 'Line numbers', exact: true }).click();
    await page.getByRole('button', { name: 'Close dialog' }).click();
    await expect(activeDocument(page).locator('.cm-lineNumbers')).toHaveCount(0);
    await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('quarro.test.workspace.v3') || '{}').preferences?.uiSize)).toBe(17);
    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'monochrome');
    await expect(activeDocument(page).locator('.cm-lineNumbers')).toHaveCount(0);
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await expect(page.getByLabel('Interface size', { exact: true })).toHaveValue('17');
    await page.getByLabel('Application zoom', { exact: true }).fill('150');
    await page.getByLabel('Application zoom', { exact: true }).press('Tab');
    const bounds = await page.getByRole('dialog').boundingBox();
    expect(bounds!.y).toBeGreaterThanOrEqual(0);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(900);
    expect((await page.locator('.app-shell').boundingBox())!.height).toBe(900);
    await page.keyboard.press('Control+0');
    await expect(page.getByLabel('Application zoom', { exact: true })).toHaveValue('100');
});

test('grid virtualization, filtering, inspector, and real table structure/relationships', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    await runQuery(doc);
    await expect(doc.locator('.results-footer')).toContainText('of 125 loaded rows');
    expect(await doc.locator('tbody tr:not(.spacer)').count()).toBeLessThan(40);
    await doc.locator('.grid-scroll').evaluate((el) => {
        el.scrollTop = 3500;
    });
    await doc.getByLabel('Search loaded rows').fill('Sofia');
    await expect(doc.locator('tbody')).toContainText('Sofia García');
    await doc.locator('.cell').first().dblclick();
    await expect(doc.locator('.row-inspector')).toBeVisible();
    await doc.getByRole('button', { name: 'JSON', exact: true }).click();
    await expect(doc.locator('.row-inspector pre')).toContainText('Sofia García');

    await objectButton(page, 'users').click();
    const table = activeDocument(page);
    await expect(table.locator('.data-grid')).toBeVisible();
    await table.getByRole('button', { name: 'Structure', exact: true }).click();
    await expect(table.locator('.metadata-table')).toContainText('email');

    await objectButton(page, 'orders').click();
    const orders = activeDocument(page);
    await orders.getByRole('button', { name: 'Relationships', exact: true }).click();
    await orders.getByRole('button', { name: 'orders.user_id users.id' }).click();
    await expect(activeDocument(page).locator('.target strong')).toHaveText('users');
});

test("Structure tab and View Definition show SQLite's own verbatim CREATE TABLE text, with no Constraints section SQLite cannot back", async ({ page }) => {
    await page.goto('/');
    await objectButton(page, 'users').click();
    const table = activeDocument(page);
    await expect(table.locator('.data-grid')).toBeVisible();
    await table.getByRole('button', { name: 'Structure', exact: true }).click();
    await expect(table.locator('.metadata-table')).toContainText('email');
    // SQLite has no named CHECK/UNIQUE constraint catalog to query separately
    // from its own CREATE TABLE text, so it does not implement
    // ConstraintDiscoverer; the Structure tab must show no Constraints
    // section at all for it, not an empty one.
    await expect(table.getByText('Constraints', { exact: false })).toHaveCount(0);

    await table.getByRole('button', { name: 'View definition' }).click();
    const dialog = page.getByRole('dialog', { name: 'users' });
    await expect(dialog).toBeVisible();
    await expect(dialog.locator('.modal-header')).toContainText('Definition');
    await expect(dialog.locator('pre')).toContainText('CREATE TABLE users');
    await expect(dialog.locator('pre')).toContainText('email TEXT NOT NULL');
    await expect(dialog).not.toContainText('Reconstructed');
});

test('relationship navigation opens related rows filtered to the selected row, not an unfiltered table', async ({ page }) => {
    await page.goto('/');
    await objectButton(page, 'orders').click();
    const orders = activeDocument(page);
    await expect(orders.locator('.data-grid')).toBeVisible();
    const firstRow = orders.locator('tbody tr:not(.spacer)').first();
    const userId = (await firstRow.locator('td').nth(2).innerText()).trim();

    await firstRow.locator('td').nth(2).locator('.cell').click();
    await orders.getByRole('button', { name: 'Relationships', exact: true }).click();
    await orders.getByRole('button', { name: 'orders.user_id users.id' }).click();

    const users = activeDocument(page);
    await expect(users.locator('.target strong')).toHaveText('users');
    await expect(users.getByLabel('Filter table')).toHaveValue(userId);
    await expect(users.locator('.filter-field-exact')).toHaveText('= id');
    await expect(users.locator('.results-footer')).toContainText('of 1 loaded rows');
    const rows = users.locator('tbody tr:not(.spacer)');
    await expect(rows).toHaveCount(1);
    await expect(rows.first().locator('td').nth(1)).toHaveText(userId);
});

test('schema diagram opens tables, filters, focuses relationships, and exports SVG and PNG', async ({ page }) => {
    await page.goto('/');
    const browser = page.locator('.session-host:not([hidden]) .object-browser');
    await browser.getByRole('button', { name: /Schema diagram/ }).click();
    const diagram = activeDocument(page);
    await expect(diagram.locator('.schema-diagram')).toBeVisible();
    await expect(diagram.locator('.schema-node').filter({ hasText: 'users' })).toBeVisible();
    await expect(diagram.locator('.schema-node').filter({ has: page.getByText('orders', { exact: true }) })).toBeVisible();

    await diagram.getByLabel('Search diagram').fill('orders');
    await expect(diagram.locator('.schema-node').filter({ has: page.getByText('orders', { exact: true }) })).toBeVisible();
    await expect(diagram.locator('.schema-node').filter({ hasText: 'users' })).toHaveCount(0);
    await diagram.getByLabel('Clear diagram search').click();

    const usersNode = diagram.locator('.schema-node').filter({ hasText: 'users' });
    await usersNode.getByRole('button', { name: 'Related', exact: true }).click();
    await expect(diagram.getByRole('button', { name: /Related: users/ })).toBeVisible();
    await expect(diagram.locator('.diagram-edge.related').first()).toBeVisible();

    const [svgDownload] = await Promise.all([page.waitForEvent('download'), diagram.getByRole('button', { name: 'Export schema diagram as SVG', exact: true }).click()]);
    expect(svgDownload.suggestedFilename()).toBe('schema-diagram.svg');
    const svgPath = await svgDownload.path();
    expect(svgPath).not.toBeNull();
    const svg = await import('node:fs/promises').then((fs) => fs.readFile(svgPath!, 'utf8'));
    expect(svg).toContain('<svg');
    expect(svg).toContain('users');
    expect(svg).toContain('orders');
    expect(svg).toContain('user_id → id');

    const [pngDownload] = await Promise.all([page.waitForEvent('download'), diagram.getByRole('button', { name: 'Export schema diagram as PNG', exact: true }).click()]);
    expect(pngDownload.suggestedFilename()).toBe('schema-diagram.png');
    const pngPath = await pngDownload.path();
    expect(pngPath).not.toBeNull();
    const png = await import('node:fs/promises').then((fs) => fs.readFile(pngPath!));
    expect([...png.subarray(0, 8)]).toEqual([137, 80, 78, 71, 13, 10, 26, 10]);
    expect(png.length).toBeGreaterThan(1000);

    await usersNode.getByRole('button', { name: 'Open table', exact: true }).click();
    await expect(activeDocument(page).getByRole('button', { name: 'Structure', exact: true })).toBeVisible();
});

test('visual query builder joins tables, filters, and runs generated SQL', async ({ page }) => {
    await page.goto('/');
    await page.locator('.session-host:not([hidden]) summary[aria-label="Document and layout menu"]').click();
    await page.getByRole('button', { name: 'New visual query', exact: true }).click();

    const doc = activeDocument(page);
    await expect(doc.locator('.visual-query-builder')).toBeVisible();

    const tableSearch = doc.getByLabel('Add a table to the query');
    await tableSearch.fill('users');
    await doc.locator('.table-picker button', { hasText: /^users$/ }).click();
    await tableSearch.fill('orders');
    await doc.locator('.table-picker button', { hasText: /^orders$/ }).click();

    const usersNode = doc.locator('.query-node').filter({ hasText: 'users' });
    const ordersNode = doc.locator('.query-node').filter({ hasText: 'orders' });
    await expect(usersNode).toBeVisible();
    await expect(ordersNode).toBeVisible();

    await usersNode.getByRole('button', { name: /Join to/ }).click();
    const joinPanel = doc.locator('.join-inspector');
    await expect(joinPanel).toBeVisible();
    await expect(joinPanel.getByRole('button', { name: 'Left table', exact: true })).toContainText('t1 · users');
    await expect(joinPanel.getByRole('button', { name: 'Right table', exact: true })).toContainText('t2 · orders');
    await chooseOption(joinPanel, 'Left column', 'id');
    await chooseOption(joinPanel, 'Right column', 'user_id');
    await joinPanel.getByRole('button', { name: 'Add join', exact: true }).click();
    await expect(joinPanel).toHaveCount(0);

    await usersNode
        .locator('.query-column')
        .filter({ has: page.locator('.column-name', { hasText: /^name$/ }) })
        .locator('.input-control.checkbox')
        .click();
    await ordersNode
        .locator('.query-column')
        .filter({ has: page.locator('.column-name', { hasText: /^total$/ }) })
        .locator('.input-control.checkbox')
        .click();

    await doc.getByRole('button', { name: 'Add filter', exact: true }).click();
    const filterRow = doc.locator('.query-condition-row').first();
    await chooseOption(filterRow, 'Filter column', 't2.status');
    await filterRow.getByPlaceholder('value').fill('completed');

    const sql = doc.locator('.query-sql-preview pre');
    await expect(sql).toContainText('SELECT');
    await expect(sql).toContainText('JOIN');
    await expect(sql).toContainText('WHERE');
    await expect(sql).toContainText("'completed'");

    await doc.getByRole('button', { name: 'Run visual query', exact: true }).click();
    await expect(doc.locator('.results-body')).toBeVisible();

    await doc.getByRole('button', { name: 'Open generated SQL in a new query', exact: true }).click();
    await expect(activeDocument(page).locator('.cm-content')).toContainText('SELECT');
});

test('visual query builder: dragging a table from the object browser drops it onto the canvas', async ({ page }) => {
    await page.goto('/');
    await page.locator('.session-host:not([hidden]) summary[aria-label="Document and layout menu"]').click();
    await page.getByRole('button', { name: 'New visual query', exact: true }).click();

    const doc = activeDocument(page);
    await expect(doc.locator('.visual-query-builder')).toBeVisible();
    await expect(doc.locator('.diagram-empty')).toBeVisible();

    const source = objectButton(page, 'categories');
    const target = doc.locator('.query-canvas');

    // HTML5 drag-and-drop needs one DataTransfer shared across dragstart/drop
    // so the target can read back what dragstart wrote into it.
    const dataTransfer = await page.evaluateHandle(() => new DataTransfer());
    await source.dispatchEvent('dragstart', { dataTransfer });
    await expect(source).toHaveClass(/dragging/);
    await target.dispatchEvent('dragenter', { dataTransfer });
    await expect(target).toHaveClass(/drag-active/);
    await target.dispatchEvent('dragover', { dataTransfer });
    await target.dispatchEvent('drop', { dataTransfer });
    await expect(target).not.toHaveClass(/drag-active/);
    // A real drag fires 'dragend' on the source once the drop completes;
    // dispatchEvent doesn't do this automatically, so it's simulated here to
    // clear the drag-preview state the same way a genuine drag would.
    await source.dispatchEvent('dragend', { dataTransfer });
    await expect(source).not.toHaveClass(/dragging/);

    const categoriesNode = doc.locator('.query-node').filter({ hasText: 'categories' });
    await expect(categoriesNode).toBeVisible();
    await expect(doc.locator('.diagram-empty')).toHaveCount(0);

    // Dropping the same table again is a no-op with a clear notification, not
    // a duplicate node or a crash.
    const dataTransfer2 = await page.evaluateHandle(() => new DataTransfer());
    await source.dispatchEvent('dragstart', { dataTransfer: dataTransfer2 });
    await target.dispatchEvent('dragover', { dataTransfer: dataTransfer2 });
    await target.dispatchEvent('drop', { dataTransfer: dataTransfer2 });
    await expect(page.getByText('is already on the canvas.')).toBeVisible();
    await expect(doc.locator('.query-node').filter({ hasText: 'categories' })).toHaveCount(1);
});

test('visual query builder: multi-select and drag column rows onto the panels', async ({ page }) => {
    await page.goto('/');
    await page.locator('.session-host:not([hidden]) summary[aria-label="Document and layout menu"]').click();
    await page.getByRole('button', { name: 'New visual query', exact: true }).click();

    const doc = activeDocument(page);
    const tableSearch = doc.getByLabel('Add a table to the query');
    await tableSearch.fill('orders');
    await doc.locator('.table-picker button', { hasText: /^orders$/ }).click();

    const ordersNode = doc.locator('.query-node').filter({ hasText: 'orders' });
    const columnRow = (name: string) => ordersNode.locator('.query-column').filter({ has: page.locator('.column-name', { hasText: new RegExp(`^${name}$`) }) });

    // Click selects one row; Ctrl/Cmd-click toggles another into the
    // selection — the file-manager model, not a contiguous Shift-range here.
    await columnRow('total').click();
    await columnRow('status').click({ modifiers: ['ControlOrMeta'] });
    await expect(columnRow('total')).toHaveClass(/row-selected/);
    await expect(columnRow('status')).toHaveClass(/row-selected/);
    await expect(columnRow('currency')).not.toHaveClass(/row-selected/);

    const columnsPanel = doc.locator('.query-panel-section').filter({ hasText: 'Columns' }).first();

    // Dragging the row that's part of the multi-selection carries the whole
    // selection in one motion, and both dragged rows plus the drop target
    // show the drag/drop-active visual states while the gesture is in
    // flight.
    const columnsDataTransfer = await page.evaluateHandle(() => new DataTransfer());
    await columnRow('status').dispatchEvent('dragstart', { dataTransfer: columnsDataTransfer });
    await expect(columnRow('total')).toHaveClass(/dragging/);
    await expect(columnRow('status')).toHaveClass(/dragging/);
    await expect(columnRow('currency')).not.toHaveClass(/dragging/);
    await columnsPanel.dispatchEvent('dragenter', { dataTransfer: columnsDataTransfer });
    await expect(columnsPanel).toHaveClass(/drag-active/);
    await columnsPanel.dispatchEvent('dragover', { dataTransfer: columnsDataTransfer });
    await columnsPanel.dispatchEvent('drop', { dataTransfer: columnsDataTransfer });
    await expect(columnsPanel).not.toHaveClass(/drag-active/);
    await columnRow('status').dispatchEvent('dragend', { dataTransfer: columnsDataTransfer });
    await expect(columnRow('total')).not.toHaveClass(/dragging/);

    await expect(columnsPanel.locator('.query-column-row')).toHaveCount(2);
    await expect(columnsPanel.locator('.query-column-ref', { hasText: 't1.total' })).toBeVisible();
    await expect(columnsPanel.locator('.query-column-ref', { hasText: 't1.status' })).toBeVisible();

    // Dragging a single, unselected row drags just that one row — no
    // click-to-select step required first.
    const filtersPanel = doc.locator('.query-panel-section').filter({ hasText: 'Filters' }).first();
    const filterDataTransfer = await page.evaluateHandle(() => new DataTransfer());
    await columnRow('currency').dispatchEvent('dragstart', { dataTransfer: filterDataTransfer });
    await filtersPanel.dispatchEvent('dragover', { dataTransfer: filterDataTransfer });
    await filtersPanel.dispatchEvent('drop', { dataTransfer: filterDataTransfer });

    const filterRow = filtersPanel.locator('.query-condition-row').first();
    await expect(filterRow.getByRole('button', { name: 'Filter column', exact: true })).toContainText('t1.currency');

    const orderPanel = doc.locator('.query-panel-section').filter({ hasText: 'Order by' }).first();
    const orderDataTransfer = await page.evaluateHandle(() => new DataTransfer());
    await columnRow('created_at').dispatchEvent('dragstart', { dataTransfer: orderDataTransfer });
    await orderPanel.dispatchEvent('dragover', { dataTransfer: orderDataTransfer });
    await orderPanel.dispatchEvent('drop', { dataTransfer: orderDataTransfer });

    const orderRow = orderPanel.locator('.query-condition-row').first();
    await expect(orderRow.getByRole('button', { name: 'Order by column', exact: true })).toContainText('t1.created_at');

    // The per-row checkbox still works unchanged for single-column, no-drag
    // selection of the actual SELECT list.
    await columnRow('id').locator('.input-control.checkbox').click();
    await expect(columnsPanel.locator('.query-column-row')).toHaveCount(3);
    await expect(columnsPanel.locator('.query-column-ref', { hasText: 't1.id' })).toBeVisible();
});

test('visual query builder: drawing a line between two column handles creates a join', async ({ page }) => {
    await page.goto('/');
    const menuSummary = page.locator('.session-host:not([hidden]) summary[aria-label="Document and layout menu"]');
    await menuSummary.click();
    await page.getByRole('button', { name: 'New visual query', exact: true }).click();
    // The native <details> menu doesn't auto-close on selecting an item;
    // close it explicitly so it can't overlap later clicks near the top of
    // the document toolbar (it does here, once the join editor's own close
    // button renders in that area).
    await menuSummary.click();

    const doc = activeDocument(page);

    // Drag both tables onto the canvas far apart (rather than the default
    // search-and-click auto-layout, which places them close together): the
    // resulting join edge needs to be long enough that its label overlay —
    // an absolutely-positioned div centered on the edge, wider than a short
    // edge itself — doesn't cover the whole path and make it unclickable.
    const canvasBox = await doc.locator('.query-canvas').boundingBox();
    if (!canvasBox) throw new Error('canvas bounding box not available');
    const dropAt = async (name: string, x: number, y: number) => {
        const dataTransfer = await page.evaluateHandle(() => new DataTransfer());
        await objectButton(page, name).dispatchEvent('dragstart', { dataTransfer });
        await doc.locator('.query-canvas').dispatchEvent('dragover', { dataTransfer });
        await doc.locator('.query-canvas').dispatchEvent('drop', { dataTransfer, clientX: x, clientY: y });
    };
    await dropAt('orders', canvasBox.x + 80, canvasBox.y + 80);
    await dropAt('users', canvasBox.x + canvasBox.width - 120, canvasBox.y + 80);

    const ordersNode = doc.locator('.query-node').filter({ hasText: 'orders' });
    const usersNode = doc.locator('.query-node').filter({ hasText: 'users' });
    await ordersNode
        .locator('.query-column')
        .filter({ has: page.locator('.column-name', { hasText: /^id$/ }) })
        .locator('.input-control.checkbox')
        .click();
    await usersNode
        .locator('.query-column')
        .filter({ has: page.locator('.column-name', { hasText: /^name$/ }) })
        .locator('.input-control.checkbox')
        .click();

    // orders is t1, users is t2 (alphabetically-stable alias assignment order
    // matches the order tables were added in). Draw a line from orders'
    // user_id source handle to users' id target handle.
    const sourceHandle = doc.locator('.svelte-flow__handle-right[data-nodeid="t1"][data-handleid="user_id"]');
    const targetHandle = doc.locator('.svelte-flow__handle-left[data-nodeid="t2"][data-handleid="id"]');
    await expect(sourceHandle).toBeAttached();
    await expect(targetHandle).toBeAttached();

    const from = await sourceHandle.boundingBox();
    const to = await targetHandle.boundingBox();
    if (!from || !to) throw new Error('handle bounding boxes not available');

    await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
    await page.mouse.down();
    await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 12 });
    await page.mouse.up();

    const sql = doc.locator('.query-sql-preview pre');
    await expect(sql).toContainText('JOIN');

    // Clicking the drawn join edge still opens the same JoinEditor a
    // suggested/accepted join would — join drawing is a new source for the
    // existing edit path, not a separate one. Click the wider invisible
    // `edge-interaction` hit-path xyflow renders for exactly this purpose,
    // offset from center: the visible edge-label overlay sits centered on
    // the edge in its own absolutely-positioned container and intercepts
    // pointer events there.
    await doc
        .locator('.svelte-flow__edge-interaction')
        .first()
        .click({ position: { x: 90, y: 2 } });
    await expect(doc.locator('.join-inspector')).toBeVisible();
    await doc.getByRole('button', { name: 'Close join editor', exact: true }).click();

    // The join drawn purely by dragging runs for real against the SQLite
    // demo backend, exactly like a join built through any other path.
    await doc.getByRole('button', { name: 'Run visual query', exact: true }).click();
    await expect(doc.locator('.results-body')).toBeVisible();
});

test('visual query builder: row selection is keyboard-operable and copy SQL works', async ({ page }) => {
    await page.goto('/');
    await page.locator('.session-host:not([hidden]) summary[aria-label="Document and layout menu"]').click();
    await page.getByRole('button', { name: 'New visual query', exact: true }).click();

    const doc = activeDocument(page);
    const tableSearch = doc.getByLabel('Add a table to the query');
    await tableSearch.fill('orders');
    await doc.locator('.table-picker button', { hasText: /^orders$/ }).click();

    const ordersNode = doc.locator('.query-node').filter({ hasText: 'orders' });
    const columnRow = (name: string) => ordersNode.locator('.query-column').filter({ has: page.locator('.column-name', { hasText: new RegExp(`^${name}$`) }) });

    // Every drag gesture this redesign added has a working non-drag path
    // that keyboard/mouse-only users already exercise elsewhere: search-and-
    // click still adds a table (used above, same as the pre-existing test),
    // the per-row checkbox still adds a single output column with no drag
    // (checked at the end of this test), and "Join to…"/Add filter/Add order
    // by stay ordinary forms (covered by the original visual-query-builder
    // test, unchanged). What's new here: multi-select itself — Enter/Space
    // toggling a row, with modifiers — must be reachable by keyboard alone,
    // and moving focus off a row must never get stuck.
    const idRow = columnRow('id');
    const userIdRow = columnRow('user_id');
    const totalRow = columnRow('total');
    await idRow.focus();
    await page.keyboard.press('Enter');
    await expect(idRow).toHaveClass(/row-selected/);

    // Each row's own checkbox is a natural Tab stop too, so moving to the
    // next row takes two Tabs — proving, incidentally, that the row doesn't
    // trap focus inside itself either.
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    await expect(userIdRow).toBeFocused();
    await page.keyboard.press('Shift+Enter');
    await expect(idRow).toHaveClass(/row-selected/);
    await expect(userIdRow).toHaveClass(/row-selected/);

    // Focus keeps moving normally after using the keyboard to select —
    // selecting a row is not a keyboard trap.
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    await expect(totalRow).toBeFocused();

    // The per-row checkbox (a plain, no-drag, single-column selection) still
    // works unchanged.
    await totalRow.locator('.input-control.checkbox').click();
    await expect(doc.locator('.query-panel-section').filter({ hasText: 'Columns' }).first()).toContainText('t1.total');

    // Copy SQL (Phase D): enabled once the compiler has produced SQL, copies
    // the exact generated text.
    const copyButton = doc.getByRole('button', { name: 'Copy generated SQL', exact: true });
    await expect(copyButton).toBeEnabled();
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await copyButton.click();
    await expect(page.getByText('Copied to clipboard.')).toBeVisible();
    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    const sqlText = await doc.locator('.query-sql-preview pre').textContent();
    expect(clipboardText).toBe(sqlText);
    expect(clipboardText).toContain('SELECT');
});

test('results panel can be collapsed and reopened per document', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    await runQuery(doc);
    await expect(doc.locator('.results-body')).toBeVisible();

    await doc.getByRole('button', { name: 'Hide results panel', exact: true }).click();
    await expect(doc.locator('.results-body')).toHaveCount(0);
    await expect(doc.getByRole('button', { name: /Results hidden/ })).toBeVisible();

    await doc.getByRole('button', { name: /Results hidden/ }).click();
    await expect(doc.locator('.results-body')).toBeVisible();
});

test('Run All executes multiple statements and shows per-statement results', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+a');
    const table = `e2e_scratch_${Date.now()}`;
    await page.keyboard.type(`CREATE TABLE ${table} (id INTEGER); INSERT INTO ${table} VALUES (1), (2), (3); SELECT * FROM ${table};`);
    await doc.locator('summary[aria-label="Run options"]').click();
    await doc.getByRole('button', { name: 'Run All', exact: true }).click();
    await expect(doc.locator('.result-set-tabs button')).toHaveCount(3);
    await expect(doc.locator('.result-set-tabs button').nth(1)).toBeVisible();
    await doc.locator('.result-set-tabs button').nth(1).click();
    await expect(doc.locator('.empty-state')).toContainText('3 rows affected');
    await doc.locator('.result-set-tabs button').nth(2).click();
    await expect(doc.locator('.results-footer')).toContainText('of 3 loaded rows');
});

test('a failing statement stops the batch and surfaces the error', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type('SELECT 1; SELEKT nonsense; SELECT 2;');
    await doc.locator('summary[aria-label="Run options"]').click();
    await doc.getByRole('button', { name: 'Run All', exact: true }).click();
    await expect(doc.locator('.result-set-tabs button')).toHaveCount(2);
    await doc.getByRole('button', { name: 'Messages', exact: true }).click();
    await expect(doc.locator('.messages p')).toHaveClass(/error-text/);
});

test('go to definition jumps from the editor to the table structure view', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type('SELECT * FROM orders');
    await page.keyboard.press('F12');
    const opened = activeDocument(page);
    await expect(opened.getByRole('button', { name: 'Structure', exact: true })).toBeVisible();
    await expect(opened.locator('.metadata-table')).toContainText('user_id');
});

test('alias-aware and CTE-aware completion suggest the right joined/CTE columns', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type('SELECT 1 FROM orders o WHERE o.');
    await page.keyboard.press('Control+Space');
    await expect(doc.locator('.cm-tooltip-autocomplete')).toBeVisible();
    await expect(doc.locator('.cm-tooltip-autocomplete')).toContainText('status');
    await page.keyboard.press('Escape');

    // The CTE's namespace is rebuilt on a debounced re-parse of the document
    // text (see QueryEditor's languageExtension effect), not on every
    // keystroke — give it a moment to land before relying on it.
    await page.keyboard.press('Control+a');
    await page.keyboard.type('WITH recent_orders AS (SELECT id, total FROM orders) SELECT ');
    await page.waitForTimeout(400);
    await page.keyboard.type('recent_orders.');
    await page.keyboard.press('Control+Space');
    await expect(doc.locator('.cm-tooltip-autocomplete')).toBeVisible();
    await expect(doc.locator('.cm-tooltip-autocomplete')).toContainText('total');
});

test('table columns can be hidden and reordered, and the layout persists across reload', async ({ page }) => {
    await page.goto('/');
    await objectButton(page, 'orders').click();
    const doc = activeDocument(page);
    await expect(doc.locator('.results-footer')).toContainText('of 82 loaded rows');

    await doc.locator('summary[aria-label="Columns"]').click();
    await doc.getByRole('button', { name: 'currency', exact: true }).click();
    await expect(doc.locator('.data-grid')).not.toContainText('currency');
    await doc.getByRole('button', { name: 'Move user_id right' }).click();
    const headerTextsBefore = await doc.locator('.data-grid th .column-label span:not(.type-mark)').allTextContents();
    expect(headerTextsBefore[0]).toBe('id');
    expect(headerTextsBefore[1]).toBe('total');
    expect(headerTextsBefore[2]).toBe('user_id');

    await page.reload();
    const reloadedDoc = activeDocument(page);
    await expect(reloadedDoc.locator('.connection-warning')).toContainText('disconnected');
    await expect(reloadedDoc.locator('.data-grid')).toHaveCount(0);
    await reloadedDoc.getByRole('button', { name: 'Reconnect', exact: true }).click();
    await expect(reloadedDoc.locator('.results-footer')).toContainText('of 82 loaded rows');
    await expect(reloadedDoc.locator('.data-grid')).not.toContainText('currency');
    const headerTextsAfter = await reloadedDoc.locator('.data-grid th .column-label span:not(.type-mark)').allTextContents();
    expect(headerTextsAfter.slice(0, 3)).toEqual(['id', 'total', 'user_id']);
});

test('server-side table filter narrows real rows and Load More paginates', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'Results', exact: true }).click();
    await page.getByLabel('Result row limit', { exact: true }).fill('50');
    await page.getByLabel('Result row limit', { exact: true }).press('Tab');
    await page.getByRole('button', { name: 'Close dialog' }).click();

    await objectButton(page, 'users').click();
    const doc = activeDocument(page);
    // The users table document auto-fetches on session mount, which can race
    // ahead of the resultLimit change above; force a fresh fetch under the
    // now-current limit rather than relying on that race.
    await doc.getByRole('button', { name: 'Refresh table', exact: true }).click();
    await expect(doc.locator('.results-footer')).toContainText('1–50 of 50 loaded rows');
    await expect(doc.locator('.results-footer')).toContainText('more available');

    await doc.getByRole('button', { name: 'Load more rows' }).click();
    await expect(doc.locator('.results-footer')).toContainText('1–100 of 100 loaded rows');

    await doc.getByLabel('Filter table', { exact: true }).fill('invited');
    await doc.getByLabel('Filter table', { exact: true }).blur();
    await expect(doc.locator('tbody tr:not(.spacer)').first()).toBeVisible();
    const footerText = await doc.locator('.results-footer').first().textContent();
    const match = footerText?.match(/of (\d+) loaded rows/);
    expect(match).not.toBeNull();
    const filteredCount = Number(match?.[1]);
    expect(filteredCount).toBeGreaterThan(0);
    expect(filteredCount).toBeLessThan(100);
    await expect(doc.locator('tbody')).toContainText('invited');
    await expect(doc.locator('tbody')).not.toContainText('active');
});

test('small windows and system theme changes keep primary controls reachable', async ({ page }) => {
    await page.setViewportSize({ width: 900, height: 700 });
    await page.emulateMedia({ colorScheme: 'light' });
    await page.goto('/');
    // Monochrome is the app default; explicitly opt into System before
    // asserting that OS colour-scheme changes are followed.
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'System', exact: true }).click();
    await page.getByRole('button', { name: 'Close dialog' }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'monochrome-light');
    await page.emulateMedia({ colorScheme: 'dark' });
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'monochrome');
    await expect(page.locator('.session-host:not([hidden]) .sidebar-wrapper')).not.toBeVisible();
    await expect(activeDocument(page).getByRole('button', { name: 'Run query' })).toBeInViewport();
    await page.locator('.session-host:not([hidden])').getByRole('button', { name: 'Toggle explorer' }).click();
    await expect(page.locator('.session-host:not([hidden]) .sidebar-wrapper')).toBeVisible();
    await page.locator('.session-host:not([hidden])').getByRole('button', { name: 'Toggle explorer' }).click();
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'Monochrome Light', exact: true }).click();
    await page.emulateMedia({ colorScheme: 'dark' });
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'monochrome-light');
});

test('wide results virtualize columns and do not persist fetched data', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    await expect(doc.getByRole('button', { name: 'Run query', exact: true })).toBeEnabled();
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+a');
    await page.keyboard.insertText('SELECT ' + Array.from({ length: 300 }, (_, i) => `${i} AS c${i}`).join(', '));
    await runQuery(doc);
    await expect(doc.locator('.results-footer')).toContainText('1 loaded rows');
    expect(await doc.locator('.data-grid th .column-label').count()).toBeLessThan(20);
    await doc.locator('.grid-scroll').evaluate((el) => (el.scrollLeft = el.scrollWidth));
    await expect(doc.locator('.data-grid')).toContainText('c299');
    await expect
        .poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('quarro.test.workspace.v3') || '{}').sessions?.[0]?.documents?.[0]?.sql?.includes('c299')))
        .toBe(true);
    const cached = await page.evaluate(() => JSON.parse(localStorage.getItem('quarro.test.workspace.v3') || '{}').sessions[0].documents[0]);
    expect(cached.results).toBeUndefined();
});

test('row details switches modes and stages, reverts, and applies NULL edits', async ({ page }) => {
    await page.goto('/');
    await objectButton(page, 'users').click();
    const doc = activeDocument(page);
    await expect(doc.locator('.data-grid')).toBeVisible();
    await doc.locator('.cell').first().dblclick();
    const drawer = doc.getByRole('complementary', { name: 'Row details' });
    await expect(drawer.getByRole('tab', { name: 'Edit', exact: true })).toHaveAttribute('aria-selected', 'true');
    const name = drawer.getByRole('textbox', { name: 'name', exact: true });
    const original = await name.inputValue();
    await name.fill('Staged drawer test');
    await expect(drawer.getByRole('button', { name: 'Apply all (1)', exact: true })).toBeEnabled();
    await drawer.getByRole('tab', { name: 'Inspect', exact: true }).click();
    await drawer.getByRole('button', { name: 'JSON', exact: true }).click();
    await expect(drawer.locator('pre')).toContainText(original);
    await expect(drawer.locator('pre')).not.toContainText('Staged drawer test');
    await drawer.getByRole('tab', { name: /Edit/ }).click();
    await expect(name).toHaveValue('Staged drawer test');
    await drawer
        .locator('.editor-field')
        .filter({ has: page.getByRole('textbox', { name: 'name', exact: true }) })
        .getByRole('button', { name: 'Revert', exact: true })
        .click();
    await expect(drawer.getByRole('button', { name: 'Apply all (0)', exact: true })).toBeDisabled();

    const nullable = drawer.locator('.editor-field').filter({ has: page.getByRole('textbox', { name: 'last_login', exact: true }) });
    const nullToggle = nullable.getByRole('button', { name: 'NULL', exact: true });
    if ((await nullToggle.getAttribute('aria-pressed')) === 'true') await nullToggle.click();
    await nullToggle.click();
    await expect(nullable.locator('textarea')).toBeDisabled();
    await nullToggle.click();
    await expect(nullable.locator('textarea')).toBeEnabled();
    if (await drawer.getByRole('button', { name: 'Discard all', exact: true }).isEnabled()) await drawer.getByRole('button', { name: 'Discard all', exact: true }).click();
    // Leave data untouched; verify apply + rollback using a document-owned transaction.
    await doc.getByRole('button', { name: 'Begin transaction', exact: true }).click();
    await name.fill('Applied drawer test');
    await drawer.getByRole('button', { name: 'Preview SQL', exact: true }).click();
    const preview = drawer.getByRole('region', { name: 'Generated SQL preview' });
    await expect(preview).toContainText('UPDATE');
    await expect(preview).toContainText('Bound values');
    await expect(preview).toContainText('Applied drawer test');
    await name.fill('Recomputed drawer test');
    await expect(preview).toContainText('Recomputed drawer test');
    await name.fill('Applied drawer test');
    await expect(preview).toContainText('Applied drawer test');
    await drawer.getByRole('button', { name: 'Apply all (1)', exact: true }).click();
    await expect(doc.locator('tbody')).toContainText('Applied drawer test');
    await doc.getByRole('button', { name: 'Rollback transaction', exact: true }).click();
    await expect(doc.locator('tbody')).toContainText(original);
    await expect(doc.locator('tbody')).not.toContainText('Applied drawer test');
    for (const theme of ['monochrome', 'monochrome-light']) {
        await page.evaluate((theme) => (document.documentElement.dataset.theme = theme), theme);
        await page.setViewportSize({ width: 1024, height: 768 });
        await expect(drawer).toBeVisible();
        await expect(drawer.getByRole('button', { name: /Apply all/ })).toBeVisible();
        await page.screenshot({ path: `/tmp/quarro-row-details-${theme}.png` });
    }
});

test('density defaults to comfortable and compact keeps virtual rows aligned', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    const header = page.locator('.session-bar');
    const brandMark = page.locator('.brand-mark:visible');
    const densityMetrics = () =>
        page.evaluate(() => {
            const height = (selector: string) => Math.round(document.querySelector(selector)!.getBoundingClientRect().height);
            const paddingTop = (selector: string) => Math.round(parseFloat(getComputedStyle(document.querySelector(selector)!).paddingTop));
            return {
                documentBar: height('.document-bar'),
                queryToolbar: height('.query-toolbar'),
                browserHeading: height('.browser-heading'),
                resultsHeader: height('.results-header'),
                gridToolbar: height('.grid-toolbar'),
                gridHeader: height('.data-grid th'),
                editorPaddingTop: paddingTop('.query-editor .cm-content'),
                treePaddingTop: paddingTop('.tree-object'),
                button: height('.query-toolbar .button'),
            };
        });
    await runQuery(doc);
    await expect(page.locator('html')).toHaveAttribute('data-density', 'comfortable');
    expect(Math.round((await header.boundingBox())!.height)).toBe(50);
    expect(Math.round((await brandMark.boundingBox())!.width)).toBe(22);
    expect(await densityMetrics()).toEqual({
        documentBar: 43,
        queryToolbar: 54,
        browserHeading: 49,
        resultsHeader: 43,
        gridToolbar: 52,
        gridHeader: 37,
        editorPaddingTop: 20,
        treePaddingTop: 8,
        button: 30,
    });
    const rowHeight = () =>
        doc
            .locator('.data-grid .cell')
            .first()
            .evaluate((el) => el.getBoundingClientRect().height);
    expect(await rowHeight()).toBe(33);
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await chooseOption(page, 'Interface density', 'Compact');
    await expect(page.locator('html')).toHaveAttribute('data-density', 'compact');
    await page.getByRole('button', { name: 'Close dialog' }).click();
    expect(Math.round((await header.boundingBox())!.height)).toBe(44);
    expect(Math.round((await brandMark.boundingBox())!.width)).toBe(20);
    expect(await densityMetrics()).toEqual({
        documentBar: 35,
        queryToolbar: 44,
        browserHeading: 40,
        resultsHeader: 35,
        gridToolbar: 42,
        gridHeader: 31,
        editorPaddingTop: 12,
        treePaddingTop: 5,
        button: 28,
    });
    expect(await rowHeight()).toBe(27);
    await doc.locator('.grid-scroll').evaluate((el) => {
        el.scrollTop = 2000;
    });
    await expect(doc.locator('tbody .cell').first()).toBeAttached();
    await expect
        .poll(() =>
            page.evaluate(() => {
                const saved = JSON.parse(localStorage.getItem('quarro.test.workspace.v3') || '{}');
                return [saved.densityVersion, saved.preferences?.density];
            }),
        )
        .toEqual([2, 'compact']);
    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-density', 'compact');
    // Legacy Compact adopts the renamed baseline once.
    await page.addInitScript(() => {
        const key = 'quarro.test.workspace.v3';
        const saved = JSON.parse(localStorage.getItem(key)!);
        delete saved.densityVersion;
        localStorage.setItem(key, JSON.stringify(saved));
    });
    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-density', 'comfortable');
});

test('density compacts canvas chrome and dialog spacing', async ({ page }) => {
    await page.goto('/');
    await page
        .locator('.object-browser')
        .getByRole('button', { name: /^Schema diagram\b/ })
        .click();
    const toolbar = page.locator('.diagram-toolbar');
    const flowControl = page.locator('.diagram-canvas .svelte-flow__controls-button').first();
    await expect(toolbar).toBeVisible();
    expect(Math.round((await toolbar.boundingBox())!.height)).toBe(52);
    expect(Math.round((await flowControl.boundingBox())!.width)).toBe(28);

    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    const settings = page.locator('.settings-content');
    expect(Math.round(parseFloat(await settings.evaluate((el) => getComputedStyle(el).paddingLeft)))).toBe(30);
    await chooseOption(page, 'Interface density', 'Compact');
    expect(Math.round(parseFloat(await settings.evaluate((el) => getComputedStyle(el).paddingLeft)))).toBe(22);
    await page.getByRole('button', { name: 'Close dialog' }).click();

    expect(Math.round((await toolbar.boundingBox())!.height)).toBe(43);
    expect(Math.round((await flowControl.boundingBox())!.width)).toBe(24);
});

test('running a parameterized statement prompts for values and binds them safely', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type('SELECT * FROM users WHERE id = :id');
    await expect(editor).toContainText('SELECT * FROM users WHERE id = :id');
    await runQuery(doc);
    const dialog = page.getByRole('dialog', { name: 'Run with parameters' });
    await expect(dialog).toBeVisible();
    await dialog.getByLabel('Value for :id').fill('1');
    await dialog.getByRole('button', { name: 'Run', exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(doc.locator('.results-footer')).toContainText('of 1 loaded rows');

    // Re-running the same statement reuses the confirmed value without
    // reprompting; editing the SQL to add a new placeholder prompts again.
    await runQuery(doc);
    await expect(dialog).not.toBeVisible();
    await expect(doc.locator('.results-footer')).toContainText('of 1 loaded rows');

    await editor.click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type('SELECT * FROM users WHERE id = :id AND role = :role');
    await expect(editor).toContainText('SELECT * FROM users WHERE id = :id AND role = :role');
    await runQuery(doc);
    await expect(dialog).toBeVisible();
    await expect(dialog.getByLabel('Value for :id')).toHaveValue('1');
    await dialog.getByLabel('Value for :role').fill('admin');
    await dialog.getByRole('button', { name: 'Run', exact: true }).click();
    await expect(dialog).not.toBeVisible();
});

test('the command palette runs discoverable actions', async ({ page }) => {
    await page.goto('/');
    await page.keyboard.press('Control+k');
    const palette = page.getByRole('dialog', { name: 'Command palette' });
    await expect(palette).toBeVisible();
    await page.getByLabel('Command palette search').fill('settings');
    await page.keyboard.press('Enter');
    await expect(palette).not.toBeVisible();
    await expect(page.getByRole('dialog').first()).toBeVisible();
    await page.getByRole('button', { name: 'Close dialog' }).click();

    await page.keyboard.press('Control+k');
    await page.getByLabel('Command palette search').fill('go to table: users');
    await page.getByRole('option', { name: /Go to table: users/ }).click();
    await expect(activeDocument(page)).toContainText('users');
});

test('saved queries can be favorited, renamed, and deleted', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+a');
    await page.keyboard.type('SELECT 1');
    await doc.getByRole('button', { name: 'Save query', exact: true }).click();

    await page.getByRole('button', { name: /Saved queries/ }).click();
    const row = page.locator('.saved-item-row').first();
    await expect(row).toBeVisible();
    await row.getByRole('button', { name: /Mark as favorite/ }).click();
    await expect(row.getByRole('button', { name: /Remove from favorites/ })).toBeVisible();

    await row.getByLabel(/Rename/).fill('My favorite query');
    await row.getByLabel(/Rename/).press('Tab');
    await expect(row.getByLabel(/Rename/)).toHaveValue('My favorite query');
    await expect(row.getByRole('button', { name: /Delete My favorite query/ })).toBeVisible();

    await row.getByRole('button', { name: /Delete My favorite query/ }).click();
    await expect(page.locator('.saved-item-row')).toHaveCount(0);
});

test('query history records outcome and duration, and can be cleared', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    await runQuery(doc);
    await page.getByRole('button', { name: /Query history/ }).click();
    const entry = page.locator('.saved-item-row').first();
    await expect(entry).toContainText('ms');
    await expect(entry).not.toContainText('failed');

    await page.getByRole('button', { name: 'Clear', exact: true }).click();
    await expect(page.locator('.saved-item-row')).toHaveCount(0);
});

test('exporting loaded query rows triggers a real browser download', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    await runQuery(doc);
    await doc.getByRole('button', { name: 'Export results', exact: true }).click();
    const panel = doc.locator('.transfer-panel');
    await expect(panel).toBeVisible();
    await expect(panel.getByRole('button', { name: 'Scope', exact: true })).toContainText('Loaded rows');
    const [download] = await Promise.all([page.waitForEvent('download'), panel.getByRole('button', { name: 'Export', exact: true }).click()]);
    expect(download.suggestedFilename()).toBe('users.csv');
    // A side panel, not a modal: it stays open (and the rest of the workspace
    // stays usable) after a quick client-side export.
    await expect(panel).toBeVisible();
});

test('a table export past the loaded scope goes through the native destination dialog', async ({ page }) => {
    await page.goto('/');
    await objectButton(page, 'users').click();
    const doc = activeDocument(page);
    await expect(doc.locator('.data-grid')).toBeVisible();
    await doc.getByRole('button', { name: 'Export results', exact: true }).click();
    const panel = doc.locator('.transfer-panel');
    await expect(panel.getByRole('button', { name: 'Scope', exact: true })).toContainText('All rows');
    await chooseOption(panel, 'Format', 'SQL (INSERT statements)');
    await panel.getByRole('button', { name: 'Choose destination & export', exact: true }).click();
    // The Playwright test harness runs the real Go backend in server mode,
    // where native file dialogs are unavailable by design (see
    // internal/app/dialogs.go) — this exercises that real boundary rather than
    // a mocked one, and confirms it fails gracefully instead of hanging.
    await expect(page.locator('.toast')).toContainText('not available in server mode');
    await expect(panel).toBeVisible();
    // The rest of the document (grid, toolbar) is still interactive — the
    // panel never blocked the view.
    await expect(doc.locator('.data-grid')).toBeVisible();
});

test('importing a CSV goes through the native open-file dialog', async ({ page }) => {
    await page.goto('/');
    await objectButton(page, 'users').click();
    const doc = activeDocument(page);
    await expect(doc.locator('.data-grid')).toBeVisible();
    await doc.getByRole('button', { name: 'Import CSV', exact: true }).click();
    const panel = doc.locator('.transfer-panel');
    await expect(panel.getByRole('tab', { name: 'Import', exact: true })).toHaveAttribute('aria-selected', 'true');
    await expect(panel.getByRole('button', { name: 'Source encoding', exact: true })).toContainText('Auto / UTF-8');
    await panel.getByRole('button', { name: 'Source encoding', exact: true }).click();
    await page.getByRole('listbox', { name: 'Source encoding', exact: true }).getByRole('option', { name: 'Windows-1252', exact: true }).click();
    await expect(panel.getByRole('button', { name: 'Source encoding', exact: true })).toContainText('Windows-1252');
    await panel.getByRole('button', { name: 'Choose CSV file…', exact: true }).click();
    await expect(page.locator('.toast')).toContainText('not available in server mode');
    await expect(panel).toBeVisible();
});

test('the transfer panel and row inspector share one dockable slot', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    await runQuery(doc);
    await doc.locator('.cell').first().dblclick();
    await expect(doc.locator('.row-details')).toBeVisible();
    await doc.getByRole('button', { name: 'Export results', exact: true }).click();
    await expect(doc.locator('.transfer-panel')).toBeVisible();
    await expect(doc.locator('.row-details')).not.toBeVisible();
    await doc.getByRole('button', { name: 'Toggle row inspector' }).click();
    await expect(doc.locator('.row-details')).toBeVisible();
    await expect(doc.locator('.transfer-panel')).not.toBeVisible();
});

test('explain opens a graph view for the demo query, and gates ANALYZE on SQLite', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    // The demo document's seeded SQL (fixtures.ts) is a plain SELECT from
    // users, with the cursor already inside it.
    await doc.getByRole('button', { name: 'Explain statement', exact: true }).click();

    const explainDoc = activeDocument(page);
    await expect(explainDoc.locator('.explain-node').first()).toBeVisible();
    await expect(explainDoc.locator('.explain-status')).toContainText('SQLite reports no cost or execution metrics');

    // SQLite has no EXPLAIN ANALYZE equivalent; the toggle stays disabled and
    // clicking it must not switch modes or trigger a re-run.
    const analyzeToggle = explainDoc.getByRole('button', { name: 'Analyze', exact: true });
    await expect(analyzeToggle).toBeDisabled();

    await explainDoc.locator('.explain-node').first().click();
    await expect(explainDoc.locator('.explain-detail')).toBeVisible();
    await expect(explainDoc.locator('.explain-detail h3')).not.toBeEmpty();
    await explainDoc.getByRole('button', { name: 'Close', exact: true }).click();
    await expect(explainDoc.locator('.explain-detail')).toHaveCount(0);
});

test('explain view switcher renders all five views from one cached plan with no re-run', async ({ page }) => {
    await page.goto('/');
    const doc = activeDocument(page);
    await doc.getByRole('button', { name: 'Explain statement', exact: true }).click();

    const explainDoc = activeDocument(page);
    await expect(explainDoc.locator('.explain-node').first()).toBeVisible();
    const switcher = explainDoc.getByRole('group', { name: 'Explain view' });

    // Table View: same tree, as an expandable table; the root row's children
    // can be collapsed and re-expanded without losing the row.
    await switcher.getByRole('button', { name: 'Table', exact: true }).click();
    await expect(explainDoc.locator('.explain-status')).not.toContainText('Running');
    const rows = explainDoc.locator('.explain-plan-table tbody tr');
    const rowCountExpanded = await rows.count();
    expect(rowCountExpanded).toBeGreaterThan(0);
    const toggle = explainDoc.locator('.explain-collapse-toggle').first();
    await toggle.click();
    await expect(rows).toHaveCount(rowCountExpanded - 1);
    await toggle.click();
    await expect(rows).toHaveCount(rowCountExpanded);

    // Diagram View: a bar per node, with a working metric picker.
    await switcher.getByRole('button', { name: 'Diagram', exact: true }).click();
    await expect(explainDoc.locator('.explain-status')).not.toContainText('Running');
    await expect(explainDoc.locator('.explain-diagram-row').first()).toBeVisible();
    await explainDoc.getByRole('group', { name: 'Diagram metric' }).getByRole('button', { name: 'Rows', exact: true }).click();
    await expect(explainDoc.locator('.explain-diagram-row').first()).toBeVisible();

    // Stats View: plan-wide aggregates, no node selected/no detail panel.
    await switcher.getByRole('button', { name: 'Stats', exact: true }).click();
    await expect(explainDoc.locator('.explain-status')).not.toContainText('Running');
    await expect(explainDoc.locator('.explain-stat-card').first()).toBeVisible();
    await expect(explainDoc.locator('.explain-detail')).toHaveCount(0);

    // Raw Output: the unmodified engine response, and a working copy action.
    await switcher.getByRole('button', { name: 'Raw', exact: true }).click();
    await expect(explainDoc.locator('.explain-status')).not.toContainText('Running');
    await expect(explainDoc.locator('.explain-raw-pre')).toContainText('SCAN');
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await explainDoc.getByRole('button', { name: 'Copy raw output', exact: true }).click();
    // copy() asks the Go clipboard first, so the write lands asynchronously.
    await expect(page.getByText('Copied to clipboard.')).toBeVisible();
    const copied = await page.evaluate(() => navigator.clipboard.readText());
    expect(copied).toContain('SCAN');

    // Back to Graph: still the same rendered plan, not a fresh run.
    await switcher.getByRole('button', { name: 'Graph', exact: true }).click();
    await expect(explainDoc.locator('.explain-status')).not.toContainText('Running');
    await expect(explainDoc.locator('.explain-node').first()).toBeVisible();
});

test('"Show in editor" jumps from a plan node to its table reference in the original query', async ({ page }) => {
    await page.goto('/');
    const sourceDoc = activeDocument(page);
    await sourceDoc.getByRole('button', { name: 'Explain statement', exact: true }).click();

    const explainDoc = activeDocument(page);
    // The root ".explain-node" is the synthetic "QUERY PLAN" wrapper with no
    // table reference of its own; the actual scan of "users" is its child.
    const scanNode = explainDoc.locator('.explain-node').filter({ hasText: 'SCAN' });
    await expect(scanNode).toBeVisible();
    await scanNode.click();
    await explainDoc.getByRole('button', { name: 'Show in editor', exact: true }).click();

    // Switching tabs makes the original query document active again.
    const active = activeDocument(page);
    await expect(active.locator('.query-editor')).toBeVisible();
    await expect(active.getByText('Explain 1')).toHaveCount(0);

    // The editor selected the exact table reference ("users") and focused
    // itself, not just switched tabs.
    await expect(active.locator('.cm-content')).toBeFocused();
    const selected = await page.evaluate(() => window.getSelection()?.toString());
    expect(selected).toBe('users');
});

test('the Linux title bar setting switches to Quarro window chrome and Go keeps the choice', async ({ page }) => {
    // DesktopService only offers the choice on Linux, and the test server
    // reports the host OS.
    test.skip(process.platform !== 'linux', 'title bar choice is Linux-only');
    await page.goto('/');
    const bar = page.locator('.session-bar');
    await expect(bar).not.toHaveClass(/titlebar/);
    await expect(page.getByRole('button', { name: 'Minimise', exact: true })).toHaveCount(0);

    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await chooseOption(page, 'Title bar', 'Quarro');
    await page.getByRole('button', { name: 'Close dialog' }).click();
    await expect(bar).toHaveClass(/titlebar/);
    await expect(page.getByRole('button', { name: 'Minimise', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Close window', exact: true })).toBeVisible();
    // Double-click maximises from empty bar space only. The runtime sends
    // Window.ToggleMaximise as {object: 6, method: 39} to /wails/runtime.
    let toggles = 0;
    page.on('request', (request) => {
        const body = request.url().endsWith('/wails/runtime') ? request.postDataJSON() : null;
        if (body?.object === 6 && body?.method === 39) toggles++;
    });
    const box = (await bar.boundingBox())!;
    await bar.dblclick({ position: { x: box.width / 2, y: box.height / 2 } });
    await expect.poll(() => toggles).toBe(1);
    await page.getByRole('button', { name: 'New session', exact: true }).dblclick();
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    expect(toggles).toBe(1);
    // Tabs and buttons opt out of the drag region; empty bar space drags.
    expect(await bar.evaluate((el) => getComputedStyle(el).getPropertyValue('--wails-draggable').trim())).toBe('drag');
    expect(await page.getByRole('button', { name: 'New session', exact: true }).evaluate((el) => getComputedStyle(el).getPropertyValue('--wails-draggable').trim())).toBe(
        'no-drag',
    );

    // Go's window.json wins over a stale localStorage mirror.
    await page.evaluate(() => {
        const key = 'quarro.test.workspace.v3';
        const saved = JSON.parse(localStorage.getItem(key) || '{}');
        saved.preferences = { ...saved.preferences, titleBar: 'system' };
        localStorage.setItem(key, JSON.stringify(saved));
    });
    await page.reload();
    await expect(bar).toHaveClass(/titlebar/);

    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await chooseOption(page, 'Title bar', 'System');
    await page.getByRole('button', { name: 'Close dialog' }).click();
    await expect(bar).not.toHaveClass(/titlebar/);
    await expect(page.getByRole('button', { name: 'Minimise', exact: true })).toHaveCount(0);
    // With the system title bar, the session bar is not a title bar.
    await bar.dblclick({ position: { x: box.width / 2, y: box.height / 2 } });
    await page.waitForTimeout(300);
    expect(toggles).toBe(1);
});

test('a cached newer release shows an update notice that the setting and dismissal hide', async ({ page }) => {
    const version = /\n {2}version: "([^"]+)"/.exec(readFileSync(new URL('../../build/config.yml', import.meta.url), 'utf8'))?.[1];
    await page.goto('/');
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await expect(page.locator('.settings-nav-bottom')).toContainText(`Quarro ${version}`);
    await page.getByRole('button', { name: 'Close dialog' }).click();

    // The test build never calls GitHub, so seed what a prod check would cache.
    await page.evaluate(() =>
        localStorage.setItem(
            'quarro.test.updates',
            JSON.stringify({
                checkedAt: Date.now(),
                latest: { available: true, version: '99.0.0', url: 'https://github.com/yazeed1s/quarro/releases/tag/v99.0.0' },
                dismissed: '',
            }),
        ),
    );
    await page.reload();
    const notice = page.locator('.update-notice');
    await expect(notice).toContainText('Quarro 99.0.0 is available.');

    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'Privacy', exact: true }).click();
    const toggle = page.getByRole('button', { name: 'Check for updates', exact: true });
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-pressed', 'false');
    await expect(notice).toHaveCount(0);
    await toggle.click();
    await expect(notice).toBeVisible();
    await page.getByRole('button', { name: 'Close dialog' }).click();

    await page.getByRole('button', { name: 'Dismiss update notice', exact: true }).click();
    await expect(notice).toHaveCount(0);
    await page.reload();
    await expect(page.locator('.session-bar')).toBeVisible();
    await expect(notice).toHaveCount(0);
});

// Makes the test server's DesktopService.Chrome() report another OS, so the
// macOS and Windows window chrome can be checked from a Linux test run.
async function fakeDesktopChrome(page: import('@playwright/test').Page, chrome: Record<string, unknown>) {
    const binding = readFileSync(new URL('../bindings/github.com/yazeed1s/quarro/internal/app/desktopservice.ts', import.meta.url), 'utf8');
    const methodID = Number(/function Chrome\(\)[\s\S]*?ByID\((\d+)\)/.exec(binding)?.[1]);
    await page.route('**/wails/runtime', (route) => {
        const body = route.request().postDataJSON();
        if (body?.object === 0 && body?.args?.methodID === methodID) return route.fulfill({ json: { titleBar: 'system', configurable: false, ...chrome } });
        return route.continue();
    });
}

test('Windows chrome: native-style caption buttons flush in the corner of a draggable bar', async ({ page }) => {
    await fakeDesktopChrome(page, { platform: 'windows', frameless: true, trafficLights: false });
    await page.goto('/');
    const bar = page.locator('.session-bar');
    await expect(bar).toHaveClass(/titlebar/);
    await expect(bar).toHaveClass(/caption-buttons/);
    const close = page.getByRole('button', { name: 'Close window', exact: true });
    await expect(page.getByRole('button', { name: 'Minimise', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Maximise', exact: true })).toBeVisible();
    await expect(close).toHaveClass(/caption-button/);
    // Native caption buttons: 46px wide, full bar height, touching the right edge.
    const [closeBox, barBox] = [(await close.boundingBox())!, (await bar.boundingBox())!];
    expect(Math.round(closeBox.width)).toBe(46);
    expect(Math.round(closeBox.x + closeBox.width)).toBe(Math.round(barBox.x + barBox.width));
    expect(Math.round(closeBox.height)).toBeGreaterThanOrEqual(Math.round(barBox.height) - 1);
    expect(await close.evaluate((el) => getComputedStyle(el).getPropertyValue('--wails-draggable').trim())).toBe('no-drag');
    // The title bar choice is Linux-only.
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Title bar', exact: true })).toHaveCount(0);
});

test('macOS chrome: room for the traffic lights and no Quarro window buttons', async ({ page }) => {
    await fakeDesktopChrome(page, { platform: 'darwin', frameless: false, trafficLights: true });
    await page.goto('/');
    const bar = page.locator('.session-bar');
    await expect(bar).toHaveClass(/titlebar/);
    await expect(bar).toHaveClass(/traffic-lights/);
    await expect(page.getByRole('button', { name: 'Minimise', exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Close window', exact: true })).toHaveCount(0);
    expect(await bar.evaluate((el) => getComputedStyle(el).paddingLeft)).toBe('80px');
    expect(await bar.evaluate((el) => getComputedStyle(el).getPropertyValue('--wails-draggable').trim())).toBe('drag');
});

test('the app icon setting asks Go to swap the icon and hides where it cannot', async ({ page }) => {
    const binding = readFileSync(new URL('../bindings/github.com/yazeed1s/quarro/internal/app/desktopservice.ts', import.meta.url), 'utf8');
    const methodID = Number(/function SetAppIcon\([\s\S]*?ByID\((\d+)/.exec(binding)?.[1]);
    const chrome = { platform: 'darwin', frameless: false, trafficLights: true, appIcon: 'dark', appIconConfigurable: true };
    await fakeDesktopChrome(page, chrome);
    const requested: unknown[] = [];
    // Registered last, so it sees each call before fakeDesktopChrome's route.
    await page.route('**/wails/runtime', (route) => {
        const body = route.request().postDataJSON();
        if (body?.object !== 0 || body?.args?.methodID !== methodID) return route.fallback();
        requested.push(body.args.args[0]);
        return route.fulfill({ json: { ...chrome, appIcon: body.args.args[0] } });
    });
    await page.goto('/');
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await chooseOption(page, 'App icon', 'Light');
    await expect.poll(() => requested).toEqual(['light']);

    // Hidden wherever Go reports the choice is unavailable.
    await fakeDesktopChrome(page, { platform: 'freebsd', frameless: false, trafficLights: false, appIcon: 'dark', appIconConfigurable: false });
    await page.reload();
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await expect(page.getByRole('button', { name: 'App icon', exact: true })).toHaveCount(0);
});

test('the brand mark contrasts with the theme, with HiDPI renditions', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' });
    await page.goto('/');
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'System', exact: true }).click();
    await page.getByRole('button', { name: 'Close dialog' }).click();
    const dark = page.locator('.brand-mark-dark-asset');
    const light = page.locator('.brand-mark-light-asset');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'monochrome-light');
    await expect(dark).toBeVisible();
    await expect(light).toBeHidden();
    await page.emulateMedia({ colorScheme: 'dark' });
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'monochrome');
    await expect(light).toBeVisible();
    await expect(dark).toBeHidden();
    const box = (await light.boundingBox())!;
    expect([Math.round(box.width), Math.round(box.height)]).toEqual([22, 22]);
    expect(await light.getAttribute('srcset')).toMatch(/ 2x, .* 3x$/);
    const brand = page.getByRole('button', { name: 'Quarro connection manager', exact: true });
    await expect(brand).toBeVisible();
    await expect(brand).toHaveText('');
    expect(Math.round((await page.locator('.session-bar').boundingBox())!.height)).toBe(50);
    await page.setViewportSize({ width: 640, height: 700 });
    await expect(brand).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
});

test('read-only profiles are visible and enforced beyond disabled mutation controls', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const manager = page.getByRole('dialog');
    await manager.getByLabel('Options for Sample database', { exact: true }).click();
    await manager.getByRole('button', { name: 'Edit profile', exact: true }).click();
    const readOnly = manager.getByRole('button', { name: 'Open this connection as read-only', exact: true });
    await readOnly.click();
    await expect(readOnly).toHaveAttribute('aria-pressed', 'true');
    await manager.getByRole('button', { name: 'Save profile', exact: true }).click();
    await expect(manager.getByText('READ ONLY', { exact: true })).toBeVisible();
    await manager.getByRole('button', { name: 'Close dialog', exact: true }).click();

    // The already-open session retains the mode it was connected with. Close
    // it and reconnect from the updated profile to create a read-only session.
    await page.getByRole('button', { name: 'Close session Sample database', exact: true }).click();
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    await page.getByRole('button', { name: 'Open Sample database', exact: true }).click();

    await expect(page.locator('.status-center')).toContainText('Read-only session');
    const doc = activeDocument(page);

    const editor = doc.locator('.cm-content');
    await editor.click();
    await page.keyboard.press('Control+A');
    await page.keyboard.type('DELETE FROM users WHERE id = 1;');
    await doc.locator('.run-group summary').click();
    await doc.getByRole('button', { name: 'Run All', exact: true }).click();
    await expect(doc.getByRole('heading', { name: 'Statement failed', exact: true })).toBeVisible();
    await expect(doc.locator('.error-text')).toContainText(/read.?only/i);

    await objectButton(page, 'users').click();
    const table = activeDocument(page);
    await expect(table.getByRole('button', { name: 'Begin transaction', exact: true })).toBeDisabled();
    await expect(table.getByRole('button', { name: 'Insert row', exact: true })).toBeDisabled();
    await expect(table.getByRole('button', { name: 'Delete row', exact: true })).toBeDisabled();
    await expect(table.getByRole('button', { name: 'Import CSV', exact: true })).toBeDisabled();
});
