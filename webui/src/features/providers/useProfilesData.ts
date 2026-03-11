import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { useFeatures } from '../../app/useFeatures';
import { apiDelete, apiGet, apiPatch, apiPost } from '../../lib/api';
import { parseCommaSeparatedValues } from '../../lib/format';

export type MessageState = {
  type: 'info' | 'success' | 'error';
  text: string;
};

export type ProfileFormState = {
  name: string;
  provider: string;
  model: string;
  baseUrl: string;
  authRef: string;
  enabled: string;
};

export type PolicyFormState = {
  name: string;
  action: string;
  priority: string;
  reason: string;
  teams: string;
  projects: string;
  environments: string;
  templateIds: string;
  providers: string;
  hostIds: string;
  hostLabels: string;
  agentIds: string;
  allowedTools: string;
  maxTimeoutMs: string;
  maxRetryBudget: string;
  enabled: string;
};

export type TriggerFormState = {
  name: string;
  type: string;
  templateId: string;
  provider: string;
  hostIds: string;
  hostLabels: string;
  maxConcurrency: string;
  timezone: string;
  webhookSecret: string;
  githubCommand: string;
  githubLabel: string;
  githubRepository: string;
  cron: string;
  inputs: string;
  policyApprove: boolean;
};

export const EMPTY_PROFILE_FORM: ProfileFormState = {
  name: '',
  provider: '',
  model: '',
  baseUrl: '',
  authRef: '',
  enabled: 'true',
};

export const EMPTY_POLICY_FORM: PolicyFormState = {
  name: '',
  action: 'ask',
  priority: '0',
  reason: '',
  teams: '',
  projects: '',
  environments: '',
  templateIds: '',
  providers: '',
  hostIds: '',
  hostLabels: '',
  agentIds: '',
  allowedTools: '',
  maxTimeoutMs: '',
  maxRetryBudget: '',
  enabled: 'true',
};

export const EMPTY_TRIGGER_FORM: TriggerFormState = {
  name: '',
  type: 'webhook',
  templateId: '',
  provider: '',
  hostIds: '',
  hostLabels: '',
  maxConcurrency: '',
  timezone: 'UTC',
  webhookSecret: '',
  githubCommand: '',
  githubLabel: '',
  githubRepository: '',
  cron: '',
  inputs: '',
  policyApprove: false,
};

export function normalizeBoolString(value: unknown): string {
  return value === false || String(value || '').trim().toLowerCase() === 'false' ? 'false' : 'true';
}

export function renderInputsText(value: unknown): string {
  const inputs = value && typeof value === 'object' ? value as Record<string, unknown> : {};
  return Object.keys(inputs)
    .sort((left, right) => left.localeCompare(right))
    .map((key) => `${key}=${String(inputs[key] || '')}`)
    .join('\n');
}

function parseInputsText(value: string): Record<string, string> {
  const result: Record<string, string> = {};
  String(value || '')
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .forEach((line) => {
      const separator = line.indexOf('=');
      if (separator <= 0) return;
      const key = line.slice(0, separator).trim();
      const nextValue = line.slice(separator + 1).trim();
      if (!key) return;
      result[key] = nextValue;
    });
  return result;
}

