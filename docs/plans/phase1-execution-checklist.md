# Phase 1 Execution Checklist and Exit Gate Criteria

> **Issue:** #76 · **Status:** Plan · **Track:** A3
>
> **Note:** This is a historical planning checklist and is not the canonical implementation source.
> For current architecture and operational source-of-truth, use [`docs/current-architecture.md`](../current-architecture.md), [`docs/Agent_Installation_Platform_PRD.md`](../Agent_Installation_Platform_PRD.md), and [`ARCHITECTURE.md`](../../ARCHITECTURE.md).

## Goals

- Provide a milestone checklist template that every Phase 1 work item must satisfy before it can be considered "done."
- Define clear Definition-of-Done (DoD) criteria per milestone so reviewers and contributors share the same bar.
- Build a risk register that surfaces blockers early and tracks mitigations.
- Codify gate definitions that map directly to Kanban column transitions.

## Non-Goals

- Tooling implementation for automated gate enforcement (Phase 2).
- CI/CD pipeline changes—this document covers process, not automation.
- Scope beyond Phase 1 milestones.

---

## 1. Milestone Checklist Template

Every milestone tracked in Phase 1 **must** have a checklist entry in the following format:

| Field | Description | Example |
|-------|-------------|---------|
| **ID** | Unique milestone identifier | `M-B1` |
| **Title** | Short description | Memory Package Spec |
| **Owner** | GitHub handle of the DRI | `@dev01lay2` |
| **Due Date** | Target completion date (ISO 8601) | `2026-03-01` |
| **Dependencies** | List of milestone IDs this blocks on | `M-A1, M-A2` |
| **Risk Level** | `high` / `medium` / `low` | `medium` |
| **Status** | `not-started` / `in-progress` / `review` / `done` | `in-progress` |

### Checklist Items per Milestone

- [ ] Design document merged
- [ ] Implementation PR(s) opened
- [ ] Unit tests passing (≥80 % coverage for new code)
- [ ] Integration / E2E tests passing (where applicable)
- [ ] Documentation updated (README, API docs, examples)
- [ ] Peer review approved (≥1 reviewer)
- [ ] No open `high` or `critical` issues linked to milestone
- [ ] Demo or walkthrough recorded/presented to team

---

## 2. Definition of Done (DoD) per Milestone

### A-track (Planning & Process)

| Criterion | Required |
|-----------|----------|
| Plan document merged to `docs/plans/` | ✅ |
| Acceptance criteria listed and reviewable | ✅ |
| Risk register entry created | ✅ |
| Stakeholder sign-off (PR approval) | ✅ |

### B-track (Core Features)

| Criterion | Required |
|-----------|----------|
| All A-track criteria | ✅ |
| Feature code merged to `main` | ✅ |
| Unit test coverage ≥ 80 % for new packages | ✅ |
| API contract documented (OpenAPI / protobuf / schema) | ✅ |
| No regressions in `go test ./...` | ✅ |

### C-track (Operations & Quality)

| Criterion | Required |
|-----------|----------|
| All A-track criteria | ✅ |
| Operational runbook or config merged | ✅ |
| E2E test cases defined and at least 1 automated | ✅ |
| Metrics / log evidence of correctness | ✅ |

---

## 3. Risk Register

| Risk ID | Description | Level | Likelihood | Impact | Mitigation | Owner | Status |
|---------|-------------|-------|------------|--------|------------|-------|--------|
| R-01 | Memory schema changes late in Phase 1 break downstream importers | high | medium | high | Lock schema after B1 sign-off; version bump required for breaking changes | `@dev01lay2` | open |
| R-02 | Disk protection logic not tested under real low-disk conditions | medium | medium | medium | Add CI job with constrained tmpfs; manual test on staging | `@carrier-maintainers` | open |
| R-03 | E2E test matrix too large to run in CI time budget | medium | low | medium | Prioritize happy-path; run full matrix nightly | `@carrier-maintainers` | open |
| R-04 | Dependency on external runtime availability for install tests | low | low | low | Mock runtime binaries for CI; real binaries for nightly | `@carrier-maintainers` | open |

### Risk Levels

- **High** — Blocks Phase 1 exit; must have mitigation plan within 1 week of identification.
- **Medium** — Degrades quality or delays timeline; mitigation should be planned within 2 weeks.
- **Low** — Acceptable risk; document and revisit if conditions change.

---

## 4. Kanban Gate Definitions

The project Kanban board uses the following columns. Transitioning between columns requires the listed gate criteria to be met.

### Column: Backlog → In Progress

- [ ] Issue has a clear title, description, and acceptance criteria
- [ ] Owner assigned
- [ ] Dependencies identified and not blocked

### Column: In Progress → In Review

- [ ] All code committed and pushed to a feature branch
- [ ] PR opened with passing CI checks
- [ ] Self-review completed (no pending placeholders, no debug code)
- [ ] Linked to parent issue via `Closes #N`

### Column: In Review → Done

- [ ] At least 1 approving review
- [ ] All CI checks green
- [ ] DoD criteria for the relevant track satisfied
- [ ] Risk register updated if new risks discovered
- [ ] PR squash-merged to `main`

### Column: Done → Released (Phase 1 Exit Gate)

All milestones in `Done` **and**:

- [ ] Full E2E test matrix passing (see #85)
- [ ] No open `high` risk items in the register
- [ ] All plan documents merged
- [ ] Stakeholder sign-off recorded in a tracking issue

---

## Acceptance Criteria

1. This document is merged and referenced from the project README or top-level docs index.
2. Every Phase 1 issue has a corresponding checklist entry using the template above.
3. Risk register contains at least the initial set of identified risks.
4. Kanban column definitions are documented and agreed upon by the team.

## Timeline Estimate

| Task | Estimate |
|------|----------|
| Draft and review this document | 1 day |
| Populate checklist for all existing milestones | 1 day |
| Team review and sign-off | 2 days |
| **Total** | **~4 days** |
