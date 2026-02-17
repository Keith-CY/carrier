import type { Provider, GatewayResponse } from "../contracts/commands";

export type ProviderFailureResponse = {
  provider: Provider;
  response: GatewayResponse;
};

export type FailureParityDrift = {
  provider: Provider;
  field: "result" | "errorCode" | "message";
  expected: string;
  actual: string;
};

export function collectFailureParityDrifts(
  responses: readonly ProviderFailureResponse[],
): FailureParityDrift[] {
  if (responses.length === 0) {
    return [];
  }

  const drifts: FailureParityDrift[] = [];
  const baseline = responses[0];
  const baselineCode = baseline.response.errorCode ?? "";
  const baselineMessage = baseline.response.message;

  for (const entry of responses) {
    if (entry.response.result !== "error") {
      drifts.push({
        provider: entry.provider,
        field: "result",
        expected: "error",
        actual: entry.response.result,
      });
    }
  }

  if (baseline.response.result !== "error") {
    return drifts;
  }

  for (const entry of responses.slice(1)) {
    if (entry.response.result !== "error") {
      continue;
    }
    const code = entry.response.errorCode ?? "";
    if (code !== baselineCode) {
      drifts.push({
        provider: entry.provider,
        field: "errorCode",
        expected: baselineCode,
        actual: code,
      });
    }
    if (entry.response.message !== baselineMessage) {
      drifts.push({
        provider: entry.provider,
        field: "message",
        expected: baselineMessage,
        actual: entry.response.message,
      });
    }
  }

  return drifts;
}

export function formatFailureParityDrifts(
  responses: readonly ProviderFailureResponse[],
  drifts: readonly FailureParityDrift[],
): string {
  if (responses.length === 0) {
    return "no provider responses to compare";
  }

  const baseline = responses[0];
  const lines = [
    `failure parity drift detected (baseline=${baseline.provider})`,
    ...drifts.map((drift) =>
      `provider=${drift.provider} field=${drift.field} expected=${JSON.stringify(drift.expected)} actual=${JSON.stringify(drift.actual)}`),
  ];
  return lines.join("\n");
}

export function assertFailureParity(
  responses: readonly ProviderFailureResponse[],
): void {
  const drifts = collectFailureParityDrifts(responses);
  if (drifts.length === 0) {
    return;
  }
  throw new Error(formatFailureParityDrifts(responses, drifts));
}
