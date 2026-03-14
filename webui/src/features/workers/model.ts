import { formatAgeSeconds, formatDateTime } from '../../lib/format';

export function normalizeWorkers(payload: any): any[] {
  return Array.isArray(payload?.workers) ? payload.workers : [];
}

export function buildWorkerSummaryPayload(workers: any[], queueSummary: any) {
  return {
    total: workers.length,
    active: workers.filter((item) => ['busy', 'provisioning', 'reclaiming', 'ready'].includes(String(item?.state || '').trim().toLowerCase())).length,
    busy: workers.filter((item) => String(item?.state || '').trim().toLowerCase() === 'busy').length,
    local: workers.filter((item) => String(item?.hostId || '').trim() === 'local').length,
    remote: workers.filter((item) => String(item?.hostId || '').trim() !== 'local').length,
    stale: workers.filter((item) => !!item?.stale).length,
    queueSummary: queueSummary || {},
  };
}

export function filterWorkers(workers: any[], searchValue: string, stateFilter: string): any[] {
  return workers.filter((worker: any) => {
    const haystack = [
      worker?.id,
      worker?.hostId,
      worker?.hostName,
      worker?.agentId,
      worker?.source,
      worker?.executionId,
    ].map((value) => String(value || '').trim().toLowerCase()).join(' ');
    if (searchValue.trim() && !haystack.includes(searchValue.trim().toLowerCase())) return false;
    const state = String(worker?.state || '').trim().toLowerCase();
    switch (stateFilter) {
      case 'active':
        return ['busy', 'provisioning', 'reclaiming', 'ready'].includes(state);
      case 'stale':
        return !!worker?.stale;
      case 'available':
      case 'managed':
      case 'stopped':
      case 'error':
      case 'reclaimed':
        return state === stateFilter;
      default:
        return true;
    }
  });
}

export function workerMetaLines(worker: any): string[] {
  const metaLines = [];
  if (worker?.executionId) metaLines.push(`execution: ${String(worker.executionId)}`);
  if (worker?.runtimeState) metaLines.push(`runtime: ${String(worker.runtimeState)}`);
  if (worker?.runtimeMode) metaLines.push(`runtime mode: ${String(worker.runtimeMode)}`);
  if (worker?.health) metaLines.push(`health: ${String(worker.health)}`);
  if (worker?.taskCount) metaLines.push(`tasks: ${String(worker.taskCount)}`);
  if (worker?.queuePosition) metaLines.push(`queue position: ${String(worker.queuePosition)}`);
  if (worker?.lastSyncStatus) metaLines.push(`sync: ${String(worker.lastSyncStatus)}`);
  if (worker?.driftState) metaLines.push(`drift: ${String(worker.driftState)}`);
  if (worker?.leaseState) metaLines.push(`lease state: ${String(worker.leaseState)}`);
  if (worker?.staleReason) metaLines.push(`stale reason: ${String(worker.staleReason)}`);
  if (worker?.leaseAgeSec) metaLines.push(`lease age: ${formatAgeSeconds(worker.leaseAgeSec)}`);
  if (worker?.heartbeatAgeSec) metaLines.push(`heartbeat age: ${formatAgeSeconds(worker.heartbeatAgeSec)}`);
  if (worker?.lastHeartbeatAt) metaLines.push(`last heartbeat: ${formatDateTime(worker.lastHeartbeatAt)}`);
  if (worker?.heartbeatAt) metaLines.push(`heartbeat: ${formatDateTime(worker.heartbeatAt)}`);
  if (worker?.updatedAt) metaLines.push(`updated: ${formatDateTime(worker.updatedAt)}`);
  return metaLines;
}
