#!/usr/bin/env bun

type Issue = {
  number: number;
  title: string;
  url: string;
  createdAt: string;
};

/**
 * Normalize a review-followup issue title for duplicate detection.
 *
 * We strip the `[review-followup]` tag and the leading PR reference prefix
 * (`PR #NNN: `) but keep punctuation intact so that issues with different
 * semantic meaning but similar words are not grouped as false-positive
 * duplicates.
 */
function normalizeTitle(title: string): string {
  return title
    .toLowerCase()
    .replace(/\[review-followup\]/g, "")
    .replace(/review-followup/g, "")
    .replace(/^\s*pr\s*#\d+:\s*/, "")
    .replace(/\s+/g, " ")
    .trim();
}

function runGh(args: string[]): string {
  const proc = Bun.spawnSync(["gh", ...args], {
    stdout: "pipe",
    stderr: "pipe",
  });

  if (proc.exitCode !== 0) {
    const stderr = Buffer.from(proc.stderr).toString("utf8").trim();
    throw new Error(stderr || "gh command failed");
  }

  return Buffer.from(proc.stdout).toString("utf8");
}

function main(): void {
  const repo = process.argv[2] ?? "Keith-CY/carrier";
  const output = runGh([
    "issue",
    "list",
    "--repo",
    repo,
    "--state",
    "open",
    "--search",
    "[review-followup] in:title",
    "--limit",
    "300",
    "--json",
    "number,title,url,createdAt",
  ]);

  const issues = JSON.parse(output) as Issue[];
  const grouped = new Map<string, Issue[]>();

  for (const issue of issues) {
    const key = normalizeTitle(issue.title);
    if (!key) {
      continue;
    }
    const list = grouped.get(key) ?? [];
    list.push(issue);
    grouped.set(key, list);
  }

  const duplicates = [...grouped.entries()]
    .map(([key, items]) => ({ key, items: items.sort((a, b) => a.number - b.number) }))
    .filter((entry) => entry.items.length > 1)
    .sort((a, b) => b.items.length - a.items.length || a.key.localeCompare(b.key));

  console.log(`Review-followup duplicate detector`);
  console.log(`Repository: ${repo}`);
  console.log(`Open [review-followup] issues scanned: ${issues.length}`);

  if (duplicates.length === 0) {
    console.log("No duplicate candidates found.");
    return;
  }

  console.log(`Duplicate groups: ${duplicates.length}`);
  console.log("");

  duplicates.forEach((group, index) => {
    console.log(`${index + 1}. Normalized key: ${group.key}`);
    for (const issue of group.items) {
      console.log(`   - #${issue.number} ${issue.title}`);
    }
    console.log("");
  });

  console.log("Note: report is read-only; no issues are modified.");
}

try {
  main();
} catch (error) {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`Error: ${message}`);
  process.exit(1);
}
