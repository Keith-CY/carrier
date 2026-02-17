import { describe, expect, test } from "bun:test";
import { assertFailureParity, collectFailureParityDrifts, formatFailureParityDrifts, type ProviderFailureResponse } from "./failure_parity";

function err(provider: ProviderFailureResponse["provider"], errorCode: string, message: string): ProviderFailureResponse {
  return {
    provider,
    response: {
      requestId: `req-${provider}`,
      result: "error",
      errorCode,
      message,
    },
  };
}

describe("failure parity checks", () => {
  test("returns no drift when providers share identical failure response", () => {
    const responses = [
      err("telegram", "E_NOT_INSTALLED", "agent is not installed"),
      err("discord", "E_NOT_INSTALLED", "agent is not installed"),
      err("feishu", "E_NOT_INSTALLED", "agent is not installed"),
    ] as const;

    expect(collectFailureParityDrifts(responses)).toEqual([]);
    expect(() => assertFailureParity(responses)).not.toThrow();
  });

  test("reports response-shape drift by provider and field", () => {
    const responses = [
      err("telegram", "E_NOT_INSTALLED", "agent is not installed"),
      err("discord", "E_ALREADY_RUNNING", "agent already running"),
      {
        provider: "feishu" as const,
        response: {
          requestId: "req-feishu",
          result: "ok" as const,
          message: "unexpected success",
        },
      },
    ];

    const drifts = collectFailureParityDrifts(responses);
    expect(drifts).toEqual([
      { provider: "feishu", field: "result", expected: "error", actual: "ok" },
      { provider: "discord", field: "errorCode", expected: "E_NOT_INSTALLED", actual: "E_ALREADY_RUNNING" },
      { provider: "discord", field: "message", expected: "agent is not installed", actual: "agent already running" },
    ]);

    const message = formatFailureParityDrifts(responses, drifts);
    expect(message).toContain("baseline=telegram");
    expect(message).toContain("provider=discord field=errorCode");
    expect(message).toContain("provider=feishu field=result");
    expect(() => assertFailureParity(responses)).toThrow("failure parity drift detected");
  });
});
