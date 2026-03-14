import { useEffect, useRef, useState } from 'react';
import { apiGet } from '../../lib/api';
import {
  createLogEntry,
  diffAppendedLines,
  LOG_ENTRY_LIMIT,
  normalizeLineList,
  parseLogLine,
  type LogEntry,
} from './model';

export function useLogStream(
  selectedAgent: string,
  fallbackAgentId: string,
  setStatusBase: (next: string) => void,
) {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [paused, setPaused] = useState(false);
  const [buffer, setBuffer] = useState<string[]>([]);

  const nextIdRef = useRef(1);
  const eventSourceRef = useRef<EventSource | null>(null);
  const pollActiveRef = useRef(false);
  const pollTimerRef = useRef<number | null>(null);
  const polledLinesRef = useRef<string[]>([]);
  const pausedRef = useRef(false);
  const bufferRef = useRef<string[]>([]);

  useEffect(
    () => () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
      pollActiveRef.current = false;
      if (pollTimerRef.current) {
        window.clearTimeout(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    },
    [],
  );

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

  const addSystemLog = (message: string, level: 'INFO' | 'WARN' | 'ERROR' = 'INFO') => {
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
        appendEntries([createLogEntry(nextIdRef.current++, 'ERROR', `poll error: ${(error as Error).message}`)]);
      }
      if (!pollActiveRef.current) return;
      pollTimerRef.current = window.setTimeout(poll, 2000);
    };

    void poll();
  };

  const connect = () => {
    const agentId = selectedAgent || fallbackAgentId;
    if (!agentId) {
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
    setStatusBase(`Connecting to ${agentId}...`);

    let sseUrl = `/api/v1/logs/stream?agent=${encodeURIComponent(agentId)}`;
    const token = localStorage.getItem('carrier_token');
    if (token) sseUrl += `&token=${encodeURIComponent(token)}`;

    try {
      const source = new EventSource(sseUrl);
      eventSourceRef.current = source;
      source.onopen = () => {
        setStatusBase(`Connected to ${agentId} via SSE.`);
      };
      source.onmessage = (event) => {
        ingestLogLines([event.data], true);
      };
      source.onerror = () => {
        if (eventSourceRef.current !== source) return;
        source.close();
        eventSourceRef.current = null;
        addSystemLog('SSE disconnected, falling back to polling.', 'WARN');
        startPolling(agentId);
      };
    } catch {
      addSystemLog('SSE unavailable, using polling.', 'WARN');
      startPolling(agentId);
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

  return {
    entries,
    paused,
    bufferLength: buffer.length,
    connect,
    togglePause,
    clear,
  };
}
