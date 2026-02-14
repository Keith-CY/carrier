const { extractNbsLines } = require("./extract-nbs");

// --- Issue #36: /g regex lastIndex state ---
describe("extractNbsLines - regex /g safety (issue #36)", () => {
  test("repeated calls return consistent results (no lastIndex leak)", () => {
    const text = "NBS: suggestion A\nNBS: suggestion B";
    const r1 = extractNbsLines(text);
    const r2 = extractNbsLines(text);
    const r3 = extractNbsLines(text);
    expect(r1).toEqual(["suggestion A", "suggestion B"]);
    expect(r2).toEqual(r1);
    expect(r3).toEqual(r1);
  });

  test("different texts in sequence extract correctly", () => {
    const t1 = "NBS: first";
    const t2 = "NBS: second";
    expect(extractNbsLines(t1)).toEqual(["first"]);
    expect(extractNbsLines(t2)).toEqual(["second"]);
    expect(extractNbsLines(t1)).toEqual(["first"]);
  });
});

// --- Issue #271: determinism with multiple NBS lines ---
describe("extractNbsLines - determinism (issue #271)", () => {
  test("multi-line input with mixed normal text + NBS lines", () => {
    const text = [
      "Overall looks good.",
      "NBS: Add edge-case test for missing request_id.",
      "Some other comment here.",
      "NBS: Clarify fallback behavior in README.",
      "Final note.",
    ].join("\n");
    const result = extractNbsLines(text);
    expect(result).toEqual([
      "Add edge-case test for missing request_id.",
      "Clarify fallback behavior in README.",
    ]);
  });

  test("one suggestion per NBS line, no merged/duplicated outputs", () => {
    const text = "NBS: A\nNBS: B\nNBS: C";
    expect(extractNbsLines(text)).toEqual(["A", "B", "C"]);
    expect(extractNbsLines(text)).toHaveLength(3);
  });

  test("stable across repeated function calls in same process", () => {
    const text = "NBS: stable result";
    for (let i = 0; i < 10; i++) {
      expect(extractNbsLines(text)).toEqual(["stable result"]);
    }
  });
});

// --- Issue #275: multilingual and whitespace edge cases ---
describe("extractNbsLines - multilingual and whitespace (issue #275)", () => {
  test("mixed English/Chinese text with valid NBS lines", () => {
    const text = [
      "这段代码看起来不错。",
      "NBS: 考虑添加对空输入的边界测试。",
      "Good implementation overall.",
      "NBS: Add timeout handling for slow responses.",
    ].join("\n");
    expect(extractNbsLines(text)).toEqual([
      "考虑添加对空输入的边界测试。",
      "Add timeout handling for slow responses.",
    ]);
  });

  test("leading spaces/tabs before NBS:", () => {
    const text = "  NBS: indented\n\tNBS: tabbed\n    NBS: deep indent";
    expect(extractNbsLines(text)).toEqual(["indented", "tabbed", "deep indent"]);
  });

  test("multiple NBS lines separated by blank lines", () => {
    const text = "NBS: first\n\n\nNBS: second\n\nNBS: third";
    expect(extractNbsLines(text)).toEqual(["first", "second", "third"]);
  });

  test("near-miss variants are NOT extracted", () => {
    const text = [
      "NB S: not valid",
      "NBS : also not valid with space before colon",
      "XNBS: prefix mismatch",
      "nbs: lowercase should work (case insensitive)",
    ].join("\n");
    const result = extractNbsLines(text);
    // Only "nbs:" (case insensitive) should match
    expect(result).toEqual(["lowercase should work (case insensitive)"]);
  });
});

// --- Issue #277: punctuation and bullet-style boundary cases ---
describe("extractNbsLines - punctuation and bullet boundaries (issue #277)", () => {
  test("NBS lines ending with various punctuation", () => {
    const text = [
      "NBS: Suggestion ending with period.",
      "NBS: Suggestion ending with semicolon;",
      "NBS: Suggestion ending with paren)",
    ].join("\n");
    expect(extractNbsLines(text)).toEqual([
      "Suggestion ending with period.",
      "Suggestion ending with semicolon;",
      "Suggestion ending with paren)",
    ]);
  });

  test("mixed list prefixes before NBS:", () => {
    const text = [
      "- NBS: dash bullet",
      "* NBS: asterisk bullet",
      "1. NBS: numbered bullet",
    ].join("\n");
    // The regex matches NBS: at line start with optional whitespace
    // Bullet prefixes like "- " before NBS: won't match because "- " is not whitespace only
    // This tests current behavior
    const result = extractNbsLines(text);
    // "- NBS:" has non-whitespace before NBS:, should not match with strict regex
    // But our regex is /^\s*NBS:/gmi — "- " is not \s, so these should NOT match
    expect(result).toEqual([]);
  });

  test("surrounding non-NBS lines are ignored", () => {
    const text = [
      "This is a normal comment.",
      "NBS: The actual suggestion.",
      "Another normal line.",
      "Not an NBS suggestion.",
    ].join("\n");
    expect(extractNbsLines(text)).toEqual(["The actual suggestion."]);
  });
});

// --- Issue #332: newline variant handling ---
describe("extractNbsLines - newline variants (issue #332)", () => {
  test("unix newlines (\\n)", () => {
    const text = "NBS: unix first\nNBS: unix second";
    expect(extractNbsLines(text)).toEqual(["unix first", "unix second"]);
  });

  test("windows newlines (\\r\\n)", () => {
    const text = "NBS: win first\r\nNBS: win second";
    expect(extractNbsLines(text)).toEqual(["win first", "win second"]);
  });

  test("lines with trailing spaces/tabs", () => {
    const text = "NBS: trailing space   \nNBS: trailing tab\t\t";
    const result = extractNbsLines(text);
    expect(result).toEqual(["trailing space", "trailing tab"]);
  });

  test("extracted lines are identical across newline variants", () => {
    const unix = "NBS: same suggestion\nNBS: another one";
    const win = "NBS: same suggestion\r\nNBS: another one";
    expect(extractNbsLines(unix)).toEqual(extractNbsLines(win));
  });
});

// --- Edge cases ---
describe("extractNbsLines - edge cases", () => {
  test("empty/null input returns empty array", () => {
    expect(extractNbsLines("")).toEqual([]);
    expect(extractNbsLines(null)).toEqual([]);
    expect(extractNbsLines(undefined)).toEqual([]);
  });

  test("NBS: with only whitespace after colon is ignored", () => {
    const text = "NBS:   \nNBS: valid";
    expect(extractNbsLines(text)).toEqual(["valid"]);
  });
});
