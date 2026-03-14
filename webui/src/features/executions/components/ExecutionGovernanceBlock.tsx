import { formatDateTime, formatMilliseconds, formatUSD, toFiniteNumber } from '../../../lib/format';

export function ExecutionGovernanceBlock({ execution }: { execution: any }) {
  const authorization = execution?.authorization && typeof execution.authorization === 'object' ? execution.authorization : {};
  const requiredMemory = Array.isArray(execution?.requiredMemory) ? execution.requiredMemory.map((item: unknown) => String(item || '').trim()).filter(Boolean) : [];
  const memoryProvenance = Array.isArray(execution?.memoryProvenance) ? execution.memoryProvenance.map((item: unknown) => String(item || '').trim()).filter(Boolean) : [];
  const distillOutputs = Array.isArray(execution?.distillOutputs) ? execution.distillOutputs.map((item: unknown) => String(item || '').trim()).filter(Boolean) : [];
  const providerResolutions = Array.isArray(execution?.governance?.providerResolutions) ? execution.governance.providerResolutions : [];

  return (
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
  );
}
