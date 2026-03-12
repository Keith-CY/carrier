import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

test.describe('Agent Detail', () => {
  test('renders agent status and supports start/stop actions', async ({ page }) => {
    await mockAPIs(page);
    let startCalls = 0;
    let stopCalls = 0;
    let skillToggleCalls = 0;
    let skillEnabled = true;
    let mcpToggleCalls = 0;
    let mcpEnabled = true;

    await page.route('**/api/v1/agents/agent-alpha/capabilities', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          skillSummary: { installedCount: 1, enabledCount: skillEnabled ? 1 : 0, disabledCount: skillEnabled ? 0 : 1 },
          skills: [{ name: 'toolbox', enabled: skillEnabled }],
          mcp: {
            servers: [{ name: 'repo', health: mcpEnabled ? 'healthy' : 'stopped', enabled: mcpEnabled, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 }],
            visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/launcher', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          agentId: 'agent-alpha',
          status: { id: 'agent-alpha', runtimeState: 'running', health: 'healthy' },
          heartbeat: { state: 'fresh', ageSeconds: 12, lastActivityAt: '2026-03-12T03:59:48Z' },
          memory: { contractId: 'memory-alpha', contractDigest: 'sha256:abc' },
          providerReadiness: { provider: 'openrouter', authMode: 'api_key', credentialConfigured: true, ready: true },
          cron: {
            count: 2,
            nextRunAt: '2026-03-13T00:00:00Z',
            lastRunAt: '2026-03-12T23:55:00Z',
            lastResult: 'succeeded',
            jobs: [
              { id: 'cron-1', prompt: 'check launcher', nextRunAt: '2026-03-13T00:00:00Z', lastRunAt: '2026-03-12T23:55:00Z', lastResult: 'succeeded' },
              { id: 'cron-2', prompt: 'refresh heartbeat', nextRunAt: '2026-03-13T01:00:00Z', lastResult: 'scheduled' },
            ],
          },
          session: { instanceId: 'instance-1', channel: 'telegram', isolation: true, runtimeState: 'running' },
          capabilities: {
            skillSummary: { installedCount: 1, enabledCount: skillEnabled ? 1 : 0, disabledCount: skillEnabled ? 0 : 1 },
            skills: [{ name: 'toolbox', enabled: skillEnabled }],
            mcp: {
              servers: [{ name: 'repo', health: mcpEnabled ? 'healthy' : 'stopped', enabled: mcpEnabled, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 }],
              visibleTools: [{ name: 'repo_search', description: 'Search code' }],
            },
          },
        }),
      });
    });

    await page.route('**/api/v1/agents/agent-alpha/start', async (route) => {
      startCalls += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' });
    });
    await page.route('**/api/v1/agents/agent-alpha/stop', async (route) => {
      stopCalls += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' });
    });
    await page.route('**/api/v1/agents/agent-alpha/skills/toolbox', async (route) => {
      skillToggleCalls += 1;
      skillEnabled = false;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          skillSummary: { installedCount: 1, enabledCount: 0, disabledCount: 1 },
          skills: [{ name: 'toolbox', enabled: false }],
          mcp: {
            servers: [{ name: 'repo', health: 'healthy', visibleToolCount: 1, hiddenToolCount: 0 }],
            visibleTools: [{ name: 'repo_search', description: 'Search code' }],
          },
        }),
      });
    });
    await page.route('**/api/v1/agents/agent-alpha/mcp/repo', async (route) => {
      mcpToggleCalls += 1;
      mcpEnabled = false;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          mcp: {
            servers: [{ name: 'repo', health: 'stopped', enabled: false, manageable: true, visibleToolCount: 1, hiddenToolCount: 0 }],
            visibleTools: [],
          },
        }),
      });
    });

    await loginWithToken(page, '/#/agents/agent-alpha');

    await expect(page.locator('#view-agent-detail')).toBeVisible();
    await expect(page.locator('#agent-detail-content')).toContainText('Agent: agent-alpha');
    await expect(page.locator('#agent-detail-content')).toContainText('Heartbeat');
    await expect(page.locator('#agent-detail-content')).toContainText('fresh');
    await expect(page.locator('#agent-detail-content')).toContainText('openrouter');
    await expect(page.locator('#agent-detail-content')).toContainText('memory-alpha');
    await expect(page.locator('#agent-detail-content')).toContainText('Cron');
    await expect(page.locator('#agent-detail-content')).toContainText('2 job(s)');
    await expect(page.locator('#agent-detail-content')).toContainText('check launcher');
    await expect(page.locator('#agent-detail-content')).toContainText('1 installed · 1 enabled · 0 disabled');
    await expect(page.locator('#agent-detail-content')).toContainText('"runtimeState": "running"');

    await page.getByRole('button', { name: 'Disable', exact: true }).click();
    await expect.poll(() => skillToggleCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Skill toolbox disabled.');

    await page.getByRole('button', { name: 'Disable MCP' }).click();
    await expect.poll(() => mcpToggleCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('MCP server repo disabled.');

    await page.getByRole('button', { name: '▶ Start' }).evaluate((element: HTMLButtonElement) => element.click());
    await expect.poll(() => startCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Agent start requested.');

    const stopButton = page.getByRole('button', { name: '⏹ Stop' });
    await expect(stopButton).toBeEnabled();
    await stopButton.evaluate((element: HTMLButtonElement) => element.click());
    await expect.poll(() => stopCalls).toBe(1);
    await expect(page.locator('#agent-detail-content')).toContainText('Agent stop requested.');

    await page.getByRole('button', { name: '← Back' }).evaluate((element: HTMLButtonElement) => element.click());
    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.locator('.agent-card').first()).toBeVisible();
  });
});
