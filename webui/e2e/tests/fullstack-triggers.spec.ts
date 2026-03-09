import { expect, test } from '@playwright/test';
import { gatewayFetch, gatewayJSON, loginWithRole, uniqueSuffix, waitForExecutionByID } from './fullstack_helpers';

test.describe.configure({ mode: 'serial' });

test.describe('Fullstack Trigger Runtime', () => {
  test('creates a webhook trigger in the UI and launches a real execution', async ({ page, request }) => {
    const suffix = uniqueSuffix('trigger');
    const triggerName = 'incident webhook ' + suffix;
    const triggerSecret = 'secret-' + suffix;

    await loginWithRole(page, 'admin', '/#/policies');
    await page.fill('#trigger-name', triggerName);
    await page.selectOption('#trigger-type', 'webhook');
    await page.selectOption('#trigger-template-id', 'pr-triage');
    await page.fill('#trigger-provider', 'openrouter');
    await page.fill('#trigger-host-ids', 'local');
    await page.fill('#trigger-webhook-secret', triggerSecret);
    await page.fill('#trigger-inputs', 'repository={{payload.repository}}\nprNumber={{payload.prNumber}}\nfocus={{payload.focus}}');
    await page.click('#trigger-save');

    await expect(page.locator('#profiles-msg')).toContainText('Execution trigger saved.');
    await expect(page.locator('#execution-triggers-list')).toContainText(triggerName);

    const triggersPayload = await gatewayJSON(request, 'admin', 'GET', '/api/v1/triggers');
    const trigger = (triggersPayload.triggers || []).find((item: Record<string, unknown>) => String(item.name || '') === triggerName);
    expect(trigger).toBeTruthy();
    const triggerID = String(trigger.id || '').trim();
    expect(triggerID).not.toBe('');

    const fireResponse = await request.fetch('/api/v1/triggers/webhook/' + encodeURIComponent(triggerID), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Carrier-Trigger-Secret': triggerSecret,
      },
      data: {
        repository: 'Keith-CY/carrier',
        prNumber: '1554',
        focus: 'control-plane regression risk',
      },
    });
    expect(fireResponse.status()).toBe(202);
    const firePayload = JSON.parse(await fireResponse.text());
    expect(firePayload.triggered).toBe(true);
    const executionID = String(firePayload.execution?.id || '').trim();
    expect(executionID).not.toBe('');

    await waitForExecutionByID(request, 'admin', executionID);

    await loginWithRole(page, 'admin', '/#/executions/' + encodeURIComponent(executionID));
    await expect(page.locator('#executions-detail')).toContainText('Trigger');
    await expect(page.locator('#executions-detail')).toContainText('source: webhook');
    await expect(page.locator('#executions-detail')).toContainText('id: ' + triggerID);
    await expect(page.locator('#executions-detail')).toContainText('event: webhook');
    await expect(page.locator('#executions-detail')).toContainText('initiator: webhook:' + triggerID);
    await expect(page.locator('#executions-detail')).toContainText('Memory contract:');
    await expect(page.locator('#executions-detail')).toContainText('shared:code-review');
    await expect(page.locator('#executions-detail')).toContainText('shared:pull-requests');
    await expect(page.locator('#executions-detail')).toContainText('Distill outputs: shared:pr-lessons');
    await expect(page.locator('#executions-detail')).toContainText('Requested provider: openrouter');
    await expect(page.locator('#executions-detail')).toContainText('status=unbound');

    const deleteResponse = await gatewayFetch(request, 'admin', '/api/v1/triggers/' + encodeURIComponent(triggerID), { method: 'DELETE' });
    expect(deleteResponse.status()).toBe(200);
  });
});
