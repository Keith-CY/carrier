import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

test.describe('Memory View', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
  });

  test('lists memory, searches records, and runs instance memory actions', async ({ page }) => {
    await loginWithToken(page, '/#/memory');

    await expect(page.locator('#view-memory')).toBeVisible();
    await expect(page.locator('#memory-entry-list')).toContainText('public.team.v1');
    await expect(page.locator('#memory-entry-list')).toContainText('agent-a.private.v1');
    await expect(page.locator('#memory-summary')).toContainText('entries=2');
    await expect(page.locator('#memory-summary')).toContainText('attachments=2');
    await expect(page.locator('#memory-summary')).toContainText('grants=1');

    await page.fill('#memory-search-query', 'fusion');
    await page.click('#memory-search-run');
    await expect(page.locator('#memory-search-results')).toContainText('rec-1');
    await expect(page.locator('#memory-search-results')).toContainText('fusion memory');

    await page.fill('#memory-instance-id', 'picoclaw-main');
    await page.fill('#memory-instance-scope', 'shared:profile');
    await page.click('#memory-attach');
    await expect(page.locator('#memory-action-msg')).toContainText('attached');

    await page.click('#memory-detach');
    await expect(page.locator('#memory-action-msg')).toContainText('detached');

    await page.check('#memory-distill-dry-run');
    await page.fill('#memory-distill-reason', 'promote learnings');
    await page.click('#memory-distill');
    await expect(page.locator('#memory-action-msg')).toContainText('distill-1');
    await expect(page.locator('#memory-action-msg')).toContainText('picoclaw-main');
  });
});
