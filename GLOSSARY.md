# Carrier Glossary

## 1. Product and Architecture

- **Carrier**
  - The overall local-first control plane for onboarding, managing, and operating AI agent instances (local and remote).
- **Daemon**
  - The local runtime service that executes agent lifecycle operations and holds authoritative local state.
- **Gateway**
  - The transport and policy layer that normalizes requests from chat providers and forwards them to Daemon APIs.
- **WebUI**
  - The browser-based local management interface.
- **BaseAgent**
  - Built-in assistant runtime responsible for command interpretation, lifecycle tool dispatch, triage, and repair guidance/escalation.

## 2. Agent and Lifecycle Concepts

- **Agent**
  - A managed process defined by catalog manifest metadata (for example, `openclaw`).
- **Candidate**
  - A catalog entry discoverable in the platform but not fully lifecycle-approved in the current phase.
- **Agent ID**
  - A stable identifier for an agent definition (for example, `openclaw`).
- **Instance**
  - A concrete deployed runtime of an agent on a host (local or remote).
- **Manifest**
  - A declarative definition for install/start/stop/upgrade lifecycle commands, environment requirements, health checks, memory, and diagnostics.
- **Catalog**
  - Registry of known agent manifests.
- **Install**
  - Lifecycle operation that prepares an agent runtime and marks it as installed.
- **Uninstall**
  - Lifecycle operation that removes an installed agent and associated runtime artifacts.
- **Start**
  - Runtime operation that launches an installed agent process.
- **Stop**
  - Runtime operation that stops a running agent process.
- **Status**
  - Query operation returning install/runtime/health summaries.
- **Logs**
  - Runtime output retrieval for an agent.
- **Upgrade**
  - Update operation that applies agent version/update strategy.
- **Diagnose**
  - Operation that packages sanitized diagnostics into a zip artifact for debugging.
- **Install State**
  - Persistent lifecycle state such as `not_installed`, `installed`, `broken`.
- **Runtime State**
  - Current process state such as `stopped`, `starting`, `running`, `crashing`, `crash_loop`.
- **Health**
  - Health state such as `unknown`, `healthy`, `degraded`, `unhealthy`.
- **Crash Loop**
  - A rapid restart-failure pattern with recovery policy and cooldown handling.
- **Restart Count**
  - Counter for start attempts and recovery loops.
- **Fleet Status**
  - Aggregated status for all local agents.

## 3. Commands and Interfaces

- **Command Contract**
  - Unified request format for all providers: `<provider> <chat_id> <request_id> [session_token] <command> [...args]`.
- **GatewayResponse**
  - Standard response envelope containing `requestId`, `result`, `message`, optional `errorCode`, `sessionToken`, `downloadUrl`, and handoff fields.
- **`/pair <code>`**
  - Establishes authenticated chat session between provider chat and gateway.
- **`/agents`**
  - List known agents and install status.
- **`/add`**
  - Chat shortcut currently blocked for credential safety under production policy.
- **`/install <agent_id> [host_id]`**
  - Trigger install; policy may require explicit `host_id` in current configuration.
- **`/start <agent_id>`**
  - Start a local or remote-linked instance.
- **`/stop <agent_id>`**
  - Stop an instance.
- **`/status [agent_id]`**
  - Get one-agent or fleet status.
- **`/logs <agent_id> [tail]`**
  - Fetch runtime logs and optional downloadable artifact for larger outputs.
- **`/upgrade <agent_id>`**
  - Trigger upgrade flow.
- **`/diagnose <agent_id>`**
  - Produce a diagnostics artifact.
- **`/diagnose-consent <agent_id> <yes|no>`**
  - Record user consent and create diagnosis handoff state.
- **`/boundaries`**
  - Show BaseAgent policy/boundary summary.
- **`/providers`**
  - Show configured LLM providers and active provider.
- **`/sessions`**
  - Show recent chat-session metadata.
- **`/tools`**
  - Show BaseAgent tool map.
