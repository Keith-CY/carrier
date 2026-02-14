# Phase 1 Exit Gate Review and Release Readiness

## Overview

This document defines the exit-gate criteria, review process, and release-readiness checklist for Carrier Phase 1. The goal is to provide leadership with a clear Go/No-Go framework backed by measurable evidence.

## Exit Gate Criteria

### Functional Completeness

| Criterion | Target | Measurement |
|-----------|--------|-------------|
| Agent lifecycle (install/start/stop/upgrade) | All paths tested | E2E test matrix pass rate ≥ 95% |
| Crash-loop detection and recovery | Configurable, tested | Unit + integration tests green |
| Memory model (Per-Agent, Shared, Public) | Attach/detach working | E2E tests cover all modes |
| Base Agent triage + diagnose | LLM-assisted analysis functional | Manual + automated validation |
| Chat control (Telegram/Discord/Feishu) | Commands routed correctly | Gateway integration tests |
| WSL2 support | Documented and tested | WSL2 support matrix validated |

### Quality Gates

| Gate | Target |
|------|--------|
| Unit test coverage | ≥ 80% on daemon core packages |
| No P0 bugs open | 0 unresolved P0 issues |
| No security findings (critical/high) | 0 open critical/high findings |
| Documentation complete | PRD, Implementation Plan, Product Design aligned |
| CI pipeline green on main | All checks passing |

### Operational Readiness

- [ ] Upgrade path tested (at least one version-to-version upgrade)
- [ ] Audit logging functional and inspectable (`AuditBufferStatus` endpoint)
- [ ] Diagnosis handoff flow tested end-to-end
- [ ] Log redaction verified for sensitive data

## Review Process

1. **Metric Snapshot**: Collect TTFH (time-to-first-healthy), command success rate, diagnose coverage from CI artifacts and test runs.
2. **P0 Review**: Enumerate all open P0 issues; each must be resolved or explicitly deferred with risk acceptance.
3. **Evidence Assembly**: Gather test reports, coverage data, and manual validation results into a single review artifact.
4. **Decision Meeting**: Present evidence to stakeholders. Output: Go / No-Go / Conditional-Go with explicit conditions.

## Release Readiness Checklist

- [ ] All exit-gate criteria met (table above)
- [ ] CHANGELOG drafted for Phase 1 release
- [ ] Migration notes prepared (if any breaking changes)
- [ ] Phase 2 roadmap items identified and triaged
- [ ] Onboarding guide validated by at least one new user

## Phase 2 Entry Conditions

If Go decision is made:
1. Phase 1 tagged and released
2. Phase 2 planning issues created
3. Retrospective completed and lessons documented

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Undiscovered P0 in edge cases | Medium | High | Extend E2E matrix; add fuzzing in Phase 2 |
| WSL2 compatibility gaps | Low | Medium | WSL2 support matrix covers known distros |
| Memory model race conditions | Low | High | Mutex coverage in lifecycle service |

## Decision Template

```
Date: ____
Decision: [ ] Go  [ ] No-Go  [ ] Conditional-Go
Conditions (if any): ____
Evidence reviewed: ____
Participants: ____
```
