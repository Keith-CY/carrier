import type { DashboardData } from '../useDashboardData';

export function AddAgentModal({ data }: { data: DashboardData }) {
  const { addAgentModalOpen, setAddAgentModalOpen, agentCatalog, agentCatalogQuery, navigate } = data;

  return (
    <div id="add-agent-overlay" className={`overlay${addAgentModalOpen ? '' : ' hidden'}`}>
      <div className="card install-modal">
        <div className="section-head">
          <h3>Add Agent</h3>
        </div>
        <p className="text-dim">Select an agent to start the add flow.</p>
        <ul id="add-agent-options" className="agent-select-list">
          {agentCatalog.map((item: any) => {
            const id = String(item?.id || '').trim();
            return (
              <li key={id}>
                <button
                  type="button"
                  className="agent-select-item"
                  onClick={() => {
                    setAddAgentModalOpen(false);
                    navigate(`/add/${encodeURIComponent(id)}`);
                  }}
                >
                  {id}
                </button>
              </li>
            );
          })}
        </ul>
        <div id="add-agent-msg">
          {agentCatalogQuery.isLoading ? <p className="msg-info">Loading agents…</p> : null}
          {agentCatalogQuery.isError ? <p className="msg-error">{`Error loading agents: ${(agentCatalogQuery.error as Error).message}`}</p> : null}
          {!agentCatalogQuery.isLoading && !agentCatalogQuery.isError && !agentCatalog.length ? <p className="msg-error">No agents available.</p> : null}
        </div>
        <div className="btn-row">
          <button id="add-agent-cancel" className="btn-secondary" type="button" onClick={() => setAddAgentModalOpen(false)}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
