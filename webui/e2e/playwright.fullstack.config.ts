import { defineConfig } from '@playwright/test';

const baseURL = process.env.CARRIER_E2E_BASE_URL || 'http://127.0.0.1:8787';

export default defineConfig({
  testDir: './tests',
  testMatch: ['fullstack-*.spec.ts'],
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? 'github' : 'list',
  outputDir: './test-results-fullstack',
  use: {
    baseURL,
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
});