- **Error Codes**
  - Canonical gateway errors such as `E_SESSION_REQUIRED`, `E_PAIR_CODE_INVALID`, `E_INSTALL_GUI_ONLY`, `E_AUTH_INVALID`, etc.

## 4. Sessions, Security, and Auth

- **Session**
  - Pairing-bound authentication context scoped by `(provider, chat_id)`.
- **Session Token**
  - Short-lived authentication credential returned by `/pair`.
- **Pairing Code**
  - One-time code used to bind chat to a session.
- **Session Required**
  - Commands that require a valid active session.
- **Actor**
  - Audit identity of operation initiator (`provider:chat_id` or system actor).
- **Provider**
  - Inbound transport source (`telegram`, `discord`, `feishu`).
- **Chat Adapter**
  - Provider-specific webhook or transport integration.
- **Provider Isolation Principle**
  - Adapters only transport messages and formatting; no provider-specific command branching.
- **Gateway API Token**
  - Optional API authentication for `/command`.
- **Request ID**
  - Per-request correlation identifier used across logs and audit.
- **Rate Limit**
  - Request throttling guard for chat command and pairing flows.
- **Authorization (Gateway)**
  - Validation of session tokens and transport channel contexts.
- **Signature Verification**
  - Provider webhook validation (`telegram`, `discord`, `feishu`) against secret/header fields.
- **Authorization Token for Daemon APIs**
  - Optional server-level token configured by daemon settings.

## 5. Config, Channels, and Models

- **`config.v2.json`**
  - User/system configuration for channels, models, and base-agent options.
- **Config Version**
  - Backward compatibility guard with explicit expected version.
- **Channel**
  - Messaging transport configuration (`telegram`, `discord`, `feishu`) including transport mode and auth fields.
- **Transport Mode**
  - Channel transport behavior (`auto`, `webhook`, `polling`).
- **Model List**
  - Ordered provider/model declarations used for default model resolution.
- **Default Model**
  - Selected model alias used when no explicit model name is provided.
- **Model Provider**
  - LLM backend identifier (for example, `openai`, `openai-codex`).
- **Credential Ref**
  - Key used by credential store to resolve provider tokens/keys.
- **Credential Store**
  - Storage layer for provider credentials and secret access.
- **Env Var Overrides**
  - `CARRIER_*` variables overriding daemon and runtime behavior.
- **`CARRIER_SERVER_HOST`**
  - Daemon bind host.
- **`CARRIER_SERVER_PORT`**
  - Daemon bind port.
- **`CARRIER_SERVER_API_TOKEN`**
  - Optional API token for daemon access.
- **`CARRIER_LOG_LEVEL`**
  - Logging verbosity.
- **`CARRIER_CRASH_THRESHOLD`**
  - Crash count threshold for loop protection.
- **`CARRIER_CRASH_WINDOW`**
  - Time window for crash-loop detection.
- **`CARRIER_CRASH_COOLDOWN`**
  - Cooldown before retry recovery in restart suppression.

## 6. Memory Platform

- **Memory**
  - Persistent knowledge/context package consumed by agents.
- **Memory Type**
  - `per_agent`, `shared`, or `public`.
- **Per-Agent Memory**
  - Memory scoped to a single agent owner.
- **Shared Memory**
  - Reusable, cross-agent memory resource.
- **Public Memory**
  - Template or global memory package, generally read-only.
- **Memory State**
  - Lifecycle state: `created`, `mounted`, `detached`, `archived`.
- **Mount**
  - Operation attaching memory to an agent instance.
- **Detach**
  - Remove active mount without deleting memory record.
- **Archive**
  - Terminal memory lifecycle state.
- **Attach**
  - Bind a memory resource to an agent with policy checks.
- **Access Mode**
  - `ro` (read-only) or `rw` (read-write).
- **Memory Audit**
  - Immutable or mutable memory action records (create/import/mount/export, etc.).
- **Grant**
  - Permission artifact for explicit memory access in advanced scope models.
