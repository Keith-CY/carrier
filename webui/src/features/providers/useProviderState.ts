import { useEffect, useState } from 'react';
import {
  EMPTY_PROFILE_FORM,
  type ProfileFormState,
} from './shared';

export function useProviderState(profiles: Array<{ id?: unknown }>, hosts: Array<{ id?: unknown }>) {
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
    if (!bindingProfileId && profiles.length) {
      setBindingProfileId(String(profiles[0]?.id || '').trim());
    }
  }, [bindingProfileId, profiles]);

  useEffect(() => {
    if (!profileTestHostId && hosts.length) {
      setProfileTestHostId(String(hosts[0]?.id || '').trim());
    }
    if (!previewHostId && hosts.length) {
      setPreviewHostId(String(hosts[0]?.id || '').trim());
    }
  }, [hosts, previewHostId, profileTestHostId]);

  return {
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
    setPreviewTextValue,
  };
}

