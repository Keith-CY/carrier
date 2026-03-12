import type { LogsData } from '../useLogsData';

export function LogsToolbarCard({ data }: { data: LogsData }) {
  return (
    <div className="log-controls">
      <label htmlFor="log-agent">Agent:</label>
      <select id="log-agent" value={data.selectedAgent} onChange={(event) => data.setSelectedAgent(event.target.value)}>
        {data.options.map((option) => (
          <option key={option.value || option.label} value={option.value}>{option.label}</option>
        ))}
      </select>
      <button id="log-connect" className="btn-sm" type="button" onClick={data.connect}>Connect</button>
      <button id="log-pause" className="btn-sm btn-secondary" type="button" onClick={data.togglePause}>{data.paused ? 'Resume' : 'Pause'}</button>
      <button id="log-clear" className="btn-sm btn-secondary" type="button" onClick={data.clear}>Clear</button>
      <input
        id="log-search"
        type="text"
        placeholder="Search logs..."
        autoComplete="off"
        aria-label="Search logs"
        value={data.searchQuery}
        onChange={(event) => data.setSearchQuery(event.target.value)}
      />
    </div>
  );
}
