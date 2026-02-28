import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken } from './helpers';

test.describe('SSE and Polling', () => {
  test('receives logs via SSE stream', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/logs');

    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    const logOutput = page.locator('#log-output');
    await expect(logOutput).toContainText('agent started');
    await expect(logOutput).toContainText('worker heartbeat');
  });

  test('falls back to polling when SSE fails', async ({ page }) => {
    // Mock APIs but override SSE to fail immediately
    await mockAPIs(page);
    await page.route('**/api/v1/logs/stream*', (route) => route.abort('connectionrefused'));

    await loginWithToken(page, '/#/logs');
    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    const logOutput = page.locator('#log-output');
    // Should show SSE disconnect message and then poll results
    await expect(logOutput).toContainText('agent started', { timeout: 5000 });
  });

  test('polling mode continues to fetch logs', async ({ page }) => {
    let pollCount = 0;
    await mockAPIs(page);
    // Force SSE failure
    await page.route('**/api/v1/logs/stream*', (route) => route.abort('connectionrefused'));
    // Track polling calls
    await page.route('**/api/v1/agents/*/logs', (route) => {
      pollCount++;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ lines: [`[INFO] poll result ${pollCount}`] }),
      });
    });

    await loginWithToken(page, '/#/logs');
    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    // Wait for at least 2 poll cycles (2s interval)
    await page.waitForTimeout(5000);
    expect(pollCount).toBeGreaterThanOrEqual(2);
  });
});
