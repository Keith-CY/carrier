import {
  executionSearchText,
  executionTemplateValue,
  executionTriggerValue,
  isExecutionTerminalStatus,
} from './ExecutionDetailContent';

type ExecutionFilters = {
  searchValue: string;
  statusFilter: string;
  templateFilter: string;
  triggerFilter: string;
};

export function normalizeExecutions(payload: any): any[] {
  const executions = Array.isArray(payload?.executions) ? payload.executions : [];
  return executions.slice().sort((left, right) => {
    const a = new Date(String(left?.updatedAt || '')).getTime() || 0;
    const b = new Date(String(right?.updatedAt || '')).getTime() || 0;
    return b - a;
  });
}

export function filterExecutions(executions: any[], filters: ExecutionFilters): any[] {
  return executions.filter((execution) => {
    const normalizedQuery = filters.searchValue.trim().toLowerCase();
    if (normalizedQuery && !executionSearchText(execution).includes(normalizedQuery)) return false;
    if (filters.templateFilter !== 'all' && executionTemplateValue(execution) !== filters.templateFilter) return false;
    if (filters.triggerFilter !== 'all' && executionTriggerValue(execution) !== filters.triggerFilter) return false;
    const status = String(execution?.status || '').trim().toLowerCase();
    switch (filters.statusFilter) {
      case 'active':
        return !isExecutionTerminalStatus(status);
      case 'completed':
        return status === 'completed' || status === 'partial_completed';
      case 'failed':
        return status === 'failed' || status === 'retryable_failed' || status === 'declined';
      case 'cancelled':
        return status === 'cancelled';
      default:
        return true;
    }
  });
}

export function selectExecutionId({
  routeExecutionId,
  selectedExecutionId,
  filteredExecutions,
}: {
  routeExecutionId: string;
  selectedExecutionId: string;
  filteredExecutions: any[];
}): string {
  if (routeExecutionId) return routeExecutionId;
  if (selectedExecutionId && filteredExecutions.some((item) => String(item?.id || '').trim() === selectedExecutionId)) {
    return selectedExecutionId;
  }
  return String(filteredExecutions[0]?.id || '').trim();
}
