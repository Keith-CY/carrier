import { executionAttributionParts } from '../ExecutionDetailContent';
import type { ExecutionsData } from '../useExecutionsData';

export function ExecutionsListCard({ data }: { data: ExecutionsData }) {
  const { filteredExecutions, selectedExecutionId, setSelectedExecutionId, navigate, executionCounts } = data;

  return (
    <div className="card">
      <div id="executions-list" className="executions-list">
        {filteredExecutions.map((execution) => {
          const executionIdValue = String(execution?.id || '').trim();
          const counts = executionCounts(execution);
          return (
            <button
              key={executionIdValue}
              type="button"
              className={`agent-card execution-card execution-list-card${selectedExecutionId === executionIdValue ? ' active' : ''}`}
              onClick={() => {
                setSelectedExecutionId(executionIdValue);
                navigate(`/executions/${encodeURIComponent(executionIdValue)}`);
              }}
            >
              <div className="section-head">
                <h4>{executionIdValue || 'execution'}</h4>
                <span className={String(execution?.status || '').trim().toLowerCase() === 'completed' ? 'badge badge-ok' : 'badge badge-unknown'}>
                  {String(execution?.status || 'unknown').trim()}
                </span>
              </div>
              <div className="execution-goal">{String(execution?.goal || '').trim() || '(no goal)'}</div>
              <div className="instance-meta">
                Tasks: {counts.taskUnits.length} · Completed: {counts.completed} · Failed: {counts.failed} · Updated: {String(execution?.updatedAt || '').trim() || 'n/a'}
              </div>
              {executionAttributionParts(execution).length ? <div className="instance-meta">{executionAttributionParts(execution).join(' · ')}</div> : null}
            </button>
          );
        })}
      </div>
    </div>
  );
}
