import { toFiniteNumber } from '../../../lib/format';
import { executionStatusBadgeClass } from './detailShared';

export function ExecutionResultsBlock({ execution, workers }: { execution: any; workers: any[] }) {
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
