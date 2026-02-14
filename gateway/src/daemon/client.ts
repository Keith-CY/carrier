export type HandoffStatus = "pending" | "declined";

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

export interface DaemonClient {
  createRemoteDiagnosisHandoff(input: CreateRemoteDiagnosisHandoffInput): Promise<RemoteDiagnosisHandoff>;
}

export class RemoteDiagnosisNotNeededError extends Error {
  readonly code = "E_REMOTE_DIAG_NOT_NEEDED";

  constructor(message = "remote diagnosis is not required for this agent") {
    super(message);
    this.name = "RemoteDiagnosisNotNeededError";
  }
}

type AgentRemoteDiagnosisState = {
  needsRemoteDiagnosis: boolean;
  lastDiagnoseFile?: string;
};

// Placeholder in-memory daemon adapter for M4 routing.
// This is intentionally shaped to match daemon lifecycle CreateRemoteDiagnosisHandoff semantics.
export class InMemoryDaemonClient implements DaemonClient {
  private nextID = 0;
  private readonly states = new Map<string, AgentRemoteDiagnosisState>();

  setRemoteDiagnosisState(agentId: string, state: AgentRemoteDiagnosisState): void {
    this.states.set(agentId, state);
  }

  async createRemoteDiagnosisHandoff(input: CreateRemoteDiagnosisHandoffInput): Promise<RemoteDiagnosisHandoff> {
    const state = this.states.get(input.agentId);
    if (!state?.needsRemoteDiagnosis) {
      throw new RemoteDiagnosisNotNeededError();
    }

    this.nextID += 1;
    return {
      id: `handoff-${this.nextID}`,
      agentId: input.agentId,
      consent: input.consent,
      artifactRef: state.lastDiagnoseFile ?? "",
      status: input.consent ? "pending" : "declined",
      createdAt: new Date().toISOString(),
    };
  }
}
