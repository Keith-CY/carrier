# Carrier Web UI Architecture (Decoupled)

This document captures the browser-first direction for Carrier UI.

## Decision

- Remove embedded desktop GUI runtime (Gio-based module).
- Keep `carrier` runtime focused on daemon/gateway services.
- Build/serve UI as a decoupled browser client over localhost APIs.

## Why

- Stronger separation between runtime and presentation layers.
- Faster UI iteration with web technologies.
- Cleaner CI/runtime dependency surface (no GUI build deps in core runtime path).

## Boundaries

- **Daemon/Gateway**: system operations, agent lifecycle, logs, onboarding APIs.
- **Web UI**: form UX, orchestration views, filtering/search, live log console.
- **Contract**: strict API boundary over HTTP/SSE/WebSocket on localhost.

## Migration Guidance

1. Keep APIs backward-compatible where possible.
2. Avoid UI logic in daemon packages.
3. Introduce versioned API contracts for UI consumption.
4. Add integration tests that validate Web UI contract behavior independent of UI framework.
