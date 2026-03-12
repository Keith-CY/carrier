import { formatHostMeta, updateEditorFromHost } from '../model';
import type { HostsData } from '../useHostsData';

export function HostsList({ data }: { data: HostsData }) {
  const {
    hosts,
    canManageHosts,
    hostOps,
    handleCheckHost,
    showManageHost,
    startEditingHost,
    setEditingHostId,
    setEditor,
    setServersMessage,
    handleDeleteHost,
  } = data;

  return (
    <div id="servers-list" className="agent-grid server-host-list">
      {hosts.length === 0 ? (
        <div className="card">No remote servers configured.</div>
      ) : hosts.map((host) => {
        const hostId = String(host?.id || '');
        return (
          <div key={hostId} className="agent-card">
            <h4>{String(host?.name || hostId)}</h4>
            <div className="instance-meta" style={{ whiteSpace: 'pre-line' }}>{formatHostMeta(host, hostOps[hostId])}</div>
            {canManageHosts ? (
              <div className="btn-row">
                <button className="btn-sm btn-secondary" onClick={() => void handleCheckHost(hostId)}>Check</button>
                <button className="btn-sm" onClick={() => showManageHost(hostId)}>Manage</button>
                <button
                  className="btn-sm btn-secondary"
                  onClick={() => {
                    if (typeof startEditingHost === 'function') {
                      startEditingHost(host);
                      return;
                    }
                    setEditingHostId(hostId);
                    setEditor(updateEditorFromHost(host));
                    setServersMessage({ type: 'info', text: `Editing remote host: ${hostId}` });
                  }}
                >
                  Edit
                </button>
                <button className="btn-sm btn-danger" onClick={() => void handleDeleteHost(hostId)}>Delete</button>
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
