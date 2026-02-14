import { describe, expect, test, afterEach } from "bun:test";
import { createServer } from "./server";
import { InMemoryDaemonClient } from "./daemon/client";
import { DownloadTokenStore } from "./downloads/token_store";
import { SessionStore } from "./session/store";
let server: ReturnType<typeof createServer> | undefined;

afterEach(() => {
  server?.stop(true);
  server = undefined;
});

function setup() {
  const sessions = new SessionStore();
  const daemon = new InMemoryDaemonClient();
  const downloads = new DownloadTokenStore();
  server = createServer({
    port: 0, // random port
    deps: { daemon, sessions, downloads },
  });
  const base = `http://localhost:${server.port}`;
  return { base, sessions, daemon, downloads };
}

describe("gateway HTTP server", () => {
  test("healthz", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/healthz`);
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("ok");
  });

  test("404 for unknown route", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/unknown`);
    expect(res.status).toBe(404);
  });

  test("telegram webhook - pair command", async () => {
    const { base, sessions } = setup();
    const code = sessions.issuePairCode().code;
    const res = await fetch(`${base}/webhook/telegram`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        message: {
          chat: { id: 12345 },
          text: `/pair ${code}`,
        },
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.result).toBe("ok");
    expect(body.message).toContain("paired");
  });

  test("discord webhook - non-command acknowledged", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/discord`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        channel_id: "ch1",
        content: "hello world",
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.ok).toBe(true);
  });

  test("feishu webhook - challenge response", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/feishu`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ challenge: "abc123" }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.challenge).toBe("abc123");
  });

  test("download with invalid token returns 404", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/download/invalid-token`);
    expect(res.status).toBe(404);
  });

  test("download with valid token", async () => {
    const { base, downloads } = setup();
    const token = downloads.issue("/tmp/test.zip");
    const res = await fetch(`${base}/download/${token.token}`);
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fileRef).toBe("/tmp/test.zip");
  });
});

describe("webhook parser error paths", () => {
  test("telegram - missing message field -> 400 parse error", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/telegram`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ update_id: 123 }),
    });
    expect(res.status).toBe(400);
    const body = await res.json();
    expect(body.error).toBe("could not parse webhook body");
  });

  test("telegram - missing chat.id -> 400 parse error", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/telegram`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: { text: "/help" } }),
    });
    // chat is undefined so chatId becomes "" which triggers null return
    expect(res.status).toBe(400);
    const body = await res.json();
    expect(body.error).toBe("could not parse webhook body");
  });

  test("telegram - missing text -> non-command acknowledged", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/telegram`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: { chat: { id: 1 } } }),
    });
    // text defaults to "" via String(undefined ?? ""), chatId = "1"
    // Parser returns null when text is empty
    expect(res.status).toBe(400);
    const body = await res.json();
    expect(body.error).toBe("could not parse webhook body");
  });

  test("discord - empty payload -> 400 parse error", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/discord`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    });
    expect(res.status).toBe(400);
    const body = await res.json();
    expect(body.error).toBe("could not parse webhook body");
  });

  test("feishu - missing event.message -> 400 parse error", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/feishu`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ event: {} }),
    });
    expect(res.status).toBe(400);
    const body = await res.json();
    expect(body.error).toBe("could not parse webhook body");
  });

  test("feishu - malformed message.content JSON -> handles gracefully", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/feishu`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        event: {
          message: {
            chat_id: "chat1",
            content: "{not valid json",
          },
        },
      }),
    });
    // Malformed JSON falls through to raw content, which doesn't start with /
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.ok).toBe(true);
  });

  test("invalid JSON body -> 400", async () => {
    const { base } = setup();
    const res = await fetch(`${base}/webhook/telegram`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "not json at all",
    });
    expect(res.status).toBe(400);
    const body = await res.json();
    expect(body.error).toBe("invalid JSON body");
  });
});
