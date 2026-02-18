import type { GatewayCommand, GatewayResponse } from "../contracts/commands";
import type { DaemonClient, DaemonAgentState, RequestContext } from "../daemon/client";
import { DaemonClientError } from "../daemon/client";

/**
 * In-memory store for onboard session env vars.
 * Keyed by `${provider}:${chatId}`.
 */
export class OnboardStore {
  private envVars = new Map<string, Map<string, string>>();

  setEnv(key: string, envName: string, envValue: string): void {
    let vars = this.envVars.get(key);
    if (!vars) {
      vars = new Map();
      this.envVars.set(key, vars);
    }
    vars.set(envName, envValue);
  }

  getEnv(key: string): Map<string, string> {
    return this.envVars.get(key) ?? new Map();
  }

  clearEnv(key: string): void {
    this.envVars.delete(key);
  }
}

export type OnboardDependencies = {
  daemon: DaemonClient;
  onboardStore: OnboardStore;
};

export async function handleOnboardCommand(
  cmd: GatewayCommand,
  deps: OnboardDependencies,
): Promise<GatewayResponse> {
  const ctx: RequestContext = {
    actor: `${cmd.provider}:${cmd.chatId}`,
    requestId: cmd.requestId,
  };
  const sessionKey = `${cmd.provider}:${cmd.chatId}`;
  const [subcommand, ...subArgs] = cmd.args;

  try {
    if (!subcommand) {
      return await showWelcome(cmd.requestId, deps.daemon, ctx);
    }

    switch (subcommand) {
      case "env":
        return handleEnv(cmd.requestId, subArgs, sessionKey, deps.onboardStore);
      case "install":
        return await handleInstall(cmd.requestId, subArgs, sessionKey, deps);
      case "status":
        return await handleStatus(cmd.requestId, deps.daemon, ctx);
      default:
        return {
          requestId: cmd.requestId,
          result: "error",
          errorCode: "E_USAGE",
          message: "usage: /onboard [env KEY=VALUE | install <agent_id> | status]",
        };
    }
  } catch (error) {
    if (error instanceof DaemonClientError) {
      return {
        requestId: cmd.requestId,
        result: "error",
        errorCode: error.code,
        message: error.message,
      };
    }
    return {
      requestId: cmd.requestId,
      result: "error",
      errorCode: "E_COMMAND_FAILED",
      message: error instanceof Error ? error.message : "unknown error",
    };
  }
}

async function showWelcome(
  requestId: string,
  daemon: DaemonClient,
  ctx: RequestContext,
): Promise<GatewayResponse> {
  const agents = await daemon.listAgents(ctx);
  const lines = ["🚀 Welcome to Carrier! Let's set up your agents.", ""];
  lines.push("Available agents:");
  for (let i = 0; i < agents.length; i++) {
    const a = agents[i];
    const stateTag = a.installState === "installed" ? " [installed]" : "";
    lines.push(`${i + 1}. ${a.name} (${a.id})${stateTag}`);
  }
  lines.push("");
  lines.push("Reply with:");
  lines.push("/onboard install <agent_id> — install and start an agent");
  lines.push("/onboard env <KEY>=<VALUE> — set environment variables");
  lines.push("/onboard status — check agent states");

  return {
    requestId,
    result: "ok",
    message: lines.join("\n"),
  };
}

function handleEnv(
  requestId: string,
  args: string[],
  sessionKey: string,
  store: OnboardStore,
): GatewayResponse {
  const pair = args.join(" ");
  const eqIdx = pair.indexOf("=");
  if (eqIdx <= 0) {
    return {
      requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "usage: /onboard env <KEY>=<VALUE>",
    };
  }
  const envName = pair.slice(0, eqIdx).trim();
  const envValue = pair.slice(eqIdx + 1).trim();
  if (!envName || !envValue) {
    return {
      requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "usage: /onboard env <KEY>=<VALUE>",
    };
  }
  store.setEnv(sessionKey, envName, envValue);
  return {
    requestId,
    result: "ok",
    message: `${envName} set. Ready to install agents.`,
  };
}

async function handleInstall(
  requestId: string,
  args: string[],
  sessionKey: string,
  deps: OnboardDependencies,
): Promise<GatewayResponse> {
  const [agentId] = args;
  if (!agentId) {
    return {
      requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "usage: /onboard install <agent_id>",
    };
  }

  const ctx: RequestContext = {
    actor: sessionKey,
    requestId,
  };

  // Install the agent
  await deps.daemon.installAgent(agentId, ctx);

  // Start the agent
  try {
    await deps.daemon.startAgent(agentId, ctx);
  } catch (error) {
    // If start fails, still report install success
    if (error instanceof DaemonClientError) {
      return {
        requestId,
        result: "ok",
        message: `${agentId} installed but failed to start: ${error.message}`,
      };
    }
    throw error;
  }

  // Get status to report health
  const statuses = await deps.daemon.getStatus(agentId, ctx);
  const status = statuses[0];
  const health = status ? status.health : "unknown";

  return {
    requestId,
    result: "ok",
    message: `${agentId} installed and running (${health})`,
  };
}

async function handleStatus(
  requestId: string,
  daemon: DaemonClient,
  ctx: RequestContext,
): Promise<GatewayResponse> {
  const statuses = await daemon.getStatus(undefined, ctx);
  if (statuses.length === 0) {
    return {
      requestId,
      result: "ok",
      message: "No agents configured.",
    };
  }

  const lines = ["Agent status:"];
  for (const s of statuses) {
    const emoji = s.health === "healthy" ? "🟢" : s.runtimeState === "stopped" ? "⚪" : "🔴";
    lines.push(`${emoji} ${s.name} (${s.id}): ${s.installState}, ${s.runtimeState}, health=${s.health}`);
  }

  return {
    requestId,
    result: "ok",
    message: lines.join("\n"),
  };
}
