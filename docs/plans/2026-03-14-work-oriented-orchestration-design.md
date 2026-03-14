# Carrier Work-Oriented Orchestration Design

**Scope:** Define a first-class work model for Carrier so long-running engineering workflows are represented as durable work items and runs, while reusing the existing execution, policy, evidence, memory, and worker control plane.

## Goal

Make Carrier work-oriented rather than prompt-oriented.

Today, Carrier is execution-first: a user gives a goal, Carrier decomposes it, and one execution is created. That works for short-lived orchestration, but it is too thin for longer-running engineering flows that need durable ownership, resume, review, publishing, and workspace isolation.

The new design introduces a stable work model:

- `Project` defines the code source and workflow contract.
- `WorkItem` defines the long-lived task humans care about.
- `Run` defines one execution attempt for that work item.
- `Execution` remains the existing Carrier control-plane primitive under the run.
- `TaskGraph` remains internal planner detail.

## Non-Goals

- Replacing the existing execution plane with a second orchestrator.
- Making GitHub issues or PRs the source of truth.
- Exposing internal task decomposition as the primary UI object.
- Introducing GitHub App hosting, Linear, or Jira in v1.
- Maintaining backward-compatibility for old `~/.carrier` root layout.

## Why Work Items Exist Separately From Task Decomposition

Carrier already has decomposition and planning. That is not the same thing as durable work tracking.

Decomposition answers: "How should this run be executed right now?"

A work item answers: "What is the long-lived task, why does it exist, what counts as done, and what run is currently responsible for it?"

Those are different concerns.

If Carrier only stores planner outputs, it loses:

- durable ownership and queueing
- stable task identity across retries and resumes
- publication state and review readiness
- external source linking
- human-facing lifecycle independent from one execution attempt

So the model must stay layered:

- `WorkItem`: human and product object
- `Run`: machine supervision object
- `Execution`: execution-plane object
- `TaskGraph`: internal planner object

## Current Carrier Baseline

Carrier already provides the lower layers needed for this design:

- execution creation, policy, evidence, audit, retry/rerun/clone
- worker leases and inventory
- memory contract and provenance
- managed agents, heartbeat, launcher, cron, subagent jobs
- local and isolated runtime substrates

That means the new system should not create a parallel orchestration stack. It should bind new work-oriented objects onto the existing execution plane.

## Core Model

### Project

A `Project` is the stable source substrate for work.

It defines:

- canonical source repo
- default branch
- workflow contract path, defaulting to `WORKFLOW.md`
- workflow digest
- sync state

Carrier owns the canonical project substrate in:

- `~/.carrier/projects/<project-id>/`

This avoids depending on a developer's ad hoc local checkout and gives Carrier a deterministic base for worktree creation and run recovery.

### WorkItem

A `WorkItem` is the primary product object.

It defines:

- title and description
- acceptance criteria
- priority and labels
- source metadata
- current work state
- latest run linkage

v1 is local-first:

- Carrier native work items are the source of truth.
- External systems are adapters.

### Run

A `Run` is one supervised attempt to advance a work item.

It owns:

- workspace selection
- isolation backend selection
- heartbeat and lease
- phase progression
- execution binding
- verification
- publish status
- cleanup state

A work item may have many runs over time, but only one active run at once.

### Execution

`Execution` remains the existing Carrier execution-plane primitive.

A run creates one primary execution and delegates the actual task graph to the existing planner/execution lifecycle.

This keeps policy, evidence, audit, worker scheduling, and memory behavior unified.

### TaskGraph

The task graph remains internal.

It is still visible in detail/debug contexts, but v1 does not promote it to a top-level product object.

## Local Storage Layout

Carrier adopts a three-root local substrate:

- `~/.carrier/app`
- `~/.carrier/projects`
- `~/.carrier/works`

### `~/.carrier/app`

Application-level state:

- config
- credentials
- managed instance state
- gateway and daemon stores
- global indexes

### `~/.carrier/projects`

Canonical project substrate:

- bare repo mirror or canonical checkout
- derived worktrees
- per-project workflow snapshot state

### `~/.carrier/works`

Work-oriented control state:

- work items
- runs
- append-only event logs
- publish records
- verification records
- evidence indexes

