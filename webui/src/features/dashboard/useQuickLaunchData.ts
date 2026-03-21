import { CUSTOM_GOAL_PRESET_ID } from './model';
import { useQuickLaunchCatalogData } from './useQuickLaunchCatalogData';
import { useQuickLaunchDraftState } from './useQuickLaunchDraftState';
import { useQuickLaunchMutations } from './useQuickLaunchMutations';

export function useQuickLaunchData(enabled: boolean) {
  const draftState = useQuickLaunchDraftState();
  const selectedTemplateId = draftState.quickLaunchDraft.selectedPresetId === CUSTOM_GOAL_PRESET_ID
    ? ''
    : draftState.quickLaunchDraft.selectedPresetId;
  const catalogData = useQuickLaunchCatalogData(enabled, selectedTemplateId);
  const mutationData = useQuickLaunchMutations({
    quickLaunchDraft: draftState.quickLaunchDraft,
    quickLaunchPlan: draftState.quickLaunchPlan,
    setQuickLaunchPlan: draftState.setQuickLaunchPlan,
    setQuickLaunchMessage: draftState.setQuickLaunchMessage,
  });

  return {
    ...draftState,
    ...catalogData,
    ...mutationData,
  };
}
