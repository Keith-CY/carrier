import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken } from './helpers';

test.describe('Visual Regression', () => {
  test('login page', async ({ page }) => {
    await mockAPIs(page, { healthOk: false });
    await page.goto('/');
    await expect(page.locator('#login-overlay')).toBeVisible();
    await expect(page).toHaveScreenshot('login.png', { fullPage: true });
  });

  test('welcome page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/welcome');
    await expect(page.locator('#view-welcome')).toBeVisible();
    await expect(page).toHaveScreenshot('welcome.png', { fullPage: true });
  });

  test('setup page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/setup');
    await expect(page.locator('#view-setup')).toBeVisible();
    await expect(page).toHaveScreenshot('setup.png', { fullPage: true });
  });

  test('dashboard page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/dashboard');
    await expect(page.locator('.agent-card')).toHaveCount(3);
    await expect(page).toHaveScreenshot('dashboard.png', { fullPage: true });
  });

  test('logs page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/logs');
    await expect(page.locator('#view-logs')).toBeVisible();
    await expect(page).toHaveScreenshot('logs.png', { fullPage: true });
  });

  test('chat page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/chat');
    await expect(page.locator('#view-chat')).toBeVisible();
    await expect(page).toHaveScreenshot('chat.png', { fullPage: true });
  });

  test('settings page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/settings');
    await expect(page.locator('#view-settings')).toBeVisible();
    await expect(page).toHaveScreenshot('settings.png', { fullPage: true });
  });
});
