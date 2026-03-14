import { parseCommaSeparatedValues } from '../../../lib/format';
import type { usePoliciesData } from '../usePoliciesData';

type PoliciesData = ReturnType<typeof usePoliciesData>;

export function PoliciesList({ data }: { data: PoliciesData }) {
  const { policies, canManagePolicies, deletePolicyMutation } = data;
  return (
    <div id="execution-policies-list" className="agent-grid" style={{ marginTop: '12px' }}>
      {policies.length ? policies.map((policy: any) => {
        const policyId = String(policy?.id || '').trim();
        const teams = parseCommaSeparatedValues(Array.isArray(policy?.teams) ? policy.teams.join(', ') : String(policy?.teams || '')).sort((left, right) => left.localeCompare(right));
        const projects = parseCommaSeparatedValues(Array.isArray(policy?.projects) ? policy.projects.join(', ') : String(policy?.projects || '')).sort((left, right) => left.localeCompare(right));
        const environments = parseCommaSeparatedValues(Array.isArray(policy?.environments) ? policy.environments.join(', ') : String(policy?.environments || '')).sort((left, right) => left.localeCompare(right));
        const templateIds = parseCommaSeparatedValues(Array.isArray(policy?.templateIds) ? policy.templateIds.join(', ') : String(policy?.templateIds || '')).sort((left, right) => left.localeCompare(right));
        const hostLabels = parseCommaSeparatedValues(Array.isArray(policy?.hostLabels) ? policy.hostLabels.join(', ') : String(policy?.hostLabels || '')).sort((left, right) => left.localeCompare(right));
        const allowedTools = parseCommaSeparatedValues(Array.isArray(policy?.allowedTools) ? policy.allowedTools.join(', ') : String(policy?.allowedTools || '')).sort((left, right) => left.localeCompare(right));
        return (
          <div key={policyId} className="agent-card">
            <h4>{String(policy?.name || policyId)}</h4>
            <div className="instance-meta">
              <div>{String(policy?.action || 'ask')}</div>
              {teams.length ? <div>teams: {teams.join(', ')}</div> : null}
              {projects.length ? <div>projects: {projects.join(', ')}</div> : null}
              {environments.length ? <div>environments: {environments.join(', ')}</div> : null}
              {templateIds.length ? <div>templates: {templateIds.join(', ')}</div> : null}
              {hostLabels.length ? <div>host labels: {hostLabels.join(', ')}</div> : null}
              {allowedTools.length ? <div>allowed tools: {allowedTools.join(', ')}</div> : null}
              {policy?.maxTaskTimeoutMs ? <div>max timeout: {String(policy.maxTaskTimeoutMs)}ms</div> : null}
              {policy?.maxRetryBudget != null ? <div>max retry: {String(policy.maxRetryBudget)}</div> : null}
            </div>
            {canManagePolicies ? <div className="btn-row"><button type="button" className="btn-sm btn-danger" onClick={() => { if (window.confirm('Delete execution policy?')) deletePolicyMutation.mutate(policyId); }}>Delete</button></div> : null}
          </div>
        );
      }) : <div className="card">No execution policies configured.</div>}
    </div>
  );
}
