import { useMemo, useState } from 'react';
import { LOG_FILTER_LEVELS, type LogLevel } from './model';
import { useLogStream } from './useLogStream';
import { useLogTargets } from './useLogTargets';

export function useLogsData() {
  const [searchQuery, setSearchQuery] = useState('');
  const [filters, setFilters] = useState<Record<LogLevel, boolean>>({
    DEBUG: true,
    INFO: true,
    WARN: true,
    ERROR: true,
    UNKNOWN: true,
  });

  const targets = useLogTargets();
  const stream = useLogStream(targets.selectedAgent, targets.options[0]?.value || '', targets.setStatusBase);

  const visibleEntries = useMemo(() => {
    const normalizedQuery = searchQuery.trim().toLowerCase();
    return stream.entries.filter((entry) => {
      if (Object.prototype.hasOwnProperty.call(filters, entry.level) && !filters[entry.level]) return false;
      if (!normalizedQuery) return true;
      return `${entry.timestamp} ${entry.level} ${entry.message}`.toLowerCase().includes(normalizedQuery);
    });
  }, [filters, searchQuery, stream.entries]);

  const statusText = useMemo(() => {
    const parts = [targets.statusBase];
    if (stream.entries.length > 0) parts.push(`showing ${visibleEntries.length}/${stream.entries.length}`);
    if (stream.paused) parts.push('paused');
    if (stream.bufferLength) parts.push(`buffered ${stream.bufferLength}`);
    return parts.filter(Boolean).join(' · ');
  }, [stream.bufferLength, stream.entries.length, stream.paused, targets.statusBase, visibleEntries.length]);

  return {
    ...targets,
    paused: stream.paused,
    searchQuery,
    setSearchQuery,
    filters,
    setFilters,
    visibleEntries,
    statusText,
    connect: stream.connect,
    togglePause: stream.togglePause,
    clear: stream.clear,
  };
}

export type LogsData = ReturnType<typeof useLogsData>;
export type { LogEntry, LogLevel, LogOption } from './model';
export { LOG_FILTER_LEVELS } from './model';
