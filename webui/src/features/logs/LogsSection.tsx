import { PageShell } from '../../app/page-shell';
import type { LogsData } from './useLogsData';
import { LogsFilters } from './components/LogsFilters';
import { LogsTable } from './components/LogsTable';
import { LogsToolbarCard } from './components/LogsToolbarCard';

export function LogsSection({ data }: { data: LogsData }) {
  return (
    <PageShell
      id="view-logs"
      className="page-logs"
      eyebrow="Observe"
      title="Logs"
      description="Inspect runtime output, narrow incidents with filters, and keep diagnostics readable under pressure."
    >
      <div className="card logs-panel">
        <LogsToolbarCard data={data} />
        <LogsFilters data={data} />
        <LogsTable data={data} />
      </div>
    </PageShell>
  );
}
