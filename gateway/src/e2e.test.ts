import { describe, test, expect, beforeEach } from "bun:test";
import { createGatewayRuntime, type GatewayRuntime } from "./server";
import { InMemoryDaemonClient } from "./daemon/client";
import { SessionStore } from "./session/store";
import type { GatewayResponse } from "./contracts/commands";

async function sendCommand(runtime: GatewayRuntime, input: string, sessionToken?: string | null): Promise<GatewayResponse> {
  const headers: Record<string, string> = { "content-type": "application/json" };
  
  // Auto-detect session token from SessionStore if not explicitly provided
  if (sessionToken === undefined) {
    const parts = input.trim().split(/\s+/);
    if (parts.length >= 2) {
      const [provider, chatId] = parts;
      const session = runtime.deps.sessions.getSession(provider as any, chatId);
      if (session) {
        sessionToken = session.sessionToken;
      }
    }
  }
  
  if (sessionToken) {
    headers["authorization"] = `Bearer ${sessionToken}`;
  }
  
  const response = await runtime.fetch(new Request("http://localhost/command", {
    method: "POST",
    headers,
    body: JSON.stringify({ input }),
  }));
  return await response.json() as GatewayResponse;
}

/** Issue a pair code known to both daemon and session store. */
function issuePairCode(daemon: InMemoryDaemonClient, sessions: SessionStore): string {
  const { code } = sessions.issuePairCode();
  daemon.registerPairCode(code);
  return code;
}

