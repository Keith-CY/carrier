import type { usePoliciesData } from '../usePoliciesData';
import { EMPTY_TRIGGER_FORM } from '../../providers/shared';

type PoliciesData = ReturnType<typeof usePoliciesData>;

export function TriggerEditorCard({ data }: { data: PoliciesData }) {
  const { triggerForm, setTriggerForm, editingTriggerId, setEditingTriggerId, templates, canManagePolicies, saveTriggerMutation } = data;
  return (
    <div className="card" style={{ marginTop: '12px' }}>
      <h3>Execution Triggers</h3>
      <div id="trigger-editor-state" className="text-dim" style={{ marginBottom: '8px' }}>{editingTriggerId ? `Editing trigger: ${editingTriggerId}` : ''}</div>
      <div className="form-grid">
        <div><label htmlFor="trigger-name">Name</label><input id="trigger-name" type="text" value={triggerForm.name} onChange={(event) => setTriggerForm((current) => ({ ...current, name: event.target.value }))} placeholder="incident webhook" /></div>
        <div><label htmlFor="trigger-type">Type</label><select id="trigger-type" value={triggerForm.type} onChange={(event) => setTriggerForm((current) => ({ ...current, type: event.target.value }))}><option value="webhook">webhook</option><option value="github">github</option><option value="schedule">schedule</option></select></div>
        <div><label htmlFor="trigger-template-id">Template</label><select id="trigger-template-id" value={triggerForm.templateId} onChange={(event) => setTriggerForm((current) => ({ ...current, templateId: event.target.value }))}>{templates.map((template: any) => <option key={String(template?.id || '')} value={String(template?.id || '')}>{String(template?.name || template?.id || '')}</option>)}</select></div>
        <div><label htmlFor="trigger-provider">Provider</label><input id="trigger-provider" type="text" value={triggerForm.provider} onChange={(event) => setTriggerForm((current) => ({ ...current, provider: event.target.value }))} placeholder="openrouter" /></div>
        <div><label htmlFor="trigger-host-ids">Host IDs</label><input id="trigger-host-ids" type="text" value={triggerForm.hostIds} onChange={(event) => setTriggerForm((current) => ({ ...current, hostIds: event.target.value }))} placeholder="host-1, host-2" /></div>
        <div><label htmlFor="trigger-host-labels">Host Labels</label><input id="trigger-host-labels" type="text" value={triggerForm.hostLabels} onChange={(event) => setTriggerForm((current) => ({ ...current, hostLabels: event.target.value }))} placeholder="prod, gpu" /></div>
        <div><label htmlFor="trigger-max-concurrency">Max Concurrency</label><input id="trigger-max-concurrency" type="number" value={triggerForm.maxConcurrency} onChange={(event) => setTriggerForm((current) => ({ ...current, maxConcurrency: event.target.value }))} placeholder="2" /></div>
        <div><label htmlFor="trigger-timezone">Timezone</label><input id="trigger-timezone" type="text" value={triggerForm.timezone} onChange={(event) => setTriggerForm((current) => ({ ...current, timezone: event.target.value }))} placeholder="UTC" /></div>
        <div><label htmlFor="trigger-webhook-secret">Webhook Secret</label><input id="trigger-webhook-secret" type="text" value={triggerForm.webhookSecret} onChange={(event) => setTriggerForm((current) => ({ ...current, webhookSecret: event.target.value }))} placeholder="secret" /></div>
        <div><label htmlFor="trigger-github-command">GitHub Command</label><input id="trigger-github-command" type="text" value={triggerForm.githubCommand} onChange={(event) => setTriggerForm((current) => ({ ...current, githubCommand: event.target.value }))} placeholder="/carrier triage" /></div>
        <div><label htmlFor="trigger-github-label">GitHub Label</label><input id="trigger-github-label" type="text" value={triggerForm.githubLabel} onChange={(event) => setTriggerForm((current) => ({ ...current, githubLabel: event.target.value }))} placeholder="needs-triage" /></div>
        <div><label htmlFor="trigger-github-repository">GitHub Repository</label><input id="trigger-github-repository" type="text" value={triggerForm.githubRepository} onChange={(event) => setTriggerForm((current) => ({ ...current, githubRepository: event.target.value }))} placeholder="Keith-CY/carrier" /></div>
        <div><label htmlFor="trigger-cron">Cron</label><input id="trigger-cron" type="text" value={triggerForm.cron} onChange={(event) => setTriggerForm((current) => ({ ...current, cron: event.target.value }))} placeholder="0 * * * *" /></div>
        <div><label htmlFor="trigger-policy-approve">Policy Approve</label><input id="trigger-policy-approve" type="checkbox" checked={triggerForm.policyApprove} onChange={(event) => setTriggerForm((current) => ({ ...current, policyApprove: event.target.checked }))} /></div>
        <div style={{ gridColumn: '1 / -1' }}><label htmlFor="trigger-inputs">Inputs</label><textarea id="trigger-inputs" rows={4} value={triggerForm.inputs} onChange={(event) => setTriggerForm((current) => ({ ...current, inputs: event.target.value }))} placeholder={'service={{payload.service}}\nenvironment={{payload.environment}}'} /></div>
      </div>
      <div className="btn-row">
        <button id="trigger-save" disabled={!canManagePolicies} onClick={() => saveTriggerMutation.mutate()}>{editingTriggerId ? 'Update Trigger' : 'Save Trigger'}</button>
        <button id="trigger-cancel-edit" className={`btn-sm btn-secondary${editingTriggerId ? '' : ' hidden'}`} type="button" onClick={() => { setEditingTriggerId(''); setTriggerForm((current) => ({ ...EMPTY_TRIGGER_FORM, templateId: current.templateId || String(templates[0]?.id || '').trim() })); }}>Cancel Edit</button>
      </div>
    </div>
  );
}
