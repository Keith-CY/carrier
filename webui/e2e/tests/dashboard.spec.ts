import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken, MOCK_INSTANCES } from './helpers';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/dashboard');
  });

  test('displays agent cards', async ({ page }) => {
    const cards = page.locator('.agent-card');
    await expect(cards).toHaveCount(MOCK_INSTANCES.length);

    // Verify instance names
    for (const instance of MOCK_INSTANCES) {
      await expect(page.locator('.agent-card h4', { hasText: instance.id })).toBeVisible();
    }
  });

  test('shows correct status emoji', async ({ page }) => {
    // running → 🟢, error → 🔴, stopped → ⚪
    const statuses = page.locator('.agent-status');
    await expect(statuses.nth(0)).toContainText('🟢');
    await expect(statuses.nth(1)).toContainText('🔴');
    await expect(statuses.nth(2)).toContainText('⚪');
  });

  test('start and stop buttons are clickable', async ({ page }) => {
    const firstCard = page.locator('.agent-card').first();
    const startBtn = firstCard.locator('button', { hasText: 'Start' });
    const stopBtn = firstCard.locator('button', { hasText: 'Stop' });

    await expect(startBtn).toBeVisible();
    await expect(stopBtn).toBeVisible();

    // Click should not throw (API is mocked)
    await startBtn.click();
    await stopBtn.click();
  });

  test('refresh button reloads agents', async ({ page }) => {
    let fetchCount = 0;
    await page.route('**/api/v1/instances', (route) => {
      fetchCount++;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(MOCK_INSTANCES),
      });
    });

    // Initial load already fetched once, click refresh
    await page.click('#refresh-instances');
    // Wait for cards to re-render
    await expect(page.locator('.agent-card')).toHaveCount(MOCK_INSTANCES.length);
    expect(fetchCount).toBeGreaterThanOrEqual(1);
  });
});
