import { describe, expect, test, beforeEach, afterEach } from "bun:test";
import { createGatewayRuntime, createRuntimeDependencies } from "./server";
import { InMemoryDaemonClient } from "./daemon/client";
import { SessionStore } from "./session/store";
import { DownloadTokenStore } from "./downloads/token_store";
import { writeFile, mkdir, rm } from "node:fs/promises";
import { join } from "node:path";

function makeDeps() {
  return {
    daemon: new InMemoryDaemonClient(),
    sessions: new SessionStore(),
    downloads: new DownloadTokenStore(),
  };
}

describe("ARTIFACT_ROOT security", () => {
  let originalArtifactRoot: string | undefined;
  let testDir: string;

  beforeEach(async () => {
    originalArtifactRoot = process.env.ARTIFACT_ROOT;
    testDir = `/tmp/artifact-test-${crypto.randomUUID()}`;
    await mkdir(testDir, { recursive: true });
  });

  afterEach(async () => {
    if (originalArtifactRoot !== undefined) {
      process.env.ARTIFACT_ROOT = originalArtifactRoot;
    } else {
      delete process.env.ARTIFACT_ROOT;
    }
    await rm(testDir, { recursive: true, force: true });
  });

  test("defaults to ./artifacts when ARTIFACT_ROOT is not set", async () => {
    delete process.env.ARTIFACT_ROOT;
    
    const deps = makeDeps();
    const testFile = join(process.cwd(), "artifacts", `test-${crypto.randomUUID()}.txt`);
    await mkdir(join(process.cwd(), "artifacts"), { recursive: true });
    await writeFile(testFile, "test content");
    
    try {
      const token = deps.downloads.issue(testFile, 300, false);
      const runtime = createGatewayRuntime({ deps });
      
      const response = await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
      
      expect(response.status).toBe(200);
      const text = await response.text();
      expect(text).toBe("test content");
    } finally {
      await rm(testFile, { force: true });
    }
  });

  test("allows reading from configured ARTIFACT_ROOT", async () => {
    process.env.ARTIFACT_ROOT = testDir;
    
    const deps = makeDeps();
    const testFile = join(testDir, `test-${crypto.randomUUID()}.txt`);
    await writeFile(testFile, "allowed content");
    
    const token = deps.downloads.issue(testFile, 300, false);
    const runtime = createGatewayRuntime({ deps });
    
    const response = await runtime.fetch(
      new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
    );
    
    expect(response.status).toBe(200);
    const text = await response.text();
    expect(text).toBe("allowed content");
  });

  test("blocks reading from outside ARTIFACT_ROOT", async () => {
    process.env.ARTIFACT_ROOT = testDir;
    
    const deps = makeDeps();
    const outsideFile = `/tmp/outside-${crypto.randomUUID()}.txt`;
    await writeFile(outsideFile, "forbidden content");
    
    try {
      const token = deps.downloads.issue(outsideFile, 300, false);
      const runtime = createGatewayRuntime({ deps });
      
      const response = await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
      
      // Should return 404 because file is outside artifact root
      expect(response.status).toBe(404);
    } finally {
      await rm(outsideFile, { force: true });
    }
  });

  test("blocks path traversal attacks", async () => {
    process.env.ARTIFACT_ROOT = testDir;
    
    const deps = makeDeps();
    const testFile = join(testDir, "allowed.txt");
    await writeFile(testFile, "allowed");
    
    // Try to traverse outside using ..
    const traversalPath = join(testDir, "..", "outside.txt");
    await writeFile(join(testDir, "..", "outside.txt"), "forbidden");
    
    try {
      const token = deps.downloads.issue(traversalPath, 300, false);
      const runtime = createGatewayRuntime({ deps });
      
      const response = await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
      
      // Should be blocked
      expect(response.status).toBe(404);
    } finally {
      await rm(join(testDir, "..", "outside.txt"), { force: true });
    }
  });

  test("rejects ARTIFACT_ROOT set to /", async () => {
    expect(() => {
      process.env.ARTIFACT_ROOT = "/";
      // This will trigger validation when createGatewayRuntime tries to use defaultReadFile
      const deps = makeDeps();
      // The validation happens in defaultReadFile, so we need to actually trigger a read
      // For now, just creating the runtime shouldn't fail - the error happens during file read
    }).not.toThrow();
    
    // But when we try to read a file, it should fail
    process.env.ARTIFACT_ROOT = "/";
    const deps = makeDeps();
    const token = deps.downloads.issue("/etc/passwd", 300, false);
    const runtime = createGatewayRuntime({ deps });
    
    await expect(async () => {
      await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
    }).toThrow();
  });

  test("rejects ARTIFACT_ROOT set to /etc", async () => {
    process.env.ARTIFACT_ROOT = "/etc";
    const deps = makeDeps();
    const token = deps.downloads.issue("/etc/passwd", 300, false);
    const runtime = createGatewayRuntime({ deps });
    
    await expect(async () => {
      await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
    }).toThrow();
  });

  test("rejects ARTIFACT_ROOT set to /usr", async () => {
    process.env.ARTIFACT_ROOT = "/usr";
    const deps = makeDeps();
    const token = deps.downloads.issue("/usr/bin/env", 300, false);
    const runtime = createGatewayRuntime({ deps });
    
    await expect(async () => {
      await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
    }).toThrow();
  });

  test("rejects ARTIFACT_ROOT set to /var", async () => {
    process.env.ARTIFACT_ROOT = "/var";
    const deps = makeDeps();
    const token = deps.downloads.issue("/var/log/test.log", 300, false);
    const runtime = createGatewayRuntime({ deps });
    
    await expect(async () => {
      await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
    }).toThrow();
  });

  test("rejects ARTIFACT_ROOT set to /root", async () => {
    process.env.ARTIFACT_ROOT = "/root";
    const deps = makeDeps();
    const token = deps.downloads.issue("/root/.bashrc", 300, false);
    const runtime = createGatewayRuntime({ deps });
    
    await expect(async () => {
      await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
    }).toThrow();
  });

  test("rejects subdirectories of dangerous paths", async () => {
    process.env.ARTIFACT_ROOT = "/etc/config";
    const deps = makeDeps();
    const token = deps.downloads.issue("/etc/config/test.conf", 300, false);
    const runtime = createGatewayRuntime({ deps });
    
    await expect(async () => {
      await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
    }).toThrow();
  });

  test("resolves relative ARTIFACT_ROOT to absolute path", async () => {
    process.env.ARTIFACT_ROOT = "./test-artifacts";
    
    const deps = makeDeps();
    const resolvedDir = join(process.cwd(), "test-artifacts");
    await mkdir(resolvedDir, { recursive: true });
    const testFile = join(resolvedDir, `test-${crypto.randomUUID()}.txt`);
    await writeFile(testFile, "relative path content");
    
    try {
      const token = deps.downloads.issue(testFile, 300, false);
      const runtime = createGatewayRuntime({ deps });
      
      const response = await runtime.fetch(
        new Request(`http://gateway.local${deps.downloads.toDownloadURL(token)}`)
      );
      
      expect(response.status).toBe(200);
      const text = await response.text();
      expect(text).toBe("relative path content");
    } finally {
      await rm(testFile, { force: true });
      await rm(resolvedDir, { recursive: true, force: true });
    }
  });
});
