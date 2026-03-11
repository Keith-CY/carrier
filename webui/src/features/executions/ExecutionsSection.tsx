import type { ExecutionsData } from './useExecutionsData';
import { ExecutionDetailCard } from './components/ExecutionDetailCard';
import { ExecutionsListCard } from './components/ExecutionsListCard';
import { ExecutionsToolbarCard } from './components/ExecutionsToolbarCard';

export function ExecutionsSection({ data }: { data: ExecutionsData }) {
  return (
    <section id="view-executions" className="view">
      <div className="section-head">
        <h2>Executions</h2>
        <div className="section-actions">
          <button id="executions-refresh" className="btn-sm btn-secondary" onClick={() => void data.refreshExecutions()}>
            Refresh
          </button>
        </div>
      </div>

      <ExecutionsToolbarCard data={data} />

      <div className="executions-layout">
        <ExecutionsListCard data={data} />
        <ExecutionDetailCard data={data} />
      </div>
    </section>
  );
}
