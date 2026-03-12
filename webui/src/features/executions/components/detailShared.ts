export function executionStatusBadgeClass(status: unknown): string {
  const normalized = String(status || '').trim().toLowerCase();
  if (normalized === 'completed') return 'badge badge-ok';
  if (normalized === 'partial_completed') return 'badge badge-warn';
  if (['failed', 'retryable_failed', 'declined', 'cancelled'].includes(normalized)) return 'badge badge-error';
  return 'badge badge-unknown';
}

export function artifactDownloadPath(executionID: string, artifactID: string): string {
  return `/api/v1/orchestrator/executions/${encodeURIComponent(String(executionID || '').trim())}/artifacts/${encodeURIComponent(String(artifactID || '').trim())}`;
}
