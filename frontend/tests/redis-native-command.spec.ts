import { expect, test } from '@playwright/test';

// Exercises the Redis command editor against a disposable Redis seeded by
// seed_redis_types: completion from the server's own command list, a
// command reply in the normal result grid, saved queries, history, and
// loaded-row export, several selected lines as several result tabs, and a
// refused connection-state command. The redis package's TestExecuteNative
// covers the full refusal table and read-only write detection.
test('Redis command editor runs commands through the normal query workflow', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_REDIS_URL, 'Run scripts/test-engine.sh redis-native-command');
    const [host, port] = process.env.QUARRO_TEST_REDIS_URL!.split(':');

    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('Command Redis');
    await dialog.getByRole('group', { name: 'Database engine' }).getByRole('button', { name: /Redis/ }).click();
    await dialog.getByLabel('Host', { exact: true }).fill(host);
    await dialog.getByLabel('Port', { exact: true }).fill(port);
    await dialog.getByLabel('Database index', { exact: true }).fill('0');
    await dialog.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(dialog.locator('.inline-note').last()).toContainText('Connected successfully');
    await dialog.getByRole('button', { name: 'Save profile', exact: true }).click();
    await dialog.getByRole('button', { name: 'Open Command Redis', exact: true }).click();

    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    const editor = doc.getByLabel('Redis command editor');
    await expect(editor).toBeVisible();
    await expect(doc.locator('.editor-caption')).toContainText('Redis command');

    // Completion comes from the server's COMMAND LIST.
    await editor.click();
    await editor.pressSequentially('hgeta');
    await expect(page.locator('.cm-tooltip-autocomplete')).toContainText('HGETALL');
    await page.keyboard.press('Escape');

    await editor.fill('HGETALL user:1');
    await doc.getByRole('button', { name: 'Run query', exact: true }).click();
    await expect(doc.locator('.data-grid')).toBeVisible();
    await expect(doc.locator('tbody')).toContainText('name');
    await expect(doc.locator('tbody')).toContainText('Ada');
    await expect(doc.locator('tbody')).toContainText('plan');

    await doc.getByRole('button', { name: 'Save query', exact: true }).click();
    await page.getByRole('button', { name: /Saved queries/ }).click();
    await expect(page.getByLabel('Rename Query 1')).toHaveValue('Query 1');

    await doc.getByRole('button', { name: 'Export results', exact: true }).click();
    const transfer = doc.getByLabel('Import and export');
    await expect(transfer.getByRole('button', { name: 'Scope' })).toContainText('Loaded rows');
    const download = page.waitForEvent('download');
    await transfer.getByRole('button', { name: 'Export', exact: true }).click();
    await expect(await download).toBeTruthy();
    await transfer.getByRole('button', { name: 'Close data transfer panel', exact: true }).click();

    // Two selected lines run as two statements; SELECT is refused (it would
    // move one pooled connection to another database) without running.
    await editor.fill('LLEN queue\nSELECT 3');
    await editor.press('ControlOrMeta+a');
    await doc.getByRole('button', { name: 'Run query', exact: true }).click();
    const tabs = doc.locator('.result-set-tabs button');
    await expect(tabs).toHaveCount(2);
    await expect(doc.locator('tbody')).toContainText('150');
    await tabs.nth(1).click();
    await expect(doc.locator('.empty-state')).toContainText('database selector');

    await page.getByRole('button', { name: /Query history/ }).click();
    await expect(page.locator('.session-host:not([hidden]) .object-browser')).toContainText('Query 1');
});
