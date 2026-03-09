import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs, MOCK_TEMPLATES } from './helpers';

test.describe('Policies Trigger Management', () => {
  test('supports create, edit, disable, and delete flow', async ({ page }) => {
    await mockAPIs(page);

    const triggers: Array<Record<string, unknown>> = [
      {
        id: 'trigger-1',
        name: 'incident webhook',
        type: 'webhook',
        templateId: 'incident-diagnosis',
        enabled: true,
        createdBy: 'admin',
        config: {
          webhookSecretConfigured: true,
          hostIds: ['host-1'],
          inputs: {
            service: '{{payload.service}}',
            environment: '{{payload.environment}}',
            incidentSummary: '{{payload.summary}}',
          },
        },
      },
    ];
    let createCalls = 0;
    let patchCalls = 0;
    let deleteCalls = 0;
    let lastCreateBody: Record<string, unknown> | null = null;
    let lastPatchBody: Record<string, unknown> | null = null;

    await page.route('**/api/v1/templates', async (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ result: 'ok', templates: MOCK_TEMPLATES }),
      }),
    );

    await page.route('**/api/v1/triggers', async (route) => {
      const req = route.request();
      if (req.method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', triggers }),
        });
      }
      if (req.method() === 'POST') {
        createCalls += 1;
        lastCreateBody = req.postDataJSON() as Record<string, unknown>;
        const config = ((lastCreateBody?.config as Record<string, unknown>) || {});
        const trigger = {
          id: 'trigger-2',
          ...lastCreateBody,
          enabled: true,
          config: {
            ...config,
            webhookSecretConfigured: !!config.webhookSecret,
          },
        };
        triggers.unshift(trigger);
        return route.fulfill({
          status: 201,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', trigger }),
        });
      }
      return route.fallback();
    });

    await page.route('**/api/v1/triggers/*', async (route) => {
      const req = route.request();
      const id = decodeURIComponent(new URL(req.url()).pathname.split('/').slice(-1)[0] || '');
      const idx = triggers.findIndex((item) => String(item.id) === id);
      if (idx < 0) {
        return route.fulfill({
          status: 404,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'error', message: 'not found' }),
        });
      }
      if (req.method() === 'GET') {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', trigger: triggers[idx] }),
        });
      }
      if (req.method() === 'PATCH') {
        patchCalls += 1;
        lastPatchBody = req.postDataJSON() as Record<string, unknown>;
        const current = triggers[idx];
        const nextConfig = {
          ...((current.config as Record<string, unknown>) || {}),
          ...((lastPatchBody?.config as Record<string, unknown>) || {}),
        };
        triggers[idx] = {
          ...current,
          ...lastPatchBody,
          id,
          config: {
            ...nextConfig,
            webhookSecretConfigured: !!(nextConfig.webhookSecret || nextConfig.webhookSecretConfigured),
          },
        };
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', trigger: triggers[idx] }),
        });
      }
      if (req.method() === 'DELETE') {
        deleteCalls += 1;
        triggers.splice(idx, 1);
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ result: 'ok', deleted: true }),
        });
      }
      return route.fallback();
    });

    await loginWithToken(page, '/#/policies');

    await expect(page.locator('#execution-triggers-list .agent-card h4', { hasText: 'incident webhook' })).toBeVisible();

    await page.fill('#trigger-name', 'nightly smoke');
    await page.selectOption('#trigger-type', 'schedule');
    await page.selectOption('#trigger-template-id', 'incident-diagnosis');
    await page.fill('#trigger-inputs', 'service=checkout\nenvironment=prod\nincidentSummary=nightly smoke run');
    await page.fill('#trigger-host-ids', 'host-1');
    await page.fill('#trigger-cron', '0 * * * *');
    await page.fill('#trigger-timezone', 'UTC');
    await page.click('#trigger-policy-approve');
    await page.click('#trigger-save');

    await expect.poll(() => createCalls).toBe(1);
    expect(lastCreateBody?.type).toBe('schedule');
    expect(lastCreateBody?.templateId).toBe('incident-diagnosis');
    await expect(page.locator('#execution-triggers-list .agent-card h4', { hasText: 'nightly smoke' })).toBeVisible();
    await expect(page.locator('#profiles-msg')).toContainText('Execution trigger saved.');

    const firstCard = page.locator('#execution-triggers-list .agent-card').filter({ hasText: 'incident webhook' }).first();
    await firstCard.locator('button', { hasText: 'Disable' }).click();
    await expect.poll(() => patchCalls).toBe(1);
    expect(lastPatchBody?.enabled).toBe(false);
    await expect(firstCard).toContainText('enabled: false');

    await firstCard.locator('button', { hasText: 'Edit' }).click();
    await expect(page.locator('#trigger-editor-state')).toContainText('Editing trigger');
    await page.fill('#trigger-name', 'incident webhook prod');
    await page.click('#trigger-save');
    await expect.poll(() => patchCalls).toBe(2);
    expect(lastPatchBody?.name).toBe('incident webhook prod');
    await expect(page.locator('#execution-triggers-list .agent-card h4', { hasText: 'incident webhook prod' })).toBeVisible();

    page.once('dialog', (dialog) => dialog.accept());
    await page.locator('#execution-triggers-list .agent-card').filter({ hasText: 'incident webhook prod' }).first().locator('button', { hasText: 'Delete' }).click();
    await expect.poll(() => deleteCalls).toBe(1);
    await expect(page.locator('#execution-triggers-list .agent-card h4', { hasText: 'incident webhook prod' })).toHaveCount(0);
  });
});
