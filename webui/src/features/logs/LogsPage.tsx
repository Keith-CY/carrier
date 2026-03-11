import { useEffect, useMemo, useRef, useState } from 'react';
import { apiGet } from '../../lib/api';

type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | 'UNKNOWN';

type LogEntry = {
  id: number;
  timestamp: string;
  level: LogLevel;
  message: string;
};

type LogOption = {
  value: string;
  label: string;
};

const LOG_FILTER_LEVELS: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR'];
const LOG_ENTRY_LIMIT = 2000;

function normalizeLogLevel(level: unknown): LogLevel {
  const raw = String(level || '').trim().toUpperCase();
  if (!raw) return 'UNKNOWN';
  if (raw === 'WARNING') return 'WARN';
  if (raw === 'ERR') return 'ERROR';
  if (raw === 'TRACE') return 'DEBUG';
  return LOG_FILTER_LEVELS.includes(raw as LogLevel) ? (raw as LogLevel) : 'UNKNOWN';
}

function createLogEntry(id: number, level: unknown, message: unknown, timestamp?: unknown): LogEntry {
  return {
    id,
    timestamp: String(timestamp || '').trim() || new Date().toISOString(),
    level: normalizeLogLevel(level),
    message: String(message == null ? '' : message),
  };
}

function parseLogLine(line: unknown, nextId: number): LogEntry | null {
  const rawLine = String(line == null ? '' : line);
  const trimmed = rawLine.trim();
  if (!trimmed || /^returned \d+ log lines for /i.test(trimmed)) return null;

  let timestamp = '';
  let level: LogLevel = 'UNKNOWN';
  let message = trimmed;

  if (trimmed.startsWith('{') && trimmed.endsWith('}')) {
    try {
      const parsed = JSON.parse(trimmed);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        timestamp = String(parsed.time || parsed.timestamp || parsed.ts || '').trim();
        level = normalizeLogLevel(parsed.level || parsed.severity || parsed.lvl);
        const msg = parsed.message !== undefined ? parsed.message : (parsed.msg !== undefined ? parsed.msg : parsed.text);
        if (msg !== undefined) message = typeof msg === 'string' ? msg : JSON.stringify(msg);
      }
    } catch {}
  }

  if (level === 'UNKNOWN') {
    const bracketMatch = trimmed.match(/^\[([A-Za-z]+)\]\s*(.*)$/);
    if (bracketMatch) {
      level = normalizeLogLevel(bracketMatch[1]);
      message = (bracketMatch[2] || '').trim();
    }
  }

  if (!timestamp) {
    const timedMatch = trimmed.match(/^([0-9]{4}-[0-9]{2}-[0-9]{2}[T ][^\s]+)\s+([A-Za-z]+)\s*(.*)$/);
    if (timedMatch) {
      timestamp = timedMatch[1].trim();
      level = normalizeLogLevel(timedMatch[2]);
      message = (timedMatch[3] || '').trim();
    }
  }

  return createLogEntry(nextId, level, message || trimmed, timestamp);
}

function normalizeLineList(lines: unknown[]): string[] {
  return lines.map((line) => String(line == null ? '' : line)).filter((line) => line.trim().length > 0);
}

function diffAppendedLines(previous: string[], next: string[]): string[] {
  if (!next.length) return [];
  if (!previous.length) return next;

  const separator = Symbol('log-overlap-separator');
  const sequence = next.concat([separator as unknown as string], previous);
  const prefix = new Array(sequence.length).fill(0);

  for (let i = 1; i < sequence.length; i += 1) {
    let j = prefix[i - 1];
    while (j > 0 && sequence[i] !== sequence[j]) j = prefix[j - 1];
    if (sequence[i] === sequence[j]) j += 1;
    prefix[i] = j;
  }

  const overlap = Math.min(next.length, prefix[prefix.length - 1] || 0);
  return next.slice(overlap);
}

