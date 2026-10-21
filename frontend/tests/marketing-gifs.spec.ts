import { expect, test, type Page } from '@playwright/test';

// Short screen recordings for the marketing site (website/public/gifs/).
// Run against the same real backend as marketing-shots.spec.ts:
//   npm run test:server   (in one terminal)
//   npm run test:marketing -- tests/marketing-gifs.spec.ts
// Each test records a .webm to test-results/; convert to a trimmed, looping
// GIF with ffmpeg, e.g.:
//   ffmpeg -ss 2.2 -i input.webm -vf "fps=14,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse" -loop 0 output.gif

test.use({
    viewport: { width: 1440, height: 900 },
    video: { mode: 'on', size: { width: 1440, height: 900 } },
});

async function prepareTheme(page: Page) {
    await page.getByRole('button', { name: 'Settings', exact: true }).click();
    await page.getByRole('button', { name: 'Monochrome', exact: true }).click();
    await page.getByLabel('Application zoom', { exact: true }).fill('110');
    await page.getByLabel('Application zoom', { exact: true }).blur();
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
}

// "Run query" resolves the statement at the cursor, whose position can race
// freshly-typed text; "Run All" always runs the whole editor and isn't
// sensitive to that race (see marketing-shots.spec.ts).
async function runAll(doc: ReturnType<Page['locator']>) {
    const runAllBtn = doc.getByRole('button', { name: 'Run All', exact: true });
    const wasOpen = await runAllBtn.isVisible();
    if (!wasOpen) await doc.locator('.run-group summary').click();
    await doc.getByRole('button', { name: 'Run All', exact: true }).click();
    if (!wasOpen) await doc.locator('.run-group summary').click();
}

test('gif-1-type-and-run', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await doc.locator('.cm-content').click();
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Backspace');
    await page.waitForTimeout(200);
    await page.keyboard.type('SELECT u.name, u.email, o.status, o.total\nFROM users u\nJOIN orders o ON o.user_id = u.id\nORDER BY o.created_at DESC\nLIMIT 20;', { delay: 18 });
    await page.waitForTimeout(300);
    await runAll(doc);
    await expect(doc.locator('.data-grid')).toBeVisible();
    await page.waitForTimeout(400);
    await expect(doc.getByRole('cell', { name: 'Olivia Rhye' }).first()).toBeVisible();
    await page.waitForTimeout(1200);
});

test('gif-2-command-palette', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page.keyboard.press('Control+k');
    await expect(page.locator('.palette')).toBeVisible();
    await page.waitForTimeout(400);
    await page.keyboard.type('diagram', { delay: 55 });
    await page.waitForTimeout(500);
    await page.keyboard.press('Enter');
    await expect(page.locator('.diagram-canvas')).toBeVisible();
    await page.waitForTimeout(1000);
});

test('gif-3-visual-query-builder', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page.locator('.document-bar details.dropdown summary').click();
    await page.getByRole('button', { name: 'New visual query', exact: true }).click();
    await page.locator('.document-bar details.dropdown summary').click();
    const doc = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(doc.locator('.visual-query-builder')).toBeVisible();
    await page.waitForTimeout(300);

    const addTable = doc.getByLabel('Add a table to the query', { exact: true });
    await addTable.pressSequentially('orders', { delay: 60 });
    await page.waitForTimeout(200);
    await doc.getByRole('option', { name: 'orders', exact: true }).click();
    await page.waitForTimeout(300);
    await addTable.pressSequentially('users', { delay: 60 });
    await page.waitForTimeout(200);
    await doc.getByRole('option', { name: 'users', exact: true }).click();
    await page.waitForTimeout(400);

    const ordersNode = doc.locator('.query-node').filter({ hasText: 'orders' });
    const usersNode = doc.locator('.query-node').filter({ hasText: 'users' });
    await ordersNode.getByRole('button', { name: 'Join to…', exact: true }).click();
    const joinPanel = doc.locator('.join-inspector');
    await page.waitForTimeout(300);
    await joinPanel.locator('.inspector-field').filter({ hasText: 'Left column' }).locator('select').selectOption('user_id');
    await page.waitForTimeout(250);
    await joinPanel.locator('.inspector-field').filter({ hasText: 'Right column' }).locator('select').selectOption('id');
    await page.waitForTimeout(250);
    await joinPanel.getByRole('button', { name: 'Add join', exact: true }).click();
    await page.waitForTimeout(300);

    for (const name of ['status', 'total']) {
        await ordersNode.locator('.query-column').filter({ hasText: name }).locator('input').check();
        await page.waitForTimeout(250);
    }
    for (const name of ['name', 'email']) {
        await usersNode.locator('.query-column').filter({ hasText: name }).locator('input').check();
        await page.waitForTimeout(250);
    }
    await page.waitForTimeout(1000);
});

test('gif-4-chart-live', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page.locator('.object-browser').getByRole('button', { name: 'Visualize orders', exact: true }).click();
    const chart = page.locator('.session-host:not([hidden]) .document-host:not([hidden])');
    await expect(chart.locator('.chart-canvas canvas')).toBeVisible();
    await chart.getByLabel('X field', { exact: true }).selectOption('status');
    await chart.getByLabel('Y field', { exact: true }).selectOption('id');
    await chart.getByLabel('Aggregate', { exact: true }).selectOption('count');
    await page.waitForTimeout(900);
    await chart.getByLabel('Chart type', { exact: true }).selectOption('pie');
    await page.waitForTimeout(1100);
    await chart.getByLabel('Chart type', { exact: true }).selectOption('bar');
    await page.waitForTimeout(900);
});

test('gif-5-diagram-related', async ({ page }) => {
    await page.goto('/');
    await prepareTheme(page);
    await page
        .locator('.object-browser')
        .getByRole('button', { name: /^Schema diagram\b/ })
        .click();
    const diagram = page.locator('.session-host:not([hidden]) .diagram-canvas');
    await expect(diagram).toBeVisible();
    await page.waitForTimeout(2200);
    const ordersNode = page.locator('.schema-node').filter({ hasText: 'orders' });
    await ordersNode.getByRole('button', { name: 'Related', exact: true }).click();
    await page.waitForTimeout(2200);
});
