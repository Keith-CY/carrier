import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken } from './helpers';

test.describe('Logs', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/logs');
  });

  test('shows agent selector and Connect button', async ({ page }) => {
    await expect(page.locator('#log-agent')).toBeVisible();
    await expect(page.locator('#log-connect')).toBeVisible();
  });

  test('Connect shows log output', async ({ page }) => {
    // Wait for agent dropdown to be populated
    await expect(page.locator('#log-agent option')).not.toHaveCount(0);

    await page.click('#log-connect');

    // Log output should have content (from SSE mock or poll fallback)
    const logOutput = page.locator('#log-output');
    await expect(logOutput).not.toBeEmpty();
  });

  test('Clear button empties log output', async ({ page }) => {
    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    const logOutput = page.locator('#log-output');
    await expect(logOutput).not.toBeEmpty();

    await page.click('#log-clear');
    await expect(logOutput).toBeEmpty();
  });
});
