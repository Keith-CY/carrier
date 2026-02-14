import type { GatewayCommand, GatewayResponse } from "./contracts/commands";
import {
  RemoteDiagnosisNotNeededError,
  type DaemonClient,
} from "./daemon/client";

export function parseInput(input: string): GatewayCommand {
  const parts = input.trim().split(/\s+/);
  const [provider, chatId, requestId, name, ...args] = parts;

  return {
    provider: provider as GatewayCommand["provider"],
    chatId,
    requestId,
    name: name as GatewayCommand["name"],
    args,
  };
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

export async function handleCommand(cmd: GatewayCommand, daemon: DaemonClient): Promise<GatewayResponse> {
  if (cmd.name !== "/diagnose-consent") {
    // Contract-only scaffold for other M4 command routing.
    return {
      requestId: cmd.requestId,
      result: "ok",
      message: `[${cmd.provider}] ${cmd.name} accepted for ${cmd.chatId}`,
    };
  }

  const [agentId, consentRaw] = cmd.args;
  if (!agentId) {
    return {
      requestId: cmd.requestId,
      result: "error",
      message: "usage: /diagnose-consent <agent_id> <yes|no>",
    };
  }

  const consent = parseConsentFlag(consentRaw);
  if (consent === null) {
    return {
      requestId: cmd.requestId,
      result: "error",
      message: "invalid consent flag: expected yes or no",
    };
  }

  try {
    const handoff = await daemon.createRemoteDiagnosisHandoff({
      agentId,
      consent,
      actor: `${cmd.provider}:${cmd.chatId}`,
      requestId: cmd.requestId,
    });

    return {
      requestId: cmd.requestId,
      result: "ok",
      message: `remote diagnosis consent recorded for ${agentId}`,
      handoffId: handoff.id,
      handoffStatus: handoff.status,
      downloadUrl: handoff.artifactRef || undefined,
    };
  } catch (error) {
    if (error instanceof RemoteDiagnosisNotNeededError) {
      return {
        requestId: cmd.requestId,
        result: "error",
        message: `${error.code}: ${error.message}`,
      };
    }
    return {
      requestId: cmd.requestId,
      result: "error",
      message: `failed to create remote diagnosis handoff: ${
        error instanceof Error ? error.message : "unknown error"
      }`,
    };
  }
}
