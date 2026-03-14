export type ApiError = Error & {
  status?: number;
  payload?: unknown;
};

export type ProviderAuthStatus = {
  id: string;
  name: string;
  authMode: string;
  envVar?: string;
  category?: string;
  configured: boolean;
  reusable: boolean;
  hasSavedCredential?: boolean;
  credentialBackend?: string;
};

export type ChannelStatus = {
  id: string;
  displayName: string;
  supportsWebhook: boolean;
  supportsPolling: boolean;
  supportsPairing: boolean;
  requiresBotToken: boolean;
  requiresWebhookSecret: boolean;
  supportsWebUI: boolean;
  supportsGatewayCmd: boolean;
  supportsProviderSetup: boolean;
  configured: boolean;
  configuredAt?: string;
};

export type ProviderAuthStatusPayload = {
  providers?: ProviderAuthStatus[];
};

export type ChannelStatusPayload = {
  channels?: ChannelStatus[];
};

function handleUnauthorized(): void {
  if (typeof window === 'undefined') return;
  window.localStorage.removeItem('carrier_token');
  window.dispatchEvent(new CustomEvent('carrier:auth-expired'));
}

function readGatewayToken(): string {
  if (typeof window === 'undefined') return '';
  return String(window.localStorage.getItem('carrier_token') || '').trim();
}

function buildHeaders(): HeadersInit {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  const token = readGatewayToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  return headers;
}

export async function apiRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
  const response = await fetch(path, {
    method,
    headers: buildHeaders(),
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 401) handleUnauthorized();
  const raw = await response.text();
  let payload: unknown = {};
  if (raw) {
    try {
      payload = JSON.parse(raw);
    } catch {
      if (!response.ok) {
        const error = new Error(raw || `Request failed (${response.status})`) as ApiError;
        error.status = response.status;
        throw error;
      }
      return raw as T;
    }
  }
  if (!response.ok) {
    const data = payload as Record<string, any>;
    const message =
      data?.message ||
      data?.errorCode ||
      data?.error?.message ||
      data?.error?.code ||
      data?.error ||
      `Request failed (${response.status})`;
    const error = new Error(String(message)) as ApiError;
    error.status = response.status;
    error.payload = payload;
    throw error;
  }
  return payload as T;
}

export async function apiGet<T>(path: string): Promise<T> {
  return apiRequest<T>('GET', path);
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return apiRequest<T>('POST', path, body);
}

export async function apiPatch<T>(path: string, body?: unknown): Promise<T> {
  return apiRequest<T>('PATCH', path, body);
}

export async function apiDelete<T>(path: string): Promise<T> {
  return apiRequest<T>('DELETE', path);
}

export async function downloadFromAPI(path: string, filename: string): Promise<void> {
  const response = await fetch(path, {
    headers: buildHeaders(),
  });
  if (response.status === 401) handleUnauthorized();
  if (!response.ok) {
    throw new Error((await response.text()) || `Request failed (${response.status})`);
  }
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
