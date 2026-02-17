export type ParsedDownloadPath = {
  token: string;
  requestedFileName: string;
};

export type FileNameCompareResult = {
  expectedFileName: string;
  requestedFileName: string;
  matches: boolean;
};

export function parseDownloadPath(pathname: string): ParsedDownloadPath | null {
  const parts = pathname.split("/").filter(Boolean);
  if (parts.length !== 3 || parts[0] !== "downloads") {
    return null;
  }

  const token = parts[1] ?? "";
  if (!token) {
    return null;
  }

  try {
    const requestedFileName = decodeURIComponent(parts[2] ?? "");
    return { token, requestedFileName };
  } catch {
    return null;
  }
}

export function normalizeDownloadFileName(fileName: string): string {
  return fileName.trim();
}

export function expectedFileNameFromRef(fileRef: string): string {
  let separator: string | RegExp;
  if (/^[A-Za-z]:[\\/]/.test(fileRef)) {
    // Windows paths may mix / and \, so split on both
    separator = /[\/\\]/;
  } else if (fileRef.includes("/")) {
    separator = "/";
  } else {
    separator = "\\";
  }
  const parts = fileRef.split(separator);
  return parts.pop() || "artifact.bin";
}

export function compareRequestedFileName(input: {
  requestedFileName: string;
  fileRef: string;
}): FileNameCompareResult {
  const expectedFileName = normalizeDownloadFileName(expectedFileNameFromRef(input.fileRef));
  const requestedFileName = normalizeDownloadFileName(input.requestedFileName);
  return {
    expectedFileName,
    requestedFileName,
    matches: expectedFileName === requestedFileName,
  };
}

export function buildContentDisposition(filename: string): string {
  const hasNonASCII = /[^\x00-\x7F]/.test(filename);
  if (hasNonASCII) {
    const asciiFallback = filename.replace(/[^\x00-\x7F]/g, "_");
    const escapedFallback = asciiFallback.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
    const encodedFilename = encodeURIComponent(filename);
    return `attachment; filename="${escapedFallback}"; filename*=UTF-8''${encodedFilename}`;
  }

  const escapedFilename = filename.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
  return `attachment; filename="${escapedFilename}"`;
}
