import { useMutation } from '@tanstack/react-query';
import { apiDelete, apiGet, apiPatch, apiPost } from '../../lib/api';
import {
  EMPTY_PROFILE_FORM,
  previewText,
  type MessageState,
  type ProfileFormState,
} from './shared';

type UseProviderMutationsArgs = {
  profileForm: ProfileFormState;
  editingProfileId: string;
  bindingTargetType: string;
  bindingTargetId: string;
  bindingProfileId: string;
  profileTestHostId: string;
  previewHostId: string;
  previewAgentId: string;
  setProfileForm: (value: ProfileFormState) => void;
  setEditingProfileId: (value: string) => void;
  setPreviewTextValue: (value: string) => void;
  refreshAll: () => Promise<void>;
  setMessage: (value: MessageState) => void;
  hosts: Array<{ id?: unknown }>;
};

export function useProviderMutations({
  profileForm,
  editingProfileId,
  bindingTargetType,
  bindingTargetId,
  bindingProfileId,
  profileTestHostId,
  previewHostId,
  previewAgentId,
  setProfileForm,
  setEditingProfileId,
  setPreviewTextValue,
  refreshAll,
  setMessage,
  hosts,
}: UseProviderMutationsArgs) {
  const saveProfileMutation = useMutation({
    mutationFn: async () => {
      const payload = {
        name: profileForm.name.trim(),
        provider: profileForm.provider.trim(),
        model: profileForm.model.trim(),
        baseUrl: profileForm.baseUrl.trim(),
        authRef: profileForm.authRef.trim(),
        enabled: profileForm.enabled === 'true',
      };
      if (editingProfileId) {
        return apiPatch<any>(`/api/v1/provider-profiles/${encodeURIComponent(editingProfileId)}`, payload);
      }
      return apiPost<any>('/api/v1/provider-profiles', payload);
    },
    onSuccess: async () => {
      await refreshAll();
      setProfileForm(EMPTY_PROFILE_FORM);
      setEditingProfileId('');
      setMessage({ type: 'success', text: 'Provider profile saved.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Provider profile save failed: ${error.message}` }),
  });

  const deleteProfileMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/provider-profiles/${encodeURIComponent(id)}`),
    onSuccess: async () => {
      await refreshAll();
      setMessage({ type: 'success', text: 'Provider profile deleted.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Provider profile delete failed: ${error.message}` }),
  });

  const testProfileMutation = useMutation({
    mutationFn: (id: string) =>
      apiPost<any>(`/api/v1/provider-profiles/${encodeURIComponent(id)}/test`, {
        hostId: profileTestHostId || (hosts.length ? String(hosts[0]?.id || '').trim() : ''),
      }),
    onSuccess: () => setMessage({ type: 'success', text: 'Profile test succeeded.' }),
    onError: (error: Error) => setMessage({ type: 'error', text: `Profile test failed: ${error.message}` }),
  });

  const saveBindingMutation = useMutation({
    mutationFn: () =>
      apiPost<any>('/api/v1/provider-bindings', {
        profileId: bindingProfileId,
        targetType: bindingTargetType,
        targetId: bindingTargetId.trim(),
      }),
    onSuccess: async () => {
      await refreshAll();
      setMessage({ type: 'success', text: 'Provider binding saved.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Provider binding save failed: ${error.message}` }),
  });

  const deleteBindingMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/provider-bindings/${encodeURIComponent(id)}`),
    onSuccess: async () => {
      await refreshAll();
      setMessage({ type: 'success', text: 'Provider binding deleted.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Provider binding delete failed: ${error.message}` }),
  });

  const previewMutation = useMutation({
    mutationFn: () =>
      apiGet<any>(
        `/api/v1/provider-governance/resolve?hostId=${encodeURIComponent(previewHostId)}&agentId=${encodeURIComponent(
          previewAgentId.trim(),
        )}`,
      ),
    onSuccess: (payload) => setPreviewTextValue(previewText(payload?.resolution)),
    onError: (error: Error) => setPreviewTextValue(`Resolution failed: ${error.message}`),
  });

  return {
    saveProfileMutation,
    deleteProfileMutation,
    testProfileMutation,
    saveBindingMutation,
    deleteBindingMutation,
    previewMutation,
  };
}

