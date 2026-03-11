import { formatDateTime } from '../../../lib/format';
import { executionStatusBadgeClass } from './detailShared';

export function ExecutionSummaryBlock({ execution }: { execution: any }) {
  const statusText = String(execution?.status || 'unknown').trim();

  return (
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
  );
}
