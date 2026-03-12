import { useState } from 'react';
import { defaultQuickLaunchDraft, toggleHostSelection, type QuickLaunchDraft, type QuickLaunchMode } from './model';

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
    setQuickLaunchMode: (mode: QuickLaunchMode) => setQuickLaunchDraft((current) => ({ ...current, mode })),
    setQuickLaunchGoal: (goal: string) => setQuickLaunchDraft((current) => ({ ...current, goal })),
    setQuickLaunchProvider: (provider: string) => setQuickLaunchDraft((current) => ({ ...current, provider })),
    setQuickLaunchMaxConcurrency: (maxConcurrency: string) => setQuickLaunchDraft((current) => ({ ...current, maxConcurrency })),
    setQuickLaunchHostLabels: (hostLabels: string) => setQuickLaunchDraft((current) => ({ ...current, hostLabels })),
    setQuickLaunchTemplateId: (templateId: string) => setQuickLaunchDraft((current) => ({ ...current, templateId, templateInputs: {} })),
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

export type QuickLaunchDraftState = ReturnType<typeof useQuickLaunchDraftState>;
