import { expect, test } from '@playwright/test';

// Confirms the nested JSON viewer fix: before it, display()'s bare
// String(value) fallback rendered a nested sub-document as "[object Object]"
// and an array as a comma-joined list, in both the grid cell and the row
// inspector's Fields mode. Skipped unless scripts/test-engine.sh's
// mongo-json-tree mode has set QUARRO_TEST_MONGO_URL against a real
// disposable MongoDB seeded with a document carrying a nested sub-document
// and an array field.
test('nested JSON fields render as an explorable tree, not [object Object]', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_MONGO_URL, 'Run scripts/test-engine.sh mongo-json-tree');
    const [host, port] = process.env.QUARRO_TEST_MONGO_URL!.split(':');
    const database = process.env.QUARRO_TEST_MONGO_DB ?? 'quarro_json_tree';

    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('JSON Tree Mongo');
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
    await dialog.getByRole('button', { name: 'Open JSON Tree Mongo', exact: true }).click();
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(2);

    const browser = page.locator('.session-host:not([hidden]) .object-browser');
    await browser
        .locator('button.tree-object')
        .filter({ has: page.getByText('profiles', { exact: true }) })
        .click();
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(doc.locator('.data-grid')).toBeVisible();

    // The grid cell itself: a nested sub-document and an array must show a
    // legible summary, never the bare String(value) fallback.
    await expect(doc.locator('tbody')).toContainText('{2 fields}');
    await expect(doc.locator('tbody')).toContainText('[2 items]');
    await expect(doc.locator('tbody')).not.toContainText('[object Object]');
    await expect(doc.locator('tbody')).not.toContainText('engineer,writer');

    // The row inspector's Fields mode: the nested value is an actual
    // expandable tree, not a second flat text dump.
    await doc.locator('.cell').filter({ hasText: '{2 fields}' }).first().dblclick();
    const inspector = doc.locator('.row-inspector');
    await expect(inspector).toBeVisible();
    const addressField = inspector.locator('.inspector-field').filter({ has: page.getByText('address', { exact: true }) });
    await expect(addressField).toBeVisible();
    const addressNode = addressField.locator('.json-node');
    await expect(addressNode).toHaveAttribute('open', '');
    await expect(addressNode).toContainText('city');
    await expect(addressNode).toContainText('London');
});
