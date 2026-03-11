export type HostRecord = Record<string, any>;

export type OperationSummary = {
  operation: string;
  success: boolean;
  requestId?: string;
  durationMs?: number;
  error?: string;
  at: string;
};

export type MessageState = {
  type: 'info' | 'success' | 'error';
  text: string;
};

export type ManageFormState = {
  agentId: string;
  sessionId: string;
  logTail: string;
  syncMode: string;
  rollbackCommit: string;
  codeagentBackend: string;
  codeagentWorkspaceRoot: string;
  codeagentCapability: string;
  codeagentWriteMode: string;
  codeagentCommand: string;
  codeagentPath: string;
  codeagentContent: string;
};

export type HostEditorState = {
  name: string;
  authMode: string;
  host: string;
  port: string;
  user: string;
  runtimeMode: string;
  labels: string;
  keyPath: string;
  sshConfigHost: string;
};

export const DEFAULT_EDITOR: HostEditorState = {
  name: '',
  authMode: 'private_key',
  host: '',
  port: '22',
  user: '',
  runtimeMode: 'on_demand',
  labels: '',
  keyPath: '',
  sshConfigHost: '',
};

export const DEFAULT_MANAGE_FORM: ManageFormState = {
  agentId: 'main',
  sessionId: '',
  logTail: '200',
  syncMode: 'pull_validate_push',
  rollbackCommit: '',
  codeagentBackend: 'codex',
  codeagentWorkspaceRoot: '/workspace',
  codeagentCapability: 'run_shell',
  codeagentWriteMode: 'overwrite',
  codeagentCommand: '',
  codeagentPath: '',
  codeagentContent: '',
};

export function normalizeHosts(payload: any): HostRecord[] {
  return Array.isArray(payload?.hosts) ? payload.hosts : [];
}

export function normalizeSSHAliases(payload: any): string[] {
  const hosts = Array.isArray(payload?.hosts) ? payload.hosts : [];
  return hosts
    .map((value) => String(value || '').trim())
    .filter(Boolean)
    .filter((value, index, list) => list.indexOf(value) === index)
    .sort((left, right) => left.localeCompare(right));
}

