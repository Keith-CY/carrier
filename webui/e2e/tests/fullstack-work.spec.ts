import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { expect, test } from '@playwright/test';
import { gatewayJSON, loginWithRole, uniqueSuffix } from './fullstack_helpers';

test.describe.configure({ mode: 'serial' });

function createWorkTestRepo(name: string): string {
  const repoDir = mkdtempSync(path.join(tmpdir(), `${name}-`));
  writeFileSync(path.join(repoDir, 'WORKFLOW.md'), '# Workflow\n\n- verify with go test ./...\n', 'utf8');
  writeFileSync(path.join(repoDir, 'README.md'), '# Test Repo\n', 'utf8');
  execFileSync('git', ['init', '-b', 'main'], { cwd: repoDir });
  execFileSync('git', ['config', 'user.name', 'Carrier E2E'], { cwd: repoDir });
  execFileSync('git', ['config', 'user.email', 'carrier-e2e@example.com'], { cwd: repoDir });
  execFileSync('git', ['add', 'WORKFLOW.md', 'README.md'], { cwd: repoDir });
  execFileSync('git', ['commit', '-m', 'init'], { cwd: repoDir });
  return repoDir;
}

test.describe('Fullstack Work-Oriented Orchestration', () => {
  test('creates a local work item, starts a run, exports evidence, and cleans up the workspace', async ({ page, request }) => {
    const repoDir = createWorkTestRepo(uniqueSuffix('carrier-work'));
    test.info().attach('work-repo', { body: repoDir, contentType: 'text/plain' });
    try {
      const projectID = uniqueSuffix('proj').replace(/[^a-zA-Z0-9_-]/g, '_');
      const workItemID = uniqueSuffix('work').replace(/[^a-zA-Z0-9_-]/g, '_');

      await gatewayJSON(request, 'admin', 'POST', '/api/v1/work/projects', {
        id: projectID,
        name: 'Work E2E Project',
        sourceType: 'local',
        sourceRef: repoDir,
        defaultBranch: 'main',
        workflowPath: 'WORKFLOW.md',
      }, 201);
      const projectPayload = await gatewayJSON(request, 'admin', 'POST', `/api/v1/work/projects/${encodeURIComponent(projectID)}/sync`);
      expect(String(projectPayload.project?.workflowDigest || '')).toContain('sha256:');

      const itemPayload = await gatewayJSON(request, 'admin', 'POST', '/api/v1/work/items', {
        id: workItemID,
        projectId: projectID,
        title: 'Investigate worker drift',
        description: 'Verify work -> run -> execution linkage.',
        acceptance: ['Record work snapshots in evidence', 'Allow workspace cleanup'],
        priority: 'high',
      }, 201);
      expect(String(itemPayload.item?.id || '')).toBe(workItemID);

      const runPayload = await gatewayJSON(request, 'admin', 'POST', `/api/v1/work/items/${encodeURIComponent(workItemID)}/runs`, {
        backend: 'local_sandboxed',
      }, 201);
      const run = runPayload.run || {};
      expect(String(run.id || '')).toContain('run_');
      expect(String(run.executionId || '')).toContain('-');
      expect(existsSync(String(run.workspacePath || ''))).toBe(true);

      const evidencePayload = await gatewayJSON(request, 'admin', 'GET', `/api/v1/orchestrator/executions/${encodeURIComponent(String(run.executionId))}/evidence?format=json`);
      expect(String(evidencePayload.evidence?.workItemSnapshot?.id || '')).toBe(workItemID);
      expect(String(evidencePayload.evidence?.runSnapshot?.id || '')).toBe(String(run.id));
      expect(String(evidencePayload.evidence?.workspaceManifest?.workspacePath || '')).toBe(String(run.workspacePath || ''));

      await loginWithRole(page, 'admin', `/work/runs/${encodeURIComponent(String(run.id))}`);
      await expect(page.locator('#view-work-run')).toContainText(String(run.id));
      await expect(page.locator('#view-work-run')).toContainText('Open Execution');
      await expect(page.locator('#view-work-run')).toContainText('Open Evidence');

      await page.locator('#work-run-open-execution').click();
      await expect(page.locator('#view-executions')).toContainText('Work Context');
      await expect(page.locator('#view-executions')).toContainText(workItemID);

      await loginWithRole(page, 'admin', `/work/runs/${encodeURIComponent(String(run.id))}`);
      await page.locator('#work-run-reclaim').click();
      await expect(page.locator('#view-work-run')).toContainText('executing');

      await page.locator('#work-run-cleanup').click();
      await expect
        .poll(() => existsSync(String(run.workspacePath || '')), { timeout: 10000 })
        .toBe(false);
    } finally {
      rmSync(repoDir, { recursive: true, force: true });
    }
  });
});
