import type { DashboardData } from '../useDashboardData';

function statusIcon(value: unknown): string {
  const status = String(value || '').trim().toLowerCase();
  if (status === 'running' || status === 'healthy') return '🟢';
  if (status === 'error') return '🔴';
  return '⚪';
}

export function InstalledAgentsCard({ data }: { data: DashboardData }) {
  const { instancesQuery, instances, runningInstances, refreshInstances, handleInstanceAction, setAddAgentModalOpen } = data;

  return (
    <>
      <div className="section-head">
        <h2>Installed Agents</h2>
        <div className="section-actions">
          <button id="dashboard-add-agent" type="button" onClick={() => setAddAgentModalOpen(true)}>Add Agent</button>
          <button id="refresh-instances" className="btn-sm btn-secondary" onClick={() => void refreshInstances()}>
            Refresh
          </button>
        </div>
      </div>
      <p id="instance-summary" className="text-dim">
        {instancesQuery.isLoading ? 'Loading…' : instances.length ? `Total: ${instances.length} · Running: ${runningInstances}` : 'Use Add Agent to install and configure a new instance.'}
      </p>
      <div id="instance-list" className="agent-grid">
        {!instancesQuery.isLoading && !instances.length ? 'No installed agent instances.' : instances.map((item: any) => {
          const instanceId = String(item?.id || item?.ID || '').trim();
          const agentId = String(item?.agent_id || item?.agentID || item?.agent || item?.type || 'unknown').trim();
          const runtime = String(item?.runtime_state || item?.runtimeState || item?.runtime || 'unknown').trim();
          const pairRequired = !!(item?.pair_required || item?.pairRequired);
          const pairedChatId = String(item?.paired_chat_id || item?.pairedChatId || '').trim();
          let metaText = `Type: ${agentId} · Channel: ${String(item?.channel || 'n/a')} · Provider: ${String(item?.provider || 'n/a')}`;
          if (pairRequired) metaText += ' · Pair: required';
          else if (pairedChatId) metaText += ` · Paired chat: ${pairedChatId}`;
          return (
            <div key={instanceId} className="agent-card">
              <h4>{instanceId}</h4>
              <div className="agent-status">{statusIcon(runtime)} {runtime}</div>
              <div className="instance-meta">{metaText}</div>
              <div className="btn-row">
                <button className="btn-sm" onClick={() => void handleInstanceAction(instanceId, 'start')}>Start</button>
                <button className="btn-sm btn-secondary" onClick={() => void handleInstanceAction(instanceId, 'stop')}>Stop</button>
                <button className="btn-sm btn-danger" onClick={() => void handleInstanceAction(instanceId, 'uninstall')}>Uninstall</button>
              </div>
            </div>
          );
        })}
      </div>
    </>
  );
}
