import { PageShell } from '../../app/page-shell';
import { useHostsData } from './useHostsData';
import { HostEditorCard, HostManagePanel, HostsList } from './HostsSections';

export function HostsPage() {
  const data = useHostsData();
  const hosts = Array.isArray(data.hosts) ? data.hosts : [];
  const labeledHosts = hosts.filter((host) => Array.isArray(host?.labels) && host.labels.length > 0).length;
  const selectedHostName = data.selectedHost ? String(data.selectedHost?.name || data.selectedHost?.id || data.selectedHostId || 'None') : 'None';

  if (data.featuresLoading) {
    return (
      <PageShell
        id="view-servers"
        className="page-hosts"
        eyebrow="Operate"
        title="Hosts"
        description="Manage the remote estate, sync agent runtimes, and inspect host-level operational state."
      >
        <div className="card">Loading hosts…</div>
      </PageShell>
    );
  }

  return (
    <PageShell
      id="view-servers"
      className="page-hosts"
      eyebrow="Operate"
      title="Hosts"
      description="Add infrastructure targets, validate connectivity, and drill into runtime state from a single host operations workspace."
      actions={(
        <button id="servers-refresh" className="btn-sm btn-secondary" onClick={() => void data.refresh()}>
          Refresh
        </button>
      )}
      stats={[
        { label: 'Inventory', value: String(hosts.length) },
        { label: 'Labeled', value: String(labeledHosts) },
        { label: 'Selected Host', value: selectedHostName },
      ]}
    >
      <div className="hosts-premium-layout">
        <div className="hosts-premium-stack">
          <HostEditorCard data={data} />
          <HostsList data={data} />
        </div>
        <div className="hosts-premium-stack">
          {!data.selectedHost ? (
            <div className="card hosts-manage-placeholder">
              <h3>Runtime Workbench</h3>
              <p className="text-dim">Select a host from the inventory to inspect logs, instances, config, sessions, and memory.</p>
            </div>
          ) : null}
          <HostManagePanel data={data} />
        </div>
      </div>
    </PageShell>
  );
}
