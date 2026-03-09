import { expect, test } from '@playwright/test';
import { gatewayFetch, gatewayJSON, loginWithRole, uniqueSuffix } from './fullstack_helpers';

test.describe.configure({ mode: 'serial' });

test.describe('Fullstack RBAC and Policy Enforcement', () => {
  test('enforces role permissions and policy approval on real gateway APIs', async ({ page, request }) => {
    const policyName = uniqueSuffix('rbac-ask');
    const policyPayload = await gatewayJSON(request, 'admin', 'POST', '/api/v1/orchestrator/policies', {
      name: policyName,
      action: 'ask',
      reason: 'explicit approver review required for openrouter runs',
      requestedProviders: ['openrouter'],
      enabled: true,
      priority: 500,
    });
    const policyID = String(policyPayload.policy?.id || '').trim();
    expect(policyID).not.toBe('');

    try {
      const viewerPolicyCreate = await gatewayFetch(request, 'viewer', '/api/v1/orchestrator/policies', {
        method: 'POST',
        data: { name: uniqueSuffix('viewer-denied'), action: 'allow' },
      });
      expect(viewerPolicyCreate.status()).toBe(403);

      const operatorPolicyCreate = await gatewayFetch(request, 'operator', '/api/v1/orchestrator/policies', {
        method: 'POST',
        data: { name: uniqueSuffix('operator-denied'), action: 'allow' },
      });
      expect(operatorPolicyCreate.status()).toBe(403);

      const executionBody = {
        goal: 'Collect control-plane audit context',
        requestedProvider: 'openrouter',
        requiredWorkers: [{ hostId: 'local', agentId: 'zeroclaw', count: 1 }],
        taskUnits: [{ id: 'task-1', input: 'collect control-plane audit context', hostId: 'local', agentId: 'zeroclaw' }],
      };

      const viewerExecutionCreate = await gatewayFetch(request, 'viewer', '/api/v1/orchestrator/executions', {
        method: 'POST',
        data: executionBody,
      });
      expect(viewerExecutionCreate.status()).toBe(403);

      const operatorExecutionPayload = await gatewayJSON(request, 'operator', 'POST', '/api/v1/orchestrator/executions', executionBody, 201);
      const executionID = String(operatorExecutionPayload.execution?.id || '').trim();
      expect(executionID).not.toBe('');
      expect(String(operatorExecutionPayload.execution?.policy?.decision || '')).toBe('ask');

      const operatorAuthorize = await gatewayFetch(request, 'operator', '/api/v1/orchestrator/executions/' + encodeURIComponent(executionID) + '/authorize', {
        method: 'POST',
        data: { approved: true, actor: 'operator-ui' },
      });
      expect(operatorAuthorize.status()).toBe(403);

      const approverBlocked = await gatewayFetch(request, 'approver', '/api/v1/orchestrator/executions/' + encodeURIComponent(executionID) + '/authorize', {
        method: 'POST',
        data: { approved: true, actor: 'approver-ui' },
      });
      expect(approverBlocked.status()).toBe(409);

      const approverAuthorizedPayload = await gatewayJSON(
        request,
        'approver',
        'POST',
        '/api/v1/orchestrator/executions/' + encodeURIComponent(executionID) + '/authorize',
        { approved: true, actor: 'approver-ui', policyApproved: true },
        [200, 202],
      );
      expect(String(approverAuthorizedPayload.execution?.policy?.approvedBy || '')).toBe('approver-ui');

      await loginWithRole(page, 'viewer', '/#/memory');
      await expect(page.locator('#memory-attach')).toBeDisabled();
      await expect(page.locator('#memory-detach')).toBeDisabled();
      await expect(page.locator('#memory-distill')).toBeDisabled();

      await loginWithRole(page, 'admin', '/#/policies');
      await expect(page.locator('#execution-policy-save')).toBeEnabled();
      await expect(page.locator('#trigger-save')).toBeEnabled();
    } finally {
      if (policyID) {
        await gatewayFetch(request, 'admin', '/api/v1/orchestrator/policies/' + encodeURIComponent(policyID), { method: 'DELETE' });
      }
    }
  });
});
