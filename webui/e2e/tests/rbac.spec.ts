import { expect, test } from '@playwright/test';
import { ADMIN_AUTHZ, loginWithToken, mockAPIs, mockOrchestrationAPIs } from './helpers';

function authzFor(role: 'viewer' | 'operator' | 'approver' | 'admin') {
  const permissionsByRole = {
    viewer: {
      viewExecutions: true,
      launchExecutions: false,
      approveExecutions: false,
      managePolicies: false,
      manageProviders: false,
      manageHosts: false,
    },
    operator: {
      viewExecutions: true,
      launchExecutions: true,
      approveExecutions: false,
      managePolicies: false,
      manageProviders: false,
      manageHosts: false,
    },
    approver: {
      viewExecutions: true,
      launchExecutions: false,
      approveExecutions: true,
      managePolicies: false,
      manageProviders: false,
      manageHosts: false,
    },
    admin: ADMIN_AUTHZ.permissions,
  } as const;

  return {
    role,
    permissions: permissionsByRole[role],
  };
}

async function mockFeaturesRole(page, role: 'viewer' | 'operator' | 'approver' | 'admin') {
  await page.route('**/api/v1/features', async (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        result: 'ok',
        features: {
          remoteControlPlaneEnabled: true,
          remoteChatEnabled: true,
          providerBindingEnabled: true,
        },
        authz: authzFor(role),
      }),
    }),
  );
}

async function navigateHash(page, hash: string) {
  await page.evaluate((nextHash: string) => {
    window.location.hash = nextHash;
  }, hash);
}

async function openExecutionFromList(page, executionId: string) {
  await navigateHash(page, '#/executions');
  const card = page.locator('#executions-list .execution-list-card').filter({ hasText: executionId }).first();
  await expect(card).toBeVisible();
  await card.click();
}

test.describe('RBAC UI', () => {
  test('viewer sees execution details but not mutating actions', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
    await mockFeaturesRole(page, 'viewer');

    await loginWithToken(page, '/#/dashboard');

    await expect(page.locator('#dashboard-quick-launch-section')).toBeHidden();

    await openExecutionFromList(page, 'exec-running');
    await expect(page.locator('#executions-detail')).toContainText('Investigate checkout latency');
    await expect(page.locator('#executions-cancel')).toBeHidden();
    await expect(page.locator('#executions-rerun')).toBeHidden();
    await expect(page.locator('#executions-clone')).toBeHidden();

    await openExecutionFromList(page, 'exec-ask');
    await expect(page.locator('#executions-policy-approve')).toBeHidden();
  });

  test('operator can launch executions but gets read-only provider and policy controls', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
    await mockFeaturesRole(page, 'operator');

    await loginWithToken(page, '/#/dashboard');

    await expect(page.locator('#dashboard-quick-launch-section')).toBeVisible();

    await openExecutionFromList(page, 'exec-running');
    await expect(page.locator('#executions-cancel')).toBeVisible();
    await expect(page.locator('#executions-policy-approve')).toBeHidden();

    await navigateHash(page, '#/profiles');
    await expect(page.locator('#profiles-msg')).toContainText('read-only access');
    await expect(page.locator('#profile-save')).toBeDisabled();
    await expect(page.locator('#binding-save')).toBeDisabled();
    await expect(page.locator('#execution-policy-save')).toBeDisabled();
    await expect(page.locator('#profiles-list')).not.toContainText('Delete');
    await expect(page.locator('#bindings-list')).not.toContainText('Delete');
    await expect(page.locator('#execution-policies-list')).not.toContainText('Delete');
  });

  test('approver can approve policy-gated executions but cannot launch or manage', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
    await mockFeaturesRole(page, 'approver');

    await loginWithToken(page, '/#/dashboard');

    await expect(page.locator('#dashboard-quick-launch-section')).toBeHidden();

    await openExecutionFromList(page, 'exec-ask');
    await expect(page.locator('#executions-policy-approve')).toBeVisible();
    await expect(page.locator('#executions-cancel')).toBeHidden();

    await navigateHash(page, '#/servers');
    await expect(page.locator('#server-save')).toBeDisabled();
    await expect(page.locator('#servers-msg')).toContainText('cannot modify remote hosts');
    await expect(page.locator('#servers-list')).not.toContainText('Delete');

    await navigateHash(page, '#/profiles');
    await expect(page.locator('#profile-save')).toBeDisabled();
    await expect(page.locator('#execution-policy-save')).toBeDisabled();
  });

  test('admin keeps full control-plane actions', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
    await mockFeaturesRole(page, 'admin');

    await loginWithToken(page, '/#/profiles');

    await expect(page.locator('#profile-save')).toBeEnabled();
    await expect(page.locator('#binding-save')).toBeEnabled();
    await expect(page.locator('#execution-policy-save')).toBeEnabled();
    await expect(page.locator('#profiles-list')).toContainText('Delete');

    await navigateHash(page, '#/servers');
    await expect(page.locator('#server-save')).toBeEnabled();
    await expect(page.locator('#servers-list')).toContainText('Delete');
  });
});
