# Carrier

Phase 1 scaffold for the Agent Installation Platform.

## Current scope
- Runtime: local host (macOS/Linux), WSL2 (Windows)
- Full lifecycle target: OpenClaw
- Candidate-only agents: Pi Mono, NanoClaw, Pico Claw
- Memory model: Per-Agent, Shared, Public
- Gateway providers: Telegram, Discord, Feishu

## Repository layout
- `daemon/`: Go daemon lifecycle, catalog, Base Agent triage interfaces
- `gateway/`: TypeScript gateway contracts and routing scaffold
- `catalog/`: OpenClaw manifest and candidate agent list
- `docs/`: Product and delivery documentation

## Development flow
- Start work from `origin/main` using a new `codex/*` branch and worktree.
- Submit all feature updates via Pull Request.
- Use GitHub Actions as test source of truth.

See `CONTRIBUTING.md` for command examples and required process.