export function parseCSV(raw: string): string[] {
  return String(raw || '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean)
    .filter((value, index, list) => list.indexOf(value) === index)
    .sort((left, right) => left.localeCompare(right));
}

export function hostEndpoint(host: HostRecord): string {
  if (String(host?.authMode || '').trim().toLowerCase() === 'ssh_config') {
    return String(host?.sshConfigHost || host?.host || '-');
  }
  const user = String(host?.user || 'user');
  const hostName = String(host?.host || '-');
  return `${user}@${hostName}`;
}

export function formatHostOperationMetaLines(operation?: OperationSummary): string[] {
  if (!operation) return [];
  const lines = [`last op: ${operation.operation} (${operation.success ? 'ok' : 'error'})`];
  if (operation.requestId) lines.push(`requestId: ${operation.requestId}`);
  if (typeof operation.durationMs === 'number') lines.push(`duration: ${Math.round(operation.durationMs)}ms`);
  if (!operation.success && operation.error) lines.push(`last error: ${operation.error}`);
  lines.push(`updated at: ${operation.at}`);
  return lines;
}

export function formatHostMeta(host: HostRecord, operation?: OperationSummary): string {
  const labels = Array.isArray(host?.labels) && host.labels.length ? host.labels.join(', ') : '-';
  return [
    `id: ${String(host?.id || '-')}`,
    `endpoint: ${hostEndpoint(host)}`,
    `auth: ${String(host?.authMode || '-')}`,
    `runtime: ${String(host?.runtimeMode || '-')}`,
    `labels: ${labels}`,
    `health: ${String(host?.lastHealth || 'unknown')}`,
    ...formatHostOperationMetaLines(operation),
  ].join('\n');
}

export function pickRemoteInstanceAgentID(instance: any): string {
  if (!instance || typeof instance !== 'object') return 'main';
  const explicit = String(instance.agentId || instance.agentID || '').trim();
  if (explicit) return explicit;
  const rawID = String(instance.id || instance.ID || '').trim();
  if (rawID.includes(':')) return String(rawID.split(':').pop() || 'main').trim() || 'main';
  return rawID || 'main';
}

export function formatInstances(entries: any[]): string {
  if (!entries.length) return 'No instances found.';
  return entries
    .map((item) => {
      const agentId = pickRemoteInstanceAgentID(item);
      const runtime = String(item?.runtimeState || item?.runtime_state || item?.status || 'unknown');
      const health = String(item?.health || 'unknown');
      return `${agentId} (runtime=${runtime}, health=${health})`;
    })
    .join('\n');
}

export function formatInstanceStatus(prefix: string, instance: any, steps: any[]): string {
  const current = instance && typeof instance === 'object' ? instance : {};
  const lines = prefix ? [prefix] : [];
  const pairs: Array<[string, unknown]> = [
    ['id', current.id || current.ID || ''],
    ['runtime', current.runtimeState || current.runtime_state || current.status || ''],
    ['health', current.health || ''],
    ['install status', typeof current.installed === 'boolean' ? (current.installed ? 'installed' : 'not installed') : ''],
    ['repair status', typeof current.repaired === 'boolean' ? (current.repaired ? 'repaired' : 'not repaired') : ''],
    ['gateway', typeof current.gatewayHealthy === 'boolean' ? (current.gatewayHealthy ? 'healthy' : 'unhealthy') : ''],
    ['sync mode', current.syncMode || current.sync_mode || ''],
    ['drift state', current.driftState || current.drift_state || ''],
    ['sync status', current.lastSyncStatus || current.last_sync_status || ''],
    ['diagnose result', current.lastDiagnoseResult || current.last_diagnose_result || current.result || ''],
    ['from commit', current.fromCommit || current.from_commit || ''],
    ['new commit', current.newCommit || current.new_commit || ''],
    ['backend', current.backend || ''],
    ['policy decision', current.policy_decision || current.policyDecision || ''],
    ['policy reason', current.policy_reason || current.policyReason || ''],
    ['reconciled', typeof current.reconciled === 'boolean' ? String(current.reconciled) : ''],
    ['rolled back', typeof (current.rolledBack ?? current.rolled_back) === 'boolean' ? String(current.rolledBack ?? current.rolled_back) : ''],
    ['healthy', typeof current.healthy === 'boolean' ? String(current.healthy) : ''],
    ['configured', typeof current.configured === 'boolean' ? String(current.configured) : ''],
    ['ok', typeof current.ok === 'boolean' ? String(current.ok) : ''],
    ['exit code', current.exit_code ?? current.exitCode ?? ''],
    ['duration ms', current.duration_ms ?? current.durationMs ?? ''],
  ];
  pairs.forEach(([label, value]) => {
    if (String(value ?? '').trim() === '') return;
    lines.push(`${label}: ${String(value)}`);
  });
  if (Array.isArray(steps) && steps.length) {
    lines.push('', 'Execution Steps');
    steps.forEach((step, index) => {
      const command = String(step?.command || step?.Command || '(no command)');
      const exitCode = step?.exitCode ?? step?.ExitCode;
      const duration = step?.durationMs ?? step?.DurationMs;
      lines.push(`#${index + 1} ${command}`);
      lines.push(`exit=${String(exitCode ?? '-')}`);
      lines.push(`duration=${typeof duration === 'number' ? Math.round(duration) : duration || '-'}${typeof duration === 'number' ? 'ms' : ''}`);
      const stdout = String(step?.stdout || step?.Stdout || '').trim();
      const stderr = String(step?.stderr || step?.Stderr || '').trim();
      if (stdout) lines.push(`[stdout]\n${stdout}`);
      if (stderr) lines.push(`[stderr]\n${stderr}`);
    });
  }
  return lines.join('\n');
}

export function formatLogs(logs: unknown): string {
  const text = String(logs || '').trim();
  return text || 'No logs available.';
}

export function formatSessions(entries: any[]): string {
  if (!entries.length) return 'No sessions found.';
  return entries
    .map((item) => {
      const sessionId = String(item?.sessionId || item?.sessionID || '-');
      const kind = String(item?.kind || '-');
      const size = String(item?.sizeBytes ?? 0);
      const modifiedAt = String(item?.modifiedAt ?? 0);
      const path = String(item?.path || '-');
      return `${sessionId} [${kind}] size=${size} modified=${modifiedAt}\n${path}`;
    })
    .join('\n\n');
}

export function formatMemory(entries: any[]): string {
  if (!entries.length) return 'No memory files found.';
  return entries
    .map((item) => `${String(item?.path || '-')} (size=${String(item?.sizeBytes ?? 0)}, modified=${String(item?.modifiedAt ?? 0)})`)
    .join('\n');
}

export function formatOperationMeta(operation: OperationSummary | null): string {
  if (!operation) return '';
  const parts = [
    `operation=${operation.operation}`,
    `status=${operation.success ? 'ok' : 'error'}`,
  ];
  if (operation.requestId) parts.push(`requestId=${operation.requestId}`);
  if (typeof operation.durationMs === 'number') parts.push(`duration=${Math.round(operation.durationMs)}ms`);
  if (operation.error) parts.push(`error=${operation.error}`);
  parts.push(`at=${operation.at}`);
  return parts.join(' · ');
}

export function nextOperation(operation: string, success: boolean, durationMs: number, requestId?: string, error?: string): OperationSummary {
  return {
    operation,
    success,
    requestId: requestId || '',
    durationMs,
    error: error || '',
    at: new Date().toISOString(),
  };
}

export function updateEditorFromHost(host: HostRecord): HostEditorState {
  return {
    name: String(host?.name || host?.id || ''),
    authMode: String(host?.authMode || 'private_key'),
    host: String(host?.host || ''),
    port: String(host?.port || 22),
    user: String(host?.user || ''),
    runtimeMode: String(host?.runtimeMode || 'on_demand'),
    labels: Array.isArray(host?.labels) ? host.labels.join(', ') : '',
    keyPath: String(host?.keyPath || ''),
    sshConfigHost: String(host?.sshConfigHost || ''),
  };
}

export function manageTarget(hostId: string, form: ManageFormState) {
  return {
    hostId: String(hostId || '').trim(),
    agentId: String(form.agentId || '').trim() || 'main',
  };
}
