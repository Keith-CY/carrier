import { useEffect, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { useFeatures } from '../../app/useFeatures';
import { apiGet } from '../../lib/api';

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

export function parseInputsText(value: string): Record<string, string> {
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

export function toInt(value: string): number | undefined {
  const parsed = parseInt(String(value || '').trim(), 10);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export function previewText(resolution: any): string {
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

export function useProviderPolicyLookups(mode: 'providers' | 'policies') {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { featureFlags, authz, isLoading: featuresLoading } = useFeatures();
  const [message, setMessage] = useState<MessageState>({ type: 'info', text: '' });

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

  return {
    featureFlags,
    authz,
    featuresLoading,
    message,
    setMessage,
    canManageProviders,
    canManagePolicies,
    hosts,
    profiles,
    bindings,
    policies,
    templates,
    triggers,
    refreshAll,
  };
}
