import type { LogsData } from './useLogsData';
import { LogsFilters } from './components/LogsFilters';
import { LogsTable } from './components/LogsTable';
import { LogsToolbarCard } from './components/LogsToolbarCard';

export function LogsSection({ data }: { data: LogsData }) {
  return (
    <section id="view-logs" className="view view-logs-surface">
      <div className="section-head">
        <h2>Logs</h2>
      </div>
      <div className="card logs-panel">
        <LogsToolbarCard data={data} />
        <LogsFilters data={data} />
        <LogsTable data={data} />
      </div>
    </section>
  );
}
