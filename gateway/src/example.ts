import { InMemoryDaemonClient } from "./daemon/client";
import { DownloadTokenStore } from "./downloads/token_store";
import { safeHandleCommand } from "./index";
import { SessionStore } from "./session/store";

const daemon = new InMemoryDaemonClient();
const sessions = new SessionStore();
const downloads = new DownloadTokenStore();

const pairCode = sessions.registerPairCode("pair-openclaw");
const commands = [
  `telegram 123 req-1 /pair ${pairCode.code}`,
  "telegram 123 req-2 /agents",
  "telegram 123 req-3 /install openclaw",
  "telegram 123 req-4 /start openclaw",
  "telegram 123 req-5 /diagnose openclaw",
];

for (const command of commands) {
  const response = await safeHandleCommand(command, {
    daemon,
    sessions,
    downloads,
  });
  console.log(JSON.stringify(response, null, 2));
}
