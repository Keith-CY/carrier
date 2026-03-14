import { APIRequestContext, Page, expect } from '@playwright/test';
import { normalizeTestRoute, pushHistoryRoute } from './helpers';

export type TestRole = 'viewer' | 'operator' | 'approver' | 'admin';

export const gatewayBaseURL = process.env.CARRIER_E2E_BASE_URL || 'http://127.0.0.1:8787';
export const daemonBaseURL = process.env.CARRIER_E2E_DAEMON_URL || 'http://127.0.0.1:9090';

const roleTokens: Record<TestRole, string> = {
  viewer: process.env.CARRIER_E2E_VIEWER_TOKEN || 'viewer-token',
  operator: process.env.CARRIER_E2E_OPERATOR_TOKEN || 'operator-token',
  approver: process.env.CARRIER_E2E_APPROVER_TOKEN || 'approver-token',
  admin: process.env.CARRIER_E2E_ADMIN_TOKEN || 'admin-token',
};

export function authHeaders(role: TestRole): Record<string, string> {
  return { Authorization: 'Bearer ' + roleTokens[role] };
}

export async function loginWithRole(page: Page, role: TestRole, url = '/dashboard', waitUntil: 'load' | 'domcontentloaded' | 'commit' = 'commit') {
  const token = roleTokens[role];
  await page.addInitScript((nextToken: string) => {
    localStorage.setItem('carrier_token', nextToken);
  }, token);
  const targetPath = normalizeTestRoute(url);
  await page.goto('/', { waitUntil });
  await page.locator('#logout-btn').waitFor({ state: 'visible' });
  if (targetPath !== '/') {
    await pushHistoryRoute(page, targetPath);
  }
  await page.waitForFunction((expectedPath: string) => {
    const nav = document.querySelector('#nav');
    const current = window.location.pathname || '/';
    return current === expectedPath || (!!nav && !nav.classList.contains('hidden'));
  }, targetPath);
}

export async function gatewayFetch(request: APIRequestContext, role: TestRole, path: string, init: Record<string, unknown> = {}) {
  const headers = {
    ...authHeaders(role),
    ...((init.headers as Record<string, string> | undefined) || {}),
  };
  return request.fetch(path, {
    ...init,
    headers,
  });
}

export async function gatewayJSON(request: APIRequestContext, role: TestRole, method: string, path: string, body?: unknown, expectedStatus: number | number[] = 200) {
  const response = await gatewayFetch(request, role, path, {
    method,
    data: body,
  });
  const allowed = Array.isArray(expectedStatus) ? expectedStatus : [expectedStatus];
  expect(allowed, `${method} ${path} status`).toContain(response.status());
  const text = await response.text();
  if (!text.trim()) return {};
  return JSON.parse(text);
}

export async function daemonJSON(request: APIRequestContext, method: string, path: string, body?: unknown, expectedStatus: number | number[] = 200) {
  const response = await request.fetch(daemonBaseURL + path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    data: body,
  });
  const allowed = Array.isArray(expectedStatus) ? expectedStatus : [expectedStatus];
  expect(allowed, `${method} ${path} status`).toContain(response.status());
  const text = await response.text();
  if (!text.trim()) return {};
  return JSON.parse(text);
}

export function uniqueSuffix(prefix: string): string {
  const tail = Math.random().toString(36).slice(2, 8);
  return `${prefix}-${Date.now()}-${tail}`;
}

export async function waitForExecutionByID(request: APIRequestContext, role: TestRole, executionID: string, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const response = await gatewayFetch(request, role, '/api/v1/orchestrator/executions/' + encodeURIComponent(executionID), { method: 'GET' });
    if (response.status() === 200) {
      return JSON.parse(await response.text());
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`timed out waiting for execution ${executionID}`);
}
