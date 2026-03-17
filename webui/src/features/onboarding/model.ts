import type { ProviderAuthStatusPayload } from '../../lib/api';
import type { WizardProvider } from './state';

export type Step = 'welcome' | 'setup' | 'agents' | 'provider' | 'config' | 'install' | 'complete';

export function flattenProviderCatalog(payload: any, authStatusPayload?: ProviderAuthStatusPayload | null): WizardProvider[] {
  const categories = payload && payload.by_category && typeof payload.by_category === 'object' ? payload.by_category : {};
  const authStatuses = Array.isArray(authStatusPayload?.providers) ? authStatusPayload.providers : [];
  const authStatusByID = new Map(authStatuses.map((provider) => [String(provider.id || '').trim().toLowerCase(), provider] as const));
  const seen = new Set<string>();
  const providers: WizardProvider[] = [];
  Object.keys(categories).forEach((key) => {
    const items = Array.isArray(categories[key]) ? categories[key] : [];
    items.forEach((provider) => {
      const id = String(provider?.id || '').trim().toLowerCase();
      if (!id || seen.has(id)) return;
      seen.add(id);
      const authStatus = authStatusByID.get(id);
      providers.push({
        ...provider,
        configured: authStatus?.configured,
        reusable: authStatus?.reusable,
        hasSavedCredential: authStatus?.hasSavedCredential,
        credentialBackend: authStatus?.credentialBackend,
      });
    });
  });
  return providers.sort((left, right) => String(left?.name || left?.id || '').localeCompare(String(right?.name || right?.id || '')));
}

export function collectEnvVars(rows: Array<{ key: string; value: string }>) {
  return rows.reduce<Record<string, string>>((acc, row) => {
    const key = String(row.key || '').trim();
    if (!key) return acc;
    acc[key] = String(row.value || '');
    return acc;
  }, {});
}

export function normalizeProviderAuthMode(authMode?: string): string {
  const normalized = String(authMode || '').trim().toLowerCase();
  return normalized || 'none';
}

export function agentRequiresChannelPairing(agentId: string): boolean {
  return String(agentId || '').trim().toLowerCase() === 'picoclaw';
}

export function shouldRequireChannelPairing(agentId: string, supportsPairing: boolean): boolean {
  return agentRequiresChannelPairing(agentId) && supportsPairing;
}

export function shouldShowProviderCredentialInput(provider: WizardProvider | null, addMode: boolean): boolean {
  const authMode = normalizeProviderAuthMode(provider?.auth_mode);
  if (authMode === 'api_key') return true;
  return addMode && authMode !== 'none';
}

export function isProviderSelectionReady(args: {
  provider: WizardProvider | null;
  addMode: boolean;
  providerApiKey: string;
  reusable: boolean;
}): boolean {
  if (!args.provider) return false;
  if (args.reusable) return true;
  const authMode = normalizeProviderAuthMode(args.provider.auth_mode);
  if (args.addMode) return authMode === 'none' || !!args.providerApiKey.trim();
  if (authMode === 'api_key') return !!args.providerApiKey.trim();
  return true;
}
