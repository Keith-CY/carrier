import fs from 'node:fs/promises';
import path from 'node:path';
import { expect, test, type Browser, type Page, type TestInfo } from '@playwright/test';
import { gatewayJSON, loginWithRole, uniqueSuffix } from './fullstack_helpers';

const remoteHostID = String(process.env.CARRIER_E2E_REMOTE_HOST_ID || '').trim();
const remoteHostName = String(process.env.CARRIER_E2E_REMOTE_HOST_NAME || '').trim();
const remoteHostLabel = remoteHostName || remoteHostID;
const fakeProviderBaseURL = String(process.env.CARRIER_VISUAL_FAKE_PROVIDER_BASE_URL || 'http://fake-llm:8080/v1').trim();
const screenshotRoot = String(process.env.CARRIER_VISUAL_ACCEPTANCE_DIR || '').trim();

type Layout = 'desktop' | 'mobile' | 'pwa';

const viewportByLayout: Record<Layout, { width: number; height: number }> = {
  desktop: { width: 1440, height: 1280 },
  mobile: { width: 430, height: 932 },
  pwa: { width: 460, height: 920 },
};

test.describe.configure({ mode: 'serial' });

async function ensureScreenshotPath(flow: string, layout: Layout, name: string) {
  if (!screenshotRoot) {
    throw new Error('CARRIER_VISUAL_ACCEPTANCE_DIR is required for fullstack visual acceptance');
  }
  const dir = path.join(screenshotRoot, flow, layout);
  await fs.mkdir(dir, { recursive: true });
  return path.join(dir, `${name}.png`);
}

async function captureFlowStep(page: Page, testInfo: TestInfo, flow: string, layout: Layout, name: string) {
  const outputPath = await ensureScreenshotPath(flow, layout, name);
  await page.screenshot({ path: outputPath, fullPage: true });
  await testInfo.attach(`${flow}-${layout}-${name}`, { path: outputPath, contentType: 'image/png' });
}

async function setLayout(page: Page, layout: Layout) {
  await page.setViewportSize(viewportByLayout[layout]);
}

async function captureQuickEntry(page: Page, testInfo: TestInfo, flow: string, name: string) {
  await setLayout(page, 'pwa');
  await loginWithRole(page, 'admin', '/quick-entry');
  await expect(page.locator('#view-quick-entry')).toBeVisible();
  await captureFlowStep(page, testInfo, flow, 'pwa', name);
}

async function withPwaPage(browser: Browser, fn: (page: Page) => Promise<void>) {
  const context = await browser.newContext({ viewport: viewportByLayout.pwa });
  const page = await context.newPage();
  try {
    await fn(page);
  } finally {
    await context.close();
  }
}

async function installRemoteInstance(page: Page, testInfo: TestInfo, agentId: string, flow: string, prefix: string, captureDesktop = false) {
  await loginWithRole(page, 'admin', '/hosts');
  await expect(page.locator('#view-servers')).toBeVisible();

  const hostCard = page.locator('#servers-list .agent-card', { hasText: remoteHostID }).first();
  await expect(hostCard).toBeVisible();
  await hostCard.getByRole('button', { name: 'Manage' }).click();

  await expect(page.locator('#server-manage-card')).toBeVisible();
  await page.fill('#server-manage-agent-id', agentId);
  if (captureDesktop) {
    await setLayout(page, 'desktop');
    await captureFlowStep(page, testInfo, flow, 'desktop', `${prefix}-before-install`);
    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, flow, 'mobile', `${prefix}-before-install`);
    await setLayout(page, 'desktop');
  }

  await page.click('#server-manage-install-instance');
  await expect(page.locator('#server-manage-stream-status')).toContainText(`Remote installer started for ${agentId}.`, {
    timeout: 30000,
  });
  if (captureDesktop) {
    await captureFlowStep(page, testInfo, flow, 'desktop', `${prefix}-streaming`);
  }
  await expect(page.locator('#server-manage-msg')).toContainText(`Install completed for ${agentId}.`, { timeout: 420000 });
  if (captureDesktop) {
    await captureFlowStep(page, testInfo, flow, 'desktop', `${prefix}-install-complete`);
  }

  await page.click('#server-manage-sync-instance');
  await expect(page.locator('#server-manage-msg')).toContainText(`Sync completed for ${agentId}.`, { timeout: 60000 });
  if (captureDesktop) {
    await captureFlowStep(page, testInfo, flow, 'desktop', `${prefix}-sync-complete`);
    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, flow, 'mobile', `${prefix}-sync-complete`);
    await setLayout(page, 'desktop');
  }
}

