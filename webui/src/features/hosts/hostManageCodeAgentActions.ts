import { apiGet, apiPost } from '../../lib/api';
import { formatInstanceStatus } from './model';
import { type HostManageOperationContext, type HostManageRuntime } from './hostManageShared';

export function createHostManageCodeAgentActions(ctx: HostManageOperationContext, runtime: HostManageRuntime) {
  async function installManageCodeAgent() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({
        type: 'info',
        text: `Installing codeagent (${ctx.manageForm.codeagentBackend}) for ${target.agentId}...`,
      });
      try {
        const payload = await runtime.performManageRequest('codeagent_install', () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/codeagent/install`,
            {
              backend: ctx.manageForm.codeagentBackend,
              workspaceRoot: ctx.manageForm.codeagentWorkspaceRoot,
            },
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('CodeAgent Install', payload?.install || {}, []));
        ctx.setManageMessage({ type: 'success', text: `CodeAgent install completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `CodeAgent install failed: ${(error as Error).message}` });
      }
    });
  }

  async function healthManageCodeAgent() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({
        type: 'info',
        text: `Checking codeagent health (${ctx.manageForm.codeagentBackend}) for ${target.agentId}...`,
      });
      try {
        const query = new URLSearchParams({
          backend: ctx.manageForm.codeagentBackend,
          workspaceRoot: ctx.manageForm.codeagentWorkspaceRoot,
        });
        const payload = await runtime.performManageRequest('codeagent_health', () =>
          apiGet<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/codeagent/health?${query.toString()}`,
          ),
        );
        ctx.setInstanceStatusText(formatInstanceStatus('CodeAgent Health', payload?.health || {}, []));
        ctx.setManageMessage({ type: 'success', text: `CodeAgent health check completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `CodeAgent health check failed: ${(error as Error).message}` });
      }
    });
  }

  async function versionManageCodeAgent() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({
        type: 'info',
        text: `Loading codeagent version (${ctx.manageForm.codeagentBackend}) for ${target.agentId}...`,
      });
      try {
        const query = new URLSearchParams({ backend: ctx.manageForm.codeagentBackend });
        const payload = await runtime.performManageRequest('codeagent_version', () =>
          apiGet<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/codeagent/version?${query.toString()}`,
          ),
        );
        ctx.setInstanceStatusText(
          formatInstanceStatus('CodeAgent Version', payload?.version || { backend: ctx.manageForm.codeagentBackend }, []),
        );
        ctx.setManageMessage({ type: 'success', text: `CodeAgent version loaded for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `CodeAgent version failed: ${(error as Error).message}` });
      }
    });
  }

  async function runManageCodeAgent() {
    const target = runtime.getTarget();
    if (!target.hostId || !target.agentId) return;
    if (
      (ctx.manageForm.codeagentCapability === 'run_shell' || ctx.manageForm.codeagentCapability === 'run_shell_redirect') &&
      !ctx.manageForm.codeagentCommand.trim()
    ) {
      ctx.setManageMessage({ type: 'error', text: 'CodeAgent command is required for run_shell capability.' });
      return;
    }
    if (
      (ctx.manageForm.codeagentCapability === 'read_file' || ctx.manageForm.codeagentCapability === 'write_file') &&
      !ctx.manageForm.codeagentPath.trim()
    ) {
      ctx.setManageMessage({ type: 'error', text: 'CodeAgent path is required for file capabilities.' });
      return;
    }
    if (
      (ctx.manageForm.codeagentCapability === 'write_file' || ctx.manageForm.codeagentCapability === 'apply_patch') &&
      !ctx.manageForm.codeagentContent
    ) {
      ctx.setManageMessage({ type: 'error', text: 'CodeAgent content is required for write/apply_patch.' });
      return;
    }
    await runtime.runManageOperation(async () => {
      ctx.setManageMessage({
        type: 'info',
        text: `Running codeagent capability ${ctx.manageForm.codeagentCapability} on ${target.agentId}...`,
      });
      try {
        const payload = await runtime.performManageRequest('codeagent_run', () =>
          apiPost<any>(
            `/api/v1/remote/hosts/${encodeURIComponent(target.hostId)}/instances/${encodeURIComponent(
              target.agentId,
            )}/codeagent/run`,
            {
              backend: ctx.manageForm.codeagentBackend,
              workspaceRoot: ctx.manageForm.codeagentWorkspaceRoot,
              capability: ctx.manageForm.codeagentCapability,
              command: ctx.manageForm.codeagentCommand,
              path: ctx.manageForm.codeagentPath,
              content: ctx.manageForm.codeagentContent,
              writeMode: ctx.manageForm.codeagentWriteMode,
            },
          ),
        );
        const runResult = payload?.run?.result || {};
        ctx.setInstanceStatusText(formatInstanceStatus('CodeAgent Run', runResult, []));
        const stdout = String(runResult?.stdout || '').trim();
        const stderr = String(runResult?.stderr || '').trim();
        ctx.setLogsText(
          [stdout ? `[stdout]\n${stdout}` : '', stderr ? `[stderr]\n${stderr}` : '']
            .filter(Boolean)
            .join('\n\n') || 'No logs available.',
        );
        const policyDecision = String(runResult?.policy_decision || '').trim();
        if (policyDecision === 'deny' || policyDecision === 'ask') {
          ctx.setManageMessage({ type: 'error', text: `CodeAgent run blocked by policy (${policyDecision}).` });
          return;
        }
        ctx.setManageMessage({ type: 'success', text: `CodeAgent run completed for ${target.agentId}.` });
      } catch (error) {
        ctx.setManageMessage({ type: 'error', text: `CodeAgent run failed: ${(error as Error).message}` });
      }
    });
  }

  return {
    installManageCodeAgent,
    healthManageCodeAgent,
    versionManageCodeAgent,
    runManageCodeAgent,
  };
}
