import type { ResponseEnvelope } from "./response";

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
  | "/diagnose";

export type BaseCommand<Name extends CommandName, Data> = {
  provider: Provider;
  chatId: string;
  requestId: string;
  name: Name;
  data: Data;
};

export type PairCommandData = {
  code: string;
};

export type AgentsCommandData = {
  filter?: string;
  includeInstalled?: boolean;
};

export type InstallCommandData = {
  agent: string;
  version?: string;
  source?: "catalog" | "github" | "local";
};

export type StartCommandData = {
  agent: string;
  profile?: string;
};

export type StopCommandData = {
  agent: string;
};

export type StatusCommandData = {
  agent?: string;
  verbose?: boolean;
};

export type LogsCommandData = {
  agent: string;
  tail?: number;
  since?: string;
};

export type UpgradeCommandData = {
  agent?: string;
  version?: string;
};

export type DiagnoseCommandData = {
  scope?: "gateway" | "agent" | "system";
  agent?: string;
};

export type PairCommand = BaseCommand<"/pair", PairCommandData>;
export type AgentsCommand = BaseCommand<"/agents", AgentsCommandData>;
export type InstallCommand = BaseCommand<"/install", InstallCommandData>;
export type StartCommand = BaseCommand<"/start", StartCommandData>;
export type StopCommand = BaseCommand<"/stop", StopCommandData>;
export type StatusCommand = BaseCommand<"/status", StatusCommandData>;
export type LogsCommand = BaseCommand<"/logs", LogsCommandData>;
export type UpgradeCommand = BaseCommand<"/upgrade", UpgradeCommandData>;
export type DiagnoseCommand = BaseCommand<"/diagnose", DiagnoseCommandData>;

export type GatewayCommand =
  | PairCommand
  | AgentsCommand
  | InstallCommand
  | StartCommand
  | StopCommand
  | StatusCommand
  | LogsCommand
  | UpgradeCommand
  | DiagnoseCommand;

export type PairResponseData = {
  paired: boolean;
  sessionToken?: string;
};

export type AgentRecord = {
  name: string;
  version: string;
  installed: boolean;
  running: boolean;
};

export type AgentsResponseData = {
  agents: AgentRecord[];
};

export type InstallResponseData = {
  agent: string;
  version: string;
  installed: boolean;
};

export type StartResponseData = {
  agent: string;
  started: boolean;
  pid?: number;
};

export type StopResponseData = {
  agent: string;
  stopped: boolean;
};

export type StatusResponseData = {
  statuses: Array<{
    agent: string;
    state: "running" | "stopped" | "error" | "unknown";
    updatedAt: string;
  }>;
};

export type LogsResponseData = {
  agent: string;
  lines: string[];
  truncated?: boolean;
};

export type UpgradeResponseData = {
  upgraded: Array<{
    agent: string;
    fromVersion: string;
    toVersion: string;
  }>;
};

export type DiagnoseResponseData = {
  checks: Array<{
    name: string;
    status: "pass" | "warn" | "fail";
    detail?: string;
  }>;
};

export type GatewayResponseData =
  | PairResponseData
  | AgentsResponseData
  | InstallResponseData
  | StartResponseData
  | StopResponseData
  | StatusResponseData
  | LogsResponseData
  | UpgradeResponseData
  | DiagnoseResponseData;

export type GatewayResponse = ResponseEnvelope<GatewayResponseData>;
