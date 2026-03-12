import { useQuickLaunchCatalogData } from './useQuickLaunchCatalogData';
import { useQuickLaunchDraftState } from './useQuickLaunchDraftState';
import { useQuickLaunchMutations } from './useQuickLaunchMutations';

export function useQuickLaunchData(enabled: boolean) {
  const draftState = useQuickLaunchDraftState();
  const catalogData = useQuickLaunchCatalogData(enabled, draftState.quickLaunchDraft.templateId);
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
