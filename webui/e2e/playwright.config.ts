import { defineConfig } from '@playwright/test';

const baseURL = 'http://127.0.0.1:19191';

export default defineConfig({
  testDir: './tests',
  testIgnore: ['fullstack-*.spec.ts'],
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? 'github' : 'list',
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
  webServer: {
    command: 'bunx serve -l 19191 -s ../static',
    port: 19191,
    reuseExistingServer: !process.env.CI,
  },
});
