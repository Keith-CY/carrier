import { useEffect, useMemo, useState } from 'react';
import { useFeatures } from '../../app/useFeatures';
import { apiDelete, apiGet, apiPatch, apiPost } from '../../lib/api';
import {
  DEFAULT_EDITOR,
  DEFAULT_MANAGE_FORM,
  type HostEditorState,
  type HostRecord,
  formatInstanceStatus,
  formatInstances,
  formatLogs,
  formatMemory,
  formatSessions,
  manageTarget,
  nextOperation,
  normalizeHosts,
  normalizeSSHAliases,
  parseCSV,
  pickRemoteInstanceAgentID,
  type ManageFormState,
  type MessageState,
  type OperationSummary,
} from './model';

export function useHostsData() {
  const { featureFlags, authz, isLoading: featuresLoading } = useFeatures();
  const canManageHosts = !!authz.permissions.manageHosts;
  const [hosts, setHosts] = useState<HostRecord[]>([]);
  const [sshAliases, setSshAliases] = useState<string[]>([]);
  const [serversMessage, setServersMessage] = useState<MessageState>({ type: 'info', text: '' });
  const [manageMessage, setManageMessage] = useState<MessageState>({ type: 'info', text: '' });
  const [editor, setEditor] = useState<HostEditorState>(DEFAULT_EDITOR);
  const [editingHostId, setEditingHostId] = useState('');
  const [selectedHostId, setSelectedHostId] = useState('');
  const [manageForm, setManageForm] = useState<ManageFormState>(DEFAULT_MANAGE_FORM);
  const [manageBusy, setManageBusy] = useState(false);
  const [editorBusy, setEditorBusy] = useState(false);
  const [hostOps, setHostOps] = useState<Record<string, OperationSummary>>({});
  const [opMeta, setOpMeta] = useState<OperationSummary | null>(null);
  const [instancesText, setInstancesText] = useState('');
  const [instanceStatusText, setInstanceStatusText] = useState('');
  const [logsText, setLogsText] = useState('');
  const [configText, setConfigText] = useState('');
  const [sessionsText, setSessionsText] = useState('');
  const [memoryText, setMemoryText] = useState('');

  const selectedHost = useMemo(
    () => hosts.find((host) => String(host?.id || '').trim() === selectedHostId) || null,
    [hosts, selectedHostId],
  );

  useEffect(() => {
    if (featuresLoading || !featureFlags.remoteControlPlaneEnabled) return;
    void refresh();
  }, [featureFlags.remoteControlPlaneEnabled, featuresLoading]);

  useEffect(() => {
    if (!selectedHostId) return;
    if (hosts.some((host) => String(host?.id || '').trim() === selectedHostId)) return;
    setSelectedHostId('');
    setOpMeta(null);
    setManageMessage({ type: 'info', text: '' });
  }, [hosts, selectedHostId]);

  useEffect(() => {
    if (canManageHosts) return;
    setServersMessage({ type: 'info', text: 'Current role cannot modify remote hosts.' });
  }, [canManageHosts]);

  async function refresh() {
    try {
      const [hostsPayload, aliasesPayload] = await Promise.all([
        apiGet<any>('/api/v1/remote/hosts'),
        apiGet<any>('/api/v1/remote/ssh-config-hosts').catch(() => ({ hosts: [] })),
      ]);
      setHosts(normalizeHosts(hostsPayload));
      setSshAliases(normalizeSSHAliases(aliasesPayload));
    } catch (error) {
      setServersMessage({ type: 'error', text: `Load failed: ${(error as Error).message}` });
      setHosts([]);
      setSshAliases([]);
    }
  }

  async function recordHostOperation(hostId: string, operation: string, work: () => Promise<any>) {
    const startedAt = performance.now();
    try {
      const payload = await work();
      const summary = nextOperation(
        operation,
        true,
        performance.now() - startedAt,
        String(payload?.requestId || payload?.requestID || '').trim(),
      );
      setHostOps((current) => ({ ...current, [hostId]: summary }));
      return { payload, summary };
    } catch (error) {
      const summary = nextOperation(operation, false, performance.now() - startedAt, '', (error as Error).message);
      setHostOps((current) => ({ ...current, [hostId]: summary }));
      throw error;
    }
  }

  async function runManageOperation(work: () => Promise<any>) {
    if (manageBusy) {
      setManageMessage({ type: 'info', text: 'Another operation is already running.' });
      return null;
    }
    setManageBusy(true);
    try {
      return await work();
    } finally {
      setManageBusy(false);
    }
  }

  function resetEditor(clearForm: boolean) {
    setEditingHostId('');
    if (clearForm) setEditor(DEFAULT_EDITOR);
  }

  function showManageHost(hostId: string) {
    setSelectedHostId(hostId);
    setInstancesText('');
    setInstanceStatusText('');
    setLogsText('');
    setConfigText('');
    setSessionsText('');
    setMemoryText('');
    setOpMeta(null);
    setManageMessage({ type: 'info', text: '' });
  }

  async function handleSaveHost() {
    if (!canManageHosts) {
      setServersMessage({ type: 'error', text: 'Current role cannot modify remote hosts.' });
      return;
    }
    const payload = {
      name: editor.name.trim(),
      host: editor.host.trim(),
      port: parseInt(editor.port.trim() || '22', 10) || 22,
      user: editor.user.trim(),
      authMode: editor.authMode.trim(),
      keyPath: editor.keyPath.trim(),
      sshConfigHost: editor.sshConfigHost.trim(),
      runtimeMode: editor.runtimeMode.trim() || 'on_demand',
      labels: parseCSV(editor.labels),
    };
    setEditorBusy(true);
    try {
      if (editingHostId) {
        await apiPatch(`/api/v1/remote/hosts/${encodeURIComponent(editingHostId)}`, payload);
        setServersMessage({ type: 'success', text: `Remote host updated: ${editingHostId}` });
      } else {
        await apiPost('/api/v1/remote/hosts', payload);
        setServersMessage({ type: 'success', text: 'Remote host saved.' });
      }
      resetEditor(true);
      await refresh();
    } catch (error) {
      setServersMessage({ type: 'error', text: `Save failed: ${(error as Error).message}` });
    } finally {
      setEditorBusy(false);
    }
  }

  async function handleDeleteHost(hostId: string) {
    if (!window.confirm(`Delete remote host ${hostId}?`)) return;
    try {
      await apiDelete(`/api/v1/remote/hosts/${encodeURIComponent(hostId)}`);
      setServersMessage({ type: 'success', text: `Deleted remote host: ${hostId}` });
      setHostOps((current) => {
        const next = { ...current };
        delete next[hostId];
        return next;
      });
      if (editingHostId === hostId) resetEditor(true);
      await refresh();
    } catch (error) {
      setServersMessage({ type: 'error', text: `Delete failed: ${(error as Error).message}` });
    }
  }

  async function handleCheckHost(hostId: string) {
    const host = hosts.find((item) => String(item?.id || '').trim() === hostId) || { id: hostId };
    await runManageOperation(async () => {
      setServersMessage({ type: 'info', text: `Running health check: ${String(host?.name || host?.id || hostId)}...` });
      try {
        const { payload } = await recordHostOperation(hostId, 'host_check', () =>
          apiPost(`/api/v1/remote/hosts/${encodeURIComponent(hostId)}/check`, {}),
        );
        setServersMessage({ type: 'success', text: `Health check completed: ${String(host?.name || host?.id || hostId)}` });
        if (selectedHostId === hostId) setManageMessage({ type: 'success', text: 'Host health check completed.' });
        await refresh();
        return payload;
      } catch (error) {
        const message = (error as Error).message;
        setServersMessage({ type: 'error', text: `Health check failed: ${message}` });
        if (selectedHostId === hostId) setManageMessage({ type: 'error', text: `Host health check failed: ${message}` });
        return null;
      }
    });
  }

  async function performManageRequest<T>(operation: string, request: () => Promise<T>) {
    const target = selectedHostId;
    const startedAt = performance.now();
    try {
      const payload = await request();
      const summary = nextOperation(operation, true, performance.now() - startedAt, String((payload as any)?.requestId || (payload as any)?.requestID || '').trim());
      if (target) setHostOps((current) => ({ ...current, [target]: summary }));
      setOpMeta(summary);
      return payload;
    } catch (error) {
      const summary = nextOperation(operation, false, performance.now() - startedAt, '', (error as Error).message);
      if (target) setHostOps((current) => ({ ...current, [target]: summary }));
      setOpMeta(summary);
      throw error;
    }
  }

  async function loadManageInstances(silent = false) {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId) return;
    await runManageOperation(async () => {
      if (!silent) setManageMessage({ type: 'info', text: `Loading instances for host ${target.hostId}...` });
      try {
        const payload = await performManageRequest('load_instances', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances`),
        );
        const instances = Array.isArray(payload?.instances) ? payload.instances : [];
        setInstancesText(formatInstances(instances));
        if (instances.length) {
          setManageForm((current) => ({ ...current, agentId: pickRemoteInstanceAgentID(instances[0]) }));
        }
        if (!silent) setManageMessage({ type: 'success', text: `Loaded ${instances.length} instances.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Load instances failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageInstanceStatus(prefix = 'Instance Status') {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Loading instance status for ${target.agentId}...` });
      try {
        const payload = await performManageRequest('instance_status', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/status`),
        );
        setInstanceStatusText(formatInstanceStatus(prefix, payload?.instance || {}, payload?.steps || []));
        setManageMessage({ type: 'success', text: `Loaded instance status for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Load instance status failed: ${(error as Error).message}` });
      }
    });
  }

  async function installManageInstance() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Live install in progress for ${target.agentId}...` });
      try {
        const payload = await performManageRequest('instance_install', () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/install/stream`, {}),
        );
        setInstanceStatusText(formatInstanceStatus('Install', payload?.install || {}, payload?.steps || []));
        setManageMessage({ type: 'success', text: `Install completed for ${target.agentId}.` });
        await loadManageInstances(true);
      } catch (error) {
        setManageMessage({ type: 'error', text: `Install failed: ${(error as Error).message}` });
      }
    });
  }

  async function repairManageInstance() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Repair in progress for ${target.agentId}...` });
      try {
        const payload = await performManageRequest('instance_repair', () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/repair`, {}),
        );
        setInstanceStatusText(formatInstanceStatus('Repair', payload?.repair || {}, payload?.steps || []));
        setManageMessage({ type: 'success', text: `Repair completed for ${target.agentId}.` });
        await loadManageInstances(true);
      } catch (error) {
        setManageMessage({ type: 'error', text: `Repair failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageLogs() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    const tail = Math.min(4000, Math.max(1, parseInt(manageForm.logTail || '200', 10) || 200));
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Loading logs for ${target.agentId} (tail=${tail})...` });
      try {
        const payload = await performManageRequest('instance_logs', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/logs?tail=${encodeURIComponent(String(tail))}`),
        );
        setLogsText(formatLogs(payload?.logs || ''));
        setManageMessage({ type: 'success', text: `Loaded logs for ${target.agentId} (tail=${tail}).` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Load logs failed: ${(error as Error).message}` });
      }
    });
  }

  async function syncManageInstance() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Sync in progress for ${target.agentId} (mode=${manageForm.syncMode})...` });
      try {
        const payload = await performManageRequest('instance_sync', () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/sync`, { mode: manageForm.syncMode }),
        );
        setInstanceStatusText(formatInstanceStatus('Sync', payload?.sync || {}, payload?.steps || []));
        setManageMessage({ type: 'success', text: `Sync completed for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Sync failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageSyncStatus() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Loading sync status for ${target.agentId}...` });
      try {
        const payload = await performManageRequest('instance_sync_status', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/sync/status`),
        );
        setInstanceStatusText(formatInstanceStatus('Sync Status', payload?.status || {}, []));
        setManageMessage({ type: 'success', text: `Sync status loaded for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Load sync status failed: ${(error as Error).message}` });
      }
    });
  }

  async function diagnoseManageInstance() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Diagnosing drift for ${target.agentId}...` });
      try {
        const payload = await performManageRequest('instance_diagnose', () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/diagnose`, {}),
        );
        setInstanceStatusText(formatInstanceStatus('Diagnose Drift', payload?.diagnose || {}, payload?.steps || []));
        setManageMessage({ type: 'success', text: `Diagnose completed for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Diagnose failed: ${(error as Error).message}` });
      }
    });
  }

  async function reconcileManageInstance() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Reconciling ${target.agentId}...` });
      try {
        const payload = await performManageRequest('instance_reconcile', () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/reconcile`, {}),
        );
        setInstanceStatusText(formatInstanceStatus('Reconcile', payload?.reconcile || {}, payload?.steps || []));
        setManageMessage({ type: 'success', text: `Reconcile completed for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Reconcile failed: ${(error as Error).message}` });
      }
    });
  }

  async function rollbackManageInstance() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Rollback in progress for ${target.agentId}...` });
      try {
        const body = manageForm.rollbackCommit.trim() ? { commit: manageForm.rollbackCommit.trim() } : {};
        const payload = await performManageRequest('instance_rollback', () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/rollback`, body),
        );
        setInstanceStatusText(formatInstanceStatus('Rollback', payload?.rollback || {}, payload?.steps || []));
        setManageMessage({ type: 'success', text: `Rollback completed for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Rollback failed: ${(error as Error).message}` });
      }
    });
  }

  async function installManageCodeAgent() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Installing codeagent (${manageForm.codeagentBackend}) for ${target.agentId}...` });
      try {
        const payload = await performManageRequest('codeagent_install', () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/codeagent/install`, {
            backend: manageForm.codeagentBackend,
            workspaceRoot: manageForm.codeagentWorkspaceRoot,
          }),
        );
        setInstanceStatusText(formatInstanceStatus('CodeAgent Install', payload?.install || {}, []));
        setManageMessage({ type: 'success', text: `CodeAgent install completed for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `CodeAgent install failed: ${(error as Error).message}` });
      }
    });
  }

  async function healthManageCodeAgent() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Checking codeagent health (${manageForm.codeagentBackend}) for ${target.agentId}...` });
      try {
        const query = new URLSearchParams({
          backend: manageForm.codeagentBackend,
          workspaceRoot: manageForm.codeagentWorkspaceRoot,
        });
        const payload = await performManageRequest('codeagent_health', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/codeagent/health?${query.toString()}`),
        );
        setInstanceStatusText(formatInstanceStatus('CodeAgent Health', payload?.health || {}, []));
        setManageMessage({ type: 'success', text: `CodeAgent health check completed for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `CodeAgent health check failed: ${(error as Error).message}` });
      }
    });
  }

  async function versionManageCodeAgent() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Loading codeagent version (${manageForm.codeagentBackend}) for ${target.agentId}...` });
      try {
        const query = new URLSearchParams({ backend: manageForm.codeagentBackend });
        const payload = await performManageRequest('codeagent_version', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/codeagent/version?${query.toString()}`),
        );
        setInstanceStatusText(formatInstanceStatus('CodeAgent Version', payload?.version || { backend: manageForm.codeagentBackend }, []));
        setManageMessage({ type: 'success', text: `CodeAgent version loaded for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `CodeAgent version failed: ${(error as Error).message}` });
      }
    });
  }

  async function runManageCodeAgent() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    if ((manageForm.codeagentCapability === 'run_shell' || manageForm.codeagentCapability === 'run_shell_redirect') && !manageForm.codeagentCommand.trim()) {
      setManageMessage({ type: 'error', text: 'CodeAgent command is required for run_shell capability.' });
      return;
    }
    if ((manageForm.codeagentCapability === 'read_file' || manageForm.codeagentCapability === 'write_file') && !manageForm.codeagentPath.trim()) {
      setManageMessage({ type: 'error', text: 'CodeAgent path is required for file capabilities.' });
      return;
    }
    if ((manageForm.codeagentCapability === 'write_file' || manageForm.codeagentCapability === 'apply_patch') && !manageForm.codeagentContent) {
      setManageMessage({ type: 'error', text: 'CodeAgent content is required for write/apply_patch.' });
      return;
    }
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Running codeagent capability ${manageForm.codeagentCapability} on ${target.agentId}...` });
      try {
        const payload = await performManageRequest('codeagent_run', () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(target.agentId)}/codeagent/run`, {
            backend: manageForm.codeagentBackend,
            workspaceRoot: manageForm.codeagentWorkspaceRoot,
            capability: manageForm.codeagentCapability,
            command: manageForm.codeagentCommand,
            path: manageForm.codeagentPath,
            content: manageForm.codeagentContent,
            writeMode: manageForm.codeagentWriteMode,
          }),
        );
        const runResult = payload?.run?.result || {};
        setInstanceStatusText(formatInstanceStatus('CodeAgent Run', runResult, []));
        const stdout = String(runResult?.stdout || '').trim();
        const stderr = String(runResult?.stderr || '').trim();
        setLogsText([stdout ? `[stdout]\n${stdout}` : '', stderr ? `[stderr]\n${stderr}` : ''].filter(Boolean).join('\n\n') || 'No logs available.');
        const policyDecision = String(runResult?.policy_decision || '').trim();
        if (policyDecision === 'deny' || policyDecision === 'ask') {
          setManageMessage({ type: 'error', text: `CodeAgent run blocked by policy (${policyDecision}).` });
          return;
        }
        setManageMessage({ type: 'success', text: `CodeAgent run completed for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `CodeAgent run failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageConfig() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Loading config for host ${target.hostId}...` });
      try {
        const payload = await performManageRequest('load_config', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/config`),
        );
        setConfigText(JSON.stringify(payload?.config || {}, null, 2));
        setManageMessage({ type: 'success', text: `Config loaded for host ${target.hostId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Load config failed: ${(error as Error).message}` });
      }
    });
  }

  async function applyManageConfig() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId) return;
    if (!configText.trim()) {
      setManageMessage({ type: 'error', text: 'Config patch cannot be empty.' });
      return;
    }
    let payload: Record<string, any>;
    try {
      payload = JSON.parse(configText);
    } catch (error) {
      setManageMessage({ type: 'error', text: `Config patch must be valid JSON: ${(error as Error).message}` });
      return;
    }
    if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
      setManageMessage({ type: 'error', text: 'Config patch must be a JSON object.' });
      return;
    }
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Applying config patch for host ${target.hostId}...` });
      try {
        const response = await performManageRequest('apply_config', () =>
          apiPatch<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/config`, payload),
        );
        setConfigText(JSON.stringify(response?.config || payload, null, 2));
        setManageMessage({ type: 'success', text: `Config patch applied for host ${target.hostId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Apply config patch failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageSessions() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Loading sessions for ${target.agentId}...` });
      try {
        const payload = await performManageRequest('load_sessions', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/sessions?agentId=${encodeURIComponent(target.agentId)}`),
        );
        const sessions = Array.isArray(payload?.sessions) ? payload.sessions : [];
        setSessionsText(formatSessions(sessions));
        setManageMessage({ type: 'success', text: `Loaded ${sessions.length} sessions for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Load sessions failed: ${(error as Error).message}` });
      }
    });
  }

  async function applySessionAction(action: 'archive' | 'delete') {
    const target = manageTarget(selectedHostId, manageForm);
    const sessionId = manageForm.sessionId.trim();
    if (!target.hostId || !target.agentId) return;
    if (!sessionId) {
      setManageMessage({ type: 'error', text: 'session id is required.' });
      return;
    }
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Session ${action} in progress for ${sessionId}...` });
      try {
        await performManageRequest(`${action}_session`, () =>
          apiPost<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/sessions/${encodeURIComponent(sessionId)}/${action}?agentId=${encodeURIComponent(target.agentId)}`, {}),
        );
        setManageMessage({ type: 'success', text: `Session ${sessionId} ${action}d.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Session ${action} failed: ${(error as Error).message}` });
      }
    });
  }

  async function loadManageMemory() {
    const target = manageTarget(selectedHostId, manageForm);
    if (!target.hostId || !target.agentId) return;
    await runManageOperation(async () => {
      setManageMessage({ type: 'info', text: `Loading memory for ${target.agentId}...` });
      try {
        const payload = await performManageRequest('load_memory', () =>
          apiGet<any>(`/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/memory?agentId=${encodeURIComponent(target.agentId)}`),
        );
        const memory = Array.isArray(payload?.memory) ? payload.memory : [];
        setMemoryText(formatMemory(memory));
        setManageMessage({ type: 'success', text: `Loaded ${memory.length} memory entries for ${target.agentId}.` });
      } catch (error) {
        setManageMessage({ type: 'error', text: `Load memory failed: ${(error as Error).message}` });
      }
    });
  }

  return {
    featureFlags,
    authz,
    featuresLoading,
    canManageHosts,
    hosts,
    sshAliases,
    serversMessage,
    setServersMessage,
    manageMessage,
    editor,
    setEditor,
    editingHostId,
    setEditingHostId,
    selectedHostId,
    selectedHost,
    manageForm,
    setManageForm,
    manageBusy,
    editorBusy,
    hostOps,
    opMeta,
    instancesText,
    instanceStatusText,
    logsText,
    configText,
    setConfigText,
    sessionsText,
    memoryText,
    refresh,
    resetEditor,
    showManageHost,
    handleSaveHost,
    handleDeleteHost,
    handleCheckHost,
    loadManageInstances,
    loadManageInstanceStatus,
    installManageInstance,
    repairManageInstance,
    loadManageLogs,
    syncManageInstance,
    loadManageSyncStatus,
    diagnoseManageInstance,
    reconcileManageInstance,
    rollbackManageInstance,
    installManageCodeAgent,
    healthManageCodeAgent,
    versionManageCodeAgent,
    runManageCodeAgent,
    loadManageConfig,
    applyManageConfig,
    loadManageSessions,
    applySessionAction,
    loadManageMemory,
  };
}
