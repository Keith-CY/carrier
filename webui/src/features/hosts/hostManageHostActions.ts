import { apiDelete, apiPost } from '../../lib/api';
import { type HostManageOperationContext, type HostManageRuntime } from './hostManageShared';

export function createHostManageHostActions(ctx: HostManageOperationContext, runtime: HostManageRuntime) {
  async function handleDeleteHost(hostId: string) {
    if (!window.confirm(`Delete remote host ${hostId}?`)) return;
    try {
      await apiDelete(`/api/v1/remote/hosts/${encodeURIComponent(hostId)}`);
      ctx.setServersMessage({ type: 'success', text: `Deleted remote host: ${hostId}` });
      ctx.setHostOps((current) => {
        const next = { ...current };
        delete next[hostId];
        return next;
      });
      if (ctx.editingHostId === hostId) ctx.resetEditor(true);
      await ctx.refresh();
    } catch (error) {
      ctx.setServersMessage({ type: 'error', text: `Delete failed: ${(error as Error).message}` });
    }
  }

  async function handleCheckHost(hostId: string) {
    const host = ctx.hosts.find((item) => String(item?.id || '').trim() === hostId) || { id: hostId };
    await runtime.runManageOperation(async () => {
      ctx.setServersMessage({
        type: 'info',
        text: `Running health check: ${String(host?.name || host?.id || hostId)}...`,
      });
      try {
        const { payload } = await runtime.recordHostOperation(hostId, 'host_check', () =>
          apiPost(`/api/v1/remote/hosts/${encodeURIComponent(hostId)}/check`, {}),
        );
        ctx.setServersMessage({
          type: 'success',
          text: `Health check completed: ${String(host?.name || host?.id || hostId)}`,
        });
        if (ctx.selectedHostId === hostId) ctx.setManageMessage({ type: 'success', text: 'Host health check completed.' });
        await ctx.refresh();
        return payload;
      } catch (error) {
        const message = (error as Error).message;
        ctx.setServersMessage({ type: 'error', text: `Health check failed: ${message}` });
        if (ctx.selectedHostId === hostId) ctx.setManageMessage({ type: 'error', text: `Host health check failed: ${message}` });
        return null;
      }
    });
  }

  return {
    handleDeleteHost,
    handleCheckHost,
  };
}
