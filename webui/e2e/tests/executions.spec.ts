import { expect, test } from '@playwright/test';
import { loginWithToken, mockAPIs, mockOrchestrationAPIs } from './helpers';

test.describe('Execution Center', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
  });

  test('dashboard quick launch previews plan and runs execution', async ({ page }) => {
    await loginWithToken(page, '/#/dashboard');

    await expect(page.locator('#quick-launch-goal')).toBeVisible();
    await page.fill('#quick-launch-goal', 'Investigate checkout latency and summarize next steps');
    await page.click('#quick-launch-advanced-toggle');
    await expect(page.locator('#quick-launch-provider')).toBeVisible();
    await expect(page.locator('#quick-launch-hosts')).toContainText('prod-host-1');
    await page.fill('#quick-launch-host-labels', 'gpu, prod');
    await page.fill('#quick-launch-max-concurrency', '2');

    await page.click('#quick-launch-preview');
    await expect(page.locator('#quick-launch-preview-card')).toBeVisible();
    await expect(page.locator('#quick-launch-preview-card')).toContainText('collect context');
    await expect(page.locator('#quick-launch-preview-card')).toContainText('draft summary');
    await expect(page.locator('#quick-launch-preview-card')).toContainText('labels[gpu,prod]/zeroclaw');
    await expect(page.locator('#quick-launch-preview-card')).toContainText('labels[gpu,prod]/picoclaw');

    await page.click('#quick-launch-run');
    await expect.poll(() => page.url()).toContain('#/executions/exec-preview-1');
    await expect(page.locator('#executions-detail')).toContainText('Investigate checkout latency');
    await expect(page.locator('#executions-detail')).toContainText('Provider Governance');
    await expect(page.locator('#executions-detail')).toContainText('Execution Policy');
    await expect(page.locator('#executions-detail')).toContainText('effective concurrency');
    await expect(page.locator('#executions-detail')).toContainText('openrouter');
    await expect(page.locator('#executions-detail')).toContainText('task-1');
  });

  test('dashboard quick launch supports template mode', async ({ page }) => {
    await loginWithToken(page, '/#/dashboard');

    await page.selectOption('#quick-launch-mode', 'template');
    await expect(page.locator('#quick-launch-template')).toBeVisible();
    await page.selectOption('#quick-launch-template', 'incident-diagnosis');
    await expect(page.locator('#quick-launch-template-inputs')).toContainText('Service');
    await expect(page.locator('#quick-launch-template-inputs')).toContainText('Environment');
    await expect(page.locator('#quick-launch-template-inputs')).toContainText('Incident Summary');

    await page.fill('#quick-launch-template-input-service', 'checkout');
    await page.fill('#quick-launch-template-input-environment', 'prod');
    await page.fill('#quick-launch-template-input-incidentSummary', 'latency regression after deploy');
    await page.click('#quick-launch-advanced-toggle');
    await page.fill('#quick-launch-host-labels', 'prod');

    await page.click('#quick-launch-preview');
    await expect(page.locator('#quick-launch-preview-card')).toBeVisible();
    await expect(page.locator('#quick-launch-preview-card')).toContainText('incident-diagnosis');
    await expect(page.locator('#quick-launch-preview-card')).toContainText('Analyze probable failure paths');

    await page.click('#quick-launch-run');
    await expect.poll(() => page.url()).toContain('#/executions/exec-preview-1');
    await expect(page.locator('#executions-detail')).toContainText('Diagnose incident for service checkout');
  });

  test('executions page filters and searches', async ({ page }) => {
    await loginWithToken(page, '/#/executions');

    await expect(page.locator('#executions-list .execution-card')).toHaveCount(4);
    await page.selectOption('#executions-status-filter', 'completed');
    await expect(page.locator('#executions-list')).toContainText('Prepare release notes');
    await expect(page.locator('#executions-list')).not.toContainText('Investigate checkout latency');

    await page.selectOption('#executions-status-filter', 'all');
    await page.fill('#executions-search', 'checkout');
    await expect(page.locator('#executions-list')).toContainText('Investigate checkout latency');
    await expect(page.locator('#executions-list')).not.toContainText('Prepare release notes');
  });

  test('direct execution route supports cancel', async ({ page }) => {
    await loginWithToken(page, '/#/executions/exec-running');

    await expect(page.locator('#executions-detail')).toContainText('Investigate checkout latency');
    await expect(page.locator('#executions-detail')).toContainText('Approved by');
    await expect(page.locator('#executions-detail')).toContainText('Execution Policy');
    await expect(page.locator('#executions-detail')).toContainText('tool mode');
    await expect(page.locator('#executions-detail')).toContainText('anthropic');
    page.once('dialog', (dialog) => dialog.accept());
    await page.click('#executions-cancel');
    await expect(page.locator('#executions-detail')).toContainText('cancelled');
  });

  test('execution detail can approve policy-gated execution', async ({ page }) => {
    await loginWithToken(page, '/#/executions/exec-ask');

    await expect(page.locator('#executions-detail')).toContainText('Decision: ask');
    await expect(page.locator('#executions-policy-approve')).toBeVisible();
    await page.click('#executions-policy-approve');
    await expect(page.locator('#executions-detail')).toContainText('Approved by: webui');
    await expect(page.locator('#executions-detail')).toContainText('status: provisioning');
  });

  test('execution detail shows lineage, artifacts, and derived execution actions', async ({ page }) => {
    await loginWithToken(page, '/#/executions/exec-complete');

    await expect(page.locator('#executions-detail')).toContainText('Execution Lineage');
    await expect(page.locator('#executions-detail')).toContainText('parent: exec-seed-release');
    await expect(page.locator('#executions-detail')).toContainText('launch reason: clone_execution');
    await expect(page.locator('#executions-detail')).toContainText('Outcome');
    await expect(page.locator('#executions-detail')).toContainText('Release notes draft compiled and attached.');
    await expect(page.locator('#executions-detail')).toContainText('Artifacts');
    const artifactLink = page.locator('#executions-detail a[href*="/api/v1/orchestrator/executions/exec-complete/artifacts/artifact-release-notes"]');
    await expect(artifactLink).toBeVisible();
    await expect(page.locator('#executions-rerun')).toBeVisible();
    await expect(page.locator('#executions-clone')).toBeVisible();

    await page.click('#executions-clone');
    await expect.poll(() => page.url()).toContain('#/executions/exec-derived-1');
    await expect(page.locator('#executions-detail')).toContainText('parent: exec-complete');
    await expect(page.locator('#executions-detail')).toContainText('launch reason: clone_execution');
    await expect(page.locator('#executions-detail')).toContainText('status: pending_authorization');
  });

  test('retry action creates execution from failed tasks only', async ({ page }) => {
    await loginWithToken(page, '/#/executions/exec-retryable');

    await expect(page.locator('#executions-detail')).toContainText('retryable_failed');
    await expect(page.locator('#executions-detail')).toContainText('Failure category: retryable_failed');
    await expect(page.locator('#executions-retry')).toBeVisible();

    await page.click('#executions-retry');
    await expect.poll(() => page.url()).toContain('#/executions/exec-derived-1');
    await expect(page.locator('#executions-detail')).toContainText('parent: exec-retryable');
    await expect(page.locator('#executions-detail')).toContainText('launch reason: retry_failed_tasks');
    await expect(page.locator('#executions-detail')).toContainText('collect rollout logs');
    await expect(page.locator('#executions-detail')).not.toContainText('summarize failures');
  });
});
