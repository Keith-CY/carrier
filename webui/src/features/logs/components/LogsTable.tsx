import { HighlightedText } from './HighlightedText';
import type { LogsData } from '../useLogsData';

export function LogsTable({ data }: { data: LogsData }) {
  const query = data.searchQuery.trim().toLowerCase();

  return (
    <>
      <p id="log-status" className="text-dim log-status">{data.statusText}</p>
      <div className="log-table">
        <div className="log-row log-row-header" aria-hidden="true">
          <span className="log-cell-time">Timestamp</span>
          <span className="log-cell-level">Level</span>
          <span className="log-cell-message">Message</span>
        </div>
        <div id="log-output" className="log-rows" role="log" aria-live="polite">
          {data.visibleEntries.map((entry) => (
            <div key={entry.id} className="log-row log-row-data" data-level={entry.level}>
              <span className="log-cell-time">
                <HighlightedText text={entry.timestamp} query={query} />
              </span>
              <span className="log-cell-level">
                <span className="log-level-pill">
                  <HighlightedText text={entry.level} query={query} />
                </span>
              </span>
              <span className="log-cell-message">
                <HighlightedText text={entry.message} query={query} />
              </span>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}
