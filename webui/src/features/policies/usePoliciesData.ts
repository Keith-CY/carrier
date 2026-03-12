import {
  useProviderPolicyLookups,
} from '../providers/shared';
import { usePolicyMutations } from './usePolicyMutations';
import { usePolicyState } from './usePolicyState';

export function usePoliciesData() {
  const lookups = useProviderPolicyLookups('policies');
  const state = usePolicyState(lookups.templates);
  const mutations = usePolicyMutations({
    policyForm: state.policyForm,
    triggerForm: state.triggerForm,
    editingTriggerId: state.editingTriggerId,
    setPolicyForm: state.setPolicyForm,
    setTriggerForm: state.setTriggerForm,
    setEditingTriggerId: state.setEditingTriggerId,
    refreshAll: lookups.refreshAll,
    setMessage: lookups.setMessage,
    templates: lookups.templates,
  });

  return {
    ...lookups,
    ...state,
    ...mutations,
  };
}
