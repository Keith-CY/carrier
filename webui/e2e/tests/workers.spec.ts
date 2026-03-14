import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs, mockOrchestrationAPIs } from './helpers';

test.describe('Workers View', () => {
  test('shows worker inventory, queue summary, stale state, and reclaim actions', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);

    await loginWithToken(page, '/#/workers');

    await expect(page.locator('#view-workers')).toBeVisible();
    await expect(page.locator('#workers-summary')).toContainText('Total: 3');
    await expect(page.locator('#workers-summary')).toContainText('Stale: 1');
    await expect(page.locator('#workers-summary')).toContainText('Queued Tasks: 2');
    await expect(page.locator('#workers-list .worker-card')).toHaveCount(3);
    await expect(page.locator('#workers-list .worker-card').filter({ hasText: 'prod-host-1 / picoclaw' })).toContainText('busy');
    await expect(page.locator('#workers-list .worker-card').filter({ hasText: 'prod-host-1 / picoclaw' })).toContainText('stale');
    await expect(page.locator('#workers-list .worker-card').filter({ hasText: 'prod-host-1 / picoclaw' })).toContainText('queue position: 1');
    await expect(page.locator('#workers-list .worker-card').filter({ hasText: 'prod-host-1 / picoclaw' })).toContainText('stale reason: heartbeat_timeout');

    await page.fill('#workers-search', 'exec-running');
    await expect(page.locator('#workers-list .worker-card')).toHaveCount(1);
    await expect(page.locator('#workers-list .worker-card')).toContainText('exec-running');

    await page.fill('#workers-search', '');
    await page.selectOption('#workers-state-filter', 'available');
    await expect(page.locator('#workers-list .worker-card')).toHaveCount(1);
    await expect(page.locator('#workers-list .worker-card')).toContainText('Local / zeroclaw');

    await page.selectOption('#workers-state-filter', 'all');
    await page.selectOption('#workers-state-filter', 'stale');
    await expect(page.locator('#workers-list .worker-card')).toHaveCount(1);
    await expect(page.locator('#workers-list .worker-card')).toContainText('stale');

    await page.selectOption('#workers-state-filter', 'all');
    await page.click('#workers-reclaim-stale');
    await expect(page.locator('#workers-msg')).toContainText('reclaimed=1');
    await expect(page.locator('#workers-list .worker-card').filter({ hasText: 'prod-host-1 / picoclaw' })).toContainText('reclaimed');

    await page.click('#workers-reclaim-idle');
    await expect(page.locator('#workers-msg')).toContainText('reclaimed=1');
  });
});
