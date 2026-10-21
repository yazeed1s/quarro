import { expect, test } from '@playwright/test';

// Exercises the Redis key list and key inspector against a real disposable
// Redis: a keyspace group lists only its own keys (not a lookalike
// "username"), every type opens into its own view, a list longer than one
// page loads the rest on demand instead of being silently capped, and a
// non-UTF-8 value shows as binary. Skipped unless scripts/test-engine.sh's
// redis-inspect mode has set QUARRO_TEST_REDIS_URL and run
// seed_redis_types against database 0.
test('Redis key inspector shows every type, paged', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_REDIS_URL, 'Run scripts/test-engine.sh redis-inspect');
    const [host, port] = process.env.QUARRO_TEST_REDIS_URL!.split(':');

    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('Inspect Redis');
    await dialog
        .getByRole('group', { name: 'Database engine' })
        .getByRole('button', { name: /Redis/ })
        .click();
    await dialog.getByLabel('Host', { exact: true }).fill(host);
    await dialog.getByLabel('Port', { exact: true }).fill(port);
    await dialog.getByLabel('Database index', { exact: true }).fill('0');
    await dialog.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(dialog.locator('.inline-note').last()).toContainText('Connected successfully');
    await dialog.getByRole('button', { name: 'Save profile', exact: true }).click();
    await dialog.getByRole('button', { name: 'Open Inspect Redis', exact: true }).click();
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(2);

    const browser = page.locator('.session-host:not([hidden]) .object-browser');
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    const keys = doc.getByRole('table', { name: 'Keys' });
    const inspector = doc.getByRole('complementary', { name: 'Key details' });
    const openGroup = async (name: string) => {
        await browser
            .locator('button.tree-object')
            .filter({ has: page.getByText(name, { exact: true }) })
            .click();
        await expect(keys).toBeVisible();
    };
    const openKey = async (name: string) => {
        await keys.getByRole('button', { name, exact: true }).click();
        await expect(inspector.locator('h3')).toHaveText(name);
    };

    // The "user" group is user:1 and user:2, never the unrelated "username".
    await openGroup('user');
    await expect(keys.locator('tbody tr')).toHaveCount(2);
    await expect(keys).toContainText('user:1');
    await expect(keys).not.toContainText('username');
    await expect(keys).toContainText('Hash');
    await expect(keys).toContainText('2 fields');

    await openKey('user:1');
    await expect(inspector).toContainText('Field');
    await expect(inspector.locator('tbody')).toContainText('Ada');
    await expect(inspector.locator('tbody')).toContainText('pro');

    // A 150-element list loads one 100-element page, then the rest.
    await openGroup('queue');
    await openKey('queue');
    await expect(inspector).toContainText('Loaded 100 of 150');
    await inspector.getByRole('button', { name: 'Load more', exact: true }).click();
    await expect(inspector).toContainText('Loaded 150 of 150');
    await expect(inspector.locator('tbody')).toContainText('item-149');
    await expect(inspector.getByRole('button', { name: 'Load more', exact: true })).toHaveCount(0);

    // A JSON string is pretty-printed, and its TTL is shown.
    await openGroup('greeting');
    await expect(keys).toContainText(/\d+m/);
    await openKey('greeting');
    await expect(inspector.locator('.key-string')).toContainText('"hello": "world"');
    await expect(inspector.locator('header small')).toContainText(/String · 23 bytes · \d+m/);

    await openGroup('scores');
    await openKey('scores');
    await expect(inspector).toContainText('Score');
    await expect(inspector.locator('tbody tr').first()).toContainText('1.5');

    await openGroup('events');
    await openKey('events');
    await expect(inspector).toContainText('1-1');
    await expect(inspector).toContainText('signup');
    await expect(inspector.getByRole('table', { name: 'Consumer groups' })).toContainText('workers');

    await openGroup('blob');
    await openKey('blob');
    await expect(inspector).toContainText('Binary value');
    await expect(inspector.locator('.key-string')).toHaveText('ff fe 00');
});
