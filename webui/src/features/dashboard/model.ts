export const CUSTOM_GOAL_PRESET_ID = 'custom-goal';
export const DEFAULT_QUICK_LAUNCH_TEMPLATE_ID = 'incident-diagnosis';

export type QuickLaunchDraft = {
  selectedPresetId: string;
  goal: string;
  templateInputs: Record<string, string>;
  provider: string;
  maxConcurrency: string;
  hostLabels: string;
  selectedHosts: string[];
};

export function normalizeInstances(data: any): any[] {
  if (Array.isArray(data)) return data;
  if (data && Array.isArray(data.instances)) return data.instances;
  return [];
}

export function normalizeAgentCatalog(data: any[]): any[] {
  return Array.isArray(data) ? data : [];
}

export function flattenProviderCatalog(payload: any): any[] {
  const categories = payload?.by_category && typeof payload.by_category === 'object' ? payload.by_category : {};
  const seen = new Set<string>();
  const providers: any[] = [];
  Object.keys(categories).forEach((key) => {
    const list = Array.isArray(categories[key]) ? categories[key] : [];
    list.forEach((provider: any) => {
      const id = String(provider?.id || '').trim().toLowerCase();
      if (!id || seen.has(id)) return;
      seen.add(id);
      providers.push(provider);
    });
  });
  providers.sort((left, right) => String(left?.name || left?.id || '').localeCompare(String(right?.name || right?.id || '')));
  return providers;
}

export function normalizeExecutions(payload: any): any[] {
  const executions = Array.isArray(payload?.executions) ? payload.executions : [];
  return executions.slice().sort((left, right) => {
    const a = new Date(String(left?.updatedAt || '')).getTime() || 0;
    const b = new Date(String(right?.updatedAt || '')).getTime() || 0;
    return b - a;
  });
}

export function defaultQuickLaunchDraft(): QuickLaunchDraft {
  return {
    selectedPresetId: DEFAULT_QUICK_LAUNCH_TEMPLATE_ID,
    goal: '',
    templateInputs: {},
    provider: '',
    maxConcurrency: '',
    hostLabels: '',
    selectedHosts: ['local'],
  };
}

function normalizeHostLabels(value: string): string[] {
  return [...new Set(value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean))]
    .sort((left, right) => left.localeCompare(right));
}

export function buildQuickLaunchPreviewRequest(draft: QuickLaunchDraft): { error: string } | { payload: any } {
  const templateId = draft.selectedPresetId === CUSTOM_GOAL_PRESET_ID ? '' : draft.selectedPresetId.trim();
  if (!templateId && !draft.goal.trim()) {
    return { error: 'Goal is required.' };
  }
  const hostLabels = normalizeHostLabels(draft.hostLabels);
  const maxConcurrency = parseInt(draft.maxConcurrency.trim(), 10);
  return {
    payload: {
      goal: templateId ? '' : draft.goal.trim(),
      templateId,
      inputs: templateId ? draft.templateInputs : {},
      provider: draft.provider.trim(),
      hostIds: hostLabels.length ? [] : draft.selectedHosts,
      hostLabels,
      maxConcurrency: Number.isFinite(maxConcurrency) ? maxConcurrency : 0,
    },
  };
}

export function toggleHostSelection(current: string[], hostId: string): string[] {
  if (current.includes(hostId)) {
    return current.filter((value) => value !== hostId);
  }
  return current.concat(hostId);
}