describe("E2E slash command test suite", () => {
  let runtime: GatewayRuntime;
  let daemon: InMemoryDaemonClient;
  let sessions: SessionStore;

  beforeEach(() => {
    daemon = new InMemoryDaemonClient();
    sessions = new SessionStore();
    runtime = createGatewayRuntime({
      deps: {
        daemon,
        sessions,
      },
    });
  });

  describe("1. Pairing", () => {
    test("/pair with valid code succeeds", async () => {
      const code = issuePairCode(daemon, sessions);
      const result = await sendCommand(runtime, `telegram chat1 req-1 /pair ${code}`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("paired telegram:chat1");
      expect(result.sessionToken).toBeTruthy();
    });

    test("/pair with invalid code fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-2 /pair invalid-code");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_PAIR_CODE_INVALID");
      expect(result.message).toContain("invalid or expired");
    });

    test("/pair with missing code returns usage error", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-3 /pair");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
      expect(result.message).toContain("/pair <code>");
    });

    test("/pair with expired code fails", async () => {
      let frozenTime = new Date("2026-01-01T12:00:00Z");
      const freezableStore = new SessionStore(() => frozenTime);
      const code = freezableStore.issuePairCode(60).code; // 60 second TTL
      
      // Advance time by 61 seconds
      frozenTime = new Date("2026-01-01T12:01:01Z");
      
      const runtimeWithFrozen = createGatewayRuntime({
        deps: {
          daemon,
          sessions: freezableStore,
        },
      });
      
      const result = await sendCommand(runtimeWithFrozen, `telegram chat1 req-4 /pair ${code}`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_PAIR_CODE_INVALID");
    });
  });

  describe("2. Agent lifecycle", () => {
    let token: string;
    
    beforeEach(async () => {
      // Pair the session first
      const code = issuePairCode(daemon, sessions);
      const pairResult = await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
      token = pairResult.sessionToken!;
    });

    test("/agents lists available agents", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-10 ${token} /agents`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("listed");
      expect(result.message).toContain("agents");
    });

    test("/install openclaw succeeds", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-11 ${token} /install openclaw`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("install completed for openclaw");
    });

    test("/start openclaw after install succeeds", async () => {
      await sendCommand(runtime, `telegram chat1 req-12a ${token} /install openclaw`);
      const result = await sendCommand(runtime, `telegram chat1 req-12 ${token} /start openclaw`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("start completed for openclaw");
    });

    test("/start without install fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-13 ${token} /start openclaw`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_NOT_INSTALLED");
    });

    test("/status shows agent state", async () => {
      await sendCommand(runtime, `telegram chat1 req-14a ${token} /install openclaw`);
      await sendCommand(runtime, `telegram chat1 req-14b ${token} /start openclaw`);
      const result = await sendCommand(runtime, `telegram chat1 req-14 ${token} /status openclaw`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("openclaw");
      expect(result.message).toContain("running");
    });

    test("/status without agent ID lists all agents", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-15 ${token} /status`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("status");
    });

    test("/stop openclaw succeeds when running", async () => {
      await sendCommand(runtime, `telegram chat1 req-16a ${token} /install openclaw`);
      await sendCommand(runtime, `telegram chat1 req-16b ${token} /start openclaw`);
      const result = await sendCommand(runtime, `telegram chat1 req-16 ${token} /stop openclaw`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("stop completed for openclaw");
    });

    test("/stop when already stopped fails", async () => {
      await sendCommand(runtime, `telegram chat1 req-17a ${token} /install openclaw`);
      const result = await sendCommand(runtime, `telegram chat1 req-17 ${token} /stop openclaw`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_ALREADY_STOPPED");
    });

    test("/start when already running fails", async () => {
      await sendCommand(runtime, `telegram chat1 req-18a ${token} /install openclaw`);
      await sendCommand(runtime, `telegram chat1 req-18b ${token} /start openclaw`);
      const result = await sendCommand(runtime, `telegram chat1 req-18 ${token} /start openclaw`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_ALREADY_RUNNING");
    });
  });

  describe("3. Logs & diagnose", () => {
    let token: string;
    
    beforeEach(async () => {
      const code = issuePairCode(daemon, sessions);
      const pairResult = await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
      token = pairResult.sessionToken!;
      await sendCommand(runtime, `telegram chat1 req-setup2 ${token} /install openclaw`);
    });

    test("/logs openclaw returns log lines", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-20 ${token} /logs openclaw`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("openclaw");
    });

    test("/logs with tail parameter works", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-21 ${token} /logs openclaw 50`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("openclaw");
    });

    test("/logs without agent ID fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-22 ${token} /logs`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/diagnose openclaw creates artifact", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-23 ${token} /diagnose openclaw`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("diagnose artifact prepared");
      expect(result.downloadUrl).toBeTruthy();
    });

    test("/diagnose without agent ID fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-24 ${token} /diagnose`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/diagnose-consent yes when needed succeeds", async () => {
      daemon.setRemoteDiagnosisState("openclaw", true);
      await sendCommand(runtime, `telegram chat1 req-25a ${token} /diagnose openclaw`);
      
      const result = await sendCommand(runtime, `telegram chat1 req-25 ${token} /diagnose-consent openclaw yes`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("consent recorded");
      expect(result.handoffId).toBeTruthy();
      expect(result.handoffStatus).toBe("pending");
    });

    test("/diagnose-consent no when needed succeeds", async () => {
      daemon.setRemoteDiagnosisState("openclaw", true);
      await sendCommand(runtime, `telegram chat1 req-26a ${token} /diagnose openclaw`);
      
      const result = await sendCommand(runtime, `telegram chat1 req-26 ${token} /diagnose-consent openclaw no`);
      
      expect(result.result).toBe("ok");
      expect(result.handoffStatus).toBe("declined");
    });

    test("/diagnose-consent when not needed fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-27 ${token} /diagnose-consent openclaw yes`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_REMOTE_DIAG_NOT_NEEDED");
    });

    test("/diagnose-consent with invalid consent flag fails", async () => {
      daemon.setRemoteDiagnosisState("openclaw", true);
      const result = await sendCommand(runtime, `telegram chat1 req-28 ${token} /diagnose-consent openclaw maybe`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_CONSENT_FLAG_INVALID");
    });
  });

  describe("4. Upgrade", () => {
    let token: string;
    
    beforeEach(async () => {
      const code = issuePairCode(daemon, sessions);
      const pairResult = await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
      token = pairResult.sessionToken!;
      await sendCommand(runtime, `telegram chat1 req-setup2 ${token} /install openclaw`);
    });

    test("/upgrade openclaw succeeds", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-30 ${token} /upgrade openclaw`);
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("upgrade completed");
      expect(result.message).toContain("0.1.0");
      expect(result.message).toContain("0.1.1");
      expect(result.message).toContain("backup");
    });

    test("/upgrade without agent ID fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-31 ${token} /upgrade`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/upgrade non-existent agent fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-32 ${token} /upgrade nonexistent`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_AGENT_NOT_FOUND");
    });
  });

  describe("5. Error cases", () => {
    test("unknown command fails", async () => {
      const code = issuePairCode(daemon, sessions);
      const pairResult = await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
      const token = pairResult.sessionToken!;
      
      const result = await sendCommand(runtime, `telegram chat1 req-40 ${token} /unknown`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_PARSE");
      expect(result.message).toContain("unknown command");
    });

    test("command without session fails", async () => {
      const result = await sendCommand(runtime, "telegram chat-unpaired req-41 /agents");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_SESSION_REQUIRED");
      expect(result.message).toContain("not paired");
    });

    test("malformed input (too few parts) fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_PARSE");
      expect(result.message).toContain("usage");
    });

    test("malformed JSON body fails", async () => {
      const response = await runtime.fetch(new Request("http://localhost/command", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: "{invalid json}",
      }));
      const result = await response.json() as GatewayResponse;
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("empty input fails", async () => {
      const response = await runtime.fetch(new Request("http://localhost/command", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ input: "" }),
      }));
      const result = await response.json() as GatewayResponse;
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("missing input field fails", async () => {
      const response = await runtime.fetch(new Request("http://localhost/command", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ notInput: "value" }),
      }));
      const result = await response.json() as GatewayResponse;
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });
  });

  describe("6. Multi-provider", () => {
    test("telegram provider works", async () => {
      const code = issuePairCode(daemon, sessions);
      const pairResult = await sendCommand(runtime, `telegram chat-tg req-50 /pair ${code}`);
      expect(pairResult.result).toBe("ok");
      const token = pairResult.sessionToken!;
      
      const agentsResult = await sendCommand(runtime, `telegram chat-tg req-51 ${token} /agents`);
      expect(agentsResult.result).toBe("ok");
    });

    test("discord provider works", async () => {
      const code = issuePairCode(daemon, sessions);
      const pairResult = await sendCommand(runtime, `discord chat-dc req-52 /pair ${code}`);
      expect(pairResult.result).toBe("ok");
      const token = pairResult.sessionToken!;
      
      const agentsResult = await sendCommand(runtime, `discord chat-dc req-53 ${token} /agents`);
      expect(agentsResult.result).toBe("ok");
    });

    test("feishu provider works", async () => {
      const code = issuePairCode(daemon, sessions);
      const pairResult = await sendCommand(runtime, `feishu chat-fs req-54 /pair ${code}`);
      expect(pairResult.result).toBe("ok");
      const token = pairResult.sessionToken!;
      
      const agentsResult = await sendCommand(runtime, `feishu chat-fs req-55 ${token} /agents`);
      expect(agentsResult.result).toBe("ok");
    });

    test("sessions are isolated by provider and chatId", async () => {
      const code1 = issuePairCode(daemon, sessions);
      const code2 = issuePairCode(daemon, sessions);
      
      const tgResult = await sendCommand(runtime, `telegram chat1 req-56 /pair ${code1}`);
      const dcResult = await sendCommand(runtime, `discord chat1 req-57 /pair ${code2}`);
      const tgToken = tgResult.sessionToken!;
      const dcToken = dcResult.sessionToken!;
      
      // telegram:chat1 should work
      const result1 = await sendCommand(runtime, `telegram chat1 req-58 ${tgToken} /agents`);
      expect(result1.result).toBe("ok");
      
      // discord:chat1 should work
      const result2 = await sendCommand(runtime, `discord chat1 req-59 ${dcToken} /agents`);
      expect(result2.result).toBe("ok");
      
      // telegram:chat2 should fail (not paired)
      const result3 = await sendCommand(runtime, "telegram chat2 req-60 /agents");
      expect(result3.result).toBe("error");
      expect(result3.errorCode).toBe("E_SESSION_REQUIRED");
    });

    test("all providers support full lifecycle", async () => {
      const providers = ["telegram", "discord", "feishu"] as const;
      
      for (const provider of providers) {
        const chatId = `chat-${provider}`;
        const reqIdBase = provider === "telegram" ? 70 : provider === "discord" ? 80 : 90;
        
        // Pair
        const code = issuePairCode(daemon, sessions);
        const pairResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase} /pair ${code}`);
        expect(pairResult.result).toBe("ok");
        const token = pairResult.sessionToken!;
        
        // Install
        const installResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase + 1} ${token} /install openclaw`);
        expect(installResult.result).toBe("ok");
        
        // Start
        const startResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase + 2} ${token} /start openclaw`);
        expect(startResult.result).toBe("ok");
        
        // Status
        const statusResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase + 3} ${token} /status openclaw`);
        expect(statusResult.result).toBe("ok");
        expect(statusResult.message).toContain("running");
        
        // Stop
        const stopResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase + 4} ${token} /stop openclaw`);
        expect(stopResult.result).toBe("ok");
      }
    });
  });

  describe("Additional edge cases", () => {
    let token: string;
    
    beforeEach(async () => {
      const code = issuePairCode(daemon, sessions);
      const pairResult = await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
      token = pairResult.sessionToken!;
    });

    test("/install with missing agent ID fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-100 ${token} /install`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/start with missing agent ID fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-101 ${token} /start`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/stop with missing agent ID fails", async () => {
      const result = await sendCommand(runtime, `telegram chat1 req-102 ${token} /stop`);
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("healthz endpoint works", async () => {
      const response = await runtime.fetch(new Request("http://localhost/healthz"));
      const result = await response.json() as { status: string };
      
      expect(response.status).toBe(200);
      expect(result.status).toBe("ok");
    });

    test("404 for unknown routes", async () => {
      const response = await runtime.fetch(new Request("http://localhost/unknown"));
      const result = await response.json() as GatewayResponse;
      
      expect(response.status).toBe(404);
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_NOT_FOUND");
    });

    test("request ID is preserved in responses", async () => {
      const requestId = "custom-req-id-999";
      const code = issuePairCode(daemon, sessions);
      const result = await sendCommand(runtime, `telegram chat-new ${requestId} /pair ${code}`);
      
      expect(result.requestId).toBe(requestId);
    });
  });
});
