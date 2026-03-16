import { expect, test } from '@playwright/test';
import { loginWithToken, TEST_TOKEN } from './helpers';

test.describe('Add OpenClaw (WebUI)', () => {
  test('accepts a pasted OpenAI Codex token without forcing Telegram pairing', async ({ page }) => {
    let addRequestBody: any = null;

    await page.route('**/healthz', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'ok' }),
      }),
    );

    await page.route('**/api/v1/channels', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          channels: [
            {
              id: 'telegram',
              displayName: 'Telegram',
              supportsProviderSetup: true,
              supportsPairing: true,
              requiresBotToken: true,
              requiresWebhookSecret: false,
              configured: false,
            },
          ],
        }),
      }),
    );

    await page.route('**/api/v1/pairing/sessions?provider=telegram', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ sessions: [] }),
      }),
    );

    await page.route('**/api/v1/providers', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          by_category: {
            builtin: [],
            custom: [
              {
                id: 'openai-codex',
                name: 'OpenAI Codex (OAuth)',
                auth_mode: 'oauth_device_code',
                env_var: 'OPENAI_CODEX_TOKEN',
                example_model: 'openai-codex/gpt-5.3-codex',
              },
            ],
            local: [],
          },
          carrier_default_provider: {
            configured: false,
            available: false,
            reusable: false,
          },
        }),
      }),
    );

    await page.route('**/api/v1/auth/providers', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          providers: [
            {
              id: 'openai-codex',
              configured: false,
              reusable: false,
              hasSavedCredential: false,
              credentialBackend: '',
            },
          ],
        }),
      }),
    );

    await page.route('**/api/v1/add', async (route) => {
      addRequestBody = route.request().postDataJSON();
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-add-openclaw-1',
          result: 'ok',
          message: 'openclaw configured, installed, and started',
          agentId: 'openclaw',
          instanceId: 'openclaw-demo-1',
          pairRequired: false,
          workspacePath: '/tmp/openclaw/workspace',
          configPath: '/tmp/openclaw/config.json',
          recordPath: '/tmp/openclaw/record.json',
        }),
      });
    });

    await page.addInitScript((token: string) => {
      localStorage.setItem('carrier_token', token);
    }, TEST_TOKEN);

    await loginWithToken(page, '/add/openclaw');

    await expect(page.locator('#view-setup')).toBeVisible();
    await expect(page.locator('#setup-title')).toContainText('OpenClaw');
    await expect(page.locator('#setup-channel-summary')).toContainText('No pairing required');
    await expect(page.locator('#setup-telegram-pair')).toBeHidden();

    await page.fill('#provider-token', 'tg-openclaw-token');
    await page.click('#setup-btn');

    await expect(page).toHaveURL(/\/provider$/);
    await expect(page.locator('#view-provider')).toBeVisible();
    await page.locator('.provider-item', { hasText: 'OpenAI Codex (OAuth)' }).click();
    await expect(page.locator('#provider-api-key')).toBeVisible();
    await expect(page.locator('#provider-next')).toBeDisabled();

    await page.fill('#provider-api-key', 'codex-token-demo');
    await expect(page.locator('#provider-next')).toBeEnabled();
    await page.click('#provider-next');

    await expect(page).toHaveURL(/\/install$/);
    await expect(page.locator('#install-summary')).toContainText('Agent: openclaw');
    await expect(page.locator('#install-summary')).toContainText('Channel: telegram');
    await expect(page.locator('#install-summary')).toContainText('Provider: OpenAI Codex (OAuth)');

    await page.click('#install-confirm');

    await expect(page).toHaveURL(/\/complete$/);
    await expect(page.locator('#complete-detail')).toContainText('Instance: openclaw-demo-1');

    expect(addRequestBody).toBeTruthy();
    expect(addRequestBody.agentId).toBe('openclaw');
    expect(addRequestBody.channel).toBe('telegram');
    expect(addRequestBody.channelToken).toBe('tg-openclaw-token');
    expect(addRequestBody.channelChatId).toBe('');
    expect(addRequestBody.providerId).toBe('openai-codex');
    expect(addRequestBody.providerToken).toBe('codex-token-demo');
    expect(addRequestBody.reuseCredential).toBe(false);
  });
});
