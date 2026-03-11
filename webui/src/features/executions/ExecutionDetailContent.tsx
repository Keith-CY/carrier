import { formatAgeSeconds, formatDateTime, formatMilliseconds, formatUSD, toFiniteNumber } from '../../lib/format';

function executionStatusBadgeClass(status: unknown): string {
  const normalized = String(status || '').trim().toLowerCase();
  if (normalized === 'completed') return 'badge badge-ok';
  if (normalized === 'partial_completed') return 'badge badge-warn';
  if (['failed', 'retryable_failed', 'declined', 'cancelled'].includes(normalized)) return 'badge badge-error';
  return 'badge badge-unknown';
}

export function isExecutionTerminalStatus(status: unknown): boolean {
  const normalized = String(status || '').trim().toLowerCase();
  return ['completed', 'partial_completed', 'failed', 'retryable_failed', 'cancelled', 'declined'].includes(normalized);
}

export function executionHasFailedTasks(execution: any): boolean {
  const results = Array.isArray(execution?.results) ? execution.results : [];
  return results.some((item) => String(item?.status || '').trim().toLowerCase() === 'failed');
}

export function artifactDownloadPath(executionID: string, artifactID: string): string {
  return `/api/v1/orchestrator/executions/${encodeURIComponent(String(executionID || '').trim())}/artifacts/${encodeURIComponent(String(artifactID || '').trim())}`;
}

function ExecutionResults({ execution, workers }: { execution: any; workers: any[] }) {
  const taskUnits = Array.isArray(execution?.taskUnits) ? execution.taskUnits : [];
  const results = Array.isArray(execution?.results) ? execution.results : [];
  const resultByTaskId = new Map(
    results
      .map((item: any) => [String(item?.taskId || '').trim(), item] as const)
      .filter(([taskId]) => taskId),
  );

  return (
    <>
      <div className="execution-detail-block">
        <div className="execution-detail-title">Workers</div>
        {workers.length ? workers.map((worker, index) => (
          <div key={`${worker?.hostId || 'local'}-${worker?.agentId || 'unknown'}-${index}`} className="execution-detail-line">
            {(String(worker?.hostId || '').trim() || 'local')}/{String(worker?.agentId || '').trim() || 'unknown'} · state={String(worker?.state || '').trim() || 'unknown'}
          </div>
        )) : <div className="text-dim">No worker leases recorded.</div>}
      </div>

      <div className="execution-detail-block">
        <div className="execution-detail-title">Task Results</div>
        {taskUnits.length ? taskUnits.map((task: any, index: number) => {
          const taskID = String(task?.id || `task-${index + 1}`).trim();
          const result = resultByTaskId.get(taskID) || {};
          const output = String(result.output || result.error || '').trim();
          const metaParts = [];
          if (result.hostId || result.agentId) metaParts.push(`${String(result.hostId || '').trim() || 'local'}/${String(result.agentId || '').trim() || 'unknown'}`);
          if (toFiniteNumber(result.attempts, 0) > 0) metaParts.push(`attempts=${toFiniteNumber(result.attempts, 0)}`);
          if (toFiniteNumber(result.latencyMs, 0) > 0) metaParts.push(`latency=${Math.round(toFiniteNumber(result.latencyMs, 0))}ms`);
          return (
            <div key={taskID} className="execution-result-item">
              <div className="execution-result-header">
                <strong>{taskID}</strong>
                <span className={executionStatusBadgeClass(result.status || execution?.status)}>
                  {String(result.status || execution?.status || 'pending').trim()}
                </span>
              </div>
              {String(task?.input || '').trim() ? <div className="execution-result-body">{String(task.input).trim()}</div> : null}
              {String(result.summary || '').trim() ? <div className="execution-result-body">{String(result.summary).trim()}</div> : null}
              {(String(result.failureReason || '').trim() || String(result.failureCategory || '').trim()) ? (
                <div className="execution-result-meta">
                  {[String(result.failureReason || '').trim() ? `reason=${String(result.failureReason).trim()}` : '', String(result.failureCategory || '').trim() ? `category=${String(result.failureCategory).trim()}` : ''].filter(Boolean).join(' · ')}
                </div>
              ) : null}
              {output ? <pre className="code-block execution-result-output">{output}</pre> : null}
              {metaParts.length ? <div className="execution-result-meta">{metaParts.join(' · ')}</div> : null}
            </div>
          );
        }) : <div className="text-dim">No task units recorded.</div>}
      </div>
    </>
  );
}

