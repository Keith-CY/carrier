import type { GatewayCommand, GatewayResponse } from "./contracts/commands";
import { errorResponse, okResponse } from "./contracts/response";

export class ParseError extends Error {
  constructor(public readonly requestId: string, message: string) {
    super(message);
    this.name = "ParseError";
  }
}

export function parseInput(input: string): GatewayCommand {
  const parts = input.trim().split(/\s+/);
  const [provider, chatId, requestId, name, ...args] = parts;

  const base = {
    provider: provider as GatewayCommand["provider"],
    chatId,
    requestId,
    name: name as GatewayCommand["name"],
  };

  switch (name) {
    case "/pair":
      if (!args[0]) throw new ParseError(requestId, "usage: /pair <code>");
      return { ...base, name, data: { code: args[0] } };
    case "/agents":
      return {
        ...base,
        name,
        data: { filter: args[0], includeInstalled: args.includes("--installed") },
      };
    case "/install":
      if (!args[0]) throw new ParseError(requestId, "usage: /install <agent> [version]");
      return { ...base, name, data: { agent: args[0], version: args[1] } };
    case "/start":
      if (!args[0]) throw new ParseError(requestId, "usage: /start <agent> [profile]");
      return { ...base, name, data: { agent: args[0], profile: args[1] } };
    case "/stop":
      if (!args[0]) throw new ParseError(requestId, "usage: /stop <agent>");
      return { ...base, name, data: { agent: args[0] } };
    case "/status":
      return { ...base, name, data: { agent: args[0], verbose: args.includes("--verbose") } };
    case "/logs":
      if (!args[0]) throw new ParseError(requestId, "usage: /logs <agent> [tail]");
      return {
        ...base,
        name,
        data: {
          agent: args[0],
          tail: args[1] ? Number.parseInt(args[1], 10) : undefined,
        },
      };
    case "/upgrade":
      return { ...base, name, data: { agent: args[0], version: args[1] } };
    case "/diagnose":
      return {
        ...base,
        name,
        data: { scope: (args[0] as "gateway" | "agent" | "system" | undefined), agent: args[1] },
      };
    default:
      throw new ParseError(requestId, `unknown command: ${name}`);
  }
}

export function handleCommand(cmd: GatewayCommand): GatewayResponse {
  // Contract-only scaffold for Phase 1 routing.
  return okResponse(cmd.requestId, {
    checks: [
      {
        name: `${cmd.provider}:${cmd.name}`,
        status: "pass",
        detail: `accepted for ${cmd.chatId}`,
      },
    ],
  });
}

export function safeHandleCommand(input: string): GatewayResponse {
  try {
    const cmd = parseInput(input);
    return handleCommand(cmd);
  } catch (e) {
    if (e instanceof ParseError) {
      return errorResponse(e.requestId, "E_PARSE", e.message);
    }
    return errorResponse("unknown", "E_INTERNAL", e instanceof Error ? e.message : "unknown error");
  }
}

const example = "telegram 123 req-1 /agents";
const response = safeHandleCommand(example);
console.log(JSON.stringify(response, null, 2));
