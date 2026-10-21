import { expect, test } from '@playwright/test';

// Confirms MongoDB document editing end to end: insert a new document,
// edit an existing one (including a nested field, through the Extended
// JSON round trip ApplyDocumentEdit/FetchDocumentJSON use), and delete one
// — all through the real UI, against a real disposable MongoDB. Skipped
// unless scripts/test-engine.sh's mongo-document-edit mode has set
// QUARRO_TEST_MONGO_URL against a database seeded with one "profiles"
// document (name: Ada, with a nested address sub-document and a tags
// array).
test('insert, edit, and delete a MongoDB document through the UI', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_MONGO_URL, 'Run scripts/test-engine.sh mongo-document-edit');
    const [host, port] = process.env.QUARRO_TEST_MONGO_URL!.split(':');
    const database = process.env.QUARRO_TEST_MONGO_DB ?? 'quarro_document_edit';

    page.on('dialog', (dialog) => dialog.accept());

    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('Document Edit Mongo');
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
    await dialog.getByRole('button', { name: 'Open Document Edit Mongo', exact: true }).click();
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(2);

    const browser = page.locator('.session-host:not([hidden]) .object-browser');
    await browser
        .locator('button.tree-object')
        .filter({ has: page.getByText('profiles', { exact: true }) })
        .click();
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(doc.locator('.data-grid')).toBeVisible();
    await expect(doc.locator('tbody')).toContainText('Ada');

    // Insert.
    await doc.getByRole('button', { name: 'Insert document', exact: true }).click();
    const insertModal = page.getByRole('dialog', { name: 'Insert document' });
    await expect(insertModal).toBeVisible();
    await insertModal.getByLabel('Document JSON').fill('{"name": "Grace", "role": "admiral"}');
    await insertModal.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(insertModal).not.toBeVisible();
    await expect(doc.locator('tbody')).toContainText('Grace');

    // Edit: select the Ada row, open Edit, confirm the nested field loaded,
    // change a top-level field, apply.
    await doc.locator('.cell').filter({ hasText: 'Ada' }).first().click();
    await doc.getByRole('button', { name: 'Toggle row inspector', exact: true }).click();
    await doc.getByRole('tab', { name: 'Edit' }).click();
    const editor = doc.locator('.document-editor-text');
    await expect(editor).toBeVisible();
    await expect(editor).toHaveValue(/London/);
    await editor.fill(await editor.inputValue().then((v) => v.replace('Ada', 'Ada Lovelace')));
    await doc.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(doc.locator('tbody')).toContainText('Ada Lovelace');

    // Delete: select the Grace row (no nested fields to worry about) and
    // delete it; the confirm() dialog is auto-accepted above.
    await doc.locator('.cell').filter({ hasText: 'Grace' }).first().click();
    await doc.getByRole('button', { name: 'Delete document', exact: true }).click();
    await expect(doc.locator('tbody')).not.toContainText('Grace');
});
