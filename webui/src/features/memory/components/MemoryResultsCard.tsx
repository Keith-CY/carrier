import type { MemoryData } from '../useMemoryData';

export function MemoryResultsCard({ data }: { data: MemoryData }) {
  const { searchResults } = data;

  return (
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
  );
}
