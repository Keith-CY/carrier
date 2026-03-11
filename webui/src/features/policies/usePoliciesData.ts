import { useEffect, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { apiDelete, apiPatch, apiPost } from '../../lib/api';
import { parseCommaSeparatedValues } from '../../lib/format';
import {
  EMPTY_POLICY_FORM,
  EMPTY_TRIGGER_FORM,
  parseInputsText,
  type PolicyFormState,
  type TriggerFormState,
  toInt,
  useProviderPolicyLookups,
} from '../providers/shared';

export function usePoliciesData() {
  const lookups = useProviderPolicyLookups('policies');
  const [policyForm, setPolicyForm] = useState<PolicyFormState>(EMPTY_POLICY_FORM);
  const [triggerForm, setTriggerForm] = useState<TriggerFormState>(EMPTY_TRIGGER_FORM);
  const [editingTriggerId, setEditingTriggerId] = useState('');

  useEffect(() => {
    if (!triggerForm.templateId && lookups.templates.length) {
      setTriggerForm((current) => ({ ...current, templateId: String(lookups.templates[0]?.id || '').trim() }));
    }
  }, [lookups.templates, triggerForm.templateId]);

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
      await lookups.refreshAll();
      setPolicyForm(EMPTY_POLICY_FORM);
      lookups.setMessage({ type: 'success', text: 'Execution policy saved.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Execution policy save failed: ${error.message}` }),
  });

  const deletePolicyMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/orchestrator/policies/${encodeURIComponent(id)}`),
    onSuccess: async () => {
      await lookups.refreshAll();
      lookups.setMessage({ type: 'success', text: 'Execution policy deleted.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Execution policy delete failed: ${error.message}` }),
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
      await lookups.refreshAll();
      setTriggerForm((current) => ({ ...EMPTY_TRIGGER_FORM, templateId: current.templateId || String(lookups.templates[0]?.id || '').trim() }));
      setEditingTriggerId('');
      lookups.setMessage({ type: 'success', text: 'Execution trigger saved.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Execution trigger save failed: ${error.message}` }),
  });

  const toggleTriggerMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      apiPatch<any>(`/api/v1/triggers/${encodeURIComponent(id)}`, { enabled }),
    onSuccess: async () => {
      await lookups.refreshAll();
      lookups.setMessage({ type: 'success', text: 'Execution trigger updated.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Execution trigger update failed: ${error.message}` }),
  });

  const deleteTriggerMutation = useMutation({
    mutationFn: (id: string) => apiDelete(`/api/v1/triggers/${encodeURIComponent(id)}`),
    onSuccess: async () => {
      await lookups.refreshAll();
      lookups.setMessage({ type: 'success', text: 'Execution trigger deleted.' });
    },
    onError: (error: Error) => lookups.setMessage({ type: 'error', text: `Execution trigger delete failed: ${error.message}` }),
  });

  return {
    ...lookups,
    policyForm,
    setPolicyForm,
    triggerForm,
    setTriggerForm,
    editingTriggerId,
    setEditingTriggerId,
    savePolicyMutation,
    deletePolicyMutation,
    saveTriggerMutation,
    toggleTriggerMutation,
    deleteTriggerMutation,
  };
}
