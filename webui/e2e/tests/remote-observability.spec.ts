import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

test.describe('Remote Observability', () => {
  test('supports operation-group filtering, anomaly-only mode, and failure-first sorting', async ({ page }) => {
    await mockAPIs(page);

    let metricCalls = 0;
    let orchestratorMetricCalls = 0;
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
    await page.route('**/api/v1/orchestrator/metrics', async (route) => {
      orchestratorMetricCalls += 1;
      const second = orchestratorMetricCalls >= 2;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          result: 'ok',
          metrics: {
            timestamp: second ? '2026-02-25T09:00:00Z' : '2026-02-25T08:00:00Z',
            executions: {
              total: second ? 5 : 4,
              running: second ? 0 : 1,
              completed: second ? 3 : 2,
              failed: 1,
              cancelled: 1,
              retryableFailed: second ? 0 : 1,
              retryCount: second ? 1 : 3,
              avgLatencyMs: second ? 61000 : 90000,
            },
            workers: {
              total: 3,
              busy: second ? 0 : 1,
              ready: 1,
              error: 1,
              stale: second ? 0 : 1,
            },
            providers: {
              requestedFailures: second ? {} : { anthropic: 1 },
              resolvedFailures: second ? {} : { anthropic: 1 },
              totalEstimatedCostUsd: second ? 0.0029 : 0.0041,
              aggregates: second
                ? [
                    { provider: 'anthropic', successes: 2, failures: 0, avgLatencyMs: 320, estimatedCostUsd: 0.0011 },
                    { provider: 'openrouter', successes: 3, failures: 0, avgLatencyMs: 210, estimatedCostUsd: 0.0018 },
                  ]
                : [
                    { provider: 'anthropic', successes: 1, failures: 1, avgLatencyMs: 510, estimatedCostUsd: 0.0025 },
                    { provider: 'openrouter', successes: 2, failures: 0, avgLatencyMs: 240, estimatedCostUsd: 0.0016 },
                  ],
              models: second
                ? [
                    { provider: 'anthropic', model: 'claude-3-7-sonnet', successes: 2, failures: 0, avgLatencyMs: 320, estimatedCostUsd: 0.0011 },
                    { provider: 'openrouter', model: 'openai/gpt-4o-mini', successes: 3, failures: 0, avgLatencyMs: 210, estimatedCostUsd: 0.0018 },
                  ]
                : [
                    { provider: 'anthropic', model: 'claude-3-7-sonnet', successes: 1, failures: 1, avgLatencyMs: 510, estimatedCostUsd: 0.0025 },
                    { provider: 'openrouter', model: 'openai/gpt-4o-mini', successes: 2, failures: 0, avgLatencyMs: 240, estimatedCostUsd: 0.0016 },
                  ],
            },
            policies: {
              deny: 1,
              ask: second ? 0 : 1,
              allow: second ? 4 : 2,
            },
          },
        }),
      });
    });

    await loginWithToken(page, '/#/remote-observability');

    await expect(page.locator('#view-remote-observability')).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Operations' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Rollout' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Executions' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Workers' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Provider Failures' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Provider Usage' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary .agent-card h4', { hasText: 'Policy Blocks' })).toBeVisible();
    await expect(page.locator('#remote-observability-summary')).toContainText('success rate: 67%');
    await expect(page.locator('#remote-observability-summary')).toContainText('state: canary');
    await expect(page.locator('#remote-observability-summary')).toContainText('running: 1');
    await expect(page.locator('#remote-observability-summary')).toContainText('retry count: 3');
    await expect(page.locator('#remote-observability-summary')).toContainText('stale: 1');
    await expect(page.locator('#remote-observability-summary')).toContainText('requested: anthropic=1');
    await expect(page.locator('#remote-observability-summary')).toContainText('estimated cost: $0.0041');
    await expect(page.locator('#remote-observability-summary')).toContainText('top provider: anthropic');
    await expect(page.locator('#remote-observability-summary')).toContainText('top model: claude-3-7-sonnet');
    await expect(page.locator('#remote-observability-summary')).toContainText('ask: 1');
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
    await expect.poll(() => orchestratorMetricCalls).toBe(2);
    await expect(page.locator('#remote-observability-summary')).toContainText('success rate: 83%');
    await expect(page.locator('#remote-observability-summary')).toContainText('state: healthy');
    await expect(page.locator('#remote-observability-summary')).toContainText('running: 0');
    await expect(page.locator('#remote-observability-summary')).toContainText('retry count: 1');
    await expect(page.locator('#remote-observability-summary')).toContainText('stale: 0');
    await expect(page.locator('#remote-observability-summary')).toContainText('requested: none');
    await expect(page.locator('#remote-observability-summary')).toContainText('estimated cost: $0.0029');
    await expect(page.locator('#remote-observability-summary')).toContainText('top provider: openrouter');
    await expect(page.locator('#remote-observability-summary')).toContainText('top model: openai/gpt-4o-mini');
    await expect(page.locator('#remote-observability-summary')).toContainText('ask: 0');
    await expect(page.locator('#remote-observability-status')).toContainText('Updated at 2026-02-25T09:00:00Z');
    await expect(page.locator('#remote-observability-status')).toContainText('rollout=healthy');
  });
});
