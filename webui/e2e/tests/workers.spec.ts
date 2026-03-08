import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs, mockOrchestrationAPIs } from './helpers';

test.describe('Workers View', () => {
  test('shows worker inventory, filters, searches, and reclaims idle workers', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);

    await loginWithToken(page, '/#/workers');

    await expect(page.locator('#view-workers')).toBeVisible();
    await expect(page.locator('#workers-summary')).toContainText('Total: 3');
    await expect(page.locator('#workers-list .worker-card')).toHaveCount(3);
    await expect(page.locator('#workers-list .worker-card').filter({ hasText: 'prod-host-1 / picoclaw' })).toContainText('busy');

    await page.fill('#workers-search', 'exec-running');
    await expect(page.locator('#workers-list .worker-card')).toHaveCount(1);
    await expect(page.locator('#workers-list .worker-card')).toContainText('exec-running');

    await page.fill('#workers-search', '');
    await page.selectOption('#workers-state-filter', 'available');
    await expect(page.locator('#workers-list .worker-card')).toHaveCount(1);
    await expect(page.locator('#workers-list .worker-card')).toContainText('Local / zeroclaw');

    await page.selectOption('#workers-state-filter', 'all');
    await page.click('#workers-reclaim-idle');
    await expect(page.locator('#workers-msg')).toContainText('reclaimed=1');

    await expect(page.locator('#workers-list .worker-card').filter({ hasText: 'prod-host-1 / picoclaw' })).toContainText('reclaimed');
  });
});
