import { test, expect } from '@playwright/test';
import { mockAPIs, TEST_TOKEN } from './helpers';

test.describe('Login', () => {
  test('shows login overlay when no token', async ({ page }) => {
    // Health returns 401 → forces login
    await mockAPIs(page, { healthOk: false });
    await page.goto('/');
    await expect(page.locator('#login-overlay')).toBeVisible();
  });

  test('shows error on invalid token', async ({ page }) => {
    // First healthz (no auth) → 401 to show login
    let callCount = 0;
    await page.route('**/healthz', (route) => {
      callCount++;
      // The init call and login attempt both go through here
      const auth = route.request().headers()['authorization'] || '';
      if (auth.includes('wrong-token') || !auth) {
        return route.fulfill({
          status: 401,
          contentType: 'application/json',
          body: '{"status":"error"}',
        });
      }
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: '{"status":"ok"}',
      });
    });

    await page.goto('/');
    await expect(page.locator('#login-overlay')).toBeVisible();

    await page.fill('#login-token', 'wrong-token');
    await page.click('#login-btn');

    await expect(page.locator('#login-msg')).toContainText('Invalid token');
  });

  test('logs in successfully with correct token', async ({ page }) => {
    await mockAPIs(page, { healthOk: false });
    // Override healthz to accept the correct token
    await page.route('**/healthz', (route) => {
      const auth = route.request().headers()['authorization'] || '';
      if (auth.includes(TEST_TOKEN)) {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: '{"status":"ok"}',
        });
      }
      return route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: '{"status":"error"}',
      });
    });

    await page.goto('/');
    await expect(page.locator('#login-overlay')).toBeVisible();

    await page.fill('#login-token', TEST_TOKEN);
    await page.click('#login-btn');

    await expect(page.locator('#login-overlay')).toBeHidden();
  });

  test('logout returns to login overlay', async ({ page }) => {
    await mockAPIs(page);
    // Pre-set token to be logged in
    await page.addInitScript((t: string) => {
      localStorage.setItem('carrier_token', t);
    }, TEST_TOKEN);
    await page.goto('/#/dashboard');

    await expect(page.locator('#login-overlay')).toBeHidden();
    await expect(page.locator('#logout-btn')).toBeVisible();

    await page.click('#logout-btn');
    await expect(page.locator('#login-overlay')).toBeVisible();
  });
});
