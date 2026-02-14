# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased] — Phase 1

### Features

- **Configuration file support (JSON)** — daemon loads settings from a JSON config file ([#213](https://github.com/Keith-CY/carrier/pull/213))
- **Health check endpoint** — monitoring endpoint for daemon health ([#215](https://github.com/Keith-CY/carrier/pull/215))
- **Rate limiting and request throttling** — gateway-level rate limiter ([#214](https://github.com/Keith-CY/carrier/pull/214))
- **Graceful shutdown** — SIGTERM/SIGINT handling for clean daemon shutdown ([#209](https://github.com/Keith-CY/carrier/pull/209))
- **Structured logging with slog** — replace ad-hoc logging with Go's slog ([#210](https://github.com/Keith-CY/carrier/pull/210))
- **Runtime pre-flight checks** — structured env var, port conflict & command validation ([#179](https://github.com/Keith-CY/carrier/pull/179))
- **Memory platform lifecycle** — memory mount policy and lifecycle management ([#181](https://github.com/Keith-CY/carrier/pull/181))
- **Enhanced manifest schema** — PRD-required fields in manifest and catalog ([#176](https://github.com/Keith-CY/carrier/pull/176))

### Bug Fixes

- **SessionStore cleanup** — add cleanup method and size accessors to gateway SessionStore ([#225](https://github.com/Keith-CY/carrier/pull/225))
- **Kanban assignee splitting** — split workload by individual assignee ([#193](https://github.com/Keith-CY/carrier/pull/193))

### Security

- **Crypto-secure token generation** — replace sequential token generation with crypto-secure tokens ([#227](https://github.com/Keith-CY/carrier/pull/227))
- **Input sanitization audit** — audit and harden command execution inputs ([#211](https://github.com/Keith-CY/carrier/pull/211))

### Refactor

- **Context propagation** — propagate `context.Context` through lifecycle Service methods ([#226](https://github.com/Keith-CY/carrier/pull/226))
- **Lifecycle file split** — split `lifecycle/service.go` into focused files ([#236](https://github.com/Keith-CY/carrier/pull/236))

### Docs

- **Package-level godoc** — add godoc comments to all daemon internal packages ([#224](https://github.com/Keith-CY/carrier/pull/224))
- **Command contract docs** — document command contracts and cross-provider consistency ([#182](https://github.com/Keith-CY/carrier/pull/182))
- **Configurable disk thresholds** — document disk threshold and state-transition policy ([#197](https://github.com/Keith-CY/carrier/pull/197))
- **PR template alignment** — align checklist item tone ([#186](https://github.com/Keith-CY/carrier/pull/186))
- **README updates** — document kanban workflow env vars, link to workflow file ([#178](https://github.com/Keith-CY/carrier/pull/178), [#199](https://github.com/Keith-CY/carrier/pull/199))
- **CI documentation** — explain E2E skip rationale, GitHub fallback token context ([#175](https://github.com/Keith-CY/carrier/pull/175), [#190](https://github.com/Keith-CY/carrier/pull/190))
- **E2E contributing hint** — add hint when carrier CLI is missing ([#180](https://github.com/Keith-CY/carrier/pull/180))

### Tests

- **Daemon test coverage report** — package-level coverage report ([#212](https://github.com/Keith-CY/carrier/pull/212))
- **E2E integration tests** — daemon-gateway integration tests ([#177](https://github.com/Keith-CY/carrier/pull/177))
- **Gateway token-store dedup** — deduplicate token-store setup in consume tests ([#189](https://github.com/Keith-CY/carrier/pull/189))
- **Test consolidation** — combine redundant expect calls, collapse duplicated exit paths ([#195](https://github.com/Keith-CY/carrier/pull/195), [#196](https://github.com/Keith-CY/carrier/pull/196), [#200](https://github.com/Keith-CY/carrier/pull/200))

### CI

- **Makefile** — add Makefile for common development tasks ([#223](https://github.com/Keith-CY/carrier/pull/223))
- **Remove redundant CI test** — already covered by `go test ./...` ([#196](https://github.com/Keith-CY/carrier/pull/196))
