import { expect, test } from '@playwright/test';

// Exercises the complete native-query route against a disposable MongoDB:
// collection selection + CodeMirror JSON pipeline -> cancellable backend
// job -> the ordinary query result grid, saved-query/history, and loaded-row
// export paths. The provider contract test separately proves write-stage
// rejection happens before MongoDB sees the pipeline.
test('native MongoDB aggregation pipeline uses the normal query result workflow', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_MONGO_URL, 'Run scripts/test-engine.sh mongo-native-query');
    const [host, port] = process.env.QUARRO_TEST_MONGO_URL!.split(':');
    const database = process.env.QUARRO_TEST_MONGO_DB ?? 'quarro_native_query';

    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('Native Query Mongo');
    await dialog
        .getByRole('group', { name: 'Database engine' })
        .getByRole('button', { name: /MongoDB/ })
        .click();
    await dialog.getByLabel('Host', { exact: true }).fill(host);
    await dialog.getByLabel('Port', { exact: true }).fill(port);
    await dialog.getByLabel('Database', { exact: true }).fill(database);
    await dialog.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(dialog.locator('.inline-note').last()).toContainText('Connected successfully');
    await dialog.getByRole('button', { name: 'Save profile', exact: true }).click();
    await dialog.getByRole('button', { name: 'Open Native Query Mongo', exact: true }).click();

    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    const editor = doc.getByLabel('Aggregation pipeline editor');
    await expect(editor).toBeVisible();
    await expect(doc.getByRole('button', { name: 'Choose a collection' })).toContainText('orders');
    await editor.fill('[{"$match":{"status":"active"}},{"$group":{"_id":"$status","sum":{"$sum":"$total"}}},{"$sort":{"sum":-1}},{"$limit":1}]');
    await doc.getByRole('button', { name: 'Run query', exact: true }).click();

    await expect(doc.locator('.data-grid')).toBeVisible();
    await expect(doc.locator('tbody')).toContainText('active');
    await expect(doc.locator('tbody')).toContainText('15');

    await doc.getByRole('button', { name: 'Save query', exact: true }).click();
    await page.getByRole('button', { name: /Saved queries/ }).click();
    await expect(page.getByLabel('Rename Query 1')).toHaveValue('Query 1');

    await doc.getByRole('button', { name: 'Export results', exact: true }).click();
    const transfer = doc.getByLabel('Import and export');
    await expect(transfer.getByRole('button', { name: 'Scope' })).toContainText('Loaded rows');
    const download = page.waitForEvent('download');
    await transfer.getByRole('button', { name: 'Export', exact: true }).click();
    await expect(await download).toBeTruthy();

    await page.getByRole('button', { name: /Query history/ }).click();
    await expect(page.locator('.session-host:not([hidden]) .object-browser')).toContainText('Query 1');
});
