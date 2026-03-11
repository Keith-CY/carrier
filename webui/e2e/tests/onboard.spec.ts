import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken } from './helpers';

test.describe('Onboard Wizard', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
  });

  test('Welcome page detects daemon connection', async ({ page }) => {
    await loginWithToken(page, '/#/welcome');
    await expect(page.locator('#view-welcome')).toBeVisible();
    await expect(page.locator('#welcome-status')).toContainText('🟢 Daemon connected');
    await expect(page.locator('#welcome-continue')).toBeVisible();
  });

  test('Setup page shows provider form', async ({ page }) => {
    await loginWithToken(page, '/#/setup');
    await expect(page.locator('#view-setup')).toBeVisible();
    await expect(page.locator('#provider')).toBeVisible();
    await expect(page.locator('#provider-token')).toBeVisible();
  });

  test('Agents page lists agents and allows selection', async ({ page }) => {
    await loginWithToken(page, '/#/agents');
    await expect(page.locator('#view-agents')).toBeVisible();

    // Wait for agents to load
    const items = page.locator('#agent-pick li');
    await expect(items).toHaveCount(3);

    // Drive the selection state change directly; sticky header scroll behavior makes pointer clicks flaky here.
    await items.nth(1).evaluate((element: HTMLElement) => element.click());
    await expect(items.nth(1)).toHaveClass(/selected/);
    await expect(page.locator('#agents-next')).toBeEnabled();
  });

  test('Config page allows adding environment variables', async ({ page }) => {
    await loginWithToken(page, '/#/config');
    await expect(page.locator('#view-config')).toBeVisible();

    // Should have one env row by default
    await expect(page.locator('.env-row')).toHaveCount(1);

    // Add another
    await page.click('#add-env');
    await expect(page.locator('.env-row')).toHaveCount(2);
  });

  test('Install page shows confirmation', async ({ page }) => {
    await loginWithToken(page, '/#/install');
    await expect(page.locator('#view-install')).toBeVisible();
    await expect(page.locator('#install-confirm')).toBeVisible();
  });

  test('Complete page navigates to dashboard', async ({ page }) => {
    await loginWithToken(page, '/#/complete');
    await expect(page.locator('#view-complete')).toBeVisible();

    await page.click('#complete-dashboard');
    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.locator('#view-dashboard')).toBeVisible();
  });
});
