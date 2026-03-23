import { useState } from 'react';
import { CUSTOM_GOAL_PRESET_ID, customGoalQuickLaunchDraft, defaultQuickLaunchDraft, toggleHostSelection, type QuickLaunchDraft } from './model';

export function useQuickLaunchDraftState() {
  const [quickLaunchMessage, setQuickLaunchMessage] = useState<{ type: string; text: string }>({ type: 'info', text: '' });
  const [quickLaunchDraft, setQuickLaunchDraft] = useState<QuickLaunchDraft>(() => defaultQuickLaunchDraft());
  const [quickLaunchAdvancedVisible, setQuickLaunchAdvancedVisible] = useState(false);
  const [quickLaunchPlan, setQuickLaunchPlan] = useState<any | null>(null);

  return {
    quickLaunchMessage,
    setQuickLaunchMessage,
    quickLaunchDraft,
    quickLaunchAdvancedVisible,
    setQuickLaunchAdvancedVisible,
    quickLaunchPlan,
    setQuickLaunchPlan,
    selectQuickLaunchPreset: (presetId: string, template?: any) => setQuickLaunchDraft((current) => {
      if (current.selectedPresetId === presetId) {
        return current;
      }
      const next = {
        ...current,
        selectedPresetId: presetId,
        templateInputs: {},
        selectedHosts: [],
      };
      if (presetId === CUSTOM_GOAL_PRESET_ID) {
        return customGoalQuickLaunchDraft(current.goal);
      }
      const defaults = template?.defaultLaunchConfig && typeof template.defaultLaunchConfig === 'object' ? template.defaultLaunchConfig : {};
      const hostLabels = Array.isArray(defaults.hostLabels)
        ? defaults.hostLabels.map((value: unknown) => String(value || '').trim()).filter(Boolean).join(', ')
        : '';
      return {
        ...next,
        provider: String(defaults.provider || '').trim(),
        maxConcurrency: Number(defaults.maxConcurrency || 0) > 0 ? String(Number(defaults.maxConcurrency || 0)) : '',
        hostLabels,
      };
    }),
    setQuickLaunchGoal: (goal: string) => setQuickLaunchDraft((current) => ({ ...current, goal })),
    setQuickLaunchProvider: (provider: string) => setQuickLaunchDraft((current) => ({ ...current, provider })),
    setQuickLaunchMaxConcurrency: (maxConcurrency: string) => setQuickLaunchDraft((current) => ({ ...current, maxConcurrency })),
    setQuickLaunchHostLabels: (hostLabels: string) => setQuickLaunchDraft((current) => ({ ...current, hostLabels })),
    setQuickLaunchTemplateInput: (key: string, value: string) => setQuickLaunchDraft((current) => ({
      ...current,
      templateInputs: { ...current.templateInputs, [key]: value },
    })),
    toggleQuickLaunchHost: (hostId: string) => setQuickLaunchDraft((current) => ({
      ...current,
      selectedHosts: toggleHostSelection(current.selectedHosts, hostId),
    })),
    resetQuickLaunch: () => {
      setQuickLaunchDraft(defaultQuickLaunchDraft());
      setQuickLaunchPlan(null);
      setQuickLaunchMessage({ type: 'info', text: '' });
    },
    clearQuickLaunchPreview: () => setQuickLaunchPlan(null),
  };
}
