import { expect, test } from '@playwright/test';
import { TEST_TOKEN } from './helpers';

test.describe('Add PicoClaw (WebUI)', () => {
  test('walks add flow end-to-end and posts /api/v1/add with reused credential', async ({ page }) => {
    let addRequestBody: any = null;
    let addRequestAuthHeader = '';

    await page.route('**/healthz', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'ok' }),
      }),
    );

    await page.route('**/api/v1/pairing/sessions?provider=telegram', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          sessions: [
            {
              provider: 'telegram',
              chatId: '418258935',
              createdAt: '2026-02-21T00:00:00Z',
              updatedAt: '2026-02-21T00:00:00Z',
            },
          ],
        }),
      }),
    );

    await page.route('**/api/v1/providers', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          by_category: {
            builtin: [
              {
                id: 'openai',
                name: 'OpenAI',
                auth_mode: 'api_key',
                env_var: 'OPENAI_API_KEY',
                example_model: 'openai/gpt-5.2',
              },
            ],
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
            configured: true,
            available: true,
            reusable: true,
            id: 'openai-codex',
            credential_backend: 'local-file',
          },
        }),
      }),
    );

    await page.route('**/api/v1/add', async (route) => {
      addRequestBody = route.request().postDataJSON();
      addRequestAuthHeader = (await route.request().headerValue('authorization')) || '';
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          requestId: 'req-add-1',
          result: 'ok',
          message: 'picoclaw configured, installed, and started',
          agentId: 'picoclaw',
          instanceId: 'picoclaw-abcdef01',
          pairRequired: false,
          pairedChatId: '418258935',
          workspacePath: '/tmp/picoclaw/workspace',
          configPath: '/tmp/picoclaw/config.json',
          recordPath: '/tmp/picoclaw/record.json',
        }),
      });
    });

    await page.addInitScript((token: string) => {
      localStorage.setItem('carrier_token', token);
    }, TEST_TOKEN);

    await page.goto('/#/add/picoclaw');

    await expect(page.locator('#view-setup')).toBeVisible();
    await expect(page.locator('#setup-title')).toContainText('Step 1');
    await expect(page.locator('#setup-title')).toContainText('PicoClaw');
    await expect(page.locator('#setup-pair-msg')).toContainText('Auto-selected Carrier paired Telegram user');

    await page.fill('#provider-token', 'tg-test-token');
    await page.click('#setup-btn');

    await expect(page).toHaveURL(/#\/provider$/);
    await expect(page.locator('#view-provider')).toBeVisible();
    await expect(page.locator('#provider-agent-name')).toContainText('Adding: picoclaw');
    await expect(page.locator('#provider-default-summary')).toContainText('Using Carrier default');
    await expect(page.locator('#provider-next')).toBeEnabled();

    await page.click('#provider-next');
    await expect(page).toHaveURL(/#\/install$/);
    await expect(page.locator('#view-install')).toBeVisible();
    await expect(page.locator('#install-summary')).toContainText('Agent: picoclaw');
    await expect(page.locator('#install-summary')).toContainText('Channel: telegram');
    await expect(page.locator('#install-summary')).toContainText('Provider: OpenAI Codex (OAuth)');

    await page.click('#install-confirm');

    await expect(page).toHaveURL(/#\/complete$/);
    await expect(page.locator('#view-complete')).toBeVisible();
    await expect(page.locator('#complete-title')).toContainText('Setup Complete');
    await expect(page.locator('#complete-detail')).toContainText('Instance: picoclaw-abcdef01');
    await expect(page.locator('#complete-detail')).toContainText('Paired chat: 418258935');

    expect(addRequestAuthHeader).toBe(`Bearer ${TEST_TOKEN}`);
    expect(addRequestBody).toBeTruthy();
    expect(addRequestBody.agentId).toBe('picoclaw');
    expect(addRequestBody.channel).toBe('telegram');
    expect(addRequestBody.channelToken).toBe('tg-test-token');
    expect(addRequestBody.channelChatId).toBe('418258935');
    expect(addRequestBody.providerId).toBe('openai-codex');
    expect(addRequestBody.providerToken).toBe('');
    expect(addRequestBody.reuseCredential).toBe(true);
  });
});
