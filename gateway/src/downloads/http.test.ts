import { describe, expect, test } from "bun:test";
import {
  buildContentDisposition,
  compareRequestedFileName,
  expectedFileNameFromRef,
  normalizeDownloadFileName,
  parseDownloadPath,
} from "./http";

describe("parseDownloadPath", () => {
  test("returns token and decoded filename for valid download path", () => {
    const parsed = parseDownloadPath("/downloads/tok-1/report%202026.txt");
    expect(parsed).toEqual({
      token: "tok-1",
      requestedFileName: "report 2026.txt",
    });
  });

  test("returns null for malformed pathname", () => {
    expect(parseDownloadPath("/downloads/tok-1")).toBeNull();
    expect(parseDownloadPath("/not-downloads/tok-1/file.txt")).toBeNull();
  });

  test("returns null for malformed percent-encoding", () => {
    expect(parseDownloadPath("/downloads/tok-1/bad%ZZ.txt")).toBeNull();
  });
});

describe("filename normalization and mismatch checks", () => {
  test("normalizes leading and trailing whitespace", () => {
    expect(normalizeDownloadFileName("  report.txt  ")).toBe("report.txt");
  });

  test("extracts filename from artifact path", () => {
    expect(expectedFileNameFromRef("/tmp/build/artifact.zip")).toBe("artifact.zip");
    expect(expectedFileNameFromRef("/tmp/build/")).toBe("artifact.bin");
    expect(expectedFileNameFromRef("C:\\tmp\\build\\artifact.zip")).toBe("artifact.zip");
    expect(expectedFileNameFromRef("/tmp/build/path\\file.txt")).toBe("path\\file.txt");
  });

  test("compares normalized requested filename against token artifact filename", () => {
    const matched = compareRequestedFileName({
      requestedFileName: "  report.zip ",
      fileRef: "/tmp/releases/report.zip",
    });
    expect(matched.matches).toBe(true);
    expect(matched.expectedFileName).toBe("report.zip");
    expect(matched.requestedFileName).toBe("report.zip");

    const mismatched = compareRequestedFileName({
      requestedFileName: "wrong.zip",
      fileRef: "/tmp/releases/report.zip",
    });
    expect(mismatched.matches).toBe(false);
    expect(mismatched.expectedFileName).toBe("report.zip");
    expect(mismatched.requestedFileName).toBe("wrong.zip");
  });
});

describe("buildContentDisposition", () => {
  test("escapes ASCII filenames with quotes and backslashes", () => {
    const header = buildContentDisposition('test"file\\name.txt');
    expect(header).toBe('attachment; filename="test\\"file\\\\name.txt"');
  });

  test("uses RFC 5987 filename* for UTF-8 text", () => {
    const header = buildContentDisposition("文件名.txt");
    expect(header).toContain('filename="___.txt"');
    expect(header).toContain("filename*=UTF-8''");
    expect(header).toContain(encodeURIComponent("文件名.txt"));
  });

  test("uses RFC 5987 filename* for emoji filename", () => {
    const header = buildContentDisposition("report📊.txt");
    expect(header).toContain('filename="report__.txt"');
    expect(header).toContain("filename*=UTF-8''");
    expect(header).toContain(encodeURIComponent("report📊.txt"));
  });
});
