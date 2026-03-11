import { useHostsData } from './useHostsData';
import { HostEditorCard, HostManagePanel, HostsList } from './HostsSections';

export function HostsPage() {
  const data = useHostsData();

  if (data.featuresLoading) {
    return <section id="view-servers" className="view view-servers-surface"><div className="card">Loading hosts…</div></section>;
  }

  return (
    <section id="view-servers" className="view view-servers-surface">
      <div className="section-head">
        <h2>Hosts</h2>
        <div className="section-actions">
          <button id="servers-refresh" className="btn-sm btn-secondary" onClick={() => void data.refresh()}>
            Refresh
          </button>
        </div>
      </div>
      <HostEditorCard data={data} />
      <HostsList data={data} />
      <HostManagePanel data={data} />
    </section>
  );
}
