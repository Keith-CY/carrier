import type { Page } from '@playwright/test';
import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs } from './helpers';

const project = {
  id: 'proj_alpha',
  name: 'Alpha',
  sourceType: 'github',
  sourceRef: 'git@github.com:acme/alpha.git',
  defaultBranch: 'main',
  workflowPath: 'WORKFLOW.md',
  workflowDigest: 'sha256:workflow-alpha',
  state: 'ready',
  lastSyncAt: '2026-03-14T10:00:00Z',
};

const item = {
  id: 'work_bug',
  projectId: 'proj_alpha',
  title: 'Fix worker drift',
  description: 'Investigate stale worker leases in the control plane.',
  acceptance: ['Document the source of drift', 'Propose remediation steps'],
  priority: 'urgent',
  source: 'github',
  sourceRef: 'issue:12',
  labels: ['sre', 'worker'],
  state: 'running',
  latestRunId: 'run_123',
  claimedByRunId: 'run_123',
  createdAt: '2026-03-13T08:00:00Z',
  updatedAt: '2026-03-14T11:05:00Z',
};

const run = {
  id: 'run_123',
  projectId: 'proj_alpha',
  workItemId: 'work_bug',
  executionId: 'exec-running',
  workspaceId: 'ws_123',
  workspacePath: '/tmp/carrier/worktrees/run_123',
  backend: 'managed_isolated',
  phase: 'executing',
  leaseOwner: 'carrier:local',
  verificationStatus: 'pending',
  publishStatus: 'pending',
  workflowDigest: 'sha256:workflow-alpha',
  createdAt: '2026-03-14T11:00:00Z',
  updatedAt: '2026-03-14T11:06:00Z',
};

async function mockWorkAPIs(page: Page) {
  await page.route('**/api/v1/work/projects', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', projects: [project] }),
    }),
  );

  await page.route('**/api/v1/work/projects/proj_alpha', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', project }),
    }),
  );

  await page.route('**/api/v1/work/items', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', items: [item] }),
    }),
  );

  await page.route('**/api/v1/work/items/work_bug', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', item }),
    }),
  );

  await page.route('**/api/v1/work/runs', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', runs: [run] }),
    }),
  );

  await page.route('**/api/v1/work/runs/run_123', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ result: 'ok', run }),
    }),
  );
}

test.describe('Work surface', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
    await mockWorkAPIs(page);
  });

  test('navigates from the top-level Work tab into project, item, and run detail pages', async ({ page }) => {
    await loginWithToken(page, '/#/dashboard');

    await expect(page.locator('#nav')).toContainText('Work');
    await page.locator('#nav').getByRole('link', { name: 'Work', exact: true }).click();

    await expect(page).toHaveURL(/\/work$/);
    await expect(page.locator('#view-work')).toContainText('Alpha');
    await expect(page.locator('#view-work')).toContainText('Fix worker drift');
    await expect(page.locator('#view-work')).toContainText('run_123');

    await page.locator('#view-work').getByRole('link', { name: 'Alpha' }).click();
    await expect(page).toHaveURL(/\/work\/projects\/proj_alpha$/);
    await expect(page.locator('#view-work-project')).toContainText('git@github.com:acme/alpha.git');
    await expect(page.locator('#view-work-project')).toContainText('WORKFLOW.md');

    await page.locator('#view-work-project').getByRole('link', { name: 'Fix worker drift' }).click();
    await expect(page).toHaveURL(/\/work\/items\/work_bug$/);
    await expect(page.locator('#view-work-item')).toContainText('Document the source of drift');
    await expect(page.locator('#view-work-item')).toContainText('urgent');

    await page.locator('#view-work-item').getByRole('link', { name: 'run_123' }).click();
    await expect(page).toHaveURL(/\/work\/runs\/run_123$/);
    await expect(page.locator('#view-work-run')).toContainText('managed isolated');
    await expect(page.locator('#view-work-run')).toContainText('/tmp/carrier/worktrees/run_123');
    await expect(page.locator('#view-work-run').getByRole('link', { name: 'exec-running' })).toHaveAttribute('href', '/executions/exec-running');
  });
});