export function ExecutionDetailContent({ execution, workers, onDownloadArtifact }: {
  execution: any;
  workers: any[];
  onDownloadArtifact: (artifactId: string, filename: string) => void | Promise<void>;
}) {
  const statusText = String(execution?.status || 'unknown').trim();
  const triggerSource = String(execution?.triggerSource || '').trim();
  const triggerID = String(execution?.triggerId || '').trim();
  const triggerEvent = String(execution?.triggerEvent || '').trim();
  const initiator = String(execution?.initiator || '').trim();
  const parentExecutionID = String(execution?.parentExecutionId || '').trim();
  const sourceExecutionID = String(execution?.sourceExecutionId || '').trim();
  const launchReason = String(execution?.launchReason || '').trim();
  const outcome = execution?.outcome && typeof execution.outcome === 'object' ? execution.outcome : {};
  const artifacts = Array.isArray(outcome?.artifacts) ? outcome.artifacts : [];
  const policy = execution?.policy && typeof execution.policy === 'object' ? execution.policy : {};
  const authorization = execution?.authorization && typeof execution.authorization === 'object' ? execution.authorization : {};
  const requiredMemory = Array.isArray(execution?.requiredMemory) ? execution.requiredMemory.map((item: unknown) => String(item || '').trim()).filter(Boolean) : [];
  const memoryProvenance = Array.isArray(execution?.memoryProvenance) ? execution.memoryProvenance.map((item: unknown) => String(item || '').trim()).filter(Boolean) : [];
  const distillOutputs = Array.isArray(execution?.distillOutputs) ? execution.distillOutputs.map((item: unknown) => String(item || '').trim()).filter(Boolean) : [];
  const providerResolutions = Array.isArray(execution?.governance?.providerResolutions) ? execution.governance.providerResolutions : [];
  const toolPolicy = policy?.toolPolicy && typeof policy.toolPolicy === 'object' ? policy.toolPolicy : {};
  const targets = Array.isArray(policy?.targets) ? policy.targets : [];

  return (
    <>
      <div className="executions-detail-summary">
        <div className="section-head">
          <div>
            <h3>{String(execution?.goal || '').trim() || '(no goal)'}</h3>
            <div className="execution-detail-line">
              ID: {String(execution?.id || '').trim()} · Updated: {formatDateTime(execution?.updatedAt)}
            </div>
          </div>
          <span className={executionStatusBadgeClass(statusText)}>{statusText || 'unknown'}</span>
        </div>
        <div className="execution-detail-line">status: {statusText || 'unknown'}</div>
        {String(execution?.error || '').trim() ? (
          <div className="execution-detail-line">Error: {String(execution.error).trim()}</div>
        ) : null}
      </div>

      {(triggerSource || triggerID || triggerEvent || initiator) ? (
        <div className="execution-detail-block">
          <div className="execution-detail-title">Trigger</div>
          {triggerSource ? <div className="execution-detail-line">source: {triggerSource}</div> : null}
          {triggerID ? <div className="execution-detail-line">id: {triggerID}</div> : null}
          {triggerEvent ? <div className="execution-detail-line">event: {triggerEvent}</div> : null}
          {initiator ? <div className="execution-detail-line">initiator: {initiator}</div> : null}
        </div>
      ) : null}

      {(parentExecutionID || sourceExecutionID || launchReason) ? (
        <div className="execution-detail-block">
          <div className="execution-detail-title">Execution Lineage</div>
          {parentExecutionID ? <div className="execution-detail-line">parent: {parentExecutionID}</div> : null}
          {sourceExecutionID ? <div className="execution-detail-line">source: {sourceExecutionID}</div> : null}
          {launchReason ? <div className="execution-detail-line">launch reason: {launchReason}</div> : null}
        </div>
      ) : null}

      {(String(outcome?.summary || '').trim() || String(outcome?.failureReason || '').trim() || String(outcome?.failureCategory || '').trim() || artifacts.length) ? (
        <div className="execution-detail-block">
          <div className="execution-detail-title">Outcome</div>
          {String(outcome?.summary || '').trim() ? <div className="execution-detail-line">Summary: {String(outcome.summary).trim()}</div> : null}
          {String(outcome?.failureReason || '').trim() ? <div className="execution-detail-line">Failure reason: {String(outcome.failureReason).trim()}</div> : null}
          {String(outcome?.failureCategory || '').trim() ? <div className="execution-detail-line">Failure category: {String(outcome.failureCategory).trim()}</div> : null}
          {artifacts.length ? <div className="execution-detail-line">Artifacts</div> : null}
          {artifacts.map((item: any) => {
            const artifactID = String(item?.id || '').trim();
            const name = String(item?.name || artifactID).trim();
            const metaParts = [];
            if (String(item?.kind || '').trim()) metaParts.push(String(item.kind).trim());
            if (String(item?.contentType || '').trim()) metaParts.push(String(item.contentType).trim());
            if (toFiniteNumber(item?.sizeBytes, 0) > 0) metaParts.push(`${toFiniteNumber(item.sizeBytes, 0)} bytes`);
            if (String(item?.createdAt || '').trim()) metaParts.push(formatDateTime(item.createdAt));
            return (
              <div key={artifactID || name}>
                <div className="execution-detail-line">{name}{metaParts.length ? ` · ${metaParts.join(' · ')}` : ''}</div>
                {artifactID ? (
                  <a
                    className="btn-sm btn-secondary"
                    href={artifactDownloadPath(String(execution?.id || '').trim(), artifactID)}
                    onClick={(event) => {
                      event.preventDefault();
                      void onDownloadArtifact(artifactID, name);
                    }}
                  >
                    Download {name}
                  </a>
                ) : null}
              </div>
            );
          })}
        </div>
      ) : null}

      {String(policy?.decision || '').trim() ? (
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
      ) : null}

      <div className="execution-detail-block">
        <div className="execution-detail-title">Approval &amp; Governance</div>
        <div className="execution-detail-line">Approved by: {String(authorization?.approvedBy || '').trim() || 'n/a'}</div>
        <div className="execution-detail-line">Approved at: {String(authorization?.approvedAt || '').trim() ? formatDateTime(authorization.approvedAt) : 'n/a'}</div>
        <div className="execution-detail-line">Infrastructure approved: {authorization?.infrastructureApproved ? 'yes' : 'no'}</div>
        {String(execution?.requestedProvider || '').trim() ? <div className="execution-detail-line">Requested provider: {String(execution.requestedProvider).trim()}</div> : null}
        {(String(execution?.memoryContractDigest || '').trim() || requiredMemory.length || memoryProvenance.length || distillOutputs.length) ? (
          <div className="execution-detail-line">
            Memory contract: {String(execution?.memoryContractDigest || '').trim() || 'n/a'} · scopes={requiredMemory.length ? requiredMemory.join(', ') : 'none'}
          </div>
        ) : null}
        {memoryProvenance.length ? <div className="execution-detail-line">Memory provenance: {memoryProvenance.join(', ')}</div> : null}
        {distillOutputs.length ? <div className="execution-detail-line">Distill outputs: {distillOutputs.join(', ')}</div> : null}
        {providerResolutions.length ? <div className="execution-detail-line">Provider Governance</div> : (
          <div className="execution-detail-line">Provider Governance: no binding resolution recorded.</div>
        )}
        {providerResolutions.map((item: any, index: number) => {
          const host = String(item?.hostId || '').trim() || 'local';
          const agent = String(item?.agentId || '').trim() || 'unknown';
          const source = String(item?.source || '').trim() || 'none';
          const provider = String(item?.provider || '').trim();
          const model = String(item?.model || '').trim();
          const profileName = String(item?.profileName || item?.profileId || '').trim();
          const status = String(item?.status || '').trim();
          const syncMode = String(item?.syncMode || '').trim();
          const driftState = String(item?.driftState || '').trim();
          const lineParts = [
            `${host}/${agent}`,
            `source=${source}`,
            profileName ? `profile=${profileName}` : '',
            provider || model ? [provider, model].filter(Boolean).join('/') : '',
            status ? `status=${status}` : '',
            syncMode ? `sync=${syncMode}` : '',
            toFiniteNumber(item?.estimatedTotalTokens, 0) > 0 ? `tokens=${toFiniteNumber(item.estimatedTotalTokens, 0)}` : '',
            toFiniteNumber(item?.estimatedCostUsd, 0) > 0 ? `cost=${formatUSD(item.estimatedCostUsd)}` : '',
            (toFiniteNumber(item?.successfulTasks, 0) > 0 || toFiniteNumber(item?.failedTasks, 0) > 0)
              ? `tasks=${toFiniteNumber(item?.successfulTasks, 0)}/${toFiniteNumber(item?.failedTasks, 0)}`
              : '',
            driftState ? `drift=${driftState}` : '',
            toFiniteNumber(item?.avgLatencyMs, 0) > 0 ? `latency=${formatMilliseconds(item.avgLatencyMs)}` : '',
          ].filter(Boolean);
          const trace = Array.isArray(item?.trace) ? item.trace : [];
          return (
            <div key={`${host}-${agent}-${index}`}>
              <div className="execution-detail-line">{lineParts.join(' · ')}</div>
              {String(item?.driftReason || '').trim() ? <div className="execution-detail-line">{String(item.driftReason).trim()}</div> : null}
              {trace.map((traceItem: any, traceIndex: number) => {
                const providerModel = [String(traceItem?.provider || '').trim(), String(traceItem?.model || '').trim()].filter(Boolean).join('/');
                return (
                  <div key={`${index}-${traceIndex}`} className="execution-detail-line">
                    {String(traceItem?.source || '').trim() || 'unknown'} [{String(traceItem?.status || '').trim() || 'unknown'}{traceItem?.selected ? ', selected' : ''}]{providerModel ? ` ${providerModel}` : ''}
                  </div>
                );
              })}
              {String(item?.message || '').trim() ? <div className="execution-detail-line">{String(item.message).trim()}</div> : null}
            </div>
          );
        })}
      </div>

      <ExecutionResults execution={execution} workers={workers} />
    </>
  );
}

