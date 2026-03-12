import { apiGet, apiPatch, apiPost } from '../../lib/api';
import { formatInstanceStatus } from './model';
import { type HostManageOperationContext, type HostManageRuntime } from './hostManageShared';

export function createHostManageMutationActions(ctx: HostManageOperationContext, runtime: HostManageRuntime) {
  async function installManageInstance() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Live install in progress for ${target.agentId}...` });
      try {
        const payload = await runtime.performManageRequest('instance_install', () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/install/stream`,
            {},
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('Install', payload?.install || {}, payload?.steps || []));
        ctx.setManageMessage({ type: 'success', text: `Install completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Install failed: ${(error as Error).message}` });
      }
    });
  }

  async function repairManageInstance() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Repair in progress for ${target.agentId}...` });
      try {
        const payload = await runtime.performManageRequest('instance_repair', () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/repair`,
            {},
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('Repair', payload?.repair || {}, payload?.steps || []));
        ctx.setManageMessage({ type: 'success', text: `Repair completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Repair failed: ${(error as Error).message}` });
      }
    });
  }

  async function syncManageInstance() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Sync in progress for ${target.agentId} (mode=${ctx.manageForm.syncMode})...` });
      try {
        const payload = await runtime.performManageRequest('instance_sync', () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/sync`,
            { mode: ctx.manageForm.syncMode },
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('Sync', payload?.sync || {}, payload?.steps || []));
        ctx.setManageMessage({ type: 'success', text: `Sync completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Sync failed: ${(error as Error).message}` });
      }
    });
  }

  async function diagnoseManageInstance() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Diagnosing drift for ${target.agentId}...` });
      try {
        const payload = await runtime.performManageRequest('instance_diagnose', () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/diagnose`,
            {},
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('Diagnose Drift', payload?.diagnose || {}, payload?.steps || []));
        ctx.setManageMessage({ type: 'success', text: `Diagnose completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Diagnose failed: ${(error as Error).message}` });
      }
    });
  }

  async function reconcileManageInstance() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Reconciling ${target.agentId}...` });
      try {
        const payload = await runtime.performManageRequest('instance_reconcile', () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/reconcile`,
            {},
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('Reconcile', payload?.reconcile || {}, payload?.steps || []));
        ctx.setManageMessage({ type: 'success', text: `Reconcile completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Reconcile failed: ${(error as Error).message}` });
      }
    });
  }

  async function rollbackManageInstance() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Rollback in progress for ${target.agentId}...` });
      try {
        const body = ctx.manageForm.rollbackCommit.trim() ? { commit: ctx.manageForm.rollbackCommit.trim() } : {};
        const payload = await runtime.performManageRequest('instance_rollback', () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/rollback`,
            body,
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('Rollback', payload?.rollback || {}, payload?.steps || []));
        ctx.setManageMessage({ type: 'success', text: `Rollback completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Rollback failed: ${(error as Error).message}` });
      }
    });
  }

  async function applyManageConfig() {
    const target = runtime.getTarget();
    if (!target.hostId) return;
    if (!ctx.configText.trim()) {
      ctx.setManageMessage({ type: 'error', text: 'Config patch cannot be empty.' });
      return;
    }
    let payload: Record<string, any>;
    try {
      payload = JSON.parse(ctx.configText);
    } catch (error) {
      ctx.setManageMessage({ type: 'error', text: `Config patch must be valid JSON: ${(error as Error).message}` });
      return;
    }
    if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
      ctx.setManageMessage({ type: 'error', text: 'Config patch must be a JSON object.' });
      return;
    }
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Applying config patch for host ${target.hostId}...` });
      try {
        const response = await runtime.performManageRequest('apply_config', () =>
          apiPatch<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/config`, payload),
        );
        ctx.setConfigText(JSON.stringify(response?.config || payload, null, 2));
        ctx.setManageMessage({ type: 'success', text: `Config patch applied for host ${target.hostId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Apply config patch failed: ${(error as Error).message}` });
      }
    });
  }

  async function applySessionAction(action: 'archive' | 'delete') {
    const target = runtime.getTarget();
    const sessionId = ctx.manageForm.sessionId.trim();
    if (!target.hostId || !target.agentId) return;
    if (!sessionId) {
      ctx.setManageMessage({ type: 'error', text: 'session id is required.' });
      return;
    }
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Session ${action} in progress for ${sessionId}...` });
      try {
        await runtime.performManageRequest(`${action}_session`, () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/sessions/${encodeURIComponent(
              sessionId,
            )}/${action}?agentId=${encodeURIComponent(target.agentId)}`,
            {},
          ),
        );
        ctx.setManageMessage({ type: 'success', text: `Session ${sessionId} ${action}d.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `Session ${action} failed: ${(error as Error).message}` });
      }
    });
  }

  return {
    installManageInstance,
    repairManageInstance,
    syncManageInstance,
    diagnoseManageInstance,
    reconcileManageInstance,
    rollbackManageInstance,
    applyManageConfig,
    applySessionAction,
  };
}
