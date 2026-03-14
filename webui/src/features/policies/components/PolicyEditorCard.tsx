import type { usePoliciesData } from '../usePoliciesData';

type PoliciesData = ReturnType<typeof usePoliciesData>;

export function PolicyEditorCard({ data }: { data: PoliciesData }) {
  const { policyForm, setPolicyForm, canManagePolicies, savePolicyMutation } = data;
  return (
    <div className="card" style={{ marginTop: '12px' }}>
      <h3>Execution Policies</h3>
      <div className="form-grid">
        <div><label htmlFor="execution-policy-name">Name</label><input id="execution-policy-name" type="text" value={policyForm.name} onChange={(event) => setPolicyForm((current) => ({ ...current, name: event.target.value }))} placeholder="review prod picoclaw runs" /></div>
        <div><label htmlFor="execution-policy-action">Action</label><select id="execution-policy-action" value={policyForm.action} onChange={(event) => setPolicyForm((current) => ({ ...current, action: event.target.value }))}><option value="ask">ask</option><option value="deny">deny</option><option value="allow">allow</option></select></div>
        <div><label htmlFor="execution-policy-priority">Priority</label><input id="execution-policy-priority" type="number" value={policyForm.priority} onChange={(event) => setPolicyForm((current) => ({ ...current, priority: event.target.value }))} min={0} max={1000} /></div>
        <div><label htmlFor="execution-policy-reason">Reason</label><input id="execution-policy-reason" type="text" value={policyForm.reason} onChange={(event) => setPolicyForm((current) => ({ ...current, reason: event.target.value }))} placeholder="operator review required" /></div>
        <div><label htmlFor="execution-policy-teams">Teams</label><input id="execution-policy-teams" type="text" value={policyForm.teams} onChange={(event) => setPolicyForm((current) => ({ ...current, teams: event.target.value }))} placeholder="platform, infra" /></div>
        <div><label htmlFor="execution-policy-projects">Projects</label><input id="execution-policy-projects" type="text" value={policyForm.projects} onChange={(event) => setPolicyForm((current) => ({ ...current, projects: event.target.value }))} placeholder="carrier, checkout" /></div>
        <div><label htmlFor="execution-policy-environments">Environments</label><input id="execution-policy-environments" type="text" value={policyForm.environments} onChange={(event) => setPolicyForm((current) => ({ ...current, environments: event.target.value }))} placeholder="prod, staging" /></div>
        <div><label htmlFor="execution-policy-template-ids">Template IDs</label><input id="execution-policy-template-ids" type="text" value={policyForm.templateIds} onChange={(event) => setPolicyForm((current) => ({ ...current, templateIds: event.target.value }))} placeholder="rollout-smoke, incident-triage" /></div>
        <div><label htmlFor="execution-policy-providers">Requested Providers</label><input id="execution-policy-providers" type="text" value={policyForm.providers} onChange={(event) => setPolicyForm((current) => ({ ...current, providers: event.target.value }))} placeholder="anthropic, openrouter" /></div>
        <div><label htmlFor="execution-policy-host-ids">Host IDs</label><input id="execution-policy-host-ids" type="text" value={policyForm.hostIds} onChange={(event) => setPolicyForm((current) => ({ ...current, hostIds: event.target.value }))} placeholder="host-1, host-2" /></div>
        <div><label htmlFor="execution-policy-host-labels">Host Labels</label><input id="execution-policy-host-labels" type="text" value={policyForm.hostLabels} onChange={(event) => setPolicyForm((current) => ({ ...current, hostLabels: event.target.value }))} placeholder="prod, gpu" /></div>
        <div><label htmlFor="execution-policy-agent-ids">Agent IDs</label><input id="execution-policy-agent-ids" type="text" value={policyForm.agentIds} onChange={(event) => setPolicyForm((current) => ({ ...current, agentIds: event.target.value }))} placeholder="picoclaw, zeroclaw" /></div>
        <div><label htmlFor="execution-policy-allowed-tools">Allowed Tools</label><input id="execution-policy-allowed-tools" type="text" value={policyForm.allowedTools} onChange={(event) => setPolicyForm((current) => ({ ...current, allowedTools: event.target.value }))} placeholder="grep, shell" /></div>
        <div><label htmlFor="execution-policy-max-timeout-ms">Max Task Timeout (ms)</label><input id="execution-policy-max-timeout-ms" type="number" value={policyForm.maxTimeoutMs} onChange={(event) => setPolicyForm((current) => ({ ...current, maxTimeoutMs: event.target.value }))} placeholder="45000" /></div>
        <div><label htmlFor="execution-policy-max-retry-budget">Max Retry Budget</label><input id="execution-policy-max-retry-budget" type="number" value={policyForm.maxRetryBudget} onChange={(event) => setPolicyForm((current) => ({ ...current, maxRetryBudget: event.target.value }))} placeholder="1" /></div>
        <div><label htmlFor="execution-policy-enabled">Enabled</label><select id="execution-policy-enabled" value={policyForm.enabled} onChange={(event) => setPolicyForm((current) => ({ ...current, enabled: event.target.value }))}><option value="true">true</option><option value="false">false</option></select></div>
      </div>
      <div className="btn-row"><button id="execution-policy-save" disabled={!canManagePolicies} onClick={() => savePolicyMutation.mutate()}>Save Policy</button></div>
    </div>
  );
}
