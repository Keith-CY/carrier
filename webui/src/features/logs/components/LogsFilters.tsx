import type { LogsData, LogLevel } from '../useLogsData';
import { LOG_FILTER_LEVELS } from '../useLogsData';

export function LogsFilters({ data }: { data: LogsData }) {
  return (
    <div className="log-filters" role="group" aria-label="Filter log levels">
      {LOG_FILTER_LEVELS.map((level: LogLevel) => (
        <label key={level} className="log-filter-pill">
          <input
            id={`log-filter-${level.toLowerCase()}`}
            type="checkbox"
            checked={data.filters[level]}
            onChange={(event) => data.setFilters((current) => ({ ...current, [level]: event.target.checked }))}
          />
          {level}
        </label>
      ))}
    </div>
  );
}
