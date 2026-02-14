export type HandoffStatus = "pending" | "declined";

export type RequestContext = {
  actor: string;
  requestId: string;
};

export type DaemonAgentState = {
  id: string;
  name: string;
  version: string;
  installed: boolean;
  runtimeState: "running" | "stopped";
  health: "healthy" | "unhealthy" | "unknown";
  needsRemoteDiagnosis: boolean;
  lastError?: string;
  updatedAt: string;
};

export type RemoteDiagnosisHandoff = {
  id: string;
  agentId: string;
  consent: boolean;
  artifactRef: string;
  status: HandoffStatus;
  createdAt: string;
};

export type CreateRemoteDiagnosisHandoffInput = {
  agentId: string;
  consent: boolean;
  actor: string;
  requestId: string;
};

export type DiagnoseResult = {
  artifactRef: string;
};

export type LogsResult = {
  lines: string[];
  truncated: boolean;
};

export type UpgradeResult = {
  agentId: string;
  fromVersion: string;
  toVersion: string;
};

export interface DaemonClient {
  listAgents(ctx: RequestContext): Promise<DaemonAgentState[]>;
  installAgent(agentId: string, ctx: RequestContext): Promise<void>;
  startAgent(agentId: string, ctx: RequestContext): Promise<void>;
  stopAgent(agentId: string, ctx: RequestContext): Promise<void>;
  getStatus(agentId: string | undefined, ctx: RequestContext): Promise<DaemonAgentState[]>;
  getLogs(agentId: string, tail: number, ctx: RequestContext): Promise<LogsResult>;
  upgradeAgent(agentId: string, ctx: RequestContext): Promise<UpgradeResult>;
  diagnoseAgent(agentId: string, ctx: RequestContext): Promise<DiagnoseResult>;
  createRemoteDiagnosisHandoff(input: CreateRemoteDiagnosisHandoffInput): Promise<RemoteDiagnosisHandoff>;
}

export class DaemonClientError extends Error {
  constructor(
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "DaemonClientError";
  }
}

export class RemoteDiagnosisNotNeededError extends DaemonClientError {
  constructor(message = "remote diagnosis is not required for this agent") {
    super("E_REMOTE_DIAG_NOT_NEEDED", message);
    this.name = "RemoteDiagnosisNotNeededError";
  }
}

type InMemoryAgentState = {
  id: string;
  name: string;
  version: string;
  installed: boolean;
  runtimeState: "running" | "stopped";
  health: "healthy" | "unhealthy" | "unknown";
  needsRemoteDiagnosis: boolean;
  lastError?: string;
  lastDiagnoseFile?: string;
  updatedAt: string;
};

type DaemonAuditEvent = {
  requestId: string;
  actor: string;
  action: string;
  target: string;
  message: string;
  timestamp: string;
};

// In-memory daemon adapter for M4 command routing.
// This keeps request context and command behavior close to the future daemon transport contract.
export class InMemoryDaemonClient implements DaemonClient {
  private nextHandoffID = 0;
  private nextDiagnoseID = 0;
  private readonly agents = new Map<string, InMemoryAgentState>();
  private readonly logs = new Map<string, string[]>();
  private readonly auditEvents: DaemonAuditEvent[] = [];

  constructor(private readonly now: () => Date = () => new Date()) {
    this.upsertAgent({
      id: "openclaw",
      name: "OpenClaw",
      version: "0.1.0",
      installed: false,
      runtimeState: "stopped",
      health: "unknown",
      needsRemoteDiagnosis: false,
      updatedAt: this.now().toISOString(),
    });
  }

  setRemoteDiagnosisState(agentId: string, needsRemoteDiagnosis: boolean): void {
    const state = this.requireAgent(agentId);
    this.upsertAgent({
      ...state,
      needsRemoteDiagnosis,
      updatedAt: this.now().toISOString(),
    });
  }

  setDiagnoseArtifact(agentId: string, fileRef: string): void {
    const state = this.requireAgent(agentId);
    this.upsertAgent({
      ...state,
      lastDiagnoseFile: fileRef,
      updatedAt: this.now().toISOString(),
    });
  }

  getAuditEvents(): DaemonAuditEvent[] {
    return this.auditEvents.slice();
  }

  async listAgents(ctx: RequestContext): Promise<DaemonAgentState[]> {
    this.recordAudit(ctx, "list_agents", "*", "listed agents");
    return [...this.agents.values()].map((state) => this.toPublicState(state));
  }

  async installAgent(agentId: string, ctx: RequestContext): Promise<void> {
    const state = this.requireAgent(agentId);
    this.upsertAgent({
      ...state,
      installed: true,
      runtimeState: "stopped",
      health: "unknown",
      lastError: undefined,
      updatedAt: this.now().toISOString(),
    });
    this.appendLog(agentId, `[install] request=${ctx.requestId} actor=${ctx.actor} installed`);
    this.recordAudit(ctx, "install", agentId, "installed");
  }

  async startAgent(agentId: string, ctx: RequestContext): Promise<void> {
    const state = this.requireAgent(agentId);
    if (!state.installed) {
      throw new DaemonClientError("E_NOT_INSTALLED", "agent is not installed");
    }
    if (state.runtimeState === "running") {
      throw new DaemonClientError("E_ALREADY_RUNNING", "agent is already running");
    }
    this.upsertAgent({
      ...state,
      runtimeState: "running",
      health: "healthy",
      lastError: undefined,
      updatedAt: this.now().toISOString(),
    });
    this.appendLog(agentId, `[start] request=${ctx.requestId} actor=${ctx.actor} started`);
    this.recordAudit(ctx, "start", agentId, "started");
  }

