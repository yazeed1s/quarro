import { defineConfig } from '@playwright/test';
import browserConfig from './playwright.config';

export default defineConfig({
    ...browserConfig,
    testMatch: '**/marketing-*.spec.ts',
    workers: 1,
});
