export type WorkProject = {
  id: string;
  name: string;
  sourceType: string;
  sourceRef: string;
  defaultBranch: string;
  workflowPath: string;
  workflowDigest: string;
  state: string;
  lastSyncAt: string;
  lastSyncError: string;
};

export type WorkItem = {
  id: string;
  projectId: string;
  title: string;
  description: string;
  acceptance: string[];
  priority: string;
  source: string;
  sourceRef: string;
  labels: string[];
  state: string;
  claimedByRunId: string;
  latestRunId: string;
  createdAt: string;
  updatedAt: string;
};

export type WorkRun = {
  id: string;
  projectId: string;
  workItemId: string;
  executionId: string;
  workspaceId: string;
  workspacePath: string;
  backend: string;
  phase: string;
  leaseOwner: string;
  leaseExpiresAt: string;
  verificationStatus: string;
  publishStatus: string;
  workflowDigest: string;
  createdAt: string;
  updatedAt: string;
};

function toText(value: unknown): string {
  return String(value || '').trim();
}

function toTextList(value: unknown): string[] {
  return Array.isArray(value) ? value.map((item) => toText(item)).filter(Boolean) : [];
}

function sortByUpdatedAtDesc<T extends { updatedAt: string; id: string }>(values: T[]): T[] {
  return values.slice().sort((left, right) => {
    const a = new Date(left.updatedAt).getTime() || 0;
    const b = new Date(right.updatedAt).getTime() || 0;
    if (a !== b) return b - a;
    return left.id.localeCompare(right.id);
  });
}

export function labelizeWorkValue(value: unknown): string {
  const normalized = toText(value);
  return normalized ? normalized.replace(/_/g, ' ') : 'unknown';
}

export function normalizeWorkProject(payload: any): WorkProject | null {
  const source = payload && typeof payload === 'object' && payload.project && typeof payload.project === 'object'
    ? payload.project
    : payload;
  if (!source || typeof source !== 'object') return null;
  const project: WorkProject = {
    id: toText(source.id),
    name: toText(source.name),
    sourceType: toText(source.sourceType),
    sourceRef: toText(source.sourceRef),
    defaultBranch: toText(source.defaultBranch),
    workflowPath: toText(source.workflowPath),
    workflowDigest: toText(source.workflowDigest),
    state: toText(source.state),
    lastSyncAt: toText(source.lastSyncAt),
    lastSyncError: toText(source.lastSyncError),
  };
  return project.id ? project : null;
}

export function normalizeWorkProjects(payload: any): WorkProject[] {
  const projects = Array.isArray(payload?.projects) ? payload.projects : [];
  return projects
    .map((project) => normalizeWorkProject(project))
    .filter(Boolean)
    .sort((left, right) => left!.name.localeCompare(right!.name) || left!.id.localeCompare(right!.id)) as WorkProject[];
}

export function normalizeWorkItem(payload: any): WorkItem | null {
  const source = payload && typeof payload === 'object' && payload.item && typeof payload.item === 'object'
    ? payload.item
    : payload;
  if (!source || typeof source !== 'object') return null;
  const item: WorkItem = {
    id: toText(source.id),
    projectId: toText(source.projectId),
    title: toText(source.title),
    description: toText(source.description),
    acceptance: toTextList(source.acceptance),
    priority: toText(source.priority),
    source: toText(source.source),
    sourceRef: toText(source.sourceRef),
    labels: toTextList(source.labels),
    state: toText(source.state),
    claimedByRunId: toText(source.claimedByRunId),
    latestRunId: toText(source.latestRunId),
    createdAt: toText(source.createdAt),
    updatedAt: toText(source.updatedAt),
  };
  return item.id ? item : null;
}

export function normalizeWorkItems(payload: any): WorkItem[] {
  const items = Array.isArray(payload?.items) ? payload.items : [];
  return sortByUpdatedAtDesc(items.map((item) => normalizeWorkItem(item)).filter(Boolean) as WorkItem[]);
}

export function normalizeWorkRun(payload: any): WorkRun | null {
  const source = payload && typeof payload === 'object' && payload.run && typeof payload.run === 'object'
    ? payload.run
    : payload;
  if (!source || typeof source !== 'object') return null;
  const run: WorkRun = {
    id: toText(source.id),
    projectId: toText(source.projectId),
    workItemId: toText(source.workItemId),
    executionId: toText(source.executionId),
    workspaceId: toText(source.workspaceId),
    workspacePath: toText(source.workspacePath),
    backend: toText(source.backend),
    phase: toText(source.phase),
    leaseOwner: toText(source.leaseOwner),
    leaseExpiresAt: toText(source.leaseExpiresAt),
    verificationStatus: toText(source.verificationStatus),
    publishStatus: toText(source.publishStatus),
    workflowDigest: toText(source.workflowDigest),
    createdAt: toText(source.createdAt),
    updatedAt: toText(source.updatedAt),
  };
  return run.id ? run : null;
}

export function normalizeWorkRuns(payload: any): WorkRun[] {
  const runs = Array.isArray(payload?.runs) ? payload.runs : [];
  return sortByUpdatedAtDesc(runs.map((run) => normalizeWorkRun(run)).filter(Boolean) as WorkRun[]);
}
