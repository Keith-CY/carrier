# Carrier Big-Bang Restructure Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Perform a one-shot top-level refactor that separates shared concerns, extracts Base Agent from daemon, promotes Gateway to an independent Go top-level module, and aligns tests/docs/automation with the new architecture.

**Architecture:** The repository is reorganized around explicit module boundaries: `webui -> gateway -> daemon -> shared` and `webui -> gateway -> baseagent -> shared`. Shared cross-cutting code (`config`, `redact`) moves into `shared`. Gateway runtime moves out of `gateway` into top-level `gateway`. Base Agent moves out of `baseagent` into top-level `baseagent` with daemon-facing adapters.

**Tech Stack:** Go modules, Bash scripts, GitHub Actions workflows, repository documentation.

---

### Task 1: Establish New Module Skeletons

**Files:**
- Create: `shared/go.mod`
- Create: `baseagent/go.mod`
- Create: `gateway/go.mod`
- Modify: `go.mod`
- Test: `go test ./...` (module-local smoke)

1. Create new top-level module manifests for `shared`, `baseagent`, `gateway`.
2. Update root `go.mod` to reference and replace all local modules.
3. Run module-local smoke checks:
   - `cd shared && go test ./...`
   - `cd baseagent && go test ./...`
   - `cd gateway && go test ./...`

### Task 2: Extract Shared Packages

**Files:**
- Move: `shared/config/* -> shared/config/*`
- Move: `shared/redact/* -> shared/redact/*`
- Modify: daemon/gateway/baseagent import callsites
- Test: `go test ./shared/...` + impacted package tests

1. Move `config` and `redact` implementations/tests into `shared`.
2. Update all imports from `carrier/shared/config|redact` to `carrier/shared/config|redact`.
3. Re-run related tests.

### Task 3: Extract Base Agent Module

**Files:**
- Move: `baseagent/* -> baseagent/*`
- Modify: `baseagent/runtime.go`, `baseagent/llm.go`, `baseagent/triager_llm.go`
- Modify: `daemon/server/server.go`, `daemon/internal/lifecycle/*`
- Test: `cd baseagent && go test ./...`, daemon lifecycle/server tests

1. Move the base agent package to top-level `baseagent`.
2. Replace daemon-internal dependencies with shared dependencies or package-level interfaces.
3. Update daemon callsites/imports to consume `carrier/baseagent`.
4. Run baseagent and daemon test suites.

### Task 4: Promote Gateway to Top-Level Module

**Files:**
- Move: `gateway/* -> gateway/*`
- Remove: `daemon/gateway/gateway.go`
- Modify: `cmd/carrier/main.go` import/calls
- Modify: test and build scripts that target old gateway paths
- Test: `cd gateway && go test ./...`, `cd daemon && go test ./...`

1. Move gateway runtime and tests to top-level `gateway`.
2. Update import paths from `carrier/gateway` and old internal package paths.
3. Ensure gateway no longer relies on daemon `internal` visibility.
4. Run gateway and daemon tests.

### Task 5: Rename Top-Level Test Directory

**Files:**
- Keep: `tests/` (no rename to `test/`)
- Modify: scripts and workflows that reference `tests/`
- Test: `./scripts/run-e2e-tests.sh` and CI command parity checks

1. Keep top-level `tests/` as the canonical directory name.
2. Update shell scripts, docs, and workflow paths.
3. Validate E2E entrypoint still resolves expected files.

### Task 6: Update Automation and Build Pipelines

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `scripts/run-all-tests.sh`
- Modify: `scripts/run-e2e-tests.sh`

1. Point test/build jobs at new module/package locations.
2. Keep command names stable where possible.
3. Verify workflow syntax validity and local command parity.

### Task 7: Update Documentation and Onboard Guide

**Files:**
- Modify: `README.md`
- Modify: `ARCHITECTURE.md`
- Modify: `docs/current-architecture.md`

1. Refresh repository map and dependency directions.
2. Rewrite onboarding section in `README.md` as an explicit step-by-step runbook.
3. Remove stale references to moved packages/paths.

### Task 8: End-to-End Verification

**Files:**
- Verify (no new files required unless fixes): all impacted modules/scripts/docs

1. Run:
   - `go test ./...`
   - `cd daemon && go test ./...`
   - `cd gateway && go test ./...`
   - `cd baseagent && go test ./...`
   - `cd webui && go test ./...`
   - `./scripts/run-all-tests.sh`
2. Fix regressions until green.
3. Publish a migration summary with known risks and cleanup follow-ups.
