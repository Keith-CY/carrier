# Carrier

> The command bridge for AI work — dispatch specialist agents, control memory flow, and ship outcomes you can audit.

Carrier is a **local-first execution and knowledge control plane** for teams doing real agent operations.

Instead of one opaque chat thread, Carrier turns intent into explicit runs:

- planned tasks
- assigned workers
- tracked artifacts
- verifiable results

## Why Carrier feels different

Most agent stacks break at production scale because too much is implicit:

- hidden behavior
- memory drift and context leakage
- weak operational visibility
- no clean replay/rollback story

Carrier makes those concerns first-class:

- **Deterministic execution** — explicit run graph, retries, reruns, and clones
- **Scoped memory discipline** — attach, distill, and grant memory with clear boundaries
- **Worker fleet control** — manage local + remote agents with lifecycle APIs
- **Evidence-first operations** — artifacts, audits, and exportable execution traces

## What you can do with Carrier today

- Orchestrate complex goals into structured execution runs
- Operate managed agents (`openclaw`, `picoclaw`, `zeroclaw`) across local and remote hosts
- Search/attach/distill memory packages through a dedicated knowledge plane
- Run deterministic remote install flows with pre-check/post-check and rollback semantics
- Review execution lineage and outcomes from CLI or WebUI

## Product surfaces

- **CLI** — deterministic operations and automation
- **WebUI** — onboarding, memory, executions, and remote control
- **Gateway APIs** — host/instance lifecycle and control-plane integration

## Read the docs

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
