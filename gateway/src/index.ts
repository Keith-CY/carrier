import type { GatewayCommand, GatewayResponse } from "./contracts/commands";

function parseInput(input: string): GatewayCommand {
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

function handleCommand(cmd: GatewayCommand): GatewayResponse {
  // Contract-only scaffold for Phase 1 routing.
  return {
    requestId: cmd.requestId,
    result: "ok",
    message: `[${cmd.provider}] ${cmd.name} accepted for ${cmd.chatId}`,
  };
}

const example = "telegram 123 req-1 /agents";
const response = handleCommand(parseInput(example));
console.log(JSON.stringify(response, null, 2));
