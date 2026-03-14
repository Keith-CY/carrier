import type { MemoryData } from '../useMemoryData';

export function MemorySearchCard({ data }: { data: MemoryData }) {
  const {
    readOnly,
    subject,
    setSubject,
    searchQuery,
    setSearchQuery,
    searchLimit,
    setSearchLimit,
    searchMinScore,
    setSearchMinScore,
    message,
    memoryQuery,
    payload,
    refreshMemory,
    runSearch,
  } = data;

  return (
    <div className="card dashboard-stack">
      <div className="section-head">
        <div>
          <h2>Memory</h2>
          <p className="text-dim">Inspect packages, search curated records, and manage instance memory actions.</p>
        </div>
        <div className="section-actions">
          <button id="memory-refresh" className="btn-sm btn-secondary" onClick={() => void refreshMemory()}>
            Refresh
          </button>
        </div>
      </div>
      <div className="form-grid">
        <div>
          <label htmlFor="memory-subject">Subject</label>
          <input id="memory-subject" type="text" placeholder="agent-a" value={subject} onChange={(event) => setSubject(event.target.value)} />
        </div>
        <div>
          <label htmlFor="memory-search-query">Search Query</label>
          <input id="memory-search-query" type="text" placeholder="fusion" value={searchQuery} onChange={(event) => setSearchQuery(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') runSearch(); }} />
        </div>
        <div>
          <label htmlFor="memory-search-limit">Limit</label>
          <input id="memory-search-limit" type="number" min={1} max={50} value={searchLimit} onChange={(event) => setSearchLimit(event.target.value)} />
        </div>
        <div>
          <label htmlFor="memory-search-min-score">Min Score</label>
          <input id="memory-search-min-score" type="number" min={0} max={1} step="0.05" value={searchMinScore} onChange={(event) => setSearchMinScore(event.target.value)} />
        </div>
      </div>
      <div className="btn-row">
        <button id="memory-search-run" className="btn-sm" onClick={runSearch}>
          Search
        </button>
      </div>
      <p id="memory-summary" className="text-dim">
        {memoryQuery.isLoading
          ? 'Loading memory…'
          : `subject=${payload.subject} · entries=${payload.entries.length} · attachments=${payload.attachments.length} · grants=${payload.grants.length} · audit=${payload.audit.length}`}
      </p>
      <div id="memory-msg">
        {readOnly ? <p className="msg-info">Current role has read-only memory access.</p> : null}
        {message.text ? <p className={`msg-${message.type}`}>{message.text}</p> : null}
      </div>
    </div>
  );
}
