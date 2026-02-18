import { beforeEach, describe, expect, test } from "bun:test";
import { handleCommand, parseInput, type GatewayDependencies } from "./index";
import type { Provider, GatewayResponse } from "./contracts/commands";
import { InMemoryDaemonClient } from "./daemon/client";
import { SessionStore } from "./session/store";
import { DownloadTokenStore } from "./downloads/token_store";
import { assertFailureParity, type ProviderFailureResponse } from "./parity/failure_parity";

/**
 * Cross-provider consistency tests.
 *
 * For each command, we run the exact same logical operation through all three
 * providers and assert that the responses are structurally identical (modulo
 * the provider-scoped session setup).
 */

const PROVIDERS: Provider[] = ["telegram", "discord", "feishu"];

function buildDeps(): GatewayDependencies {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

function pairAll(deps: GatewayDependencies, chatId = "100"): Map<Provider, string> {
  const tokens = new Map<Provider, string>();
  for (const provider of PROVIDERS) {
    deps.sessions.registerPairCode(`code-${provider}`, 300);
    const session = deps.sessions.pair({ provider, chatId, code: `code-${provider}` });
    if (session) {
      tokens.set(provider, session.sessionToken);
    }
  }
  return tokens;
}

/** Strip provider-specific bits from a response so we can compare across providers. */
function normalize(res: GatewayResponse): Omit<GatewayResponse, "requestId" | "sessionToken" | "downloadUrl"> {
  const { requestId: _r, sessionToken: _s, downloadUrl: _d, ...rest } = res;
  return rest;
}

async function runAcrossProviders(
  deps: GatewayDependencies,
  tokens: Map<Provider, string>,
  commandSuffix: string,
  chatId = "100",
): Promise<ProviderFailureResponse[]> {
  const results: ProviderFailureResponse[] = [];
  for (const provider of PROVIDERS) {
    const token = tokens.get(provider) || "";
    const input = `${provider} ${chatId} req-${provider} ${token} ${commandSuffix}`;
    const res = await handleCommand(parseInput(input), deps);
    results.push({ provider, response: res });
  }
  return results;
}

function responses(results: ProviderFailureResponse[]): GatewayResponse[] {
  return results.map((entry) => entry.response);
}

function assertAllConsistent(results: ProviderFailureResponse[]): void {
  const normalized = responses(results).map(normalize);
  for (let i = 1; i < normalized.length; i++) {
    expect(normalized[i]).toEqual(normalized[0]);
  }
}

describe("cross-provider consistency", () => {
  let deps: GatewayDependencies;
  let tokens: Map<Provider, string>;

  beforeEach(() => {
    deps = buildDeps();
    tokens = pairAll(deps);
  });

  test("/agents returns same result across all providers", async () => {
    const results = await runAcrossProviders(deps, tokens, "/agents");
    assertAllConsistent(results);
    expect(results[0].response.result).toBe("ok");
  });

  test("/install returns same result across all providers", async () => {
    const results = await runAcrossProviders(deps, tokens, "/install myagent");
    assertAllConsistent(results);
  });

  test("/start error is consistent across all providers", async () => {
    // Without installing first, all providers should get the same error
    const results = await runAcrossProviders(deps, tokens, "/start myagent");
    assertFailureParity(results);
    assertAllConsistent(results);
    expect(results[0].response.result).toBe("error");
  });

  test("/stop error is consistent across all providers", async () => {
    const results = await runAcrossProviders(deps, tokens, "/stop myagent");
    assertFailureParity(results);
    assertAllConsistent(results);
    expect(results[0].response.result).toBe("error");
  });

  test("/status returns same result across all providers", async () => {
    const results = await runAcrossProviders(deps, tokens, "/status");
    assertAllConsistent(results);
  });

  test("/logs returns same result across all providers", async () => {
    const results = await runAcrossProviders(deps, tokens, "/logs myagent 10");
    assertAllConsistent(results);
  });

  test("/upgrade returns same result across all providers", async () => {
    const results = await runAcrossProviders(deps, tokens, "/upgrade myagent");
    assertAllConsistent(results);
  });

  test("/diagnose returns same result shape across all providers", async () => {
    const results = await runAcrossProviders(deps, tokens, "/diagnose myagent");
    assertAllConsistent(results);
  });

  test("usage errors are identical across providers", async () => {
    const results = await runAcrossProviders(deps, tokens, "/install");
    assertFailureParity(results);
    assertAllConsistent(results);
    expect(results[0].response.result).toBe("error");
    expect(results[0].response.errorCode).toBe("E_USAGE");
  });

  test("session-required errors are identical across providers (unpaired)", async () => {
    const freshDeps = buildDeps();
    const results: ProviderFailureResponse[] = [];
    for (const provider of PROVIDERS) {
      const input = `${provider} 999 req-${provider} /agents`;
      const res = await handleCommand(parseInput(input), freshDeps);
      results.push({ provider, response: res });
    }
    assertFailureParity(results);
    assertAllConsistent(results);
    expect(results[0].response.errorCode).toBe("E_SESSION_REQUIRED");
  });

  test("/pair success is consistent across providers", async () => {
    const freshDeps = buildDeps();
    const results: ProviderFailureResponse[] = [];
    for (const provider of PROVIDERS) {
      if (freshDeps.daemon instanceof InMemoryDaemonClient) {
        freshDeps.daemon.registerPairCode(`p-${provider}`);
      }
      const input = `${provider} 200 req-${provider} /pair p-${provider}`;
      const res = await handleCommand(parseInput(input), freshDeps);
      results.push({ provider, response: res });
    }
    // All should succeed with session tokens
    for (const r of responses(results)) {
      expect(r.result).toBe("ok");
      expect(r.sessionToken).toBeDefined();
    }
    // result field and errorCode should be the same
    expect(responses(results).map((r) => r.result)).toEqual(["ok", "ok", "ok"]);
  });

  test("/pair invalid code is consistent across providers", async () => {
    const freshDeps = buildDeps();
    const results: ProviderFailureResponse[] = [];
    for (const provider of PROVIDERS) {
      const input = `${provider} 200 req-${provider} /pair bad-code`;
      const res = await handleCommand(parseInput(input), freshDeps);
      results.push({ provider, response: res });
    }
    assertFailureParity(results);
    assertAllConsistent(results);
    expect(results[0].response.errorCode).toBe("E_PAIR_CODE_INVALID");
  });
});
