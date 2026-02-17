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
  | "/diagnose-consent"
  | "/onboard";

export type GatewayCommand = {
  provider: Provider;
  chatId: string;
  requestId: string;
  sessionToken?: string;
  name: CommandName;
  args: string[];
};

export type GatewayResponse = {
  requestId: string;
  result: "ok" | "error";
  message: string;
  errorCode?: string;
  sessionToken?: string;
  downloadUrl?: string;
  handoffId?: string;
  handoffStatus?: "pending" | "declined";
};