- **Memory Scope**
  - Effective context controls for what memory content is visible to a runtime.
- **Memory View**
  - Runtime-consumed compiled memory representation.
- **Import / Export**
  - Transfer memory packages into or out of the store.

## 7. Remote Control Plane

- **Remote Host**
  - SSH-managed external runtime target.
- **Remote Host ID**
  - Identifier of a managed host object.
- **Remote Instance**
  - Agent runtime tied to host identity and remote path.
- **Remote Instance Discovery**
  - Probe to discover managed instances on a host.
- **Host Check**
  - Pre-flight validation for host compatibility and SSH reachability.
- **Remote Add**
  - Deterministic sequence for provisioning or installing an agent on a remote host.
- **Config Sync**
  - Profile sync pipeline between local source-of-truth and remote instance.
- **Sync Mode**
  - `always_push`, `pull_validate_push`, or `manual`.
- **Pull New Instances**
  - Confirmation-gated fetch of newly discovered remote instances.
- **Drift**
  - Mismatch between local and remote state snapshots.
- **Reconcile**
  - Operation to compute and apply deterministic remote/local alignment.
- **Rollback**
  - Revert remote profile state to a target commit.
- **Remote Install**
  - Host-aware installation workflow requiring explicit host binding and preflight checks.
- **Auth Mode**
  - Remote authentication type (for example, private key flow).
- **Runtime Mode**
  - Remote runtime scheduling mode (for example, on-demand).
- **Remote Diagnosis**
  - Remote-facing failure analysis path when local triage cannot fully resolve an issue.

## 8. Repair, Triage, and Failure Handling

- **Evidence**
  - Structured diagnostic input including log tail, exit code, probes, and system info.
- **Triager**
  - Component that produces `TriageResult` for lifecycle failures.
- **LLM Triage**
  - LLM-based failure analysis run under strict output schema.
- **Triage Result**
  - Failure summary with suggested actions, repair action, and escalation flag.
- **Repair Action**
  - Suggestion to execute bounded command for auto-recovery.
- **Allowlist**
  - Only pre-approved repair commands are permitted.
- **Repair Policy**
  - Rules for command allowlist, risk classification, blocked substrings, and confirmation requirements.
- **Auto-Repair Round**
  - Limited retry loop count for safe remediation.
- **Risk Level**
  - `low` or `high` classification.
- **Blocked Substring**
  - Hard reject list for destructive patterns.
- **High-Risk Requires Confirmation**
  - Gate requiring explicit user approval for dangerous actions.
- **Boundaries**
  - Policy document defining in-scope, out-of-scope, and command safety constraints.
- **Escalation**
  - Transition to diagnose artifact and optional remote diagnosis consent when recovery fails.
- **Remote Diagnosis Consent**
  - Explicit yes/no user decision for remote diagnosis handoff path.

## 9. Observability and Audit

- **Audit Log**
  - Persistent action trail for lifecycle and repair operations.
- **Audit Event**
  - Record fields typically include `request_id`, `actor`, `action`, `target`, `result`, `error_code`, `message`, `timestamp`.
- **Audit Action**
  - Canonical actions such as `install`, `start`, `stop`, `status`, `logs`, `upgrade`, `diagnose`, `triage`, `remote_diagnosis_consent`.
- **Audit Result**
  - `success`, `failure`, or `neutral`.
- **Handoff**
  - Persisted remote diagnosis handoff object.
- **Artifact**
  - Generated diagnostic archive (logs/state/hashes/reports) accessible via tokenized download.
- **Download Token**
  - Ephemeral proof object for artifact retrieval.
- **Logs Query**
  - Read path for lifecycle/audit log introspection.

## 10. Implementation Notes and Validation

- This glossary is intentionally business-oriented and scoped to currently implemented Carrier behavior.
- Term names follow project source conventions where practical (`baseagent`, `install_state`, `runtime_state`, `syncMode`, gateway errors, and audit actions).
