import { expect, test } from '@playwright/test';

// Proves a chart can combine one series from a SQL table and one from a
// MongoDB collection, in two different connected sessions, end to end.
// Skipped unless
// scripts/test-engine.sh's mongo-chart mode has set QUARRO_TEST_MONGO_URL
// against a real disposable MongoDB seeded with an "orders" collection.
test('a cross-engine chart combines one series from a SQL table and one from a MongoDB collection', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_MONGO_URL, 'Run scripts/test-engine.sh mongo-chart');
    const [host, port] = process.env.QUARRO_TEST_MONGO_URL!.split(':');
    const database = process.env.QUARRO_TEST_MONGO_DB ?? 'quarro_chart';

    await page.goto('/');
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(1);

    // Series 1: the SQLite demo session's "users" table.
    const sqliteBrowser = page.locator('.session-host:not([hidden]) .object-browser');
    await sqliteBrowser.getByRole('button', { name: 'Visualize users', exact: true }).click();
    const chart = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(chart.locator('.chart-document')).toBeVisible();
    await expect(chart.locator('.chart-canvas canvas')).toBeVisible();
    await expect(chart.locator('.chart-series-tab')).toHaveCount(1);

    // A second, real MongoDB session with a seeded "orders" collection.
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('Chart Mongo');
    await dialog.getByLabel('Database engine', { exact: true }).selectOption('mongodb');
    await dialog.getByLabel('Host', { exact: true }).fill(host);
    await dialog.getByLabel('Port', { exact: true }).fill(port);
    await dialog.getByLabel('Database', { exact: true }).fill(database);
    await dialog.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(dialog.locator('.inline-note').last()).toContainText('Connected successfully');
    await dialog.getByRole('button', { name: 'Save profile', exact: true }).click();
    await dialog.getByRole('button', { name: 'Open Chart Mongo', exact: true }).click();
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(2);

    // Switch back to the SQLite session's chart document and add the Mongo
    // collection as a second, independently-resolved series.
    await page.locator('.session-tabs .session-tab').first().click();
    await expect(chart.locator('.chart-document')).toBeVisible();
    await chart.getByRole('button', { name: 'Add series', exact: true }).click();
    const addSeries = page.getByRole('dialog', { name: 'Add series' });
    await addSeries.getByLabel('Session', { exact: true }).selectOption({ label: 'Chart Mongo' });
    await addSeries.getByRole('button', { name: 'Add orders as a series', exact: true }).click();

    await expect(chart.locator('.chart-series-tab')).toHaveCount(2);
    await expect(chart.locator('.chart-series-tab').last()).toContainText('Chart Mongo');
    await expect(chart.locator('.chart-mode')).not.toContainText('failed');
    await expect(chart.locator('.chart-canvas canvas')).toBeVisible();
});
