import { useEffect, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { apiDelete, apiGet, apiPatch, apiPost } from '../../lib/api';
import {
  EMPTY_PROFILE_FORM,
  type ProfileFormState,
  previewText,
  useProviderPolicyLookups,
} from './shared';

export function useProvidersData() {
  const lookups = useProviderPolicyLookups('providers');
  const [profileForm, setProfileForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM);
  const [editingProfileId, setEditingProfileId] = useState('');
  const [bindingTargetType, setBindingTargetType] = useState('host');
  const [bindingTargetId, setBindingTargetId] = useState('');
  const [bindingProfileId, setBindingProfileId] = useState('');
  const [profileTestHostId, setProfileTestHostId] = useState('');
  const [previewHostId, setPreviewHostId] = useState('');
  const [previewAgentId, setPreviewAgentId] = useState('zeroclaw');
  const [previewTextValue, setPreviewTextValue] = useState('');

  useEffect(() => {
    if (!bindingProfileId && lookups.profiles.length) {
      setBindingProfileId(String(lookups.profiles[0]?.id || '').trim());
    }
  }, [bindingProfileId, lookups.profiles]);

  useEffect(() => {
    if (!profileTestHostId && lookups.hosts.length) {
      setProfileTestHostId(String(lookups.hosts[0]?.id || '').trim());
    }
    if (!previewHostId && lookups.hosts.length) {
      setPreviewHostId(String(lookups.hosts[0]?.id || '').trim());
    }
  }, [lookups.hosts, previewHostId, profileTestHostId]);

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
      await lookups.refreshAll();
      setProfileForm(EMPTY_PROFILE_FORM);
      setEditingProfileId('');
      lookups.setMessage({ type: 'success', text: 'Provider profile saved.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Provider profile save failed: ${error.message}` }),
  });

  const deleteProfileMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/provider-profiles/${encodeURIComponent(id)}`),
    onSuccess: async () => {
      await lookups.refreshAll();
      lookups.setMessage({ type: 'success', text: 'Provider profile deleted.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Provider profile delete failed: ${error.message}` }),
  });

  const testProfileMutation = useMutation({
    mutationFn: (id: string) => apiPost<any>(`/api/v1/provider-profiles/${encodeURIComponent(id)}/test`, {
      hostId: profileTestHostId || (lookups.hosts.length ? String(lookups.hosts[0]?.id || '').trim() : ''),
    }),
    onSuccess: () => lookups.setMessage({ type: 'success', text: 'Profile test succeeded.' }),
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Profile test failed: ${error.message}` }),
  });

  const saveBindingMutation = useMutation({
    mutationFn: () => apiPost<any>('/api/v1/provider-bindings', {
      profileId: bindingProfileId,
      targetType: bindingTargetType,
      targetId: bindingTargetId.trim(),
    }),
    onSuccess: async () => {
      await lookups.refreshAll();
      lookups.setMessage({ type: 'success', text: 'Provider binding saved.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Provider binding save failed: ${error.message}` }),
  });

  const deleteBindingMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/provider-bindings/${encodeURIComponent(id)}`),
    onSuccess: async () => {
      await lookups.refreshAll();
      lookups.setMessage({ type: 'success', text: 'Provider binding deleted.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Provider binding delete failed: ${error.message}` }),
  });

  const previewMutation = useMutation({
    mutationFn: () => apiGet<any>(`/api/v1/provider-governance/resolve?hostId=${encodeURIComponent(previewHostId)}&agentId=${encodeURIComponent(previewAgentId.trim())}`),
    onSuccess: (payload) => setPreviewTextValue(previewText(payload?.resolution)),
    onError: (error: Error) => setPreviewTextValue(`Resolution failed: ${error.message}`),
  });

  return {
    ...lookups,
    profileForm,
    setProfileForm,
    editingProfileId,
    setEditingProfileId,
    bindingTargetType,
    setBindingTargetType,
    bindingTargetId,
    setBindingTargetId,
    bindingProfileId,
    setBindingProfileId,
    profileTestHostId,
    setProfileTestHostId,
    previewHostId,
    setPreviewHostId,
    previewAgentId,
    setPreviewAgentId,
    previewTextValue,
    saveProfileMutation,
    deleteProfileMutation,
    testProfileMutation,
    saveBindingMutation,
    deleteBindingMutation,
    previewMutation,
  };
}
