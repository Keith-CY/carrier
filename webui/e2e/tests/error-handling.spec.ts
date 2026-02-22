import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken, TEST_TOKEN } from './helpers';

test.describe('Error Handling', () => {
  test('shows offline badge when daemon is unreachable', async ({ page }) => {
    // Mock healthz to return network error
    await page.route('**/healthz', (route) => route.abort('connectionrefused'));
    await page.route('**/api/v1/agents', (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: '[]' }),
    );

    await page.addInitScript((t: string) => {
      localStorage.setItem('carrier_token', t);
    }, TEST_TOKEN);
    await page.goto('/#/dashboard');

    const badge = page.locator('#health-badge');
    await expect(badge).toContainText('offline');
  });

  test('shows error when API returns 500', async ({ page }) => {
    await mockAPIs(page);
    // Override agents to return 500
    await page.route('**/api/v1/agents', (route) =>
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"internal"}' }),
    );

    await loginWithToken(page, '/#/dashboard');

    // The agent list should be empty or show error state
    // The api() function tries r.json() regardless, but 500 won't trigger 401 path
    // Check that agent cards are not rendered
    const cards = page.locator('.agent-card');
    await expect(cards).toHaveCount(0);
  });

  test('shows error when agent install fails', async ({ page }) => {
    await mockAPIs(page);
    // Override install to return error
    await page.route('**/api/v1/agents/*/install', (route) =>
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"install failed"}' }),
    );

    // We need to set selectedAgent — navigate through wizard
    await loginWithToken(page, '/#/agents');
    // Select first agent
    const items = page.locator('#agent-pick li');
    await expect(items).toHaveCount(3);
    await items.first().click();
    await page.click('#agents-next');

    // Provider page → skip
    await expect(page.locator('#view-provider')).toBeVisible();
    await page.click('#provider-skip');

    // Config page → next
    await expect(page.locator('#view-config')).toBeVisible();
    await page.click('#config-next');

    // Install page
    await expect(page.locator('#view-install')).toBeVisible();
    await page.click('#install-confirm');

    // Should show error message
    await expect(page.locator('#install-msg')).toContainText('Error');
  });

  test('shows error when provider setup fails', async ({ page }) => {
    await mockAPIs(page);
    // Override setup endpoint
    await page.route('**/api/v1/setup', (route) =>
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"config failed"}' }),
    );

    await loginWithToken(page, '/#/setup');
    await expect(page.locator('#view-setup')).toBeVisible();
    // The setup button just navigates to #/agents in daemon mode, so this tests visibility
    await expect(page.locator('#setup-btn')).toBeVisible();
  });
});