export function executionCounts(execution: any) {
  const taskUnits = Array.isArray(execution?.taskUnits) ? execution.taskUnits : [];
  const results = Array.isArray(execution?.results) ? execution.results : [];
  return {
    taskUnits,
    results,
    completed: results.filter((item) => String(item?.status || '').trim().toLowerCase() === 'completed').length,
    failed: results.filter((item) => ['failed', 'cancelled'].includes(String(item?.status || '').trim().toLowerCase())).length,
  };
}

export function executionTemplateValue(execution: any): string {
  return String(execution?.templateId || '').trim();
}

export function executionTriggerValue(execution: any): string {
  return String(execution?.triggerSource || '').trim().toLowerCase();
}

export function executionTriggerLabel(execution: any): string {
  const source = String(execution?.triggerSource || '').trim();
  const triggerID = String(execution?.triggerId || '').trim();
  if (source && triggerID) return `${source}:${triggerID}`;
  return source || triggerID;
}

export function executionSearchText(execution: any): string {
  return [
    String(execution?.id || ''),
    String(execution?.goal || ''),
    String(execution?.team || ''),
    String(execution?.project || ''),
    String(execution?.environment || ''),
    executionTemplateValue(execution),
    String(execution?.triggerSource || ''),
    String(execution?.triggerId || ''),
    executionTriggerLabel(execution),
    String(execution?.initiator || ''),
  ].join(' ').trim().toLowerCase();
}

export function executionAttributionParts(execution: any): string[] {
  const parts = [];
  if (String(execution?.team || '').trim()) parts.push(`Team: ${String(execution.team).trim()}`);
  if (String(execution?.project || '').trim()) parts.push(`Project: ${String(execution.project).trim()}`);
  if (String(execution?.environment || '').trim()) parts.push(`Env: ${String(execution.environment).trim()}`);
  if (executionTemplateValue(execution)) parts.push(`Template: ${executionTemplateValue(execution)}`);
  if (executionTriggerLabel(execution)) parts.push(`Trigger: ${executionTriggerLabel(execution)}`);
  return parts;
}
