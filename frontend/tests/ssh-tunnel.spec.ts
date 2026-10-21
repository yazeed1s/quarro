import { expect, test } from '@playwright/test';

// Proves the SSH tunnel connection form end to end against a real jump host
// and a real PostgreSQL target reachable only through it (the same
// disposable containers internal/database/postgres's own
// TestSSHTunnelContract uses). Skipped unless scripts/test-engine.sh ssh has
// set QUARRO_TEST_SSH_HOST. The host-key-mismatch-rejected path is already
// proven at the Go level by that contract test; this scenario only needs to
// confirm the UI surfaces trust-on-first-use correctly, not re-prove the
// backend's own rejection logic.
test('SSH tunnel: trust a new host key, save, and reconnect without re-prompting', async ({ page }) => {
    test.skip(!process.env.QUARRO_TEST_SSH_HOST, 'Run scripts/test-engine.sh ssh');
    const sshHost = process.env.QUARRO_TEST_SSH_HOST!;
    const sshPort = process.env.QUARRO_TEST_SSH_PORT!;
    const sshUser = process.env.QUARRO_TEST_SSH_USER!;
    const sshPassword = process.env.QUARRO_TEST_SSH_PASSWORD!;
    const targetHost = process.env.QUARRO_TEST_SSH_TARGET_HOST!;
    const targetPort = process.env.QUARRO_TEST_SSH_TARGET_PORT!;
    const dbUser = process.env.QUARRO_TEST_SSH_DB_USER!;
    const dbPassword = process.env.QUARRO_TEST_SSH_DB_PASSWORD!;
    const dbName = process.env.QUARRO_TEST_SSH_DB_NAME!;

    await page.goto('/');
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'New connection', exact: true }).click();
    await dialog.getByLabel('Connection name', { exact: true }).fill('SSH Tunnel Test');
    await dialog
        .getByRole('group', { name: 'Database engine' })
        .getByRole('button', { name: /PostgreSQL/ })
        .click();
    await dialog.getByLabel('Host', { exact: true }).fill(targetHost);
    await dialog.getByLabel('Port', { exact: true }).fill(targetPort);
    await dialog.getByLabel('Username', { exact: true }).fill(dbUser);
    await dialog.getByLabel('Database', { exact: true }).fill(dbName);
    await dialog.getByLabel('Password', { exact: true }).fill(dbPassword);
    // The disposable target container has no TLS configured.
    await dialog.getByRole('button', { name: 'TLS mode', exact: true }).click();
    await dialog.getByRole('option', { name: 'Disabled (unencrypted)', exact: true }).click();

    const sshSection = dialog.locator('.connection-ssh');
    await sshSection.locator('summary').click();
    await sshSection.getByRole('button', { name: 'Connect through an SSH tunnel', exact: true }).click();
    await dialog.getByLabel('SSH host', { exact: true }).fill(sshHost);
    await dialog.getByLabel('SSH port', { exact: true }).fill(sshPort);
    await dialog.getByLabel('SSH username', { exact: true }).fill(sshUser);
    await dialog.getByLabel('SSH password', { exact: true }).fill(sshPassword);

    await dialog.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(dialog.locator('.inline-note').filter({ hasText: 'Connected successfully' })).toBeVisible();

    const trustNote = sshSection.locator('.inline-note').filter({ hasText: 'New SSH host key' });
    await expect(trustNote).toBeVisible();
    await trustNote.getByRole('button', { name: 'Trust this key', exact: true }).click();
    await expect(sshSection.locator('.inline-note').filter({ hasText: 'Trusted host key' })).toBeVisible();

    await dialog.getByRole('button', { name: 'Save profile', exact: true }).click();
    await dialog.getByRole('button', { name: 'Open SSH Tunnel Test', exact: true }).click();
    // The first run already auto-opened a session against the seeded SQLite
    // demo database; this is the second.
    await expect(page.locator('.session-tabs .session-tab')).toHaveCount(2);

    // Reopen the saved profile and test again: the host is already
    // trusted, so no new-key prompt should appear a second time.
    await page.getByRole('button', { name: 'New session', exact: true }).click();
    await dialog.locator('summary[aria-label="Options for SSH Tunnel Test"]').click();
    await dialog.getByRole('button', { name: 'Edit profile', exact: true }).click();
    await dialog.getByLabel('SSH password', { exact: true }).fill(sshPassword);
    await dialog.getByRole('button', { name: 'Test connection', exact: true }).click();
    await expect(dialog.locator('.inline-note').filter({ hasText: 'Connected successfully' })).toBeVisible();
    await expect(sshSection.locator('.inline-note').filter({ hasText: 'New SSH host key' })).not.toBeVisible();
    await expect(sshSection.locator('.inline-note').filter({ hasText: 'Trusted host key' })).toBeVisible();
});
