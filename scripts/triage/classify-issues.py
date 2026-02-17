#!/usr/bin/env python3
"""
Classify unassigned issues as LIGHTWEIGHT or HEAVY based on heuristics.
Usage: python3 scripts/triage/classify-issues.py
Requires: gh CLI with authentication.
Read-only: does not modify or close issues.

Example output:
  LIGHTWEIGHT
    #123 [docs] Add README section
    #124 [tests] Add unit test for parser

  HEAVY
    #125 [architecture] Refactor auth system
    #126 [security-critical] Fix token leak
"""
import json
import re
import subprocess
import sys

LIGHTWEIGHT_KEYWORDS = {
    "docs": ["doc", "readme", "contributing", "typo", "comment", "wording"],
    "tests": ["test", "unit test", "coverage", "fixture", "assert"],
    "ci": ["ci", "workflow", "lint", "actionlint", "shellcheck", "guard"],
    "config": ["config", "env var", "default", "flag", "option"],
    "tooling": ["script", "tooling", "helper", "utility"],
}

HEAVY_KEYWORDS = {
    "architecture": ["refactor", "redesign", "rewrite", "migration", "breaking"],
    "security-critical": ["injection", "auth bypass", "privilege", "rce", "xss", "csrf"],
    "feature": ["implement", "integration", "webhook", "provider", "e2e"],
    "data-model": ["schema", "database", "persistence", "storage", "state machine"],
}


def classify(title: str, body: str) -> tuple[str, str]:
    text = (title + " " + body).lower()
    # Evaluate HEAVY/security signals first to avoid under-classification
    # when an issue contains both lightweight and heavyweight keywords.
    for reason, keywords in HEAVY_KEYWORDS.items():
        if any(kw in text for kw in keywords):
            return "HEAVY", reason
    for reason, keywords in LIGHTWEIGHT_KEYWORDS.items():
        if any(kw in text for kw in keywords):
            return "LIGHTWEIGHT", reason
    # Default: if short body, likely lightweight
    if len(body) < 300:
        return "LIGHTWEIGHT", "short-description"
    return "HEAVY", "complex-scope"


def main():
    try:
        result = subprocess.run(
            ["gh", "issue", "list", "--state", "open", "--json",
             "number,title,body,assignees,url", "--limit", "200"],
            capture_output=True, text=True, check=True,
        )
    except (subprocess.CalledProcessError, FileNotFoundError) as e:
        print(f"ERROR: failed to fetch issues: {e}", file=sys.stderr)
        sys.exit(1)

    issues = json.loads(result.stdout)
    unassigned = [i for i in issues if not i.get("assignees")]

    lightweight = []
    heavy = []
    for issue in unassigned:
        category, reason = classify(issue["title"], issue.get("body", "") or "")
        entry = (issue["number"], issue["title"], reason)
        if category == "LIGHTWEIGHT":
            lightweight.append(entry)
        else:
            heavy.append(entry)

    print("LIGHTWEIGHT")
    if not lightweight:
        print("  (none)")
    for num, title, reason in lightweight:
        print(f"  #{num} [{reason}] {title}")

    print()
    print("HEAVY")
    if not heavy:
        print("  (none)")
    for num, title, reason in heavy:
        print(f"  #{num} [{reason}] {title}")


if __name__ == "__main__":
    main()
