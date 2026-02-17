import type { DaemonClient, DaemonAgentState, RequestContext } from "../daemon/client";

export type ProviderType = "telegram" | "discord" | "feishu" | "dummy";

export type OnboardRequest = {
  provider?: string;
  provider_token?: string;
  provider_webhook_secret?: string;
  env?: Record<string, string>;
};

export type OnboardInstallRequest = {
  agents?: string[];
  instance_name?: string;
};

export type AgentRequirements = {
  agent_id: string;
  name: string;
  description: string;
  capabilities: string[];
  env: {
    required: EnvVarInfo[];
    optional: EnvVarInfo[];
  };
};

export type EnvVarInfo = {
  name: string;
  secret?: boolean;
  default?: string;
  description?: string;
};

export type OnboardAgentInfo = {
  id: string;
  name: string;
  description: string;
  ready: boolean;
  missing_env: string[];
  capabilities: string[];
};

export type OnboardState = {
  provider: ProviderType | null;
  provider_configured: boolean;
  env: Record<string, string>;
  configured_at: string | null;
};

export type OnboardInstallResult = {
  agent_id: string;
  instance_id: string;
  install: "ok" | "error";
  start: "ok" | "error" | "skipped";
  status: string;
  error?: string;
};

const VALID_PROVIDERS: ProviderType[] = ["telegram", "discord", "feishu", "dummy"];

export class OnboardStore {
  private state: OnboardState = {
    provider: null,
    provider_configured: false,
    env: {},
    configured_at: null,
  };

  getState(): OnboardState {
    return { ...this.state, env: { ...this.state.env } };
  }

  configure(provider: ProviderType, token?: string, env?: Record<string, string>): void {
    this.state.provider = provider;
    this.state.provider_configured = !!token;
    if (env) {
      this.state.env = { ...this.state.env, ...env };
    }
    this.state.configured_at = new Date().toISOString();
  }

  getEnv(): Record<string, string> {
    return { ...this.state.env };
  }
}

export function validateProvider(provider: string): provider is ProviderType {
  return VALID_PROVIDERS.includes(provider as ProviderType);
}

export async function fetchAgentRequirements(
  daemon: DaemonClient,
  agentId: string,
  ctx: RequestContext,
): Promise<AgentRequirements | null> {
  try {
    const resp = await (daemon as any).getAgentRequirements(agentId, ctx);
    return resp as AgentRequirements;
  } catch {
    return null;
  }
}

export function computeReadiness(
  agents: DaemonAgentState[],
  requirements: Map<string, AgentRequirements>,
  providedEnv: Record<string, string>,
): OnboardAgentInfo[] {
  return agents.map((agent) => {
    const reqs = requirements.get(agent.id);
    const requiredVars = reqs?.env?.required ?? [];
    const missing = requiredVars
      .filter((v) => !providedEnv[v.name])
      .map((v) => v.name);
    return {
      id: agent.id,
      name: agent.name,
      description: reqs?.description ?? "",
      ready: missing.length === 0,
      missing_env: missing,
      capabilities: reqs?.capabilities ?? [],
    };
  });
}
