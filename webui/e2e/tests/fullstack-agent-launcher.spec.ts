import { expect, test } from '@playwright/test';
import { gatewayJSON, loginWithRole } from './fullstack_helpers';

test.describe.configure({ mode: 'serial' });

test.describe('Fullstack Agent Launcher', () => {
  test('shows launcher/runtime capability surfaces for a managed agent route', async ({ page, request }) => {
    const launcher = await gatewayJSON(request, 'admin', 'GET', '/api/v1/agents/picoclaw/launcher');
    expect(String(launcher.agentId || '')).toBe('picoclaw');
    expect(launcher.status).toBeTruthy();

    await loginWithRole(page, 'admin', '/agents/picoclaw');
    await expect(page.locator('#view-agent-detail')).toBeVisible();
    await expect(page.locator('#agent-detail-content')).toContainText('Agent: picoclaw');
    await expect(page.locator('#agent-detail-content')).toContainText('Runtime Capabilities');
    await expect(page.locator('#agent-detail-content')).toContainText('Skills');
    await expect(page.locator('#agent-detail-content')).toContainText('MCP Servers');
    await expect(page.locator('#agent-detail-content')).toContainText('"runtimeState"');
  });
});
