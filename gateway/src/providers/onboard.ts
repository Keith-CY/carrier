import type { GatewayCommand, GatewayResponse } from "../contracts/commands";
import type { DaemonClient, DaemonAgentState, RequestContext } from "../daemon/client";
import { DaemonClientError } from "../daemon/client";

/**
 * Session states for the interactive onboard flow.
 */
export type OnboardStep = "idle" | "agent_selected" | "env_configured" | "installing" | "done";

export type OnboardSession = {
  step: OnboardStep;
  selectedAgent?: string;
  envVars: Map<string, string>;
};

/**
 * In-memory store for onboard session state.
 * Keyed by `${provider}:${chatId}`.
 */
export class OnboardStore {
  private sessions = new Map<string, OnboardSession>();

  getSession(key: string): OnboardSession | undefined {
    return this.sessions.get(key);
  }

  startSession(key: string): OnboardSession {
    const session: OnboardSession = { step: "idle", envVars: new Map() };
    this.sessions.set(key, session);
    return session;
  }

  updateSession(key: string, update: Partial<OnboardSession>): OnboardSession {
    const session = this.sessions.get(key);
    if (!session) {
      throw new Error("no active session");
    }
    Object.assign(session, update);
    return session;
  }

  clearSession(key: string): void {
    this.sessions.delete(key);
  }

  hasActiveSession(key: string): boolean {
    const session = this.sessions.get(key);
    return !!session && session.step !== "idle" && session.step !== "done";
  }

  // Legacy compat helpers
  setEnv(key: string, envName: string, envValue: string): void {
    let session = this.sessions.get(key);
    if (!session) {
      session = this.startSession(key);
    }
    session.envVars.set(envName, envValue);
  }

  getEnv(key: string): Map<string, string> {
    return this.sessions.get(key)?.envVars ?? new Map();
  }

  clearEnv(key: string): void {
    const session = this.sessions.get(key);
    if (session) {
      session.envVars = new Map();
    }
  }
}

export type OnboardDependencies = {
  daemon: DaemonClient;
  onboardStore: OnboardStore;
};

/**
 * Handle `/onboard` as an interactive step-by-step flow.
 *
 * - `/onboard` with no args: starts/continues the interactive session
 * - `/onboard status`: standalone status check (works anytime)
 * - `/onboard cancel`: abort the current session
 * - `/onboard <text>`: routed as user reply in active session
 */