test.describe('Fullstack Visual Acceptance', () => {
  test.skip(!screenshotRoot, 'visual acceptance screenshots are only captured when CARRIER_VISUAL_ACCEPTANCE_DIR is set');

  let remoteProfileName = '';
  let orchestrationExecutionId = '';

  test('captures carrier onboarding across layouts', async ({ page, browser }, testInfo) => {
    test.setTimeout(240000);

    await setLayout(page, 'desktop');
    await loginWithRole(page, 'admin', '/welcome');
    await expect(page.locator('#view-welcome')).toBeVisible();
    await expect(page.locator('#welcome-status')).toContainText('Daemon connected');
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'desktop', '00-welcome');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'mobile', '00-welcome');

    await setLayout(page, 'desktop');
    await page.click('#welcome-continue');
    await expect(page.locator('#view-setup')).toBeVisible();

    await loginWithRole(page, 'admin', '/add/openclaw');
    await expect(page.locator('#view-setup')).toBeVisible();
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'desktop', '01-add-openclaw');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'mobile', '01-add-openclaw');

    await setLayout(page, 'desktop');
    await page.fill('#provider-token', 'tg-openclaw-visual');
    await page.click('#setup-btn');

    await expect(page.locator('#view-provider')).toBeVisible();
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'desktop', '02-provider');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'mobile', '02-provider');

    await setLayout(page, 'desktop');
    await page.locator('.provider-item', { hasText: 'OpenAI Codex (OAuth)' }).click();
    await page.fill('#provider-api-key', 'codex-visual-token');
    await page.click('#provider-next');

    await expect(page.locator('#view-install')).toBeVisible();
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'desktop', '03-install');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'mobile', '03-install');

    await setLayout(page, 'desktop');
    await page.click('#install-confirm');
    await expect(page.locator('#view-complete')).toBeVisible({ timeout: 60000 });
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'desktop', '04-complete');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '01-carrier-onboarding', 'mobile', '04-complete');

    await withPwaPage(browser, async (pwaPage) => {
      await captureQuickEntry(pwaPage, testInfo, '01-carrier-onboarding', '00-open-home-handoff');
    });
  });

  test('captures openclaw install and remote runtime sync across layouts', async ({ page, browser }, testInfo) => {
    test.skip(!remoteHostID, 'remote fixture host is not configured for this run');
    test.setTimeout(720000);

    await installRemoteInstance(page, testInfo, 'main', '02-openclaw-install', '00-openclaw', true);
    await installRemoteInstance(page, testInfo, 'picoclaw', '02-openclaw-install', '01-picoclaw');
    await installRemoteInstance(page, testInfo, 'zeroclaw', '02-openclaw-install', '02-zeroclaw');

    await withPwaPage(browser, async (pwaPage) => {
      await captureQuickEntry(pwaPage, testInfo, '02-openclaw-install', '00-install-handoff');
    });
  });

  test('captures provider binding and carrier chat invoking openclaw', async ({ page, browser, request }, testInfo) => {
    test.skip(!remoteHostID, 'remote fixture host is not configured for this run');
    test.setTimeout(240000);

    remoteProfileName = uniqueSuffix('fake-openai-compatible');

    await setLayout(page, 'desktop');
    await loginWithRole(page, 'admin', '/providers');
    await expect(page.locator('#view-profiles')).toBeVisible();

    await page.fill('#profile-name', remoteProfileName);
    await page.fill('#profile-provider', 'openai-compatible');
    await page.fill('#profile-model', 'gpt-4.1-mini');
    await page.fill('#profile-base-url', fakeProviderBaseURL);
    await page.fill('#profile-auth-ref', 'env:OPENAI_COMPATIBLE_API_KEY');
    await page.click('#profile-save');

    const createdProfileCard = page.locator('#profiles-list .agent-card', { hasText: remoteProfileName }).first();
    await expect(createdProfileCard).toBeVisible({ timeout: 15000 });

    await page.selectOption('#binding-profile-id', { label: remoteProfileName });
    await page.selectOption('#binding-target-type', 'host');
    await page.fill('#binding-target-id', remoteHostID);
    await page.click('#binding-save');

    await expect(page.locator('#profiles-msg')).toContainText('Provider binding saved.', { timeout: 30000 });
    await captureFlowStep(page, testInfo, '03-carrier-chat-openclaw', 'desktop', '00-provider-binding');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '03-carrier-chat-openclaw', 'mobile', '00-provider-binding');

    const bindingsPayload = await gatewayJSON(request, 'admin', 'GET', '/api/v1/provider-bindings');
    const bindings = Array.isArray(bindingsPayload.bindings) ? bindingsPayload.bindings : [];
    expect(bindings.some((binding: Record<string, unknown>) => String(binding.targetId || '') === remoteHostID)).toBe(true);

    await setLayout(page, 'desktop');
    await loginWithRole(page, 'admin', '/remote-chat');
    await expect(page.locator('#view-remote-chat')).toBeVisible();
    await page.selectOption('#remote-chat-target', 'remote');
    await page.selectOption('#remote-chat-host', remoteHostID);
    await page.selectOption('#remote-chat-instance', 'main');
    await page.selectOption('#remote-chat-profile', { label: remoteProfileName });
    await captureFlowStep(page, testInfo, '03-carrier-chat-openclaw', 'desktop', '01-target-selected');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '03-carrier-chat-openclaw', 'mobile', '01-target-selected');

    await setLayout(page, 'desktop');
    await page.fill('#remote-chat-input', 'say exactly remote openclaw ok');
    await page.click('#remote-chat-send');
    await expect(page.locator('#remote-chat-status')).toContainText('Stream finished.', { timeout: 120000 });
    await expect(page.locator('#remote-chat-messages')).toContainText('remote openclaw ok', { timeout: 120000 });
    await captureFlowStep(page, testInfo, '03-carrier-chat-openclaw', 'desktop', '02-response');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '03-carrier-chat-openclaw', 'mobile', '02-response');

    await withPwaPage(browser, async (pwaPage) => {
      await captureQuickEntry(pwaPage, testInfo, '03-carrier-chat-openclaw', '00-open-home-handoff');
    });
  });

  test('captures multi-agent orchestration and distributed execution across layouts', async ({ page, browser }, testInfo) => {
    test.skip(!remoteHostID, 'remote fixture host is not configured for this run');
    test.skip(!remoteProfileName, 'provider profile is required for orchestration coverage');
    test.setTimeout(300000);

    await setLayout(page, 'desktop');
    await loginWithRole(page, 'admin', '/dashboard');
    await expect(page.locator('#view-dashboard')).toBeVisible();

    await page.click('#quick-launch-preset-pr-triage');
    await page.fill('#quick-launch-template-input-repository', 'Keith-CY/carrier');
    await page.fill('#quick-launch-template-input-prNumber', '139');
    await page.fill('#quick-launch-template-input-focus', 'visual acceptance');
    await page.click('#quick-launch-advanced-toggle');
    await expect(page.locator('#quick-launch-provider')).toContainText('OpenAI Codex (OAuth)');
    await page.selectOption('#quick-launch-provider', { label: 'OpenAI Codex (OAuth)' });
    const localCheckbox = page.locator('#quick-launch-hosts .quick-launch-host-option', { hasText: 'local' }).locator('input[type="checkbox"]').first();
    if (await localCheckbox.isChecked()) {
      await localCheckbox.uncheck();
    }
    const hostCheckbox = page.locator('#quick-launch-hosts .quick-launch-host-option', { hasText: remoteHostLabel }).locator('input[type="checkbox"]').first();
    if (!(await hostCheckbox.isChecked())) {
      await hostCheckbox.check();
    }
    await page.click('#quick-launch-preview');

    await expect(page.locator('#quick-launch-preview-card')).toBeVisible({ timeout: 30000 });
    await expect(page.locator('#quick-launch-preview-workers')).toContainText('picoclaw');
    await expect(page.locator('#quick-launch-preview-workers')).toContainText('zeroclaw');
    await captureFlowStep(page, testInfo, '04-multi-agent-orchestration', 'desktop', '00-plan-preview');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '04-multi-agent-orchestration', 'mobile', '00-plan-preview');

    await setLayout(page, 'desktop');
    await page.click('#quick-launch-run');
    await page.waitForURL(/\/executions\/.+$/, { timeout: 60000 });
    orchestrationExecutionId = decodeURIComponent(page.url().split('/executions/')[1] || '').trim();
    await expect(page.locator('#view-executions')).toBeVisible();
    await expect(page.locator('#executions-detail')).toContainText('Execution Policy', { timeout: 120000 });
    await captureFlowStep(page, testInfo, '04-multi-agent-orchestration', 'desktop', '01-execution-detail');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '04-multi-agent-orchestration', 'mobile', '01-execution-detail');

    await withPwaPage(browser, async (pwaPage) => {
      await captureQuickEntry(pwaPage, testInfo, '04-multi-agent-orchestration', '00-active-work');
    });
  });

  test('captures supplemental surfaces across layouts', async ({ page, browser }, testInfo) => {
    test.setTimeout(180000);

    await setLayout(page, 'desktop');
    await loginWithRole(page, 'admin', '/activity');
    await expect(page.locator('#view-activity')).toBeVisible();
    if (orchestrationExecutionId) {
      await expect(page.locator('body')).toContainText(orchestrationExecutionId);
    }
    await captureFlowStep(page, testInfo, '05-supplemental', 'desktop', '00-activity');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '05-supplemental', 'mobile', '00-activity');

    await setLayout(page, 'desktop');
    await loginWithRole(page, 'admin', '/settings');
    await expect(page.locator('#view-settings')).toBeVisible();
    await captureFlowStep(page, testInfo, '05-supplemental', 'desktop', '01-settings');

    await setLayout(page, 'mobile');
    await captureFlowStep(page, testInfo, '05-supplemental', 'mobile', '01-settings');

    await withPwaPage(browser, async (pwaPage) => {
      await captureQuickEntry(pwaPage, testInfo, '05-supplemental', '00-inbox');
    });
  });
});