function buildLogOptions(agentsPayload: any, instancesPayload: any): { options: LogOption[]; emptyMessage: string } {
  const seen = new Set<string>();
  const options: LogOption[] = [];

  const appendOption = (value: unknown, label: unknown) => {
    const id = String(value || '').trim();
    if (!id || seen.has(id)) return;
    seen.add(id);
    options.push({ value: id, label: String(label || id) });
  };

  const agents = Array.isArray(agentsPayload) ? agentsPayload : [];
  agents.forEach((agent) => {
    const id = String(agent?.id || '').trim();
    if (!id) return;
    const runtimeState = String(agent?.runtimeState || agent?.runtime_state || '').trim();
    const installState = String(agent?.installState || agent?.install_state || '').trim();
    const suffix = runtimeState || installState;
    appendOption(id, suffix ? `${id} (${suffix})` : id);
  });

  const instances = Array.isArray(instancesPayload) ? instancesPayload : [];
  instances.forEach((instance) => {
    const runtimeAgentId = String(instance?.agent_id || instance?.agentID || instance?.type || '').trim();
    if (!runtimeAgentId) return;
    const instanceId = String(instance?.id || instance?.ID || '').trim();
    const runtimeState = String(instance?.runtime_state || instance?.runtimeState || '').trim();
    const labelParts = [];
    if (instanceId && instanceId !== runtimeAgentId) labelParts.push(instanceId);
    if (runtimeState) labelParts.push(runtimeState);
    const suffix = labelParts.length ? ` [${labelParts.join(', ')}]` : '';
    appendOption(runtimeAgentId, `${runtimeAgentId}${suffix}`);
  });

  if (!options.length) {
    return {
      options: [{ value: '', label: 'No agents available' }],
      emptyMessage: 'No local agents available. Start an agent first.',
    };
  }

  return {
    options,
    emptyMessage: 'Select an agent and click Connect.',
  };
}

function HighlightedText({ text, query }: { text: string; query: string }) {
  if (!query) return <>{text}</>;

  const lower = text.toLowerCase();
  const parts: Array<{ key: string; value: string; match: boolean }> = [];
  let cursor = 0;
  let matchIndex = 0;
  while (cursor < text.length) {
    const index = lower.indexOf(query, cursor);
    if (index === -1) {
      parts.push({ key: `tail-${cursor}`, value: text.slice(cursor), match: false });
      break;
    }
    if (index > cursor) {
      parts.push({ key: `plain-${cursor}`, value: text.slice(cursor, index), match: false });
    }
    parts.push({ key: `mark-${matchIndex++}`, value: text.slice(index, index + query.length), match: true });
    cursor = index + query.length;
  }

  return (
    <>
      {parts.map((part) => (part.match ? <mark key={part.key} className="log-highlight">{part.value}</mark> : <span key={part.key}>{part.value}</span>))}
    </>
  );
}

