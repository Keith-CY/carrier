"use strict";

const fs = require("node:fs");
const path = require("node:path");

const FILE_EXTENSIONS = new Set([
  ".appimage",
  ".deb",
  ".dmg",
  ".exe",
  ".msi",
  ".pkg",
  ".rpm",
  ".zip",
]);

const ARCH_PATTERNS = {
  amd64: /(x64|x86_64|amd64)/i,
  arm64: /(arm64|aarch64)/i,
};

const EXTENSION_PRIORITY = {
  linux: [".appimage", ".deb", ".rpm", ".zip", ".pkg", ".exe", ".msi", ".dmg", ".app"],
  darwin: [".app", ".dmg", ".pkg", ".zip", ".appimage", ".deb", ".rpm", ".msi", ".exe"],
  windows: [".msi", ".exe", ".zip", ".app", ".dmg", ".appimage", ".deb", ".rpm", ".pkg"],
};

function normalizePath(value) {
  return String(value).replace(/\\/g, "/");
}

function isUnderBundleOutput(fullPath) {
  return normalizePath(fullPath).includes("/release/bundle/");
}

function walkDir(dir, visit) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    const shouldDescend = visit(fullPath, entry);
    if (entry.isDirectory() && shouldDescend !== false) {
      walkDir(fullPath, visit);
    }
  }
}

function collectTauriBundleCandidates(targetDir) {
  if (!targetDir || !fs.existsSync(targetDir)) {
    return [];
  }

  const candidates = [];
  walkDir(targetDir, (fullPath, entry) => {
    const entryName = entry.name;
    const lowerName = entryName.toLowerCase();

    if (entry.isDirectory()) {
      if (lowerName.endsWith(".app") && isUnderBundleOutput(fullPath)) {
        candidates.push({ path: fullPath, ext: ".app" });
        return false;
      }
      return true;
    }

    if (!entry.isFile() || !isUnderBundleOutput(fullPath)) {
      return false;
    }

    const ext = path.extname(lowerName);
    if (FILE_EXTENSIONS.has(ext)) {
      candidates.push({ path: fullPath, ext });
    }
    return false;
  });

  return candidates.sort((a, b) => normalizePath(a.path).localeCompare(normalizePath(b.path)));
}

function getArchScore(candidatePath, goarch) {
  const arch = String(goarch || "").trim().toLowerCase();
  if (!arch || !ARCH_PATTERNS[arch]) {
    return 1;
  }

  const normalized = normalizePath(candidatePath).toLowerCase();
  const hasAmd64Token = ARCH_PATTERNS.amd64.test(normalized);
  const hasArm64Token = ARCH_PATTERNS.arm64.test(normalized);

  if (arch === "amd64") {
    if (hasAmd64Token) {
      return 2;
    }
    if (hasArm64Token) {
      return 0;
    }
    return 1;
  }

  if (arch === "arm64") {
    if (hasArm64Token) {
      return 2;
    }
    if (hasAmd64Token) {
      return 0;
    }
    return 1;
  }

  return 1;
}

function getExtensionPriority(goos, ext) {
  const os = String(goos || "").trim().toLowerCase();
  const order = EXTENSION_PRIORITY[os] || [];
  const idx = order.indexOf(ext);
  return idx === -1 ? order.length + 1 : idx;
}

function selectTauriBundle(candidates, options = {}) {
  if (!Array.isArray(candidates) || candidates.length === 0) {
    return null;
  }

  const goos = String(options.goos || "").trim().toLowerCase();
  const goarch = String(options.goarch || "").trim().toLowerCase();

  const scored = candidates.map((candidate) => ({
    ...candidate,
    normalizedPath: normalizePath(candidate.path),
    archScore: getArchScore(candidate.path, goarch),
    extPriority: getExtensionPriority(goos, candidate.ext),
  }));

  scored.sort((left, right) => {
    if (left.archScore !== right.archScore) {
      return right.archScore - left.archScore;
    }
    if (left.extPriority !== right.extPriority) {
      return left.extPriority - right.extPriority;
    }
    return left.normalizedPath.localeCompare(right.normalizedPath);
  });

  return scored[0];
}

function parseArgs(argv) {
  const out = {};
  for (let idx = 2; idx < argv.length; idx += 2) {
    const key = argv[idx];
    const value = argv[idx + 1];
    if (!key || !key.startsWith("--")) {
      throw new Error(`Unexpected argument "${key}"`);
    }
    if (value === undefined || value.startsWith("--")) {
      throw new Error(`Missing value for argument "${key}"`);
    }
    out[key.slice(2)] = value;
  }
  return out;
}

function runCli() {
  const args = parseArgs(process.argv);
  const targetDir = args["target-dir"];
  const goos = args.goos || "";
  const goarch = args.goarch || "";

  if (!targetDir) {
    throw new Error("--target-dir is required");
  }

  const candidates = collectTauriBundleCandidates(targetDir);
  const selected = selectTauriBundle(candidates, { goos, goarch });
  if (!selected) {
    throw new Error(`No Tauri bundle found under ${targetDir}/**/release/bundle`);
  }

  if (selected.archScore === 0) {
    console.error(
      `warning: selected bundle "${selected.path}" does not appear to match requested architecture "${goarch}"`,
    );
  }

  process.stdout.write(`${selected.path}\n`);
}

if (require.main === module) {
  runCli();
}

module.exports = {
  collectTauriBundleCandidates,
  getArchScore,
  selectTauriBundle,
};
