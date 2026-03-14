export function normalizeMemoryPayload(payload: any, fallbackSubject: string) {
  const source = payload && typeof payload === 'object' ? payload : {};
  return {
    subject: String(source.subject || fallbackSubject || '').trim() || 'all',
    entries: Array.isArray(source.entries) ? source.entries : [],
    attachments: Array.isArray(source.attachments) ? source.attachments : [],
    grants: Array.isArray(source.grants) ? source.grants : [],
    audit: Array.isArray(source.audit) ? source.audit : [],
  };
}

export function buildMemorySearchPayload(input: {
  subject: string;
  query: string;
  limit: string;
  minScore: string;
}): { error: string } | { payload: any } {
  const query = String(input.query || '').trim();
  if (!query) return { error: 'Search query is required.' };
  return {
    payload: {
      subject: String(input.subject || '').trim(),
      query,
      maxResults: Math.max(1, parseInt(String(input.limit || '').trim(), 10) || 10),
      minScore: Math.max(0, parseFloat(String(input.minScore || '').trim()) || 0),
    },
  };
}

export function buildMemoryInstanceAction(input: {
  action: 'attach' | 'detach' | 'distill';
  instanceId: string;
  scope: string;
  reason: string;
  dryRun: boolean;
}): { error: string } | { path: string; payload: Record<string, unknown> } {
  const instanceId = String(input.instanceId || '').trim();
  const scope = String(input.scope || '').trim();
  if (!instanceId) return { error: 'Instance ID is required.' };
  if ((input.action === 'attach' || input.action === 'detach') && !scope) {
    return { error: 'Scope is required for attach/detach.' };
  }

  const payload: Record<string, unknown> = { instanceId };
  if (input.action === 'attach') {
    payload.scope = scope;
    return { path: '/api/v1/memory/instance/attach', payload };
  }
  if (input.action === 'detach') {
    payload.scope = scope;
    return { path: '/api/v1/memory/instance/detach', payload };
  }
  if (scope) payload.scope = scope;
  if (String(input.reason || '').trim()) payload.reason = String(input.reason).trim();
  if (input.dryRun) payload.dryRun = true;
  return { path: '/api/v1/memory/instance/distill', payload };
}
