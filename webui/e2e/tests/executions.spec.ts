import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs, mockOrchestrationAPIs } from './helpers';

test.describe('Execution Center', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
  });

  test('dashboard quick launch previews plan and runs execution', async ({ page }) => {
    await loginWithToken(page, '/#/dashboard');

    await expect(page.locator('#quick-launch-goal')).toBeVisible();
    await page.fill('#quick-launch-goal', 'Investigate checkout latency and summarize next steps');
    await page.click('#quick-launch-advanced-toggle');
    await expect(page.locator('#quick-launch-provider')).toBeVisible();
    await expect(page.locator('#quick-launch-hosts')).toContainText('prod-host-1');
    await page.fill('#quick-launch-max-concurrency', '2');

    await page.click('#quick-launch-preview');
    await expect(page.locator('#quick-launch-preview-card')).toBeVisible();
    await expect(page.locator('#quick-launch-preview-card')).toContainText('collect context');
    await expect(page.locator('#quick-launch-preview-card')).toContainText('draft summary');

    await page.click('#quick-launch-run');
    await expect.poll(() => page.url()).toContain('#/executions/exec-preview-1');
    await expect(page.locator('#executions-detail')).toContainText('Investigate checkout latency');
    await expect(page.locator('#executions-detail')).toContainText('task-1');
  });

  test('executions page filters and searches', async ({ page }) => {
    await loginWithToken(page, '/#/executions');

    await expect(page.locator('#executions-list .execution-card')).toHaveCount(2);
    await page.selectOption('#executions-status-filter', 'completed');
    await expect(page.locator('#executions-list')).toContainText('Prepare release notes');
    await expect(page.locator('#executions-list')).not.toContainText('Investigate checkout latency');

    await page.selectOption('#executions-status-filter', 'all');
    await page.fill('#executions-search', 'checkout');
    await expect(page.locator('#executions-list')).toContainText('Investigate checkout latency');
    await expect(page.locator('#executions-list')).not.toContainText('Prepare release notes');
  });

  test('direct execution route supports cancel', async ({ page }) => {
    await loginWithToken(page, '/#/executions/exec-running');

    await expect(page.locator('#executions-detail')).toContainText('Investigate checkout latency');
    page.once('dialog', (dialog) => dialog.accept());
    await page.click('#executions-cancel');
    await expect(page.locator('#executions-detail')).toContainText('cancelled');
  });
});
