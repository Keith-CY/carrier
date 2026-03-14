import type { usePoliciesData } from '../usePoliciesData';
import { renderInputsText } from '../../providers/shared';

type PoliciesData = ReturnType<typeof usePoliciesData>;

export function TriggersList({ data }: { data: PoliciesData }) {
  const { triggers, templates, canManagePolicies, setEditingTriggerId, setTriggerForm, toggleTriggerMutation, deleteTriggerMutation } = data;
  return (
    <div id="execution-triggers-list" className="agent-grid" style={{ marginTop: '12px' }}>
      {triggers.length ? triggers.map((trigger: any) => {
        const triggerId = String(trigger?.id || '').trim();
        const config = trigger?.config && typeof trigger.config === 'object' ? trigger.config : {};
        const enabled = trigger?.enabled !== false;
        return (
          <div key={triggerId} className="agent-card">
            <h4>{String(trigger?.name || triggerId)}</h4>
            <div className="instance-meta">
              <div>type: {String(trigger?.type || 'unknown')}</div>
              <div>enabled: {String(enabled)}</div>
              <div>template: {String(trigger?.templateId || '')}</div>
            </div>
            {canManagePolicies ? (
              <div className="btn-row">
                <button type="button" className="btn-sm btn-secondary" onClick={() => {
                  setEditingTriggerId(triggerId);
                  setTriggerForm({
                    name: String(trigger?.name || ''),
                    type: String(trigger?.type || 'webhook'),
                    templateId: String(trigger?.templateId || String(templates[0]?.id || '')),
                    provider: String(config?.provider || ''),
                    hostIds: Array.isArray(config?.hostIds) ? config.hostIds.join(', ') : '',
                    hostLabels: Array.isArray(config?.hostLabels) ? config.hostLabels.join(', ') : '',
                    maxConcurrency: config?.maxConcurrency != null ? String(config.maxConcurrency) : '',
                    timezone: String(config?.timezone || 'UTC'),
                    webhookSecret: '',
                    githubCommand: String(config?.githubCommand || ''),
                    githubLabel: String(config?.githubLabel || ''),
                    githubRepository: String(config?.githubRepository || ''),
                    cron: String(config?.cron || ''),
                    inputs: renderInputsText(config?.inputs || {}),
                    policyApprove: !!config?.policyApprove,
                  });
                }}>Edit</button>
                <button type="button" className="btn-sm btn-secondary" onClick={() => toggleTriggerMutation.mutate({ id: triggerId, enabled: !enabled })}>{enabled ? 'Disable' : 'Enable'}</button>
                <button type="button" className="btn-sm btn-danger" onClick={() => { if (window.confirm('Delete execution trigger?')) deleteTriggerMutation.mutate(triggerId); }}>Delete</button>
              </div>
            ) : null}
          </div>
        );
      }) : <div className="card">No execution triggers configured.</div>}
    </div>
  );
}
