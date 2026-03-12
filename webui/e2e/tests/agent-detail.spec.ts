import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

test.describe('Agent Detail', () => {
  test('renders agent status and supports start/stop actions', async ({ page }) => {
    await mockAPIs(page);
    let startCalls = 0;
    let stopCalls = 0;

    await page.route('**/api/v1/agents/agent-alpha/start', async (route) => {
      startCalls += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' });
    });
    await page.route('**/api/v1/agents/agent-alpha/stop', async (route) => {
      stopCalls += 1;
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' });
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
    await expect(page.locator('#agent-detail-content')).toContainText('"runtimeState": "running"');

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
