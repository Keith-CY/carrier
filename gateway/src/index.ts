import type {
  CommandName,
  GatewayCommand,
  GatewayResponse,
} from "./contracts/commands";
import {
  DaemonClientError,
  RemoteDiagnosisNotNeededError,
  type DaemonClient,
  type RequestContext,
} from "./daemon/client";
import { DownloadTokenStore } from "./downloads/token_store";
import { RateLimiter } from "./ratelimit";
import { SessionStore } from "./session/store";

const COMMAND_NAMES: ReadonlySet<CommandName> = new Set([
  "/pair",
  "/agents",
  "/install",
  "/start",
  "/stop",
  "/status",
  "/logs",
  "/upgrade",
  "/diagnose",
  "/diagnose-consent",
]);

export type GatewayDependencies = {
  daemon: DaemonClient;
  sessions: SessionStore;
  downloads: DownloadTokenStore;
  rateLimiter?: RateLimiter;
};

export class ParseError extends Error {
  constructor(
    readonly requestId: string,
    message: string,
  ) {
    super(message);
    this.name = "ParseError";
  }
}

export function parseInput(input: string): GatewayCommand {
  const parts = input.trim().split(/\s+/);
  if (parts.length < 4) {
    throw new ParseError("unknown", "usage: <provider> <chat_id> <request_id> <command> [...args]");
  }

  const [provider, chatId, requestId, name, ...args] = parts;
  if (!COMMAND_NAMES.has(name as CommandName)) {
    throw new ParseError(requestId, `unknown command: ${name} (requestId=${requestId})`);
  }

  return {
    provider: provider as GatewayCommand["provider"],
    chatId,
    requestId,
    name: name as GatewayCommand["name"],
    args,
  };
}

