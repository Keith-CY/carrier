import { apiGet } from '../../lib/api';
import {
  formatInstanceStatus,
  formatInstances,
  formatLogs,
  formatMemory,
  formatSessions,
  pickRemoteInstanceAgentID,
} from './model';
import { type HostManageOperationContext, type HostManageRuntime } from './hostManageShared';

export function createHostManageReadActions(ctx: HostManageOperationContext, runtime: HostManageRuntime) {
  async function loadManageInstances(silent = false) {
    const target = runtime.getTarget();
    if (!target.hostId) return;
    await runtime.runManageOperation(async () => {
      if (!silent) ctx.setManageMessage({ type: 'info', text: `Loading instances for host ${target.hostId}...` });
      try {
        const payload = await runtime.performManageRequest('load_instances', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances`),
        );
        const instances = Array.isArray(payload?.instances) ? payload.instances : [];
        ctx.setInstancesText(formatInstances(instances));
        if (instances.length) {
          ctx.setManageForm((current) => ({ ...current, agentId: pickRemoteInstanceAgentID(instances[0]) }));
        }
        if (!silent) ctx.setManageMessage({ type: 'success', text: `Loaded ${instances.length} instances.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Load instances failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageInstanceStatus(prefix = 'Instance Status') {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Loading instance status for ${target.agentId}...` });
      try {
        const payload = await runtime.performManageRequest('instance_status', () =>
          apiGet<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/status`,
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus(prefix, payload?.instance || {}, payload?.steps || []));
        ctx.setManageMessage({ type: 'success', text: `Loaded instance status for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Load instance status failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageLogs() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    const tail = Math.min(4000, Math.max(1, parseInt(ctx.manageForm.logTail || '200', 10) || 200));
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Loading logs for ${target.agentId} (tail=${tail})...` });
      try {
        const payload = await runtime.performManageRequest('instance_logs', () =>
          apiGet<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/logs?tail=${encodeURIComponent(String(tail))}`,
          ),
        );
        ctx.setLogsText(formatLogs(payload?.logs || ''));
        ctx.setManageMessage({ type: 'success', text: `Loaded logs for ${target.agentId} (tail=${tail}).` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Load logs failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageSyncStatus() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Loading sync status for ${target.agentId}...` });
      try {
        const payload = await runtime.performManageRequest('instance_sync_status', () =>
          apiGet<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/sync/status`,
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('Sync Status', payload?.status || {}, []));
        ctx.setManageMessage({ type: 'success', text: `Sync status loaded for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Load sync status failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageConfig() {
    const target = runtime.getTarget();
    if (!target.hostId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Loading config for host ${target.hostId}...` });
      try {
        const payload = await runtime.performManageRequest('load_config', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/config`),
        );
        ctx.setConfigText(JSON.stringify(payload?.config || {}, null, 2));
        ctx.setManageMessage({ type: 'success', text: `Config loaded for host ${target.hostId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Load config failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageSessions() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Loading sessions for ${target.agentId}...` });
      try {
        const payload = await runtime.performManageRequest('load_sessions', () =>
          apiGet<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/sessions?agentId=${encodeURIComponent(
              target.agentId,
            )}`,
          ),
        );
        const sessions = Array.isArray(payload?.sessions) ? payload.sessions : [];
        ctx.setSessionsText(formatSessions(sessions));
        ctx.setManageMessage({ type: 'success', text: `Loaded ${sessions.length} sessions for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Load sessions failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageMemory() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Loading memory for ${target.agentId}...` });
      try {
        const payload = await runtime.performManageRequest('load_memory', () =>
          apiGet<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/memory?agentId=${encodeURIComponent(
              target.agentId,
            )}`,
          ),
        );
        const memory = Array.isArray(payload?.memory) ? payload.memory : [];
        ctx.setMemoryText(formatMemory(memory));
        ctx.setManageMessage({ type: 'success', text: `Loaded ${memory.length} memory entries for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Load memory failed: ${(error as Error).message}` });
      }
    });
  }

  return {
    loadManageInstances,
    loadManageInstanceStatus,
    loadManageLogs,
    loadManageSyncStatus,
    loadManageConfig,
    loadManageSessions,
    loadManageMemory,
  };
}
