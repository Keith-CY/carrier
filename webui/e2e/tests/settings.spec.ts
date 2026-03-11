import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs, TEST_TOKEN } from './helpers';

test.describe('Settings', () => {
  test('renders provider and remote summaries and logs out', async ({ page }) => {
    await mockAPIs(page);
    await page.route('**/api/v1/telegram/transport', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          transport: {
            selected_mode: 'bot',
            reason_code: 'configured',
            hint: 'Bot mode active',
          },
        }),
      }),
    );

    await loginWithToken(page, '/#/settings');

    await expect(page.locator('#settings-provider')).toContainText('Telegram transport: bot');
    await expect(page.locator('#settings-provider')).toContainText('Remote metrics: ops=0');
    await expect(page.locator('#settings-provider')).toContainText('Remote rollout gate: state=healthy');

    await page.locator('#settings-logout').click();
    await expect(page.locator('#login-overlay')).toBeVisible();
    await expect(page.evaluate(() => localStorage.getItem('carrier_token'))).resolves.toBeNull();

    await page.locator('#login-token').fill(TEST_TOKEN);
    await page.locator('#login-btn').click();
    await expect(page.locator('#view-settings')).toBeVisible();
  });
});