This design deliberately keeps work state outside the business repo. It avoids nested git inside product repos and makes resume, reclaim, and cleanup simpler.

## GitHub's Role

GitHub is not the source of truth in v1.

GitHub acts as an adapter with two responsibilities:

- import external issue/PR context into a Carrier work item
- publish run outcomes back to GitHub via comments, branches, PR drafts, or status notes

v1 explicitly does not do continuous bidirectional state sync. GitHub labels and issue state do not become the primary Carrier state machine.

This keeps conflict handling tractable and prevents the first version from becoming a GitHub workflow engine.

## Workflow Contract

`WORKFLOW.md` is the per-project workflow contract.

It is not a top-level product object. It is a repo-scoped execution contract that tells Carrier how to initialize, verify, and publish work for that project.

v1 contract areas:

- selection hints
- initialization checklist
- verification commands or expectations
- publish rules
- stop/resume hints

Carrier snapshots the resolved workflow contract into run artifacts and evidence, so each run records the contract it actually followed.

## Isolation Model

Isolation is layered.

### 1. Coordination Isolation

Each run is supervised independently.

It gets:

- its own lease
- its own heartbeat
- phase progression
- reclaim and cleanup handling

This gives Carrier the actor-like supervision behavior needed for long-running workflows without pretending goroutines provide system isolation.

### 2. Workspace Isolation

Each run uses a dedicated git worktree derived from the canonical project source.

This isolates:

- file mutations
- branch context
- workflow snapshots
- verification artifacts

But worktrees alone are not enough.

### 3. Runtime Isolation

Carrier selects an execution backend per run:

- `local_sandboxed`
- `managed_isolated`
- `remote_vm`

Default is:

- worktree + workspace sandbox

Escalation happens for higher-risk runs, concurrency pressure, or explicit policy requirements.

This directly reuses Carrier's existing isolation substrate:

- Linux bubblewrap
- macOS Lima + bubblewrap
- Windows WSL2 + bubblewrap
- remote managed hosts

### Why This Matters

A git worktree prevents code overlap. It does not prevent:

- port collisions
- shared `HOME` pollution
- shared temp/cache collisions
- runaway subprocess interference
- network policy leakage
- secrets bleed between runs

Those are runtime isolation concerns, so they must remain separate from workspace isolation.

## Supervisor Model

Carrier should implement run supervision with actor-like behavior, but not rely on actor semantics for security.

Each run supervisor manages:

- claim/lease
- workspace provisioning
- execution creation
- verification
- publishing
- cleanup
- heartbeat and stale-run reclaim

This delivers the operational benefits commonly associated with actor systems:

- isolated state machines
- restartability
- child-phase supervision
- clear recovery points

But execution safety still depends on sandbox and runtime backends, not on the supervisor abstraction itself.

## User-Facing Flow

v1 user flow becomes:

1. Create or import a work item.
2. Claim it or allow scheduler claim.
3. Start a run.
4. Carrier provisions workspace and picks runtime backend.
5. Carrier creates one execution and internally decomposes the work.
6. Carrier verifies results.
7. Carrier publishes outcomes locally and optionally to GitHub.
8. User resumes, reviews, or closes the work item.

This changes Carrier from:

- `goal -> execution`

into:

- `work item -> run -> execution`

## Why This Is Better Than GitHub-First

If Carrier stayed GitHub-first, the system would keep outsourcing its durable task identity and state machine to GitHub.

That would make:

- local-only projects weaker
- run recovery less coherent
- project workflow contracts harder to own
- non-GitHub adapters harder to add later

By making Carrier's local work item model primary, the system keeps the durable machine state inside Carrier and treats GitHub as one integration surface.

## Deferred Work

Not in v1:

- GitHub App deployment
- Linear/Jira adapters
- complex publish workflows
- rich cross-project dependency graphs
- end-user exposure of task graph editing
- policy-driven automatic merge or review actions

## Decision Summary

v1 is defined as:

- local-first native work items
- GitHub import/publish adapter only
- canonical project substrate under `~/.carrier/projects`
- work state under `~/.carrier/works`
- execution-native reuse of existing Carrier execution plane
- internal planner task graph hidden by default
- worktree plus tiered runtime isolation
