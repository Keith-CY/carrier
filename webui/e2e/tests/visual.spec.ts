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

  test('onboarding page', async ({ page }) => {
    await captureView(page, '/onboarding', '#view-onboarding', 'onboarding.png');
  });

  test('home page', async ({ page }) => {
    await captureView(page, '/home', '#view-home', 'home.png');
  });

  test('quick entry desktop page', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
    await loginWithToken(page, '/quick-entry');
    await expect(page.locator('#view-quick-entry')).toBeVisible();
    await expect(page).toHaveScreenshot('quick-entry-desktop.png', {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });

  test('quick entry mobile page', async ({ page }) => {
    await page.setViewportSize({ width: 430, height: 932 });
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
    await loginWithToken(page, '/quick-entry');
    await expect(page.locator('#view-quick-entry')).toBeVisible();
    await expect(page).toHaveScreenshot('quick-entry-mobile.png', {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });

  test('projects page', async ({ page }) => {
    await captureView(page, '/projects', '#view-projects', 'projects.png');
  });

  test('project detail page', async ({ page }) => {
    await captureView(page, '/projects/proj_alpha', '#view-project-detail', 'project-detail.png');
  });

  test('agent detail page', async ({ page }) => {
    await captureView(page, '/agents/agent-alpha', '#view-agent-detail', 'agent-detail.png');
  });

  test('agents page', async ({ page }) => {
    await captureView(page, '/agents', '#view-agents', 'agents.png');
  });

  test('activity page', async ({ page }) => {
    await mockAPIs(page);
    await mockOrchestrationAPIs(page);
    await loginWithToken(page, '/activity');
    await expect(page.locator('#view-activity')).toBeVisible();
    await expect(page).toHaveScreenshot('activity.png', {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });

  test('settings page', async ({ page }) => {
    await captureView(page, '/settings', '#view-settings', 'settings.png');
  });
});
