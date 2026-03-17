import type { Dispatch, SetStateAction } from 'react';
import { manageTarget, nextOperation, type HostRecord, type ManageFormState, type MessageState, type OperationSummary } from './model';

export type HostManageOperationContext = {
  hosts: HostRecord[];
  refresh: () => Promise<void>;
  selectedHostId: string;
  manageForm: ManageFormState;
  manageBusy: boolean;
  configText: string;
  setServersMessage: (message: MessageState) => void;
  setManageMessage: (message: MessageState) => void;
  setManageBusy: (next: boolean) => void;
  setHostOps: Dispatch<SetStateAction<Record<string, OperationSummary>>>;
  setOpMeta: (summary: OperationSummary | null) => void;
  setInstancesText: (text: string) => void;
  setInstanceStatusText: (text: string) => void;
  setLogsText: (text: string) => void;
  setStreamStatusText: (text: string) => void;
  setConfigText: (text: string) => void;
  setSessionsText: (text: string) => void;
  setMemoryText: (text: string) => void;
  setManageForm: Dispatch<SetStateAction<ManageFormState>>;
  editingHostId: string;
  resetEditor: (clearForm: boolean) => void;
};

export type HostManageRuntime = {
  recordHostOperation: (hostId: string, operation: string, work: () => Promise<any>) => Promise<{ payload: any; summary: OperationSummary }>;
  runManageOperation: <T>(work: () => Promise<T>) => Promise<T | null>;
  performManageRequest: <T>(operation: string, request: () => Promise<T>) => Promise<T>;
  getTarget: () => ReturnType<typeof manageTarget>;
};

export function createHostManageRuntime(ctx: HostManageOperationContext): HostManageRuntime {
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
      ctx.setHostOps((current) => ({ ...current, [hostId]: summary }));
      return { payload, summary };
    } catch (error) {
      const summary = nextOperation(operation, false, performance.now() - startedAt, '', (error as Error).message);
      ctx.setHostOps((current) => ({ ...current, [hostId]: summary }));
      throw error;
    }
  }

  async function runManageOperation<T>(work: () => Promise<T>) {
    if (ctx.manageBusy) {
      ctx.setManageMessage({ type: 'info', text: 'Another operation is already running.' });
      return null;
    }
    ctx.setManageBusy(true);
    try {
      return await work();
    } finally {
      ctx.setManageBusy(false);
    }
  }

  async function performManageRequest<T>(operation: string, request: () => Promise<T>) {
    const target = ctx.selectedHostId;
    const startedAt = performance.now();
    try {
      const payload = await request();
      const summary = nextOperation(
        operation,
        true,
        performance.now() - startedAt,
        String((payload as any)?.requestId || (payload as any)?.requestID || '').trim(),
      );
      if (target) ctx.setHostOps((current) => ({ ...current, [target]: summary }));
      ctx.setOpMeta(summary);
      return payload;
    } catch (error) {
      const summary = nextOperation(operation, false, performance.now() - startedAt, '', (error as Error).message);
      if (target) ctx.setHostOps((current) => ({ ...current, [target]: summary }));
      ctx.setOpMeta(summary);
      throw error;
    }
  }

  return {
    recordHostOperation,
    runManageOperation,
    performManageRequest,
    getTarget: () => manageTarget(ctx.selectedHostId, ctx.manageForm),
  };
}
