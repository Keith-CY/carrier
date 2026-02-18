import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken, TEST_TOKEN } from './helpers';

test.describe('Token Expiry', () => {
  test('redirects to login when API returns 401', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/dashboard');

    // Verify we're on dashboard
    await expect(page.locator('#view-dashboard')).toBeVisible();
    await expect(page.locator('#login-overlay')).toBeHidden();

    // Now override agents endpoint to return 401 (simulating token expiry)
    await page.route('**/api/v1/agents', (route) =>
      route.fulfill({ status: 401, contentType: 'application/json', body: '{"error":"unauthorized"}' }),
    );

    // Trigger a refresh that will hit the 401
    await page.click('#refresh-agents');

    // Should redirect to login
    await expect(page.locator('#login-overlay')).toBeVisible();
  });

  test('clears localStorage token on 401', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/dashboard');
    await expect(page.locator('#view-dashboard')).toBeVisible();

    // Override to return 401
    await page.route('**/api/v1/agents', (route) =>
      route.fulfill({ status: 401, contentType: 'application/json', body: '{"error":"unauthorized"}' }),
    );

    await page.click('#refresh-agents');
    await expect(page.locator('#login-overlay')).toBeVisible();

    // Check token is cleared
    const token = await page.evaluate(() => localStorage.getItem('carrier_token'));
    expect(token).toBeNull();
  });

  test('can re-login after token expiry', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/dashboard');
    await expect(page.locator('#view-dashboard')).toBeVisible();

    // Expire the token
    await page.route('**/api/v1/agents', (route) =>
      route.fulfill({ status: 401, contentType: 'application/json', body: '{"error":"unauthorized"}' }),
    );
    await page.click('#refresh-agents');
    await expect(page.locator('#login-overlay')).toBeVisible();

    // Now re-login: restore normal mocks
    await page.unrouteAll();
    await mockAPIs(page);

    // Override healthz to accept the token
    await page.route('**/healthz', (route) => {
      const auth = route.request().headers()['authorization'] || '';
      if (auth.includes(TEST_TOKEN)) {
        return route.fulfill({ status: 200, contentType: 'application/json', body: '{"status":"ok"}' });
      }
      return route.fulfill({ status: 401, contentType: 'application/json', body: '{"status":"error"}' });
    });

    await page.fill('#login-token', TEST_TOKEN);
    await page.click('#login-btn');

    await expect(page.locator('#login-overlay')).toBeHidden();
  });
});