export function LogsPage() {
  const [options, setOptions] = useState<LogOption[]>([]);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [paused, setPaused] = useState(false);
  const [buffer, setBuffer] = useState<string[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [statusBase, setStatusBase] = useState('Select an agent and click Connect.');
  const [filters, setFilters] = useState<Record<LogLevel, boolean>>({
    DEBUG: true,
    INFO: true,
    WARN: true,
    ERROR: true,
    UNKNOWN: true,
  });

  const nextIdRef = useRef(1);
  const eventSourceRef = useRef<EventSource | null>(null);
  const pollActiveRef = useRef(false);
  const pollTimerRef = useRef<number | null>(null);
  const polledLinesRef = useRef<string[]>([]);
  const pausedRef = useRef(false);
  const bufferRef = useRef<string[]>([]);

  const visibleEntries = useMemo(() => {
    const normalizedQuery = searchQuery.trim().toLowerCase();
    return entries.filter((entry) => {
      if (Object.prototype.hasOwnProperty.call(filters, entry.level) && !filters[entry.level]) return false;
      if (!normalizedQuery) return true;
      return `${entry.timestamp} ${entry.level} ${entry.message}`.toLowerCase().includes(normalizedQuery);
    });
  }, [entries, filters, searchQuery]);

  const statusText = useMemo(() => {
    const parts = [statusBase];
    if (entries.length > 0) parts.push(`showing ${visibleEntries.length}/${entries.length}`);
    if (paused) parts.push('paused');
    if (buffer.length) parts.push(`buffered ${buffer.length}`);
    return parts.filter(Boolean).join(' · ');
  }, [buffer.length, entries.length, paused, statusBase, visibleEntries.length]);

  useEffect(() => {
    let cancelled = false;
    void Promise.allSettled([
      apiGet<any>('/api/v1/agents'),
      apiGet<any>('/api/v1/instances'),
    ]).then((results) => {
      if (cancelled) return;
      const agentsPayload = results[0].status === 'fulfilled' ? results[0].value : [];
      const instancesPayload = results[1].status === 'fulfilled' ? results[1].value : [];
      const normalized = buildLogOptions(agentsPayload, instancesPayload);
      setOptions(normalized.options);
      setStatusBase(normalized.emptyMessage);
      setSelectedAgent((current) => {
        if (current && normalized.options.some((option) => option.value === current)) return current;
        return normalized.options[0]?.value || '';
      });
    });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => () => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    pollActiveRef.current = false;
    if (pollTimerRef.current) {
      window.clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  useEffect(() => {
    bufferRef.current = buffer;
  }, [buffer]);

  const appendEntries = (newEntries: LogEntry[], stickToBottom = true) => {
    if (!newEntries.length) return;
    setEntries((current) => {
      const next = current.concat(newEntries);
      return next.length > LOG_ENTRY_LIMIT ? next.slice(next.length - LOG_ENTRY_LIMIT) : next;
    });
    if (stickToBottom) {
      window.requestAnimationFrame(() => {
        const output = document.querySelector<HTMLElement>('#log-output');
        if (output) output.scrollTop = output.scrollHeight;
      });
    }
  };

  const addSystemLog = (message: string, level: LogLevel = 'INFO') => {
    appendEntries([createLogEntry(nextIdRef.current++, level, message)]);
  };

  const ingestLogLines = (lines: string[], stickToBottom = true) => {
    if (!lines.length) return;
    if (pausedRef.current) {
      setBuffer((current) => current.concat(lines.filter((line) => String(line || '').trim().length > 0)));
      return;
    }
    const parsed = lines
      .map((line) => parseLogLine(line, nextIdRef.current++))
      .filter((entry): entry is LogEntry => !!entry);
    appendEntries(parsed, stickToBottom);
  };

  const startPolling = (agentId: string) => {
    pollActiveRef.current = true;
    setStatusBase(`Connected to ${agentId} via polling.`);

    const poll = async () => {
      if (!pollActiveRef.current) return;
      try {
        const response = await apiGet<{ lines?: unknown[] }>(`/api/v1/agents/${encodeURIComponent(agentId)}/logs`);
        const normalized = normalizeLineList(Array.isArray(response?.lines) ? response.lines : []);
        const appended = diffAppendedLines(polledLinesRef.current, normalized);
        polledLinesRef.current = normalized;
        ingestLogLines(appended, true);
      } catch (error) {
        addSystemLog(`poll error: ${(error as Error).message}`, 'ERROR');
      }
      if (!pollActiveRef.current) return;
      pollTimerRef.current = window.setTimeout(poll, 2000);
    };

    void poll();
  };

  const connect = () => {
    if (!selectedAgent) {
      setStatusBase('Select an agent and click Connect.');
      return;
    }

    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
    pollActiveRef.current = false;
    if (pollTimerRef.current) {
      window.clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }

    setEntries([]);
    setBuffer([]);
    setPaused(false);
    pausedRef.current = false;
    bufferRef.current = [];
    nextIdRef.current = 1;
    polledLinesRef.current = [];
    setStatusBase(`Connecting to ${selectedAgent}...`);

    let sseUrl = `/api/v1/logs/stream?agent=${encodeURIComponent(selectedAgent)}`;
    const token = localStorage.getItem('carrier_token');
    if (token) sseUrl += `&token=${encodeURIComponent(token)}`;

    try {
      const source = new EventSource(sseUrl);
      eventSourceRef.current = source;
      source.onopen = () => {
        setStatusBase(`Connected to ${selectedAgent} via SSE.`);
      };
      source.onmessage = (event) => {
        ingestLogLines([event.data], true);
      };
      source.onerror = () => {
        if (eventSourceRef.current !== source) return;
        source.close();
        eventSourceRef.current = null;
        addSystemLog('SSE disconnected, falling back to polling.', 'WARN');
        startPolling(selectedAgent);
      };
    } catch {
      addSystemLog('SSE unavailable, using polling.', 'WARN');
      startPolling(selectedAgent);
    }
  };

  const togglePause = () => {
    if (pausedRef.current && bufferRef.current.length) {
      const flushed = bufferRef.current.slice();
      setBuffer([]);
      bufferRef.current = [];
      setPaused(false);
      pausedRef.current = false;
      window.setTimeout(() => ingestLogLines(flushed, true), 0);
      return;
    }
    const next = !pausedRef.current;
    setPaused(next);
    pausedRef.current = next;
  };

  const clear = () => {
    setEntries([]);
    setBuffer([]);
    bufferRef.current = [];
    nextIdRef.current = 1;
    polledLinesRef.current = [];
  };

  return (
    <section id="view-logs" className="view view-logs-surface">
      <div className="section-head">
        <h2>Logs</h2>
      </div>
      <div className="card logs-panel">
        <div className="log-controls">
          <label htmlFor="log-agent">Agent:</label>
          <select id="log-agent" value={selectedAgent} onChange={(event) => setSelectedAgent(event.target.value)}>
            {options.map((option) => (
              <option key={option.value || option.label} value={option.value}>{option.label}</option>
            ))}
          </select>
          <button id="log-connect" className="btn-sm" type="button" onClick={connect}>Connect</button>
          <button id="log-pause" className="btn-sm btn-secondary" type="button" onClick={togglePause}>{paused ? 'Resume' : 'Pause'}</button>
          <button id="log-clear" className="btn-sm btn-secondary" type="button" onClick={clear}>Clear</button>
          <input
            id="log-search"
            type="text"
            placeholder="Search logs..."
            autoComplete="off"
            aria-label="Search logs"
            value={searchQuery}
            onChange={(event) => setSearchQuery(event.target.value)}
          />
        </div>
        <div className="log-filters" role="group" aria-label="Filter log levels">
          {LOG_FILTER_LEVELS.map((level) => (
            <label key={level} className="log-filter-pill">
              <input
                id={`log-filter-${level.toLowerCase()}`}
                type="checkbox"
                checked={filters[level]}
                onChange={(event) => setFilters((current) => ({ ...current, [level]: event.target.checked }))}
              />
              {level}
            </label>
          ))}
        </div>
        <p id="log-status" className="text-dim log-status">{statusText}</p>
        <div className="log-table">
          <div className="log-row log-row-header" aria-hidden="true">
            <span className="log-cell-time">Timestamp</span>
            <span className="log-cell-level">Level</span>
            <span className="log-cell-message">Message</span>
          </div>
          <div id="log-output" className="log-rows" role="log" aria-live="polite">
            {visibleEntries.map((entry) => (
              <div key={entry.id} className="log-row log-row-data" data-level={entry.level}>
                <span className="log-cell-time">
                  <HighlightedText text={entry.timestamp} query={searchQuery.trim().toLowerCase()} />
                </span>
                <span className="log-cell-level">
                  <span className="log-level-pill">
                    <HighlightedText text={entry.level} query={searchQuery.trim().toLowerCase()} />
                  </span>
                </span>
                <span className="log-cell-message">
                  <HighlightedText text={entry.message} query={searchQuery.trim().toLowerCase()} />
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
