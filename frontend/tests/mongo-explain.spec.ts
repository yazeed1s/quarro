import { expect, test } from '@playwright/test';

// Proves provider.Explainer generalizes past SQL: a MongoDB collection gets
// the same Graph View rendering code a SQL query document does, through a
// different entry point (the table document's own "Explain" action, since
// MongoDB has no SQL query document to fork from). Skipped unless
// scripts/test-engine.sh's mongo-explain mode has set QUARRO_TEST_MONGO_URL
// against a real disposable MongoDB seeded with an unindexed "orders"
// collection.
test('explain from a MongoDB collection renders the same Graph View as a SQL session', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_MONGO_URL, 'Run scripts/test-engine.sh mongo-explain');
    const [host, port] = process.env.QUARRO_TEST_MONGO_URL!.split(':');
    const database = process.env.QUARRO_TEST_MONGO_DB ?? 'quarro_explain';

    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('Explain Mongo');
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
    await dialog.getByRole('button', { name: 'Open Explain Mongo', exact: true }).click();
    // The first run already auto-opened a session against the seeded SQLite
    // demo database; this is the second.
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(2);

    const browser = page.locator('.session-host:not([hidden]) .object-browser');
    await browser
        .locator('button.tree-object')
        .filter({ has: page.getByText('orders', { exact: true }) })
        .click();
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(doc.locator('.data-grid')).toBeVisible();

    const explainButton = doc.getByRole('button', { name: 'Explain collection', exact: true });
    await expect(explainButton).toBeVisible();
    await explainButton.click();

    const explainDoc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    const scanNode = explainDoc.locator('.explain-node').filter({ hasText: 'COLLSCAN' });
    await expect(scanNode).toBeVisible();
    // internal/database/explain's seqScan badge fires for MongoDB's own
    // COLLSCAN the same way it does for Postgres's Seq Scan, SQLite's SCAN,
    // and MySQL's ALL: one shared badge vocabulary, no fictitious Postgres-
    // shaped badge invented for a document store.
    await expect(scanNode.locator('.explain-badge')).toContainText('seqScan');

    // The same detail panel and view switcher a SQL session's Explain
    // document already uses, unmodified for MongoDB.
    await scanNode.click();
    await expect(explainDoc.locator('.explain-detail')).toBeVisible();
    await expect(explainDoc.locator('.explain-detail-badge')).toContainText('Sequential scan');

    await explainDoc.getByRole('group', { name: 'Explain view' }).getByRole('button', { name: 'Raw', exact: true }).click();
    await expect(explainDoc.locator('.explain-raw-pre')).toContainText('COLLSCAN');
});
