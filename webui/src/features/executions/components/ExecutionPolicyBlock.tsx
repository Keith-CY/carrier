import { formatDateTime, toFiniteNumber } from '../../../lib/format';

export function ExecutionPolicyBlock({ execution }: { execution: any }) {
  const policy = execution?.policy && typeof execution.policy === 'object' ? execution.policy : {};
  const toolPolicy = policy?.toolPolicy && typeof policy.toolPolicy === 'object' ? policy.toolPolicy : {};
  const targets = Array.isArray(policy?.targets) ? policy.targets : [];

  if (!String(policy?.decision || '').trim()) return null;

  return (
    <div className="execution-detail-block">
      <div className="execution-detail-title">Execution Policy</div>
      <div className="execution-detail-line">Decision: {String(policy.decision).trim()}</div>
      <div className="execution-detail-line">Infrastructure approval required: {policy?.requiresInfrastructureApproval ? 'yes' : 'no'}</div>
      {String(policy?.matchedRuleName || '').trim() ? <div className="execution-detail-line">Matched rule: {String(policy.matchedRuleName).trim()}</div> : null}
      {String(policy?.reason || '').trim() ? <div className="execution-detail-line">Reason: {String(policy.reason).trim()}</div> : null}
      {String(toolPolicy?.mode || '').trim() ? <div className="execution-detail-line">tool mode: {String(toolPolicy.mode).trim()}</div> : null}
      {toFiniteNumber(policy?.configuredMaxConcurrency, 0) > 0 ? <div className="execution-detail-line">configured concurrency: {toFiniteNumber(policy.configuredMaxConcurrency, 0)}</div> : null}
      {toFiniteNumber(policy?.effectiveMaxConcurrency, 0) > 0 ? <div className="execution-detail-line">effective concurrency: {toFiniteNumber(policy.effectiveMaxConcurrency, 0)}</div> : null}
      {toFiniteNumber(policy?.maxTaskTimeoutMs, 0) > 0 ? <div className="execution-detail-line">max task timeout: {toFiniteNumber(policy.maxTaskTimeoutMs, 0)}ms</div> : null}
      <div className="execution-detail-line">max retry budget: {toFiniteNumber(policy?.maxRetryBudget, 0)}</div>
      {String(policy?.summary || '').trim() ? <div className="execution-detail-line">summary: {String(policy.summary).trim()}</div> : null}
      {String(policy?.approvedBy || '').trim() ? <div className="execution-detail-line">Approved by: {String(policy.approvedBy).trim()}</div> : null}
      {String(policy?.approvedAt || '').trim() ? <div className="execution-detail-line">Approved at: {formatDateTime(policy.approvedAt)}</div> : null}
      {Array.isArray(toolPolicy?.allowedTools) && toolPolicy.allowedTools.length ? (
        <div className="execution-detail-line">Allowed tools: {toolPolicy.allowedTools.map((item: unknown) => String(item || '').trim()).filter(Boolean).join(', ')}</div>
      ) : null}
      {targets.length ? <div className="execution-detail-line">Worker scope</div> : null}
      {targets.map((item: any, index: number) => {
        const host = String(item?.hostId || '').trim();
        const hostLabels = Array.isArray(item?.hostLabels) ? item.hostLabels.map((value: unknown) => String(value || '').trim()).filter(Boolean) : [];
        return (
          <div key={`${host || hostLabels.join(',')}-${item?.agentId || 'unknown'}-${index}`} className="execution-detail-line">
            {host || (hostLabels.length ? `labels[${hostLabels.join(',')}]` : 'local')}/{String(item?.agentId || '').trim() || 'unknown'} · count={toFiniteNumber(item?.count, 1) || 1}
          </div>
        );
      })}
    </div>
  );
}
