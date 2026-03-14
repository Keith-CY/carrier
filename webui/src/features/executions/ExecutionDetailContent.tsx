import { ExecutionGovernanceBlock } from './components/ExecutionGovernanceBlock';
import { ExecutionLineageBlock } from './components/ExecutionLineageBlock';
import { ExecutionOutcomeBlock } from './components/ExecutionOutcomeBlock';
import { ExecutionPolicyBlock } from './components/ExecutionPolicyBlock';
import { ExecutionResultsBlock } from './components/ExecutionResultsBlock';
import { ExecutionSummaryBlock } from './components/ExecutionSummaryBlock';
import { ExecutionWorkContextBlock } from './components/ExecutionWorkContextBlock';
import { TriggerBlock } from './components/TriggerBlock';

export function isExecutionTerminalStatus(status: unknown): boolean {
  const normalized = String(status || '').trim().toLowerCase();
  return ['completed', 'partial_completed', 'failed', 'retryable_failed', 'cancelled', 'declined'].includes(normalized);
}

export function executionHasFailedTasks(execution: any): boolean {
  const results = Array.isArray(execution?.results) ? execution.results : [];
  return results.some((item) => String(item?.status || '').trim().toLowerCase() === 'failed');
}

export function ExecutionDetailContent({ execution, workers, onDownloadArtifact }: {
  execution: any;
  workers: any[];
  onDownloadArtifact: (artifactId: string, filename: string) => void | Promise<void>;
}) {
  return (
    <>
      <ExecutionSummaryBlock execution={execution} />
      <ExecutionWorkContextBlock execution={execution} />
      <TriggerBlock execution={execution} />
      <ExecutionLineageBlock execution={execution} />
      <ExecutionOutcomeBlock execution={execution} onDownloadArtifact={onDownloadArtifact} />
      <ExecutionPolicyBlock execution={execution} />
      <ExecutionGovernanceBlock execution={execution} />
      <ExecutionResultsBlock execution={execution} workers={workers} />
    </>
  );
}

export function executionCounts(execution: any) {
  const taskUnits = Array.isArray(execution?.taskUnits) ? execution.taskUnits : [];
  const results = Array.isArray(execution?.results) ? execution.results : [];
  return {
    taskUnits,
    results,
    completed: results.filter((item) => String(item?.status || '').trim().toLowerCase() === 'completed').length,
    failed: results.filter((item) => ['failed', 'cancelled'].includes(String(item?.status || '').trim().toLowerCase())).length,
  };
}

export function executionTemplateValue(execution: any): string {
  return String(execution?.templateId || '').trim();
}

export function executionTriggerValue(execution: any): string {
  return String(execution?.triggerSource || '').trim().toLowerCase();
}

export function executionTriggerLabel(execution: any): string {
  const source = String(execution?.triggerSource || '').trim();
  const triggerID = String(execution?.triggerId || '').trim();
  if (source && triggerID) return `${source}:${triggerID}`;
  return source || triggerID;
}

export function executionSearchText(execution: any): string {
  return [
    String(execution?.id || ''),
    String(execution?.goal || ''),
    String(execution?.team || ''),
    String(execution?.project || ''),
    String(execution?.environment || ''),
    executionTemplateValue(execution),
    String(execution?.triggerSource || ''),
    String(execution?.triggerId || ''),
    executionTriggerLabel(execution),
    String(execution?.initiator || ''),
  ].join(' ').trim().toLowerCase();
}

export function executionAttributionParts(execution: any): string[] {
  const parts = [];
  if (String(execution?.team || '').trim()) parts.push(`Team: ${String(execution.team).trim()}`);
  if (String(execution?.project || '').trim()) parts.push(`Project: ${String(execution.project).trim()}`);
  if (String(execution?.environment || '').trim()) parts.push(`Env: ${String(execution.environment).trim()}`);
  if (executionTemplateValue(execution)) parts.push(`Template: ${executionTemplateValue(execution)}`);
  if (executionTriggerLabel(execution)) parts.push(`Trigger: ${executionTriggerLabel(execution)}`);
  return parts;
}
