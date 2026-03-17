import { expect, test, type Page, type TestInfo } from '@playwright/test';
import { gatewayJSON, loginWithRole, uniqueSuffix } from './fullstack_helpers';

const remoteHostID = String(process.env.CARRIER_E2E_REMOTE_HOST_ID || '').trim();
const remoteHostName = String(process.env.CARRIER_E2E_REMOTE_HOST_NAME || '').trim();

test.describe.configure({ mode: 'serial' });

async function captureStep(page: Page, testInfo: TestInfo, name: string) {
  const screenshotPath = testInfo.outputPath(`${name}.png`);
  await page.screenshot({ path: screenshotPath, fullPage: true });
  await testInfo.attach(name, { path: screenshotPath, contentType: 'image/png' });
}

test.describe('Fullstack Product Walkthrough', () => {
  let remoteProfileName = '';

  test('captures the welcome-to-openclaw onboarding walkthrough', async ({ page }, testInfo) => {
    test.setTimeout(180000);

    await loginWithRole(page, 'admin', '/welcome');
    await expect(page.locator('#view-welcome')).toBeVisible();
    await expect(page.locator('#welcome-status')).toContainText('Daemon connected');
    await captureStep(page, testInfo, '00-welcome');

    await page.click('#welcome-continue');
    await expect(page.locator('#view-setup')).toBeVisible();
    await expect(page.locator('#setup-title')).toContainText('Step 1');
    await captureStep(page, testInfo, '01-setup-route');

    await loginWithRole(page, 'admin', '/add/openclaw');
    await expect(page.locator('#view-setup')).toBeVisible();
    await expect(page.locator('#setup-title')).toContainText('OpenClaw');
    await expect(page.locator('#setup-channel-summary')).toContainText('No pairing required');
    await expect(page.locator('#setup-telegram-pair')).toBeHidden();
    await captureStep(page, testInfo, '02-add-openclaw');

    await page.fill('#provider-token', 'tg-openclaw-fullstack');
    await page.click('#setup-btn');

    await expect(page.locator('#view-provider')).toBeVisible();
    await page.locator('.provider-item', { hasText: 'OpenAI Codex (OAuth)' }).click();
    await expect(page.locator('#provider-api-key')).toBeVisible();
    await expect(page.locator('#provider-next')).toBeDisabled();
    await captureStep(page, testInfo, '03-provider-step');
    await page.fill('#provider-api-key', 'codex-fullstack-token');
    await expect(page.locator('#provider-next')).toBeEnabled();
    await page.click('#provider-next');

    await expect(page.locator('#view-install')).toBeVisible();
    await page.click('#install-confirm');

    await expect(page.locator('#view-complete')).toBeVisible({ timeout: 60000 });
    await expect(page.locator('#complete-title')).toContainText('Setup Complete');
    await captureStep(page, testInfo, '04-openclaw-added');
  });

  test('installs and syncs the remote host through the real hosts UI', async ({ page, request }, testInfo) => {
    test.skip(!remoteHostID, 'remote fixture host is not configured for this run');
    test.setTimeout(720000);

    const hostsPayload = await gatewayJSON(request, 'admin', 'GET', '/api/v1/remote/hosts');
    const hosts = Array.isArray(hostsPayload.hosts) ? hostsPayload.hosts : [];
    expect(hosts.some((host: Record<string, unknown>) => String(host.id || '') === remoteHostID)).toBe(true);

    await loginWithRole(page, 'admin', '/hosts');
    await expect(page.locator('#view-servers')).toBeVisible();
    await expect(page.locator('#view-servers > .section-head')).toBeVisible();

    const hostCard = page.locator('#servers-list .agent-card', { hasText: remoteHostID }).first();
    await expect(hostCard).toBeVisible();
    await hostCard.getByRole('button', { name: 'Manage' }).click();

    await expect(page.locator('#server-manage-card')).toBeVisible();
    await page.fill('#server-manage-agent-id', 'main');
    await captureStep(page, testInfo, '05-host-manage-before-install');

    await page.click('#server-manage-install-instance');
    await expect(page.locator('#server-manage-stream-status')).toContainText('Remote installer started for main.', {
      timeout: 30000,
    });
    await captureStep(page, testInfo, '06-host-install-streaming');
    await expect(page.locator('#server-manage-msg')).toContainText('Install completed for main.', { timeout: 420000 });
    await captureStep(page, testInfo, '07-host-install-complete');

    await page.click('#server-manage-sync-instance');
    await expect(page.locator('#server-manage-msg')).toContainText('Sync completed for main.', { timeout: 60000 });
    await captureStep(page, testInfo, '08-host-sync-complete');

    const syncStatus = await gatewayJSON(
      request,
      'admin',
      'GET',
      `/api/v1/remote/hosts/${encodeURIComponent(remoteHostID)}/instances/main/sync/status`,
    );
    expect(String(syncStatus.status?.driftState || '')).toBe('in_sync');
    expect(String(syncStatus.status?.lastSyncStatus || '')).toBe('success');
  });

  test('creates a provider profile and binds it to the remote host through the real providers UI', async ({ page, request }, testInfo) => {
    test.skip(!remoteHostID, 'remote fixture host is not configured for this run');
    test.setTimeout(180000);

    const profileName = uniqueSuffix('openrouter-e2e');
    remoteProfileName = profileName;

    await loginWithRole(page, 'admin', '/providers');
    await expect(page.locator('#view-profiles')).toBeVisible();

    await page.fill('#profile-name', profileName);
    await page.fill('#profile-provider', 'openrouter');
    await page.fill('#profile-model', 'google/gemini-2.0-flash-001');
    await page.fill('#profile-base-url', 'https://openrouter.ai/api/v1');
    await page.fill('#profile-auth-ref', 'env:OPENROUTER_API_KEY');
    await page.click('#profile-save');

    const createdProfileCard = page.locator('#profiles-list .agent-card', { hasText: profileName }).first();
    await expect(createdProfileCard).toBeVisible({ timeout: 15000 });

    await page.selectOption('#binding-profile-id', { label: profileName });
    await page.selectOption('#binding-target-type', 'host');
    await page.fill('#binding-target-id', remoteHostID);
    await page.click('#binding-save');

    await expect(page.locator('#profiles-msg')).toContainText('Provider binding saved.', { timeout: 30000 });
    await expect(page.locator('#bindings-list .agent-card', { hasText: remoteHostID }).first()).toBeVisible();
    await captureStep(page, testInfo, '09-provider-binding');

    const bindingsPayload = await gatewayJSON(request, 'admin', 'GET', '/api/v1/provider-bindings');
    const bindings = Array.isArray(bindingsPayload.bindings) ? bindingsPayload.bindings : [];
    expect(
      bindings.some(
        (binding: Record<string, unknown>) =>
          String(binding.targetType || '') === 'host' && String(binding.targetId || '') === remoteHostID,
      ),
    ).toBe(true);
  });

  test('sends a real remote chat message and renders readable assistant text', async ({ page }, testInfo) => {
    test.skip(!remoteHostID, 'remote fixture host is not configured for this run');
    test.skip(!remoteProfileName, 'provider profile was not created in the prior walkthrough step');
    test.setTimeout(240000);

    await loginWithRole(page, 'admin', '/remote-chat');
    await expect(page.locator('#view-remote-chat')).toBeVisible();

    await page.selectOption('#remote-chat-target', 'remote');
    await page.selectOption('#remote-chat-host', remoteHostID);
    await page.selectOption('#remote-chat-instance', 'main');
    await page.selectOption('#remote-chat-profile', { label: remoteProfileName });

    await page.fill('#remote-chat-input', 'say exactly remote openclaw ok');
    await page.click('#remote-chat-send');

    await expect(page.locator('#remote-chat-status')).toContainText('Stream finished.', { timeout: 120000 });
    await expect(page.locator('#remote-chat-messages')).toContainText('remote openclaw ok', { timeout: 120000 });
    await expect(page.locator('#remote-chat-messages')).not.toContainText('"payloads":[]');
    await expect(page.locator('#remote-chat-messages')).not.toContainText('"systemPromptReport"');
    await captureStep(page, testInfo, '10-remote-chat-success');

    if (remoteHostName) {
      await expect(page.locator('#remote-chat-host')).toHaveValue(remoteHostID);
    }
  });
});