  async stopAgent(agentId: string, ctx: RequestContext): Promise<void> {
    const state = this.requireAgent(agentId);
    if (state.runtimeState === "stopped") {
      throw new DaemonClientError("E_ALREADY_STOPPED", "agent is already stopped");
    }
    this.upsertAgent({
      ...state,
      runtimeState: "stopped",
      health: "unknown",
      updatedAt: this.now().toISOString(),
    });
    this.appendLog(agentId, `[stop] request=${ctx.requestId} actor=${ctx.actor} stopped`);
    this.recordAudit(ctx, "stop", agentId, "stopped");
  }

  async getStatus(agentId: string | undefined, ctx: RequestContext): Promise<DaemonAgentState[]> {
    if (!agentId) {
      this.recordAudit(ctx, "status", "*", "status listed");
      return [...this.agents.values()].map((state) => this.toPublicState(state));
    }
    const state = this.requireAgent(agentId);
    this.recordAudit(ctx, "status", agentId, "status fetched");
    return [this.toPublicState(state)];
  }

  async getLogs(agentId: string, tail: number, ctx: RequestContext): Promise<LogsResult> {
    this.requireAgent(agentId);
    const entries = this.logs.get(agentId) ?? [];
    const safeTail = tail > 0 ? tail : 200;
    const start = entries.length > safeTail ? entries.length - safeTail : 0;
    this.recordAudit(ctx, "logs", agentId, `tail=${safeTail}`);
    return {
      lines: entries.slice(start),
      truncated: start > 0,
    };
  }

  async upgradeAgent(agentId: string, ctx: RequestContext): Promise<UpgradeResult> {
    const state = this.requireAgent(agentId);
    const fromVersion = state.version;
    const toVersion = bumpPatchVersion(fromVersion);
    this.upsertAgent({
      ...state,
      version: toVersion,
      updatedAt: this.now().toISOString(),
    });
    this.appendLog(agentId, `[upgrade] request=${ctx.requestId} actor=${ctx.actor} ${fromVersion} -> ${toVersion}`);
    this.recordAudit(ctx, "upgrade", agentId, `${fromVersion} -> ${toVersion}`);
    return {
      agentId,
      fromVersion,
      toVersion,
    };
  }

  async diagnoseAgent(agentId: string, ctx: RequestContext): Promise<DiagnoseResult> {
    const state = this.requireAgent(agentId);
    this.nextDiagnoseID += 1;
    const artifactRef = state.lastDiagnoseFile ?? `/tmp/${agentId}-diagnose-${this.nextDiagnoseID}.zip`;
    this.upsertAgent({
      ...state,
      lastDiagnoseFile: artifactRef,
      updatedAt: this.now().toISOString(),
    });
    this.appendLog(agentId, `[diagnose] request=${ctx.requestId} actor=${ctx.actor} ${artifactRef}`);
    this.recordAudit(ctx, "diagnose", agentId, artifactRef);
    return { artifactRef };
  }

  async createRemoteDiagnosisHandoff(input: CreateRemoteDiagnosisHandoffInput): Promise<RemoteDiagnosisHandoff> {
    const state = this.requireAgent(input.agentId);
    if (!state.needsRemoteDiagnosis) {
      throw new RemoteDiagnosisNotNeededError();
    }

    this.nextHandoffID += 1;
    const handoff: RemoteDiagnosisHandoff = {
      id: `handoff-${this.nextHandoffID}`,
      agentId: input.agentId,
      consent: input.consent,
      artifactRef: state.lastDiagnoseFile ?? "",
      status: input.consent ? "pending" : "declined",
      createdAt: this.now().toISOString(),
    };
    this.recordAudit(
      { actor: input.actor, requestId: input.requestId },
      "remote_diagnosis_consent",
      input.agentId,
      `consent=${input.consent} handoff=${handoff.id}`,
    );
    return handoff;
  }

  private requireAgent(agentId: string): InMemoryAgentState {
    const state = this.agents.get(agentId);
    if (!state) {
      throw new DaemonClientError("E_AGENT_NOT_FOUND", "agent not found");
    }
    return state;
  }

  private upsertAgent(state: InMemoryAgentState): void {
    this.agents.set(state.id, state);
    if (!this.logs.has(state.id)) {
      this.logs.set(state.id, []);
    }
  }

  private appendLog(agentId: string, message: string): void {
    const logs = this.logs.get(agentId) ?? [];
    logs.push(`${this.now().toISOString()} ${message}`);
    if (logs.length > 2000) {
      logs.splice(0, logs.length - 2000);
    }
    this.logs.set(agentId, logs);
  }

  private recordAudit(ctx: RequestContext, action: string, target: string, message: string): void {
    this.auditEvents.push({
      requestId: ctx.requestId,
      actor: ctx.actor,
      action,
      target,
      message,
      timestamp: this.now().toISOString(),
    });
    if (this.auditEvents.length > 2000) {
      this.auditEvents.splice(0, this.auditEvents.length - 2000);
    }
  }

  private toPublicState(state: InMemoryAgentState): DaemonAgentState {
    return {
      id: state.id,
      name: state.name,
      version: state.version,
      installed: state.installed,
      runtimeState: state.runtimeState,
      health: state.health,
      needsRemoteDiagnosis: state.needsRemoteDiagnosis,
      lastError: state.lastError,
      updatedAt: state.updatedAt,
    };
  }
}

function bumpPatchVersion(version: string): string {
  const parts = version.split(".");
  if (parts.length !== 3) {
    return version;
  }
  const patch = Number.parseInt(parts[2], 10);
  if (!Number.isFinite(patch)) {
    return version;
  }
  return `${parts[0]}.${parts[1]}.${patch + 1}`;
}
