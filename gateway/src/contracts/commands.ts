export type Provider = "telegram" | "discord" | "feishu";

export type CommandName =
  | "/pair"
  | "/agents"
  | "/install"
  | "/start"
  | "/stop"
  | "/status"
  | "/logs"
  | "/upgrade"
  | "/diagnose"
  | "/diagnose-consent";

export type GatewayCommand = {
  provider: Provider;
  chatId: string;
  requestId: string;
  name: CommandName;
  args: string[];
};

export type GatewayResponse = {
  requestId: string;
  result: "ok" | "error";
  message: string;
  downloadUrl?: string;
  handoffId?: string;
  handoffStatus?: "pending" | "declined";
};
