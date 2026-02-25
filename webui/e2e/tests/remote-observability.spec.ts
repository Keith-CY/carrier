import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

test.describe('Remote Observability', () => {
  test('supports operation-group filtering, anomaly-only mode, and failure-first sorting', async ({ page }) => {
    await mockAPIs(page);

    let metricCalls = 0;
    await page.route('**/api/v1/remote/metrics', async (route) => {
      metricCalls += 1;
      const second = metricCalls >= 2;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          metrics: {
            timestamp: second ? '2026-02-25T09:00:00Z' : '2026-02-25T08:00:00Z',
            totals: {
              total: second ? 6 : 3,
              success: second ? 5 : 2,
              failure: 1,
              successRate: second ? 5 / 6 : 2 / 3,
              avgLatencyMs: second ? 95 : 120,
              minLatencyMs: 40,
              maxLatencyMs: second ? 200 : 240,
              latencyTotalMs: second ? 570 : 360,
            },
            repair: {
              triggered: second ? 2 : 1,
              success: second ? 2 : 1,
              failure: 0,
              successRate: 1,
            },
            chatStream: {
              total: second ? 3 : 2,
              failure: 1,
              failureRate: second ? 1 / 3 : 0.5,
            },
            rollout: {
              state: second ? 'healthy' : 'canary',
              canPromote: second,
              reasons: second ? [] : ['chat stream failure rate at or above 20%'],
            },
            operations: {
              host_check: { total: 2, success: 2, failure: 0, successRate: 1, avgLatencyMs: 44 },
              instances_install: { total: second ? 2 : 1, success: second ? 1 : 0, failure: 1, successRate: second ? 0.5 : 0, avgLatencyMs: second ? 270 : 300 },
              remote_chat_stream: { total: second ? 3 : 2, success: second ? 2 : 1, failure: 1, successRate: second ? 2 / 3 : 0.5, avgLatencyMs: second ? 130 : 150 },
              provider_binding_apply: { total: second ? 2 : 1, success: second ? 2 : 1, failure: 0, successRate: 1, avgLatencyMs: second ? 980 : 1200 },
            },
          },
        }),
      });
    });

    await loginWithToken(page, '/#/remote-observability');

    await expect(page.locator('#view-remote-observability')).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Operations' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Rollout' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary')).toContainText('success rate: 67%');
    await expect(page.locator('#remote-observability-summary')).toContainText('state: canary');
    await expect(page.locator('#remote-observability-group')).toContainText('instances');
    await expect(page.locator('#remote-observability-group')).toContainText('provider');
    await expect(page.locator('#remote-observability-ops-body')).toContainText('instances_install');
    await expect(page.locator('#remote-observability-ops-body')).toContainText('remote_chat_stream');
    await expect(page.locator('#remote-observability-ops-body')).toContainText('provider_binding_apply');

    const firstOperation = page.locator('#remote-observability-ops-body tr').first().locator('td').first();
    await expect(firstOperation).toContainText('instances_install');

    await page.selectOption('#remote-observability-group', 'host');
    await expect(page.locator('#remote-observability-ops-body')).toContainText('host_check');
    await expect(page.locator('#remote-observability-ops-body')).not.toContainText('instances_install');

    await page.check('#remote-observability-anomalies');
    await expect(page.locator('#remote-observability-ops-body')).toContainText('No remote operation metrics match current filters.');

    await page.selectOption('#remote-observability-group', 'all');
    await expect(page.locator('#remote-observability-ops-body')).toContainText('instances_install');
    await expect(page.locator('#remote-observability-ops-body')).toContainText('remote_chat_stream');
    await expect(page.locator('#remote-observability-ops-body')).toContainText('provider_binding_apply');
    await expect(page.locator('#remote-observability-ops-body')).not.toContainText('host_check');
    await expect(page.locator('#remote-observability-status')).toContainText('Updated at 2026-02-25T08:00:00Z');
    await expect(page.locator('#remote-observability-status')).toContainText('anomalies only');
    await expect(page.locator('#remote-observability-status')).toContainText('rollout=canary');

    await page.click('#remote-observability-refresh');
    await expect.poll(() => metricCalls).toBe(2);
    await expect(page.locator('#remote-observability-summary')).toContainText('success rate: 83%');
    await expect(page.locator('#remote-observability-summary')).toContainText('state: healthy');
    await expect(page.locator('#remote-observability-status')).toContainText('Updated at 2026-02-25T09:00:00Z');
    await expect(page.locator('#remote-observability-status')).toContainText('rollout=healthy');
  });
});
