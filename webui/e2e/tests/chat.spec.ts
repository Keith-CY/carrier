import { test, expect } from '@playwright/test';
import { mockAPIs, loginWithToken } from './helpers';

test.describe('Chat', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPIs(page);
    await loginWithToken(page, '/#/chat');
  });

  test('send message via button', async ({ page }) => {
    await page.fill('#chat-input', 'Hello world');
    await page.click('#chat-send');

    const messages = page.locator('#chat-messages .chat-msg');
    await expect(messages.first()).toContainText('Hello world');
  });

  test('message appears in chat area', async ({ page }) => {
    await page.fill('#chat-input', 'Test message');
    await page.click('#chat-send');

    // Should see both user message and bot reply
    const messages = page.locator('#chat-messages .chat-msg');
    await expect(messages).toHaveCount(2);
    await expect(messages.nth(0)).toContainText('You:');
    await expect(messages.nth(0)).toContainText('Test message');
  });

  test('Enter key sends message', async ({ page }) => {
    await page.fill('#chat-input', 'Enter test');
    await page.press('#chat-input', 'Enter');

    const messages = page.locator('#chat-messages .chat-msg');
    await expect(messages.first()).toContainText('Enter test');
  });
});
