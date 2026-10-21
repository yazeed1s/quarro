import { defineConfig } from '@playwright/test';

export default defineConfig({
    testDir: './tests',
    testMatch: [
        '**/workspace.spec.ts',
        '**/phase6.spec.ts',
        '**/chart-multisource.spec.ts',
        '**/mongo-explain.spec.ts',
        '**/mongo-json-tree.spec.ts',
        '**/mongo-document-edit.spec.ts',
        '**/mongo-native-query.spec.ts',
        '**/redis-inspect.spec.ts',
        '**/redis-key-edit.spec.ts',
        '**/redis-native-command.spec.ts',
        '**/ssh-tunnel.spec.ts',
    ],
    fullyParallel: false,
    workers: 2,
    use: {
        baseURL: 'http://127.0.0.1:34115',
        viewport: { width: 1440, height: 900 },
        launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE } : {},
    },
    // Runs the real Go backend in Wails v3 "server mode" (-tags server; a
    // pure HTTP/WebSocket server, no GUI) with an isolated demo database, so
    // tests exercise real query execution rather than a plain static preview.
    webServer: {
        command: 'npm run test:server',
        url: 'http://127.0.0.1:34115',
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
    },
});
