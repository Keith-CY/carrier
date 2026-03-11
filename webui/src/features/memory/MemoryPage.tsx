import { useMemoryData } from './useMemoryData';

function renderMemorySection(items: { title: string; meta: string[] }[], empty: string) {
  if (!items.length) return <div className="text-dim">{empty}</div>;
  return items.map((item) => (
    <div key={`${item.title}-${item.meta.join('|')}`} className="agent-card memory-card">
      <strong>{item.title}</strong>
      {item.meta.map((line) => (
        <div key={line} className="execution-detail-line">{line}</div>
      ))}
    </div>
  ));
}

export function MemoryPage() {
  const {
    canMutate,
    readOnly,
    subject,
    setSubject,
    searchQuery,
    setSearchQuery,
    searchLimit,
    setSearchLimit,
    searchMinScore,
    setSearchMinScore,
    instanceId,
    setInstanceId,
    instanceScope,
    setInstanceScope,
    distillReason,
    setDistillReason,
    distillDryRun,
    setDistillDryRun,
    message,
    actionMessage,
    searchResults,
    memoryQuery,
    payload,
    refreshMemory,
    runSearch,
    runAction,
  } = useMemoryData();

  return (
    <section id="view-memory" className="view">
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

      <div className="card dashboard-stack">
        <div className="section-head">
          <div>
            <h3>Instance Actions</h3>
            <p className="text-dim">Attach or detach a scope, then distill learnings back into base memory.</p>
          </div>
        </div>
        <div className="form-grid">
          <div>
            <label htmlFor="memory-instance-id">Instance ID</label>
            <input id="memory-instance-id" type="text" placeholder="picoclaw-main" value={instanceId} onChange={(event) => setInstanceId(event.target.value)} />
          </div>
          <div>
            <label htmlFor="memory-instance-scope">Scope</label>
            <input id="memory-instance-scope" type="text" placeholder="shared:profile" value={instanceScope} onChange={(event) => setInstanceScope(event.target.value)} />
          </div>
          <div>
            <label htmlFor="memory-distill-reason">Distill Reason</label>
            <input id="memory-distill-reason" type="text" placeholder="promote learnings" value={distillReason} onChange={(event) => setDistillReason(event.target.value)} />
          </div>
          <div>
            <label htmlFor="memory-distill-dry-run">Distill Dry Run</label>
            <input id="memory-distill-dry-run" type="checkbox" checked={distillDryRun} onChange={(event) => setDistillDryRun(event.target.checked)} />
          </div>
        </div>
        <div className="btn-row">
          <button id="memory-attach" className="btn-sm" disabled={!canMutate} onClick={() => runAction('attach')}>Attach</button>
          <button id="memory-detach" className="btn-sm btn-secondary" disabled={!canMutate} onClick={() => runAction('detach')}>Detach</button>
          <button id="memory-distill" className="btn-sm" disabled={!canMutate} onClick={() => runAction('distill')}>Distill</button>
        </div>
        <div id="memory-action-msg">{actionMessage.text ? <p className={`msg-${actionMessage.type}`}>{actionMessage.text}</p> : null}</div>
      </div>

      <div className="card dashboard-stack">
        <div className="section-head">
          <div>
            <h3>Packages</h3>
            <p className="text-dim">Current packages, attachments, and grants for the selected subject.</p>
          </div>
        </div>
        <div id="memory-entry-list">
          <div className="memory-section">
            <h4>Entries</h4>
            {renderMemorySection(payload.entries.map((entry: any) => ({
              title: String(entry?.id || 'unknown'),
              meta: [`type: ${String(entry?.type || 'unknown')}`],
            })), 'No memory packages found.')}
          </div>
          <div className="memory-section">
            <h4>Attachments</h4>
            {renderMemorySection(payload.attachments.map((attachment: any) => ({
              title: String(attachment?.memory_id || 'unknown'),
              meta: [`agent: ${String(attachment?.agent_id || 'unknown')}`],
            })), 'No attachments for this subject.')}
          </div>
          <div className="memory-section">
            <h4>Grants</h4>
            {renderMemorySection(payload.grants.map((grant: any) => ({
              title: String(grant?.scope || 'unknown'),
              meta: [`subject: ${String(grant?.subject || 'unknown')}`],
            })), 'No grants for this subject.')}
          </div>
        </div>
      </div>

      <div className="card dashboard-stack">
        <div className="section-head">
          <div>
            <h3>Search Results</h3>
            <p className="text-dim">Curated hits that the base agent can later distill and promote.</p>
          </div>
        </div>
        <div id="memory-search-results">
          {searchResults.length ? searchResults.map((result: any) => (
            <div key={String(result?.id || 'unknown')} className="agent-card memory-card">
              <strong>{String(result?.id || 'unknown')}</strong>
              <div className="execution-detail-line">
                scope={String(result?.scope || 'unknown')} · score={Number(result?.score || 0).toFixed(2)}
              </div>
              <div className="text-dim">{String(result?.snippet || '')}</div>
            </div>
          )) : <div className="text-dim">No search results yet.</div>}
        </div>
      </div>
    </section>
  );
}
