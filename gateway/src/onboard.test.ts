import { describe, test, expect } from "bun:test";
import { createGatewayRuntime } from "./server";
import { InMemoryDaemonClient } from "./daemon/client";

const API_TOKEN = "test-onboard-token";

function setup() {
  process.env.CARRIER_GATEWAY_API_TOKEN = API_TOKEN;
  const daemon = new InMemoryDaemonClient();
  const runtime = createGatewayRuntime({ deps: { daemon } });
  const authHeaders = {
    "content-type": "application/json",
    authorization: `Bearer ${API_TOKEN}`,
  };
  return { runtime, daemon, authHeaders };
}

function teardown() {
  delete process.env.CARRIER_GATEWAY_API_TOKEN;
}

describe("POST /api/v1/onboard", () => {
  test("returns available agents", async () => {
    const { runtime, authHeaders } = setup();
    try {
      const res = await runtime.fetch(
        new Request("http://localhost/api/v1/onboard", {
          method: "POST",
          headers: authHeaders,
          body: JSON.stringify({ provider: "telegram", provider_token: "bot123", env: {} }),
        }),
      );
      expect(res.status).toBe(200);
      const body = await res.json() as any;
      expect(body.status).toBe("ready");
      expect(body.provider_configured).toBe(true);
      expect(body.available_agents).toBeInstanceOf(Array);
      expect(body.available_agents.length).toBeGreaterThan(0);
    } finally { teardown(); }
  });

  test("rejects missing provider", async () => {
    const { runtime, authHeaders } = setup();
    try {
      const res = await runtime.fetch(
        new Request("http://localhost/api/v1/onboard", {
          method: "POST",
          headers: authHeaders,
          body: JSON.stringify({ env: {} }),
        }),
      );
      expect(res.status).toBe(400);
      const body = await res.json() as any;
      expect(body.errorCode).toBe("E_MISSING_PROVIDER");
    } finally { teardown(); }
  });

  test("rejects invalid provider", async () => {
    const { runtime, authHeaders } = setup();
    try {
      const res = await runtime.fetch(
        new Request("http://localhost/api/v1/onboard", {
          method: "POST",
          headers: authHeaders,
          body: JSON.stringify({ provider: "slack" }),
        }),
      );
      expect(res.status).toBe(400);
      const body = await res.json() as any;
      expect(body.errorCode).toBe("E_INVALID_PROVIDER");
    } finally { teardown(); }
  });

  test("requires auth", async () => {
    const { runtime } = setup();
    try {
      const res = await runtime.fetch(
        new Request("http://localhost/api/v1/onboard", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ provider: "telegram" }),
        }),
      );
      expect(res.status).toBe(401);
    } finally { teardown(); }
  });
});

describe("POST /api/v1/onboard/install", () => {
  test("installs and starts agents", async () => {
    const { runtime, authHeaders } = setup();
    try {
      const res = await runtime.fetch(
        new Request("http://localhost/api/v1/onboard/install", {
          method: "POST",
          headers: authHeaders,
          body: JSON.stringify({ agents: ["openclaw"] }),
        }),
      );
      expect(res.status).toBe(200);
      const body = await res.json() as any;
      expect(body.results).toHaveLength(1);
      expect(body.results[0].agent_id).toBe("openclaw");
      expect(body.results[0].install).toBe("ok");
      expect(body.results[0].start).toBe("ok");
      expect(body.results[0].status).toBe("running");
    } finally { teardown(); }
  });

  test("rejects empty agents array", async () => {
    const { runtime, authHeaders } = setup();
    try {
      const res = await runtime.fetch(
        new Request("http://localhost/api/v1/onboard/install", {
          method: "POST",
          headers: authHeaders,
          body: JSON.stringify({ agents: [] }),
        }),
      );
      expect(res.status).toBe(400);
    } finally { teardown(); }
  });

  test("handles install failure gracefully", async () => {
    const { runtime, authHeaders } = setup();
    try {
      const res = await runtime.fetch(
        new Request("http://localhost/api/v1/onboard/install", {
          method: "POST",
          headers: authHeaders,
          body: JSON.stringify({ agents: ["nonexistent"] }),
        }),
      );
      expect(res.status).toBe(200);
      const body = await res.json() as any;
      expect(body.results[0].install).toBe("error");
    } finally { teardown(); }
  });
});

describe("GET /api/v1/onboard/status", () => {
  test("returns onboard state", async () => {
    const { runtime, authHeaders } = setup();
    try {
      const res = await runtime.fetch(
        new Request("http://localhost/api/v1/onboard/status", {
          method: "GET",
          headers: authHeaders,
        }),
      );
      expect(res.status).toBe(200);
      const body = await res.json() as any;
      expect(body.onboard).toBeDefined();
      expect(body.agents).toBeInstanceOf(Array);
    } finally { teardown(); }
  });
});
