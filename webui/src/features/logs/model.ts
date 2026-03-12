export type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | 'UNKNOWN';

export type LogEntry = {
  id: number;
  timestamp: string;
  level: LogLevel;
  message: string;
};

export type LogOption = {
  value: string;
  label: string;
};

export const LOG_FILTER_LEVELS: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR'];
export const LOG_ENTRY_LIMIT = 2000;

export function normalizeLogLevel(level: unknown): LogLevel {
  const raw = String(level || '').trim().toUpperCase();
  if (!raw) return 'UNKNOWN';
  if (raw === 'WARNING') return 'WARN';
  if (raw === 'ERR') return 'ERROR';
  if (raw === 'TRACE') return 'DEBUG';
  return LOG_FILTER_LEVELS.includes(raw as LogLevel) ? (raw as LogLevel) : 'UNKNOWN';
}

export function createLogEntry(id: number, level: unknown, message: unknown, timestamp?: unknown): LogEntry {
  return {
    id,
    timestamp: String(timestamp || '').trim() || new Date().toISOString(),
    level: normalizeLogLevel(level),
    message: String(message == null ? '' : message),
  };
}

export function parseLogLine(line: unknown, nextId: number): LogEntry | null {
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
        const msg =
          parsed.message !== undefined ? parsed.message : parsed.msg !== undefined ? parsed.msg : parsed.text;
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

export function normalizeLineList(lines: unknown[]): string[] {
  return lines.map((line) => String(line == null ? '' : line)).filter((line) => line.trim().length > 0);
}

export function diffAppendedLines(previous: string[], next: string[]): string[] {
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

export function buildLogOptions(
  agentsPayload: any,
  instancesPayload: any,
): { options: LogOption[]; emptyMessage: string } {
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

  const instances = Array.isArray(instancesPayload)
    ? instancesPayload
    : Array.isArray(instancesPayload?.instances)
      ? instancesPayload.instances
      : [];
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
