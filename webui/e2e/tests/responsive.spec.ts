import { test, expect } from '@playwright/test';
import { mockAPIs, mockOrchestrationAPIs, loginWithToken, MOCK_EXECUTIONS, MOCK_INSTANCES } from './helpers';

const MOBILE = { width: 375, height: 667 };
const TABLET = { width: 768, height: 1024 };

for (const [label, viewport] of [['Mobile', MOBILE], ['Tablet', TABLET]] as const) {
  test.describe(`Responsive — ${label} (${viewport.width}x${viewport.height})`, () => {
    test.use({ viewport });

    test('login page is usable', async ({ page }) => {
      await mockAPIs(page, { healthOk: false });
      await page.goto('/');
      await expect(page.locator('#login-overlay')).toBeVisible();
      await expect(page.locator('#login-token')).toBeVisible();
      await expect(page.locator('#login-btn')).toBeVisible();

      // Login button should be within viewport
      const box = await page.locator('#login-btn').boundingBox();
      expect(box).toBeTruthy();
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
    });

    test('dashboard renders agent cards', async ({ page }) => {
      await mockAPIs(page);
      await loginWithToken(page, '/#/dashboard');
      const cards = page.locator('.agent-card');
      await expect(cards).toHaveCount(MOCK_INSTANCES.length);

      // Each card should be visible and within viewport width
      for (let i = 0; i < MOCK_INSTANCES.length; i++) {
        const box = await cards.nth(i).boundingBox();
        expect(box).toBeTruthy();
        expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width + 1);
      }
    });

    test('navigation is visible', async ({ page }) => {
      await mockAPIs(page);
      await loginWithToken(page, '/#/dashboard');
      const nav = page.locator('#nav');
      await expect(nav).toBeVisible();

      // Nav links should be accessible
      const links = page.locator('#nav .nav-link');
      const count = await links.count();
      expect(count).toBeGreaterThan(0);
    });

    test('logs page is usable', async ({ page }) => {
      await mockAPIs(page);
      await loginWithToken(page, '/#/logs');
      await expect(page.locator('#log-agent')).toBeVisible();
      await expect(page.locator('#log-connect')).toBeVisible();
    });

    test('chat page is usable', async ({ page }) => {
      await mockAPIs(page);
      await loginWithToken(page, '/#/chat');
      await expect(page.locator('#chat-input')).toBeVisible();
      await expect(page.locator('#chat-send')).toBeVisible();
    });

    test('settings page is usable', async ({ page }) => {
      await mockAPIs(page);
      await loginWithToken(page, '/#/settings');
      await expect(page.locator('#view-settings')).toBeVisible();
    });

    test('executions page is usable', async ({ page }) => {
      await mockAPIs(page);
      await mockOrchestrationAPIs(page);
      await loginWithToken(page, '/#/dashboard');
      await page.locator('#nav .nav-link[data-route="executions"]').click();
      await expect(page.locator('#view-executions')).toBeVisible();
      await expect(page.locator('#executions-search')).toBeVisible();
      await expect(page.locator('#executions-status-filter')).toBeVisible();
      await expect(page.locator('#executions-template-filter')).toBeVisible();
      await expect(page.locator('#executions-trigger-filter')).toBeVisible();
      await expect(page.locator('#executions-list .execution-card')).toHaveCount(MOCK_EXECUTIONS.length);
    });
  });
}

test.describe('Responsive — Mobile agent grid layout', () => {
  test.use({ viewport: MOBILE });

  test('agent cards stack in single column on mobile', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/dashboard');

    const cards = page.locator('.agent-card');
    await expect(cards).toHaveCount(MOCK_INSTANCES.length);

    // All cards should have similar x position (single column)
    const positions: number[] = [];
    for (let i = 0; i < MOCK_INSTANCES.length; i++) {
      const box = await cards.nth(i).boundingBox();
      if (box) positions.push(Math.round(box.x));
    }

    // In single-column layout, all cards should start at roughly the same x
    if (positions.length > 1) {
      const unique = [...new Set(positions)];
      expect(unique.length).toBeLessThanOrEqual(2); // Allow small rounding differences
    }
  });
});
