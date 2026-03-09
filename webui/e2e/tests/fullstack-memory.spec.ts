import { expect, test } from '@playwright/test';
import { daemonJSON, loginWithRole, uniqueSuffix } from './fullstack_helpers';

test.describe.configure({ mode: 'serial' });

test.describe('Fullstack Memory Control Plane', () => {
  test('lists seeded memory, searches records, and runs real instance memory actions', async ({ page, request }) => {
    const subject = uniqueSuffix('memory-agent');
    const searchPhrase = uniqueSuffix('fusion-memory');

    await daemonJSON(request, 'POST', '/api/v2/memory/records/upsert', {
      subject,
      scope: `agent:${subject}`,
      type: 'fact',
      contentSummary: `Observed ${searchPhrase} during fullstack memory test`,
      provenance: 'e2e-fullstack',
    });
    await daemonJSON(request, 'POST', '/api/v2/memory/grants/grant', {
      subject,
      scope: 'shared:profile',
      grantedBy: 'e2e-fullstack',
      reason: 'enable memory attachment flow',
    });

    await loginWithRole(page, 'operator', '/#/memory');
    await page.fill('#memory-subject', subject);
    await page.click('#memory-refresh');

    await expect(page.locator('#memory-summary')).toContainText(`subject=${subject}`);
    await expect(page.locator('#memory-summary')).toContainText('grants=1');
    await expect(page.locator('#memory-entry-list')).toContainText('shared:profile');

    await page.fill('#memory-search-query', searchPhrase);
    await page.click('#memory-search-run');
    await expect(page.locator('#memory-search-results')).toContainText(searchPhrase);

    await page.fill('#memory-instance-id', subject);
    await page.fill('#memory-instance-scope', 'shared:profile');
    await page.click('#memory-attach');
    await expect(page.locator('#memory-action-msg')).toContainText('attached');
    const attachedSnapshot = await daemonJSON(request, 'GET', `/api/v2/memory?subject=${encodeURIComponent(subject)}`);
    expect(attachedSnapshot.instanceScopes || []).toContain('shared:profile');

    await page.click('#memory-detach');
    await expect(page.locator('#memory-action-msg')).toContainText('detached');
    const detachedSnapshot = await daemonJSON(request, 'GET', `/api/v2/memory?subject=${encodeURIComponent(subject)}`);
    expect(detachedSnapshot.instanceScopes || []).not.toContain('shared:profile');

    await page.check('#memory-distill-dry-run');
    await page.fill('#memory-distill-reason', 'promote e2e memory learnings');
    await page.click('#memory-distill');
    await expect(page.locator('#memory-action-msg')).toContainText('distill');
    await expect(page.locator('#memory-action-msg')).toContainText(subject);
  });
});
