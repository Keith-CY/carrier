"use strict";

const fs = require("node:fs");
const path = require("node:path");

const DEFAULT_ALLOWED_FILE = "daemon/credentialstore/store.go";
const LOADER_DEFINITION_REGEX = /(^|\n)func\s+(loadCredential|loadCredentialFromKeychain|loadCredentialFromFile)\s*\(/m;

function walkFiles(rootDir, out = []) {
  if (!fs.existsSync(rootDir)) {
    return out;
  }

  const entries = fs.readdirSync(rootDir, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.name === ".git" || entry.name === "node_modules") {
      continue;
    }
    const full = path.join(rootDir, entry.name);
    if (entry.isDirectory()) {
      walkFiles(full, out);
      continue;
    }
    if (entry.isFile() && full.endsWith(".go")) {
      out.push(full);
    }
  }

  return out;
}

function normalizePath(filePath) {
  return filePath.replace(/\\/g, "/").replace(/^\.\//, "");
}

function containsCredentialLoaderDefinition(content) {
  return LOADER_DEFINITION_REGEX.test(String(content || ""));
}

function collectCredentialLoaderDefinitionFiles(rootDir = ".") {
  const matches = [];
  for (const filePath of walkFiles(rootDir)) {
    const content = fs.readFileSync(filePath, "utf8");
    if (containsCredentialLoaderDefinition(content)) {
      matches.push(normalizePath(filePath));
    }
  }
  return matches.sort();
}

function validateCredentialLoaderDedup(fileList, allowedFile = DEFAULT_ALLOWED_FILE) {
  const normalizedAllowed = normalizePath(allowedFile);
  const normalized = Array.from(new Set((fileList || []).map((item) => normalizePath(item)))).sort();
  const unexpected = normalized.filter((filePath) => filePath !== normalizedAllowed);
  const hasAllowed = normalized.includes(normalizedAllowed);

  return {
    hasAllowed,
    unexpected,
    allMatches: normalized,
  };
}

function runCli(argv = process.argv) {
  const rootArgIndex = argv.indexOf("--root");
  const root = rootArgIndex >= 0 ? argv[rootArgIndex + 1] : ".";
  const allowedArgIndex = argv.indexOf("--allowed");
  const allowed = allowedArgIndex >= 0 ? argv[allowedArgIndex + 1] : DEFAULT_ALLOWED_FILE;

  if (!root) {
    throw new Error("--root requires a value");
  }
  if (!allowed) {
    throw new Error("--allowed requires a value");
  }

  const files = collectCredentialLoaderDefinitionFiles(root);
  const result = validateCredentialLoaderDedup(files, allowed);

  if (!result.hasAllowed) {
    throw new Error(`Expected credential loader definitions in ${normalizePath(allowed)}, but none were found.`);
  }
  if (result.unexpected.length > 0) {
    throw new Error(
      [
        "Credential loader definitions must stay deduplicated in the shared package.",
        `Allowed file: ${normalizePath(allowed)}`,
        `Unexpected definitions: ${result.unexpected.join(", ")}`,
      ].join(" "),
    );
  }

  console.log(`Credential loader dedupe check passed (${result.allMatches.length} definition file).`);
}

if (require.main === module) {
  runCli();
}

module.exports = {
  DEFAULT_ALLOWED_FILE,
  collectCredentialLoaderDefinitionFiles,
  containsCredentialLoaderDefinition,
  normalizePath,
  runCli,
  validateCredentialLoaderDedup,
};
