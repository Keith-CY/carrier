import { expect, test, type Page } from '@playwright/test';
import { loginWithToken, mockAPIs, mockOrchestrationAPIs } from './helpers';

async function captureView(page: Page, route: string, readySelector: string, screenshotName: string) {
  await mockAPIs(page);
  await loginWithToken(page, route);
  await expect(page.locator(readySelector)).toBeVisible();
  await expect(page).toHaveScreenshot(screenshotName, {
    fullPage: true,
    animations: 'disabled',
    caret: 'hide',
  });
}

test.describe('Visual Regression', () => {
  test('login page', async ({ page }) => {
    await mockAPIs(page, { healthOk: false });
    await page.goto('/');
    await expect(page.locator('#login-overlay')).toBeVisible();
    await expect(page).toHaveScreenshot('login.png', {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });

  test('welcome page', async ({ page }) => {
    await captureView(page, '/#/welcome', '#view-welcome', 'welcome.png');
  });

  test('setup page', async ({ page }) => {
    await captureView(page, '/#/setup', '#view-setup', 'setup.png');
  });

  test('dashboard page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/dashboard');
    await expect(page.locator('.agent-card')).toHaveCount(3);
    await expect(page).toHaveScreenshot('dashboard.png', {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });

  test('executions page', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
    await loginWithToken(page, '/#/executions');
    await expect(page.locator('#executions-list .execution-list-card').first()).toBeVisible();
    await expect(page).toHaveScreenshot('executions.png', {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });

  test('agent detail page', async ({ page }) => {
    await captureView(page, '/#/agents/agent-alpha', '#view-agent-detail', 'agent-detail.png');
  });

  test('memory page', async ({ page }) => {
    await captureView(page, '/#/memory', '#memory-summary', 'memory.png');
  });

  test('workers page', async ({ page }) => {
    await captureView(page, '/#/workers', '#workers-summary', 'workers.png');
  });

  test('hosts page', async ({ page }) => {
    await captureView(page, '/#/hosts', '#servers-list', 'hosts.png');
  });

  test('providers page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/providers');
    await expect(page.locator('#profiles-title')).toContainText('Providers');
    await expect(page).toHaveScreenshot('providers.png', {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });

  test('policies page', async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/policies');
    await expect(page.locator('#profiles-title')).toContainText('Policies');
    await expect(page).toHaveScreenshot('policies.png', {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });

  test('observability page', async ({ page }) => {
    await captureView(page, '/#/remote-observability', '#remote-observability-summary', 'observability.png');
  });

  test('logs page', async ({ page }) => {
    await captureView(page, '/#/logs', '#view-logs', 'logs.png');
  });

  test('chat page', async ({ page }) => {
    await captureView(page, '/#/chat', '#view-chat', 'chat.png');
  });

  test('remote chat page', async ({ page }) => {
    await captureView(page, '/#/remote-chat', '#view-remote-chat', 'remote-chat.png');
  });

  test('settings page', async ({ page }) => {
    await captureView(page, '/#/settings', '#view-settings', 'settings.png');
  });
});
