import { useEffect, useState } from 'react';
import {
  EMPTY_POLICY_FORM,
  EMPTY_TRIGGER_FORM,
  type PolicyFormState,
  type TriggerFormState,
} from '../providers/shared';

export function usePolicyState(templates: Array<{ id?: unknown }>) {
  const [policyForm, setPolicyForm] = useState<PolicyFormState>(EMPTY_POLICY_FORM);
  const [triggerForm, setTriggerForm] = useState<TriggerFormState>(EMPTY_TRIGGER_FORM);
  const [editingTriggerId, setEditingTriggerId] = useState('');

  useEffect(() => {
    if (!triggerForm.templateId && templates.length) {
      setTriggerForm((current) => ({ ...current, templateId: String(templates[0]?.id || '').trim() }));
    }
  }, [templates, triggerForm.templateId]);

  return {
    policyForm,
    setPolicyForm,
    triggerForm,
    setTriggerForm,
    editingTriggerId,
    setEditingTriggerId,
  };
}

