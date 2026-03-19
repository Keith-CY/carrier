import { formatDateTime } from '../../../lib/format';
import { executionAttributionParts } from '../../executions/ExecutionDetailContent';
import type { DashboardData } from '../useDashboardData';
import { DashboardExecutionDetail } from './DashboardExecutionDetail';

export function RecentExecutionsCard({ data }: { data: DashboardData }) {
  const {
    featureFlags,
    authz,
    recentExecutions,
    activeRecentExecutions,
    expandedExecutionIds,
    executionCounts,
    refreshExecutions,
    toggleExecutionExpansion,
    navigate,
  } = data;

  return (
    <section id="dashboard-executions-section" className={`card dashboard-panel dashboard-panel--executions${featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions ? '' : ' hidden'}`}>
      <div className="section-head">
        <div>
          <h2>Recent Executions</h2>
          <p className="text-dim">Track the latest plans, approvals, and task-unit progress without leaving the dashboard.</p>
        </div>
        <div className="section-actions">
          <button id="refresh-executions" className="btn-sm btn-secondary" onClick={() => void refreshExecutions()}>
            Refresh
          </button>
        </div>
      </div>
      <p id="execution-summary" className="text-dim">
        {recentExecutions.length ? `Recent: ${recentExecutions.length} · Active: ${activeRecentExecutions}` : 'No executions recorded yet.'}
      </p>
      <div id="execution-list" className="agent-grid">
        {recentExecutions.map((execution: any) => {
          const executionId = String(execution?.id || '').trim();
          const statusText = String(execution?.status || 'unknown').trim();
          const counts = executionCounts(execution);
          const isExpanded = expandedExecutionIds.has(executionId);
          return (
            <div key={executionId} className="agent-card execution-card">
              <div className="section-head">
                <h4>{executionId || 'execution'}</h4>
                <span className={statusText === 'completed' ? 'badge badge-ok' : statusText === 'running' ? 'badge badge-warn' : 'badge badge-unknown'}>{statusText || 'unknown'}</span>
              </div>
              <div className="execution-goal">{String(execution?.goal || '').trim() || '(no goal)'}</div>
              <div className="instance-meta">
                Tasks: {counts.taskUnits.length} · Completed: {counts.completed} · Failed: {counts.failed} · Updated: {formatDateTime(execution?.updatedAt)}
              </div>
              {executionAttributionParts(execution).length ? <div className="instance-meta">{executionAttributionParts(execution).join(' · ')}</div> : null}
              <div className="btn-row">
                <button className="btn-sm" onClick={() => navigate(`/executions/${encodeURIComponent(executionId)}`)}>Open</button>
                <button className="btn-sm btn-secondary" onClick={() => toggleExecutionExpansion(executionId)}>
                  {isExpanded ? 'Hide Details' : 'View Details'}
                </button>
              </div>
              <div className={`execution-details${isExpanded ? '' : ' hidden'}`}>
                {isExpanded ? <DashboardExecutionDetail executionId={executionId} /> : null}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
