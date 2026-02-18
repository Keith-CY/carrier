import {
  DaemonClientError,
  RemoteDiagnosisNotNeededError,
  type CreateRemoteDiagnosisHandoffInput,
  type DaemonAgentState,
  type DaemonClient,
  type DiagnoseResult,
  type LogsResult,
  type RemoteDiagnosisHandoff,
  type RequestContext,
  type UpgradeResult,
} from "./client";

const DEFAULT_DAEMON_BASE_URL = "http://127.0.0.1:9090";
const DEFAULT_DAEMON_TIMEOUT_MS = 30_000;

type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

type DaemonErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
  };
};

type DaemonAgentsResponse = {
  agents?: DaemonAgentState[];
};

type DaemonStatusesResponse = {
  statuses?: DaemonAgentState[];
};

export function loadDaemonBaseUrl(env: Record<string, string | undefined> = process.env): string {
  const raw = env.CARRIER_DAEMON_BASE_URL?.trim() || DEFAULT_DAEMON_BASE_URL;
  return raw.replace(/\/+$/, "");
}

export function loadDaemonTimeoutMs(env: Record<string, string | undefined> = process.env): number {
  const raw = env.CARRIER_DAEMON_TIMEOUT_MS?.trim();
  if (!raw) {
    return DEFAULT_DAEMON_TIMEOUT_MS;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return DEFAULT_DAEMON_TIMEOUT_MS;
  }
  return parsed;
}

export class HttpDaemonClient implements DaemonClient {
  constructor(
    private readonly baseUrl = loadDaemonBaseUrl(),
    private readonly fetchImpl: FetchLike = fetch,
    private readonly requestTimeoutMs: number = loadDaemonTimeoutMs(),
  ) {}

  async listAgents(ctx: RequestContext): Promise<DaemonAgentState[]> {
    const data = await this.requestJSON<DaemonAgentsResponse | DaemonAgentState[]>(
      "GET",
      "/api/v1/agents",
      undefined,
      ctx,
    );
    if (Array.isArray(data)) {
      return data;
    }
    return data.agents ?? [];
  }

  async installAgent(agentId: string, ctx: RequestContext): Promise<void> {
    await this.requestJSON("POST", `/api/v1/agents/${encodeURIComponent(agentId)}/install`, undefined, ctx);
  }

  async startAgent(agentId: string, ctx: RequestContext): Promise<void> {
    await this.requestJSON("POST", `/api/v1/agents/${encodeURIComponent(agentId)}/start`, undefined, ctx);
  }

  async stopAgent(agentId: string, ctx: RequestContext): Promise<void> {
    await this.requestJSON("POST", `/api/v1/agents/${encodeURIComponent(agentId)}/stop`, undefined, ctx);
  }

  async getStatus(agentId: string | undefined, ctx: RequestContext): Promise<DaemonAgentState[]> {
    const path = agentId
      ? `/api/v1/agents/${encodeURIComponent(agentId)}/status`
      : "/api/v1/agents/status";
    const data = await this.requestJSON<DaemonStatusesResponse | DaemonAgentState[]>( "GET", path, undefined, ctx);
    if (Array.isArray(data)) {
      return data;
    }
    return data.statuses ?? [];
  }

  async getLogs(agentId: string, tail: number, ctx: RequestContext): Promise<LogsResult> {
    const safeTail = Number.isFinite(tail) && tail > 0 ? Math.min(Math.floor(tail), 1000) : 200;
    return await this.requestJSON<LogsResult>(
      "GET",
      `/api/v1/agents/${encodeURIComponent(agentId)}/logs?tail=${safeTail}`,
      undefined,
      ctx,
    );
  }

  async getMergedLogs(tail: number, ctx: RequestContext): Promise<LogsResult> {
    const safeTail = Number.isFinite(tail) && tail > 0 ? Math.min(Math.floor(tail), 1000) : 200;
    return await this.requestJSON<LogsResult>(
      "GET",
      `/api/v1/logs?tail=${safeTail}`,
      undefined,
      ctx,
    );
  }

  async upgradeAgent(agentId: string, ctx: RequestContext): Promise<UpgradeResult> {
    return await this.requestJSON<UpgradeResult>(
      "POST",
      `/api/v1/agents/${encodeURIComponent(agentId)}/upgrade`,
      undefined,
      ctx,
    );
  }

  async diagnoseAgent(agentId: string, ctx: RequestContext): Promise<DiagnoseResult> {
    return await this.requestJSON<DiagnoseResult>(
      "POST",
      `/api/v1/agents/${encodeURIComponent(agentId)}/diagnose`,
      undefined,
      ctx,
    );
  }

  async createRemoteDiagnosisHandoff(input: CreateRemoteDiagnosisHandoffInput): Promise<RemoteDiagnosisHandoff> {
    return await this.requestJSON<RemoteDiagnosisHandoff>(
      "POST",
      "/api/v1/diagnosis/handoffs",
      input,
      {
        actor: input.actor,
        requestId: input.requestId,
      },
    );
  }

  async verifyPairCode(code: string, ctx: RequestContext): Promise<void> {
    await this.requestJSON(
      "POST",
      "/api/v1/pairing/verify-consume",
      { code },
      ctx,
    );
  }

  private async requestJSON<T>(
    method: string,
    path: string,
    body: unknown,
    ctx: RequestContext,
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const response = await this.fetchImpl(url, {
      method,
      headers: {
        "content-type": "application/json",
        "x-carrier-actor": ctx.actor,
        "x-carrier-request-id": ctx.requestId,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(this.requestTimeoutMs),
    });

    if (!response.ok) {
      throw await this.toClientError(response);
    }

    const raw = await response.text();
    if (!raw.trim()) {
      return {} as T;
    }
    return JSON.parse(raw) as T;
  }

  private async toClientError(response: Response): Promise<DaemonClientError> {
    const fallbackCode = this.mapStatusToCode(response.status);
    let message = `daemon request failed with status ${response.status}`;
    let code = fallbackCode;

    try {
      const payload = (await response.json()) as DaemonErrorEnvelope;
      if (payload?.error?.code) {
        code = payload.error.code;
      }
      if (payload?.error?.message) {
        message = payload.error.message;
      }
    } catch {
      // Keep fallback code/message when response is not JSON.
    }

    if (code === "E_REMOTE_DIAG_NOT_NEEDED") {
      return new RemoteDiagnosisNotNeededError(message);
    }
    return new DaemonClientError(code, message);
  }

  private mapStatusToCode(status: number): string {
    switch (status) {
      case 400:
        return "E_USAGE";
      case 401:
      case 403:
        return "E_SESSION_REQUIRED";
      case 404:
        return "E_AGENT_NOT_FOUND";
      default:
        return "E_COMMAND_FAILED";
    }
  }
}