export async function handleOnboardCommand(
  cmd: GatewayCommand,
  deps: OnboardDependencies,
): Promise<GatewayResponse> {
  const ctx: RequestContext = {
    actor: `${cmd.provider}:${cmd.chatId}`,
    requestId: cmd.requestId,
  };
  const sessionKey = `${cmd.provider}:${cmd.chatId}`;
  const [firstArg, ...restArgs] = cmd.args;

  try {
    // `/onboard status` always works regardless of session state
    if (firstArg === "status") {
      return await handleStatus(cmd.requestId, deps.daemon, ctx);
    }

    // `/onboard cancel` aborts an active session
    if (firstArg === "cancel") {
      return handleCancel(cmd.requestId, sessionKey, deps.onboardStore);
    }

    // No args → start a new session (show welcome, transition to awaiting agent selection)
    if (!firstArg) {
      return await startInteractiveSession(cmd.requestId, sessionKey, deps);
    }

    // If there's an active session, route the input as a reply
    const session = deps.onboardStore.getSession(sessionKey);
    if (session && session.step !== "idle" && session.step !== "done") {
      return await handleSessionReply(cmd.requestId, sessionKey, cmd.args, deps, ctx);
    }

    // No active session and unrecognized arg → treat as agent selection shortcut
    // Start session and immediately select the agent
    deps.onboardStore.startSession(sessionKey);
    deps.onboardStore.updateSession(sessionKey, { step: "idle" });
    return await handleSessionReply(cmd.requestId, sessionKey, cmd.args, deps, ctx);
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

async function startInteractiveSession(
  requestId: string,
  sessionKey: string,
  deps: OnboardDependencies,
): Promise<GatewayResponse> {
  const ctx: RequestContext = { actor: sessionKey, requestId };
  const agents = await deps.daemon.listAgents(ctx);

  // Start a fresh session
  deps.onboardStore.startSession(sessionKey);

  const lines = ["🚀 Welcome to Carrier! Let's set up your agents.", ""];
  lines.push("Available agents:");
  for (let i = 0; i < agents.length; i++) {
    const a = agents[i];
    const stateTag = a.installState === "installed" ? " [installed]" : "";
    lines.push(`${i + 1}. ${a.name} (${a.id})${stateTag}`);
  }
  lines.push("");
  lines.push("Reply with the agent name to install (e.g. `/onboard openclaw`), or `/onboard cancel` to abort.");

  return {
    requestId,
    result: "ok",
    message: lines.join("\n"),
  };
}

async function handleSessionReply(
  requestId: string,
  sessionKey: string,
  args: string[],
  deps: OnboardDependencies,
  ctx: RequestContext,
): Promise<GatewayResponse> {
  const session = deps.onboardStore.getSession(sessionKey);
  if (!session) {
    // No session, start one
    return await startInteractiveSession(requestId, sessionKey, deps);
  }

  const input = args.join(" ").trim();

  switch (session.step) {
    case "idle":
      // User is selecting an agent
      return await handleAgentSelection(requestId, sessionKey, input, deps, ctx);

    case "agent_selected":
      // User is providing env vars
      return handleEnvInput(requestId, sessionKey, input, deps.onboardStore);

    case "env_configured":
      // User is confirming install
      return await handleConfirmation(requestId, sessionKey, input, deps, ctx);

    case "installing":
      return {
        requestId,
        result: "ok",
        message: "⏳ Installation is in progress. Please wait...",
      };

    case "done":
      return {
        requestId,
        result: "ok",
        message: "✅ Onboarding is complete! Run `/onboard` to set up another agent, or `/onboard status` to check.",
      };

    default:
      return {
        requestId,
        result: "error",
        errorCode: "E_USAGE",
        message: "Unexpected state. Run `/onboard` to start over.",
      };
  }
}

async function handleAgentSelection(
  requestId: string,
  sessionKey: string,
  agentId: string,
  deps: OnboardDependencies,
  ctx: RequestContext,
): Promise<GatewayResponse> {
  if (!agentId) {
    return {
      requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "Please provide an agent name. Run `/onboard` to see available agents.",
    };
  }

  // Verify the agent exists
  const agents = await deps.daemon.listAgents(ctx);
  const agent = agents.find((a) => a.id === agentId);
  if (!agent) {
    return {
      requestId,
      result: "error",
      errorCode: "E_AGENT_NOT_FOUND",
      message: `Agent "${agentId}" not found. Run \`/onboard\` to see available agents.`,
    };
  }

  deps.onboardStore.updateSession(sessionKey, {
    step: "agent_selected",
    selectedAgent: agentId,
  });

  const lines = [
    `Selected agent: **${agent.name}** (${agent.id})`,
    "",
    "Provide any environment variables as KEY=VALUE pairs (one per message).",
    "When done, reply with `/onboard done`.",
    "To skip env vars, reply `/onboard done` now.",
  ];

  return {
    requestId,
    result: "ok",
    message: lines.join("\n"),
  };
}

function handleEnvInput(
  requestId: string,
  sessionKey: string,
  input: string,
  store: OnboardStore,
): GatewayResponse {
  // "done" transitions to confirmation
  if (input.toLowerCase() === "done") {
    store.updateSession(sessionKey, { step: "env_configured" });
    const session = store.getSession(sessionKey)!;
    const envCount = session.envVars.size;
    const envSummary = envCount > 0
      ? `\nEnvironment variables: ${[...session.envVars.keys()].join(", ")}`
      : "\nNo environment variables set.";

    const lines = [
      `Ready to install **${session.selectedAgent}**?${envSummary}`,
      "",
      "Reply `/onboard yes` to proceed or `/onboard no` to go back.",
    ];

    return {
      requestId,
      result: "ok",
      message: lines.join("\n"),
    };
  }

  // Parse KEY=VALUE
  const eqIdx = input.indexOf("=");
  if (eqIdx <= 0) {
    return {
      requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: 'Please provide env vars as KEY=VALUE, or reply `/onboard done` to continue.',
    };
  }

  const envName = input.slice(0, eqIdx).trim();
  const envValue = input.slice(eqIdx + 1).trim();
  if (!envName || !envValue) {
    return {
      requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: 'Please provide env vars as KEY=VALUE, or reply `/onboard done` to continue.',
    };
  }

  store.setEnv(sessionKey, envName, envValue);
  return {
    requestId,
    result: "ok",
    message: `✅ ${envName} set. Add more variables or reply \`/onboard done\` to continue.`,
  };
}

async function handleConfirmation(
  requestId: string,
  sessionKey: string,
  input: string,
  deps: OnboardDependencies,
  ctx: RequestContext,
): Promise<GatewayResponse> {
  const normalized = input.toLowerCase().trim();

  if (normalized === "no" || normalized === "back") {
    deps.onboardStore.updateSession(sessionKey, { step: "agent_selected" });
    return {
      requestId,
      result: "ok",
      message: "Going back. Provide env vars as KEY=VALUE, or reply `/onboard done` to continue.",
    };
  }

  if (normalized !== "yes" && normalized !== "y") {
    return {
      requestId,
      result: "error",
      errorCode: "E_USAGE",
      message: "Reply `/onboard yes` to install or `/onboard no` to go back.",
    };
  }

  const session = deps.onboardStore.getSession(sessionKey)!;
  const agentId = session.selectedAgent!;

  deps.onboardStore.updateSession(sessionKey, { step: "installing" });

  // Install the agent
  await deps.daemon.installAgent(agentId, ctx);

  // Start the agent
  try {
    await deps.daemon.startAgent(agentId, ctx);
  } catch (error) {
    deps.onboardStore.updateSession(sessionKey, { step: "done" });
    if (error instanceof DaemonClientError) {
      return {
        requestId,
        result: "ok",
        message: `${agentId} installed but failed to start: ${error.message}`,
      };
    }
    throw error;
  }

  // Get status
  const statuses = await deps.daemon.getStatus(agentId, ctx);
  const status = statuses[0];
  const health = status ? status.health : "unknown";

  deps.onboardStore.updateSession(sessionKey, { step: "done" });

  return {
    requestId,
    result: "ok",
    message: `🎉 ${agentId} installed and running (${health}). Onboarding complete!`,
  };
}

function handleCancel(
  requestId: string,
  sessionKey: string,
  store: OnboardStore,
): GatewayResponse {
  const session = store.getSession(sessionKey);
  if (!session || session.step === "idle" || session.step === "done") {
    return {
      requestId,
      result: "ok",
      message: "No active onboarding session to cancel.",
    };
  }

  store.clearSession(sessionKey);
  return {
    requestId,
    result: "ok",
    message: "🚫 Onboarding cancelled. Run `/onboard` to start again.",
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
