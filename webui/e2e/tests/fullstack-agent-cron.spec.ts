import { expect, test } from '@playwright/test';
import { gatewayFetch, gatewayJSON, loginWithRole, uniqueSuffix } from './fullstack_helpers';

test.describe.configure({ mode: 'serial' });

test.describe('Fullstack Agent Cron UX', () => {
  test('creates, lists, surfaces, and cancels managed-agent cron jobs', async ({ page, request }) => {
    const nextRunAt = new Date(Date.now() + 60 * 60 * 1000).toISOString();
    const prompt = 'cron smoke ' + uniqueSuffix('picoclaw');

    const scheduleResponse = await gatewayFetch(request, 'admin', '/api/v1/agents/picoclaw/cron', {
      method: 'POST',
      data: {
        message: prompt,
        provider: 'openrouter',
        sessionId: 'cron-ui-smoke',
        nextRunAt,
      },
    });
    const scheduleText = await scheduleResponse.text();
    expect(scheduleResponse.status(), scheduleText).toBe(200);
    const scheduled = JSON.parse(scheduleText);
    const jobID = String(scheduled.id || '').trim();
    expect(jobID).not.toBe('');
    expect(String(scheduled.lastResult || '')).toBe('scheduled');

    const listed = await gatewayJSON(request, 'admin', 'GET', '/api/v1/agents/picoclaw/cron');
    const jobs = Array.isArray(listed.jobs) ? listed.jobs : [];
    expect(jobs.some((job: Record<string, unknown>) => String(job.id || '') === jobID)).toBe(true);

    await loginWithRole(page, 'admin', '/agents/picoclaw');
    await expect(page.locator('#agent-detail-content')).toContainText('Cron');
    await expect(page.locator('#agent-detail-content')).toContainText(prompt);
    await expect(page.locator('#agent-detail-content')).toContainText(jobID);

    const cancelled = await gatewayJSON(request, 'admin', 'POST', '/api/v1/agents/picoclaw/cron/' + encodeURIComponent(jobID) + '/cancel');
    expect(String(cancelled.id || '')).toBe(jobID);
    expect(String(cancelled.lastResult || '')).toBe('cancelled');
  });
});
