import { describe, test, expect, beforeEach } from "bun:test";
import { createGatewayRuntime, type GatewayRuntime } from "./server";
import { InMemoryDaemonClient } from "./daemon/client";
import { SessionStore } from "./session/store";
import type { GatewayResponse } from "./contracts/commands";

async function sendCommand(runtime: GatewayRuntime, input: string): Promise<GatewayResponse> {
  const response = await runtime.fetch(new Request("http://localhost/command", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ input }),
  }));
  return await response.json() as GatewayResponse;
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
      const code = sessions.issuePairCode().code;
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
    beforeEach(async () => {
      // Pair the session first
      const code = sessions.issuePairCode().code;
      await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
    });

    test("/agents lists available agents", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-10 /agents");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("listed");
      expect(result.message).toContain("agents");
    });

    test("/install openclaw succeeds", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-11 /install openclaw");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("install completed for openclaw");
    });

    test("/start openclaw after install succeeds", async () => {
      await sendCommand(runtime, "telegram chat1 req-12a /install openclaw");
      const result = await sendCommand(runtime, "telegram chat1 req-12 /start openclaw");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("start completed for openclaw");
    });

    test("/start without install fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-13 /start openclaw");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_NOT_INSTALLED");
    });

    test("/status shows agent state", async () => {
      await sendCommand(runtime, "telegram chat1 req-14a /install openclaw");
      await sendCommand(runtime, "telegram chat1 req-14b /start openclaw");
      const result = await sendCommand(runtime, "telegram chat1 req-14 /status openclaw");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("openclaw");
      expect(result.message).toContain("running");
    });

    test("/status without agent ID lists all agents", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-15 /status");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("status");
    });

    test("/stop openclaw succeeds when running", async () => {
      await sendCommand(runtime, "telegram chat1 req-16a /install openclaw");
      await sendCommand(runtime, "telegram chat1 req-16b /start openclaw");
      const result = await sendCommand(runtime, "telegram chat1 req-16 /stop openclaw");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("stop completed for openclaw");
    });

    test("/stop when already stopped fails", async () => {
      await sendCommand(runtime, "telegram chat1 req-17a /install openclaw");
      const result = await sendCommand(runtime, "telegram chat1 req-17 /stop openclaw");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_ALREADY_STOPPED");
    });

    test("/start when already running fails", async () => {
      await sendCommand(runtime, "telegram chat1 req-18a /install openclaw");
      await sendCommand(runtime, "telegram chat1 req-18b /start openclaw");
      const result = await sendCommand(runtime, "telegram chat1 req-18 /start openclaw");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_ALREADY_RUNNING");
    });
  });

  describe("3. Logs & diagnose", () => {
    beforeEach(async () => {
      const code = sessions.issuePairCode().code;
      await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
      await sendCommand(runtime, "telegram chat1 req-setup2 /install openclaw");
    });

    test("/logs openclaw returns log lines", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-20 /logs openclaw");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("openclaw");
    });

    test("/logs with tail parameter works", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-21 /logs openclaw 50");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("openclaw");
    });

    test("/logs without agent ID fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-22 /logs");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/diagnose openclaw creates artifact", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-23 /diagnose openclaw");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("diagnose artifact prepared");
      expect(result.downloadUrl).toBeTruthy();
    });

    test("/diagnose without agent ID fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-24 /diagnose");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/diagnose-consent yes when needed succeeds", async () => {
      daemon.setRemoteDiagnosisState("openclaw", true);
      await sendCommand(runtime, "telegram chat1 req-25a /diagnose openclaw");
      
      const result = await sendCommand(runtime, "telegram chat1 req-25 /diagnose-consent openclaw yes");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("consent recorded");
      expect(result.handoffId).toBeTruthy();
      expect(result.handoffStatus).toBe("pending");
    });

    test("/diagnose-consent no when needed succeeds", async () => {
      daemon.setRemoteDiagnosisState("openclaw", true);
      await sendCommand(runtime, "telegram chat1 req-26a /diagnose openclaw");
      
      const result = await sendCommand(runtime, "telegram chat1 req-26 /diagnose-consent openclaw no");
      
      expect(result.result).toBe("ok");
      expect(result.handoffStatus).toBe("declined");
    });

    test("/diagnose-consent when not needed fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-27 /diagnose-consent openclaw yes");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_REMOTE_DIAG_NOT_NEEDED");
    });

    test("/diagnose-consent with invalid consent flag fails", async () => {
      daemon.setRemoteDiagnosisState("openclaw", true);
      const result = await sendCommand(runtime, "telegram chat1 req-28 /diagnose-consent openclaw maybe");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_CONSENT_FLAG_INVALID");
    });
  });

  describe("4. Upgrade", () => {
    beforeEach(async () => {
      const code = sessions.issuePairCode().code;
      await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
      await sendCommand(runtime, "telegram chat1 req-setup2 /install openclaw");
    });

    test("/upgrade openclaw succeeds", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-30 /upgrade openclaw");
      
      expect(result.result).toBe("ok");
      expect(result.message).toContain("upgrade completed");
      expect(result.message).toContain("0.1.0");
      expect(result.message).toContain("0.1.1");
      expect(result.message).toContain("backup");
    });

    test("/upgrade without agent ID fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-31 /upgrade");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/upgrade non-existent agent fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-32 /upgrade nonexistent");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_AGENT_NOT_FOUND");
    });
  });

  describe("5. Error cases", () => {
    test("unknown command fails", async () => {
      const code = sessions.issuePairCode().code;
      await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
      
      const result = await sendCommand(runtime, "telegram chat1 req-40 /unknown");
      
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
      const code = sessions.issuePairCode().code;
      const pairResult = await sendCommand(runtime, `telegram chat-tg req-50 /pair ${code}`);
      expect(pairResult.result).toBe("ok");
      
      const agentsResult = await sendCommand(runtime, "telegram chat-tg req-51 /agents");
      expect(agentsResult.result).toBe("ok");
    });

    test("discord provider works", async () => {
      const code = sessions.issuePairCode().code;
      const pairResult = await sendCommand(runtime, `discord chat-dc req-52 /pair ${code}`);
      expect(pairResult.result).toBe("ok");
      
      const agentsResult = await sendCommand(runtime, "discord chat-dc req-53 /agents");
      expect(agentsResult.result).toBe("ok");
    });

    test("feishu provider works", async () => {
      const code = sessions.issuePairCode().code;
      const pairResult = await sendCommand(runtime, `feishu chat-fs req-54 /pair ${code}`);
      expect(pairResult.result).toBe("ok");
      
      const agentsResult = await sendCommand(runtime, "feishu chat-fs req-55 /agents");
      expect(agentsResult.result).toBe("ok");
    });

    test("sessions are isolated by provider and chatId", async () => {
      const code1 = sessions.issuePairCode().code;
      const code2 = sessions.issuePairCode().code;
      
      await sendCommand(runtime, `telegram chat1 req-56 /pair ${code1}`);
      await sendCommand(runtime, `discord chat1 req-57 /pair ${code2}`);
      
      // telegram:chat1 should work
      const result1 = await sendCommand(runtime, "telegram chat1 req-58 /agents");
      expect(result1.result).toBe("ok");
      
      // discord:chat1 should work
      const result2 = await sendCommand(runtime, "discord chat1 req-59 /agents");
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
        const code = sessions.issuePairCode().code;
        const pairResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase} /pair ${code}`);
        expect(pairResult.result).toBe("ok");
        
        // Install
        const installResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase + 1} /install openclaw`);
        expect(installResult.result).toBe("ok");
        
        // Start
        const startResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase + 2} /start openclaw`);
        expect(startResult.result).toBe("ok");
        
        // Status
        const statusResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase + 3} /status openclaw`);
        expect(statusResult.result).toBe("ok");
        expect(statusResult.message).toContain("running");
        
        // Stop
        const stopResult = await sendCommand(runtime, `${provider} ${chatId} req-${reqIdBase + 4} /stop openclaw`);
        expect(stopResult.result).toBe("ok");
      }
    });
  });

  describe("Additional edge cases", () => {
    beforeEach(async () => {
      const code = sessions.issuePairCode().code;
      await sendCommand(runtime, `telegram chat1 req-setup /pair ${code}`);
    });

    test("/install with missing agent ID fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-100 /install");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/start with missing agent ID fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-101 /start");
      
      expect(result.result).toBe("error");
      expect(result.errorCode).toBe("E_USAGE");
    });

    test("/stop with missing agent ID fails", async () => {
      const result = await sendCommand(runtime, "telegram chat1 req-102 /stop");
      
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
      const code = sessions.issuePairCode().code;
      const result = await sendCommand(runtime, `telegram chat-new ${requestId} /pair ${code}`);
      
      expect(result.requestId).toBe(requestId);
    });
  });
});
