import type { Provider } from "./commands";

export type PairRequest = {
  provider: Provider;
  chatId: string;
  code: string;
};

export type SessionRecord = {
  provider: Provider;
  chatId: string;
  sessionToken: string;
  createdAt: string;
  lastSeenAt: string;
};

export type ReadOnlyDownloadToken = {
  token: string;
  fileRef: string;
  expiresAt: string;
  singleUse: boolean;
};
