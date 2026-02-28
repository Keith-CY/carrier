import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken } from './helpers';

test.describe('Logs', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/logs');
  });

  test('shows structured logs controls', async ({ page }) => {
    await expect(page.locator('#log-agent')).toBeVisible();
    await expect(page.locator('#log-connect')).toBeVisible();
    await expect(page.locator('#log-clear')).toBeVisible();
    await expect(page.locator('#log-pause')).toBeVisible();
    await expect(page.locator('#log-search')).toBeVisible();
    await expect(page.locator('#log-filter-debug')).toBeVisible();
    await expect(page.locator('#log-filter-info')).toBeVisible();
    await expect(page.locator('#log-filter-warn')).toBeVisible();
    await expect(page.locator('#log-filter-error')).toBeVisible();
  });

  test('Connect renders structured log rows', async ({ page }) => {
    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    const rows = page.locator('#log-output .log-row-data');
    await expect(rows.first()).toBeVisible();
    await expect(page.locator('#log-output .log-row-data .log-cell-time').first()).toContainText('2026-02-22');
    await expect(page.locator('#log-output .log-row-data[data-level="INFO"] .log-cell-message').first()).toContainText('agent started');
  });

  test('level filter hides unchecked levels', async ({ page }) => {
    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    await expect(page.locator('#log-output')).toContainText('request failed');
    await page.uncheck('#log-filter-error');
    await expect(page.locator('#log-output')).not.toContainText('request failed');
    await expect(page.locator('#log-output')).toContainText('queue depth high');
  });

  test('search filters rows and highlights matches', async ({ page }) => {
    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    await page.fill('#log-search', 'failed');
    await expect(page.locator('#log-output .log-row-data')).toHaveCount(1);
    await expect(page.locator('#log-output .log-row-data .log-cell-message')).toContainText('request failed');
    await expect(page.locator('#log-output mark.log-highlight')).toContainText('failed');
  });

  test('Pause buffers incoming logs and Resume flushes them', async ({ page }) => {
    await page.route('**/api/v1/logs/stream*', (route) => route.abort('connectionrefused'));

    let pollCount = 0;
    await page.route('**/api/v1/agents/*/logs', (route) => {
      pollCount++;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ lines: [`[INFO] poll cycle ${pollCount}`] }),
      });
    });

    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    const logOutput = page.locator('#log-output');
    await expect(logOutput).toContainText('poll cycle 1', { timeout: 5000 });

    await page.click('#log-pause');
    await expect(page.locator('#log-pause')).toContainText('Resume');

    await page.waitForTimeout(2500);
    expect(pollCount).toBeGreaterThanOrEqual(2);
    await expect(logOutput).not.toContainText('poll cycle 2');

    await page.click('#log-pause');
    await expect(page.locator('#log-pause')).toContainText('Pause');
    await expect(logOutput).toContainText('poll cycle 2', { timeout: 3000 });
  });

  test('Clear button empties structured log output', async ({ page }) => {
    await expect(page.locator('#log-agent option')).not.toHaveCount(0);
    await page.click('#log-connect');

    await expect(page.locator('#log-output .log-row-data')).not.toHaveCount(0);

    await page.click('#log-clear');
    await expect(page.locator('#log-output .log-row-data')).toHaveCount(0);
    await expect(page.locator('#log-output')).toBeEmpty();
  });
});