export async function handleCommand(
  cmd: GatewayCommand,
  deps: GatewayDependencies,
): Promise<GatewayResponse> {
  if (cmd.name === "/pair") {
    const [code] = cmd.args;
    if (!code) {
      return usageError(cmd.requestId, "/pair <code>");
    }
    const session = deps.sessions.pair({
      provider: cmd.provider,
      chatId: cmd.chatId,
      code,
    });
    if (!session) {
      return errorResponse(cmd.requestId, "E_PAIR_CODE_INVALID", "pairing code is invalid or expired");
    }
    return {
      requestId: cmd.requestId,
      result: "ok",
      message: `paired ${cmd.provider}:${cmd.chatId}`,
      sessionToken: session.sessionToken,
    };
  }

  const session = deps.sessions.getSession(cmd.provider, cmd.chatId);
  if (!session) {
    return errorResponse(
      cmd.requestId,
      "E_SESSION_REQUIRED",
      "chat is not paired; run /pair <code> first",
    );
  }
  deps.sessions.touch(cmd.provider, cmd.chatId);

  if (deps.rateLimiter) {
    const sessionKey = `${cmd.provider}:${cmd.chatId}`;
    const rl = deps.rateLimiter.check(sessionKey);
    if (!rl.allowed) {
      return errorResponse(cmd.requestId, rl.errorCode ?? "E_RATE_LIMITED", rl.message ?? "rate limit exceeded");
    }
  }

  const ctx = requestContext(cmd);
  try {
    switch (cmd.name) {
      case "/agents": {
        const agents = await deps.daemon.listAgents(ctx);
        const installed = agents.filter((agent) => agent.installed).length;
        return {
          requestId: cmd.requestId,
          result: "ok",
          message: `listed ${agents.length} agents (${installed} installed)`,
        };
      }
      case "/install": {
        const [agentId] = cmd.args;
        if (!agentId) {
          return usageError(cmd.requestId, "/install <agent_id>");
        }
        await deps.daemon.installAgent(agentId, ctx);
        return {
          requestId: cmd.requestId,
          result: "ok",
          message: `install completed for ${agentId}`,
        };
      }
      case "/start": {
        const [agentId] = cmd.args;
        if (!agentId) {
          return usageError(cmd.requestId, "/start <agent_id>");
        }
        await deps.daemon.startAgent(agentId, ctx);
        return {
          requestId: cmd.requestId,
          result: "ok",
          message: `start completed for ${agentId}`,
        };
      }
      case "/stop": {
        const [agentId] = cmd.args;
        if (!agentId) {
          return usageError(cmd.requestId, "/stop <agent_id>");
        }
        await deps.daemon.stopAgent(agentId, ctx);
        return {
          requestId: cmd.requestId,
          result: "ok",
          message: `stop completed for ${agentId}`,
        };
      }
      case "/status": {
        const [agentId] = cmd.args;
        const statuses = await deps.daemon.getStatus(agentId, ctx);
        if (statuses.length === 0) {
          return {
            requestId: cmd.requestId,
            result: "ok",
            message: "no agent status available",
          };
        }
        const summary = statuses
          .map((status) => `${status.id}:${status.runtimeState}/${status.health}`)
          .join(", ");
        return {
          requestId: cmd.requestId,
          result: "ok",
          message: `status ${summary}`,
        };
      }
      case "/logs": {
        const [agentId, tailRaw] = cmd.args;
        if (!agentId) {
          return usageError(cmd.requestId, "/logs <agent_id> [tail]");
        }
        const tail = parsePositiveInt(tailRaw, 200);
        const logs = await deps.daemon.getLogs(agentId, tail, ctx);
        const message = logs.lines.length === 0
          ? `no logs for ${agentId}`
          : `returned ${logs.lines.length} log lines for ${agentId}`;
        const response: GatewayResponse = {
          requestId: cmd.requestId,
          result: "ok",
          message,
        };
        if (logs.truncated || logs.lines.length > 50) {
          const token = deps.downloads.issue(`/tmp/${agentId}-logs-${cmd.requestId}.txt`);
          response.downloadUrl = deps.downloads.toDownloadURL(token);
        }
        return response;
      }
      case "/upgrade": {
        const [agentId] = cmd.args;
        if (!agentId) {
          return usageError(cmd.requestId, "/upgrade <agent_id>");
        }
        const result = await deps.daemon.upgradeAgent(agentId, ctx);
        let message = `upgrade completed for ${result.agentId}: ${result.fromVersion} -> ${result.toVersion}`;
        if (result.backupPath) {
          message += `. backup at ${result.backupPath}`;
        }
        if (result.rollbackHint) {
          message += `. rollback: ${result.rollbackHint}`;
        }
        return {
          requestId: cmd.requestId,
          result: "ok",
          message,
        };
      }
      case "/diagnose": {
        const [agentId] = cmd.args;
        if (!agentId) {
          return usageError(cmd.requestId, "/diagnose <agent_id>");
        }
        const result = await deps.daemon.diagnoseAgent(agentId, ctx);
        const token = deps.downloads.issue(result.artifactRef);
        return {
          requestId: cmd.requestId,
          result: "ok",
          message: `diagnose artifact prepared for ${agentId}`,
          downloadUrl: deps.downloads.toDownloadURL(token),
        };
      }
      case "/diagnose-consent": {
        const [agentId, consentRaw] = cmd.args;
        if (!agentId) {
          return usageError(cmd.requestId, "/diagnose-consent <agent_id> <yes|no>");
        }
        const consent = parseConsentFlag(consentRaw);
        if (consent === null) {
          return errorResponse(cmd.requestId, "E_CONSENT_FLAG_INVALID", "expected yes or no");
        }
        const handoff = await deps.daemon.createRemoteDiagnosisHandoff({
          agentId,
          consent,
          actor: ctx.actor,
          requestId: ctx.requestId,
        });
        const response: GatewayResponse = {
          requestId: cmd.requestId,
          result: "ok",
          message: `remote diagnosis consent recorded for ${agentId}`,
          handoffId: handoff.id,
          handoffStatus: handoff.status,
        };
        if (handoff.artifactRef) {
          const token = deps.downloads.issue(handoff.artifactRef);
          response.downloadUrl = deps.downloads.toDownloadURL(token);
        }
        return response;
      }
      default:
        return errorResponse(cmd.requestId, "E_COMMAND_UNSUPPORTED", `unsupported command: ${cmd.name}`);
    }
  } catch (error) {
    if (error instanceof RemoteDiagnosisNotNeededError) {
      return errorResponse(cmd.requestId, error.code, error.message);
    }
    if (error instanceof DaemonClientError) {
      return errorResponse(cmd.requestId, error.code, error.message);
    }
    return errorResponse(
      cmd.requestId,
      "E_COMMAND_FAILED",
      error instanceof Error ? error.message : "unknown error",
    );
  }
}

export async function safeHandleCommand(
  input: string,
  deps: GatewayDependencies,
): Promise<GatewayResponse> {
  try {
    const cmd = parseInput(input);
    return await handleCommand(cmd, deps);
  } catch (error) {
    if (error instanceof ParseError) {
      return errorResponse(error.requestId, "E_PARSE", error.message);
    }
    return errorResponse(
      "unknown",
      "E_PARSE",
      error instanceof Error ? error.message : "invalid command input",
    );
  }
}

function parseConsentFlag(value: string | undefined): boolean | null {
  if (!value) {
    return null;
  }
  const normalized = value.toLowerCase();
  if (["yes", "y", "true"].includes(normalized)) {
    return true;
  }
  if (["no", "n", "false"].includes(normalized)) {
    return false;
  }
  return null;
}

function parsePositiveInt(value: string | undefined, fallback: number): number {
  if (!value) {
    return fallback;
  }
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function requestContext(cmd: GatewayCommand): RequestContext {
  return {
    actor: `${cmd.provider}:${cmd.chatId}`,
    requestId: cmd.requestId,
  };
}

function usageError(requestId: string, usage: string): GatewayResponse {
  return errorResponse(requestId, "E_USAGE", `usage: ${usage}`);
}

function errorResponse(requestId: string, errorCode: string, message: string): GatewayResponse {
  return {
    requestId,
    result: "error",
    errorCode,
    message,
  };
}
