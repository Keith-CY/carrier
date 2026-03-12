import type { WizardProvider } from './state';

export type Step = 'welcome' | 'setup' | 'agents' | 'provider' | 'config' | 'install' | 'complete';

export function flattenProviderCatalog(payload: any): WizardProvider[] {
  const categories = payload && payload.by_category && typeof payload.by_category === 'object' ? payload.by_category : {};
  const seen = new Set<string>();
  const providers: WizardProvider[] = [];
  Object.keys(categories).forEach((key) => {
    const items = Array.isArray(categories[key]) ? categories[key] : [];
    items.forEach((provider) => {
      const id = String(provider?.id || '').trim().toLowerCase();
      if (!id || seen.has(id)) return;
      seen.add(id);
      providers.push(provider);
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