function toInt(value: string): number | undefined {
  const parsed = parseInt(String(value || '').trim(), 10);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function previewText(resolution: any): string {
  if (!resolution || typeof resolution !== 'object') return 'No resolution available.';
  const lines = [
    `${String(resolution.provider || 'unknown')}/${String(resolution.model || 'unknown')}`,
    `source=${String(resolution.source || 'unknown')}`,
    `drift=${String(resolution.driftState || 'unknown')}`,
  ];
  const trace = Array.isArray(resolution.trace) ? resolution.trace : [];
  trace.forEach((item) => {
    const source = String(item?.source || 'unknown');
    const status = String(item?.status || 'unknown');
    const selected = item?.selected ? ', selected' : '';
    const provider = String(item?.provider || '').trim();
    const model = String(item?.model || '').trim();
    const providerModel = provider || model ? ` ${provider}/${model}`.replace(/\/$/, '') : '';
    lines.push(`${source} [${status}${selected}]${providerModel}`);
  });
  return lines.join('\n');
}

function readOnlyMessage(mode: 'providers' | 'policies'): string {
  return mode === 'providers'
    ? 'Current role has read-only access to providers.'
    : 'Current role has read-only access to policies.';
}

export function useProfilesData(mode: 'providers' | 'policies') {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { featureFlags, authz, isLoading: featuresLoading } = useFeatures();
  const [message, setMessage] = useState<MessageState>({ type: 'info', text: '' });
  const [profileForm, setProfileForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM);
  const [editingProfileId, setEditingProfileId] = useState('');
  const [bindingTargetType, setBindingTargetType] = useState('host');
  const [bindingTargetId, setBindingTargetId] = useState('');
  const [bindingProfileId, setBindingProfileId] = useState('');
  const [profileTestHostId, setProfileTestHostId] = useState('');
  const [previewHostId, setPreviewHostId] = useState('');
  const [previewAgentId, setPreviewAgentId] = useState('zeroclaw');
  const [previewTextValue, setPreviewTextValue] = useState('');
  const [policyForm, setPolicyForm] = useState<PolicyFormState>(EMPTY_POLICY_FORM);
  const [triggerForm, setTriggerForm] = useState<TriggerFormState>(EMPTY_TRIGGER_FORM);
  const [editingTriggerId, setEditingTriggerId] = useState('');

  const canManageProviders = authz.permissions.manageProviders;
  const canManagePolicies = authz.permissions.managePolicies;

  useEffect(() => {
    if (featuresLoading) return;
    if (!featureFlags.remoteControlPlaneEnabled) {
      navigate('/dashboard', { replace: true });
    }
  }, [featureFlags.remoteControlPlaneEnabled, featuresLoading, navigate]);

  useEffect(() => {
    if (!featuresLoading && mode === 'providers' && !canManageProviders) {
      setMessage({ type: 'info', text: readOnlyMessage(mode) });
    }
    if (!featuresLoading && mode === 'policies' && !canManagePolicies) {
      setMessage({ type: 'info', text: readOnlyMessage(mode) });
    }
  }, [canManagePolicies, canManageProviders, featuresLoading, mode]);

  const hostsQuery = useQuery({
    queryKey: ['hosts'],
    queryFn: () => apiGet<any>('/api/v1/remote/hosts'),
    enabled: featureFlags.remoteControlPlaneEnabled,
  });
  const profilesQuery = useQuery({
    queryKey: ['provider-profiles'],
    queryFn: () => apiGet<any>('/api/v1/provider-profiles'),
    enabled: featureFlags.remoteControlPlaneEnabled,
  });
  const bindingsQuery = useQuery({
    queryKey: ['provider-bindings'],
    queryFn: () => apiGet<any>('/api/v1/provider-bindings'),
    enabled: featureFlags.remoteControlPlaneEnabled,
  });
  const policiesQuery = useQuery({
    queryKey: ['execution-policies'],
    queryFn: () => apiGet<any>('/api/v1/orchestrator/policies'),
    enabled: featureFlags.remoteControlPlaneEnabled,
  });
  const templatesQuery = useQuery({
    queryKey: ['templates'],
    queryFn: () => apiGet<any>('/api/v1/templates'),
    enabled: featureFlags.remoteControlPlaneEnabled,
  });
  const triggersQuery = useQuery({
    queryKey: ['triggers'],
    queryFn: () => apiGet<any>('/api/v1/triggers'),
    enabled: featureFlags.remoteControlPlaneEnabled,
  });

  const hosts = Array.isArray(hostsQuery.data?.hosts) ? hostsQuery.data.hosts : [];
  const profiles = Array.isArray(profilesQuery.data?.profiles) ? profilesQuery.data.profiles : [];
  const bindings = Array.isArray(bindingsQuery.data?.bindings) ? bindingsQuery.data.bindings : [];
  const policies = Array.isArray(policiesQuery.data?.policies) ? policiesQuery.data.policies : [];
  const templates = Array.isArray(templatesQuery.data?.templates) ? templatesQuery.data.templates : [];
  const triggers = Array.isArray(triggersQuery.data?.triggers) ? triggersQuery.data.triggers : [];

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

  useEffect(() => {
    if (!triggerForm.templateId && templates.length) {
      setTriggerForm((current) => ({ ...current, templateId: String(templates[0]?.id || '').trim() }));
    }
  }, [templates, triggerForm.templateId]);

  const refreshAll = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['hosts'] }),
      queryClient.invalidateQueries({ queryKey: ['provider-profiles'] }),
      queryClient.invalidateQueries({ queryKey: ['provider-bindings'] }),
      queryClient.invalidateQueries({ queryKey: ['execution-policies'] }),
      queryClient.invalidateQueries({ queryKey: ['templates'] }),
      queryClient.invalidateQueries({ queryKey: ['triggers'] }),
    ]);
  };

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
    mutationFn: (id: string) => apiPost<any>(`/api/v1/provider-profiles/${encodeURIComponent(id)}/test`, {
      hostId: profileTestHostId || (hosts.length ? String(hosts[0]?.id || '').trim() : ''),
    }),
    onSuccess: () => setMessage({ type: 'success', text: 'Profile test succeeded.' }),
    onError: (error: Error) => setMessage({ type: 'error', text: `Profile test failed: ${error.message}` }),
  });

  const saveBindingMutation = useMutation({
    mutationFn: () => apiPost<any>('/api/v1/provider-bindings', {
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
    mutationFn: () => apiGet<any>(`/api/v1/provider-governance/resolve?hostId=${encodeURIComponent(previewHostId)}&agentId=${encodeURIComponent(previewAgentId.trim())}`),
    onSuccess: (payload) => setPreviewTextValue(previewText(payload?.resolution)),
    onError: (error: Error) => setPreviewTextValue(`Resolution failed: ${error.message}`),
  });

  const savePolicyMutation = useMutation({
    mutationFn: () => apiPost<any>('/api/v1/orchestrator/policies', {
      name: policyForm.name.trim(),
      action: policyForm.action,
      priority: toInt(policyForm.priority) ?? 0,
      reason: policyForm.reason.trim(),
      teams: parseCommaSeparatedValues(policyForm.teams),
      projects: parseCommaSeparatedValues(policyForm.projects),
      environments: parseCommaSeparatedValues(policyForm.environments),
      templateIds: parseCommaSeparatedValues(policyForm.templateIds),
      requestedProviders: parseCommaSeparatedValues(policyForm.providers),
      hostIds: parseCommaSeparatedValues(policyForm.hostIds),
      hostLabels: parseCommaSeparatedValues(policyForm.hostLabels),
      agentIds: parseCommaSeparatedValues(policyForm.agentIds),
      allowedTools: parseCommaSeparatedValues(policyForm.allowedTools),
      maxTaskTimeoutMs: toInt(policyForm.maxTimeoutMs),
      maxRetryBudget: toInt(policyForm.maxRetryBudget),
      enabled: policyForm.enabled === 'true',
    }),
    onSuccess: async () => {
      await refreshAll();
      setPolicyForm(EMPTY_POLICY_FORM);
      setMessage({ type: 'success', text: 'Execution policy saved.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Execution policy save failed: ${error.message}` }),
  });

  const deletePolicyMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/orchestrator/policies/${encodeURIComponent(id)}`),
    onSuccess: async () => {
      await refreshAll();
      setMessage({ type: 'success', text: 'Execution policy deleted.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Execution policy delete failed: ${error.message}` }),
  });

  const saveTriggerMutation = useMutation({
    mutationFn: async () => {
      const payload = {
        name: triggerForm.name.trim(),
        type: triggerForm.type,
        templateId: triggerForm.templateId,
        config: {
          provider: triggerForm.provider.trim(),
          hostIds: parseCommaSeparatedValues(triggerForm.hostIds),
          hostLabels: parseCommaSeparatedValues(triggerForm.hostLabels),
          maxConcurrency: toInt(triggerForm.maxConcurrency),
          timezone: triggerForm.timezone.trim() || 'UTC',
          webhookSecret: triggerForm.webhookSecret.trim(),
          githubCommand: triggerForm.githubCommand.trim(),
          githubLabel: triggerForm.githubLabel.trim(),
          githubRepository: triggerForm.githubRepository.trim(),
          cron: triggerForm.cron.trim(),
          policyApprove: triggerForm.policyApprove,
          inputs: parseInputsText(triggerForm.inputs),
        },
      };
      if (editingTriggerId) {
        return apiPatch<any>(`/api/v1/triggers/${encodeURIComponent(editingTriggerId)}`, payload);
      }
      return apiPost<any>('/api/v1/triggers', payload);
    },
    onSuccess: async () => {
      await refreshAll();
      setTriggerForm((current) => ({ ...EMPTY_TRIGGER_FORM, templateId: current.templateId || String(templates[0]?.id || '').trim() }));
      setEditingTriggerId('');
      setMessage({ type: 'success', text: 'Execution trigger saved.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Execution trigger save failed: ${error.message}` }),
  });

  const toggleTriggerMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      apiPatch<any>(`/api/v1/triggers/${encodeURIComponent(id)}`, { enabled }),
    onSuccess: async () => {
      await refreshAll();
      setMessage({ type: 'success', text: 'Execution trigger updated.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Execution trigger update failed: ${error.message}` }),
  });

  const deleteTriggerMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/triggers/${encodeURIComponent(id)}`),
    onSuccess: async () => {
      await refreshAll();
      setMessage({ type: 'success', text: 'Execution trigger deleted.' });
    },
    onError: (error: Error) => setMessage({ type: 'error', text: `Execution trigger delete failed: ${error.message}` }),
  });

  return {
    featureFlags,
    authz,
    featuresLoading,
    message,
    setMessage,
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
    policyForm,
    setPolicyForm,
    triggerForm,
    setTriggerForm,
    editingTriggerId,
    setEditingTriggerId,
    canManageProviders,
    canManagePolicies,
    hosts,
    profiles,
    bindings,
    policies,
    templates,
    triggers,
    refreshAll,
    saveProfileMutation,
    deleteProfileMutation,
    testProfileMutation,
    saveBindingMutation,
    deleteBindingMutation,
    previewMutation,
    savePolicyMutation,
    deletePolicyMutation,
    saveTriggerMutation,
    toggleTriggerMutation,
    deleteTriggerMutation,
  };
}
