import { expect, test } from '@playwright/test';

// Exercises Redis key editing through the real UI against a disposable
// Redis seeded by seed_redis_types: edit a string (keeping its TTL), a
// hash field, a list element, and a sorted-set score; add and remove a set
// member; set and remove an expiry; create and delete a key. Every change is confirmed in the view
// the inspector re-reads after applying. Conflict handling is covered by
// the redis package's TestApplyKeyEdit contract test, which can change a
// key between read and apply. Skipped unless scripts/test-engine.sh's
// redis-key-edit mode has set QUARRO_TEST_REDIS_URL.
test('Redis key editing changes every editable type', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_REDIS_URL, 'Run scripts/test-engine.sh redis-key-edit');
    const [host, port] = process.env.QUARRO_TEST_REDIS_URL!.split(':');
    page.on('dialog', (dialog) => dialog.accept());

    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('Edit Redis');
    await dialog.getByRole('group', { name: 'Database engine' }).getByRole('button', { name: /Redis/ }).click();
    await dialog.getByLabel('Host', { exact: true }).fill(host);
    await dialog.getByLabel('Port', { exact: true }).fill(port);
    await dialog.getByLabel('Database index', { exact: true }).fill('0');
    await dialog.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(dialog.locator('.inline-note').last()).toContainText('Connected successfully');
    await dialog.getByRole('button', { name: 'Save profile', exact: true }).click();
    await dialog.getByRole('button', { name: 'Open Edit Redis', exact: true }).click();
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(2);

    const browser = page.locator('.session-host:not([hidden]) .object-browser');
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    const keys = doc.getByRole('table', { name: 'Keys' });
    const inspector = doc.getByRole('complementary', { name: 'Key details' });
    const rows = inspector.locator('.key-member-table tbody tr');
    const openKey = async (group: string, name: string) => {
        await browser
            .locator('button.tree-object')
            .filter({ has: page.getByText(group, { exact: true }) })
            .click();
        await keys.getByRole('button', { name, exact: true }).click();
        await expect(inspector.locator('h3')).toHaveText(name);
        await inspector.getByRole('tab', { name: 'Edit', exact: true }).click();
    };
    const editRow = async (match: string, fill: Record<string, string>) => {
        await rows.filter({ hasText: match }).getByRole('button', { name: 'Edit', exact: true }).click();
        for (const [label, value] of Object.entries(fill)) await rows.getByLabel(label, { exact: true }).fill(value);
        await rows.getByRole('button', { name: 'Save', exact: true }).click();
    };

    // String: the new value is saved and the TTL survives the write.
    await openKey('greeting', 'greeting');
    await inspector.getByLabel('String value', { exact: true }).fill('{"hello":"redis"}');
    await inspector.getByRole('button', { name: 'Apply', exact: true }).click();
    await inspector.getByRole('tab', { name: 'Inspect', exact: true }).click();
    await expect(inspector.locator('.key-string')).toContainText('"hello": "redis"');
    await expect(inspector.locator('header small')).toContainText(/String · 17 bytes · \d+m/);

    // Hash field value.
    await openKey('user', 'user:1');
    await editRow('name', { Value: 'Ada Lovelace' });
    await expect(rows.filter({ hasText: 'name' })).toContainText('Ada Lovelace');

    // Expiry: set one on a key that had none, then remove it.
    const ttl = inspector.locator('.key-ttl summary');
    await expect(ttl).toHaveText('No expiry');
    await ttl.click();
    const ttlForm = inspector.getByRole('form', { name: 'Expiry' });
    await ttlForm.getByLabel('Expires in', { exact: true }).fill('2h');
    await ttlForm.getByRole('button', { name: 'Set expiry', exact: true }).click();
    await expect(ttl).toHaveText(/^(1h 59m|2h)$/);
    await expect(keys.locator('tr', { hasText: 'user:1' })).toContainText(/1h 59m|2h/);
    await ttl.click();
    await ttlForm.getByRole('button', { name: 'Remove expiry', exact: true }).click();
    await expect(ttl).toHaveText('No expiry');

    // List element by index.
    await openKey('queue', 'queue');
    await editRow('item-000', { Value: 'first' });
    await expect(rows.first()).toContainText('first');
    await expect(inspector).toContainText('Loaded 100 of 150');

    // Sorted-set score.
    await openKey('scores', 'scores');
    await editRow('ada', { Score: '9' });
    await expect(rows.filter({ hasText: 'ada' })).toContainText('9');
    await expect(rows.last()).toContainText('ada');

    // Set: add one member, remove another.
    await openKey('tags', 'tags');
    await inspector.getByLabel('New member', { exact: true }).fill('fresh');
    await inspector.getByRole('button', { name: 'Add', exact: true }).click();
    await expect(rows.filter({ hasText: 'fresh' })).toHaveCount(1);
    await rows
        .filter({ has: page.getByText('new', { exact: true }) })
        .getByRole('button', { name: 'Remove', exact: true })
        .click();
    await expect(rows.filter({ has: page.getByText('new', { exact: true }) })).toHaveCount(0);
    await expect(rows).toHaveCount(2);

    // Create a key inside the browsed group, then delete it.
    await doc.getByRole('button', { name: 'New key', exact: true }).click();
    const create = page.getByRole('dialog', { name: 'New key' });
    await create.getByLabel('Key name', { exact: true }).fill('tags:extra');
    await create.getByLabel('Value', { exact: true }).fill('hello');
    await create.getByRole('button', { name: 'Create key', exact: true }).click();
    await expect(create).toHaveCount(0);
    await expect(inspector.locator('h3')).toHaveText('tags:extra');
    // The inspector stays in Edit mode across keys, like the row inspector.
    await expect(inspector.getByLabel('String value', { exact: true })).toHaveValue('hello');
    await expect(keys).toContainText('tags:extra');

    await inspector.getByRole('button', { name: 'Delete key', exact: true }).click();
    await expect(keys).not.toContainText('tags:extra');
    await expect(inspector.locator('h3')).toHaveText('No key selected');
});
