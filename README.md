# Carrier

> A local-first control plane for running AI worker fleets with explicit memory, deterministic execution, and auditable results.

Carrier is for teams who want more than “chat with a model.”
It turns goals into structured execution runs, routes work to managed agents, tracks artifacts/evidence, and keeps memory attached to the right worker scopes.

## Why Carrier

Most agent setups break down on three things:

- **Control** — too much hidden behavior
- **Memory discipline** — context drifts, leaks, or gets lost
- **Operations** — hard to inspect, replay, or prove what happened

Carrier is built to solve those directly.

## What Carrier Does

- **Goal → execution graph** with run history, lineage, retries, reruns, and clones
- **Managed agent lifecycle** for local and remote workers (`openclaw`, `picoclaw`, `zeroclaw`)
- **Knowledge-plane operations** (search, attach, distill, scope-aware memory grants)
- **Deterministic remote install flow** with pre-check/post-check and rollback semantics
- **Evidence-first operations** via artifacts, audits, and exportable execution records

## Product Surfaces

- **CLI** for deterministic operations and automation
- **WebUI** for onboarding, memory, executions, and remote control
- **Gateway APIs** for host/instance lifecycle and control-plane integration

## Read the Docs

- Full setup & operations guide: [`docs/guides/getting-started-and-operations.md`](./docs/guides/getting-started-and-operations.md)
- CLI reference: [`docs/carrier-cli.md`](./docs/carrier-cli.md)
- Task-first flow: [`docs/task-first-quickstart.md`](./docs/task-first-quickstart.md)
- Architecture: [`ARCHITECTURE.md`](./ARCHITECTURE.md)
- Current architecture snapshot: [`docs/current-architecture.md`](./docs/current-architecture.md)
- Remote APIs:
  - [`docs/api/remote-sync-api.md`](./docs/api/remote-sync-api.md)
  - [`docs/api/remote-codeagent-api.md`](./docs/api/remote-codeagent-api.md)

## Contributing

- Contributor guide: [`CONTRIBUTING.md`](./CONTRIBUTING.md)
- Security policy: [`SECURITY.md`](./SECURITY.md)
- Changelog: [`CHANGELOG.md`](./CHANGELOG.md)
