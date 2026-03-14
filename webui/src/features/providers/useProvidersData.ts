import {
  useProviderPolicyLookups,
} from './shared';
import { useProviderMutations } from './useProviderMutations';
import { useProviderState } from './useProviderState';

export function useProvidersData() {
  const lookups = useProviderPolicyLookups('providers');
  const state = useProviderState(lookups.profiles, lookups.hosts);
  const mutations = useProviderMutations({
    profileForm: state.profileForm,
    editingProfileId: state.editingProfileId,
    bindingTargetType: state.bindingTargetType,
    bindingTargetId: state.bindingTargetId,
    bindingProfileId: state.bindingProfileId,
    profileTestHostId: state.profileTestHostId,
    previewHostId: state.previewHostId,
    previewAgentId: state.previewAgentId,
    setProfileForm: state.setProfileForm,
    setEditingProfileId: state.setEditingProfileId,
    setPreviewTextValue: state.setPreviewTextValue,
    refreshAll: lookups.refreshAll,
    setMessage: lookups.setMessage,
    hosts: lookups.hosts,
  });

  return {
    ...lookups,
    ...state,
    ...mutations,
  };
}
