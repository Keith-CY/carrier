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

const DEFAULT_DAEMON_URL = "http://localhost:7331";

function getDaemonUrl(): string {
  return process.env.CARRIER_DAEMON_URL ?? DEFAULT_DAEMON_URL;
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const url = `${getDaemonUrl()}${path}`;
  const init: RequestInit = {
    method,
    headers: { "Content-Type": "application/json" },
  };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
  }

  let res: Response;
  try {
    res = await fetch(url, init);
  } catch (err) {
    throw new DaemonClientError(
      "E_NETWORK",
      `Failed to connect to daemon at ${url}: ${err}`,
    );
  }

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    let errorMessage = text;
    try {
      const json = JSON.parse(text);
      errorMessage = json.error ?? text;
    } catch {
      // use raw text
    }
    throw mapHttpError(res.status, errorMessage);
  }

  return (await res.json()) as T;
}

function mapHttpError(status: number, message: string): DaemonClientError {
  if (message.includes("remote diagnosis is not required")) {
    return new RemoteDiagnosisNotNeededError(message);
  }

  const codeMap: Record<number, string> = {
    400: "E_BAD_REQUEST",
    404: "E_AGENT_NOT_FOUND",
    405: "E_METHOD_NOT_ALLOWED",
    409: "E_CONFLICT",
    500: "E_INTERNAL",
  };

  return new DaemonClientError(
    codeMap[status] ?? "E_UNKNOWN",
    message,
  );
}

export class HttpDaemonClient implements DaemonClient {
  async listAgents(_ctx: RequestContext): Promise<DaemonAgentState[]> {
    return request<DaemonAgentState[]>("GET", "/api/agents");
  }

  async installAgent(agentId: string, _ctx: RequestContext): Promise<void> {
    await request("POST", "/api/install", { agentId });
  }

  async startAgent(agentId: string, _ctx: RequestContext): Promise<void> {
    await request("POST", "/api/start", { agentId });
  }

  async stopAgent(agentId: string, _ctx: RequestContext): Promise<void> {
    await request("POST", "/api/stop", { agentId });
  }

  async getStatus(
    agentId: string | undefined,
    _ctx: RequestContext,
  ): Promise<DaemonAgentState[]> {
    if (!agentId) {
      return request<DaemonAgentState[]>("GET", "/api/agents");
    }
    const state = await request<DaemonAgentState>("GET", `/api/status/${agentId}`);
    return [state];
  }

  async getLogs(
    agentId: string,
    tail: number,
    _ctx: RequestContext,
  ): Promise<LogsResult> {
    const result = await request<{ lines: string[] }>(
      "GET",
      `/api/logs/${agentId}?tail=${tail}`,
    );
    return { lines: result.lines, truncated: false };
  }

  async upgradeAgent(agentId: string, _ctx: RequestContext): Promise<UpgradeResult> {
    return request<UpgradeResult>("POST", "/api/upgrade", { agentId });
  }

  async diagnoseAgent(agentId: string, _ctx: RequestContext): Promise<DiagnoseResult> {
    return request<DiagnoseResult>("POST", "/api/diagnose", { agentId });
  }

  async createRemoteDiagnosisHandoff(
    input: CreateRemoteDiagnosisHandoffInput,
  ): Promise<RemoteDiagnosisHandoff> {
    return request<RemoteDiagnosisHandoff>(
      "POST",
      "/api/remote-diagnosis-handoff",
      input,
    );
  }
}
