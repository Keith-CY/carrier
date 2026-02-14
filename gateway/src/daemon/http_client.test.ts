import { describe, expect, test } from "bun:test";
import { HttpDaemonClient } from "./http_client";
import { DaemonClientError } from "./client";

describe("HttpDaemonClient", () => {
  test("instantiation", () => {
    const client = new HttpDaemonClient();
    expect(client).toBeDefined();
  });

  test("listAgents throws on network error", async () => {
    // Point to a port that should not be listening
    process.env.CARRIER_DAEMON_URL = "http://127.0.0.1:19999";
    const client = new HttpDaemonClient();
    try {
      await client.listAgents({ actor: "test", requestId: "r1" });
      expect(true).toBe(false); // should not reach
    } catch (err) {
      expect(err).toBeInstanceOf(DaemonClientError);
      expect((err as DaemonClientError).code).toBe("E_NETWORK");
    } finally {
      delete process.env.CARRIER_DAEMON_URL;
    }
  });
});
