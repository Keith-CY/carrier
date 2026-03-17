import { apiPatch, apiPost } from '../../lib/api';
import { formatInstanceStatus } from './model';
import { type HostManageOperationContext, type HostManageRuntime } from './hostManageShared';

type installStreamResult = {
  install?: Record<string, any>;
  finishReason?: string;
};

function parseSSEFrames(buffer: string, onEvent: (payload: Record<string, any>) => void) {
  let remaining = buffer.replaceAll('\r', '');
  for (;;) {
    const idx = remaining.indexOf('\n\n');
    if (idx < 0) break;
    const frame = remaining.slice(0, idx);
    remaining = remaining.slice(idx + 2);
    const dataLines = frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).trim());
    if (!dataLines.length) continue;
    try {
      onEvent(JSON.parse(dataLines.join('\n')) as Record<string, any>);
    } catch {
      // Ignore malformed frames and keep reading the stream.
    }
  }
  return remaining;
}

async function streamInstallRequest(
  path: string,
  onEvent: (payload: Record<string, any>) => void,
): Promise<installStreamResult> {
  const headers: Record<string, string> = {
    Accept: 'text/event-stream',
    'Content-Type': 'application/json',
  };
  if (typeof window !== 'undefined') {
    const token = String(window.localStorage.getItem('carrier_token') || '').trim();
    if (token) headers.Authorization = `Bearer ${token}`;
  }
  const response = await fetch(path, {
    method: 'POST',
    headers,
    body: JSON.stringify({}),
  });
  const errorText = await (async () => {
    if (response.ok || !response.body || response.body.getReader) return '';
    return response.text();
  })();
  if (!response.ok) {
    throw new Error(errorText || `install request failed (${response.status})`);
  }
  if (!response.body || !response.body.getReader) {
    throw new Error('streaming install is not supported in this browser');
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let latestInstall: Record<string, any> | undefined;
  let finishReason = '';
  let latestError = '';
  const handleEvent = (payload: Record<string, any>) => {
    onEvent(payload);
    switch (String(payload?.type || '').trim()) {
      case 'result':
        latestInstall = (payload.install as Record<string, any> | undefined) || undefined;
        break;
      case 'error':
        latestError = String(payload?.message || payload?.error || 'install stream failed').trim();
        break;
      case 'finish':
        finishReason = String(payload?.finishReason || '').trim();
        break;
      default:
        break;
    }
  };

  for (;;) {
    const step = await reader.read();
    if (step.done) break;
    buffer += decoder.decode(step.value, { stream: true });
    buffer = parseSSEFrames(buffer, handleEvent);
  }
  parseSSEFrames(buffer, handleEvent);

  if (latestError) {
    throw new Error(latestError);
  }
  if (!latestInstall) {
    if (finishReason) {
      return { install: {}, finishReason };
    }
    throw new Error('install stream finished without a result payload');
  }
  return { install: latestInstall, finishReason };
}

export function createHostManageMutationActions(ctx: HostManageOperationContext, runtime: HostManageRuntime) {
  async function installManageInstance() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({ type: 'info', text: `Live install in progress for ${target.agentId}...` });
      ctx.setLogsText('');
      ctx.setStreamStatusText(`Connecting to remote installer for ${target.agentId}...`);
      try {
        const logLines: string[] = [];
        const payload = await runtime.performManageRequest('instance_install', () =>
          streamInstallRequest(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/install/stream`,
            (event) => {
              const eventType = String(event?.type || '').trim();
              switch (eventType) {
                case 'start':
                  ctx.setStreamStatusText(`Remote installer started for ${target.agentId}.`);
                  break;
                case 'log': {
                  const line = String(event?.line || '').trim();
                  if (!line) break;
                  logLines.push(line);
                  ctx.setLogsText(logLines.join('\n'));
                  ctx.setStreamStatusText(`Streaming install logs for ${target.agentId}...`);
                  break;
                }
                case 'result':
                  ctx.setStreamStatusText(`Install stream returned a result for ${target.agentId}.`);
                  break;
                case 'finish':
                  ctx.setStreamStatusText(
                    `Install stream finished for ${target.agentId}${event?.finishReason ? ` (${String(event.finishReason)})` : ''}.`,
                  );
                  break;
                default:
                  break;
              }
            },
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('Install', payload?.install || {}, []));
        ctx.setManageMessage({ type: 'success', text: `Install completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setStreamStatusText(`Install stream failed for ${target.agentId}.`);
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
