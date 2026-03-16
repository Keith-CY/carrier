# Generic Integration Core Phase 1

## Goal

Land a generic Carrier integration core that can back adapter-specific marketplace contracts without baking marketplace-specific domain objects into the control plane.

Phase 1 ships:

- generic binding, execution, attempt, action, event, callback delivery, usage proof, and artifact ref models
- SQLite-backed durable integration store
- adapter-specific `one-tok` HTTP surface
- execution bridge onto existing orchestrator executions
- cooperative `pause/resume/cancel`
- signed callback outbox with retry/backoff
- per-execution callback sequence ordering
- callback replay from `eventId` or `fromSequence`
- usage proof materialization from provider governance attribution

Phase 1 does not ship:

- generic HTTP namespace
- daemon/baseagent in-flight hard pause
- callback replay audit object
- provider-native billing receipts

## Public Surface

Adapter-specific HTTP paths:

- `POST /api/v1/integrations/one-tok/bindings/verify`
- `POST /api/v1/integrations/one-tok/executions`
- `GET /api/v1/integrations/one-tok/executions/:id`
- `POST /api/v1/integrations/one-tok/executions/:id/actions`
- `POST /api/v1/integrations/one-tok/executions/:id/callbacks/replay`

`callbacks/replay` request body:

```json
{
  "fromSequence": 2
}
```

or

```json
{
  "eventId": "evt_..."
}
```

## Binding Contract

Generic binding fields relevant to callback delivery:

- `callbackUrl`
- `callbackKeyId`
- `callbackSecret`

Bindings remain generic. The `one-tok` adapter only maps its external schema onto this core object.

## Callback Delivery Model

Each integration event creates one callback delivery row when the binding has a `callbackUrl`.

Delivery semantics:

- event identity is stable: `eventId + sequence`
- signing is HMAC-SHA256 over the JSON body
- retries reuse the same event identity
- retries back off via `nextAttemptAt`
- later events for the same execution are blocked until earlier events are `delivered`
- replay resets deliveries from a requested sequence back to `pending`

Headers sent:

- `X-Carrier-Key-Id`
- `X-Carrier-Event-Id`
- `X-Carrier-Event-Sequence`
- `X-Carrier-Signature`

Callback envelope:

```json
{
  "eventId": "evt_...",
  "sequence": 1,
  "eventType": "execution.accepted",
  "bindingId": "bind_...",
  "carrierExecutionId": "cexec_...",
  "attemptId": "attempt_...",
  "createdAt": "2026-03-16T00:00:00Z",
  "payload": {
    "carrierExecutionId": "cexec_..."
  }
}
```

Callback response may include:

```json
{
  "accepted": true,
  "recommendedAction": {
    "type": "pause",
    "reason": "budget low"
  }
}
```

The response action is mapped back into the generic integration action controller.

## Usage Proof Model

Usage proofs are derived from persisted provider attribution in `OrchestratorExecution.Governance.ProviderResolutions`.

Current proof characteristics:

- `usageKind = provider_cost_estimate`
- `amountCents` derived from estimated provider cost
- `meterRef = provider:<provider>:model:<model>`
- `digest` is computed from normalized usage payload

This is a settlement-oriented estimate surface, not a provider-native invoice receipt.

## Implementation Notes

Key code paths:

- shared model: `shared/integration`
- SQLite store: `gateway/integration_store.go`
- execution bridge: `gateway/integration_executions.go`
- callback ledger + dispatcher: `gateway/integration_ledger.go`, `gateway/integration_callbacks.go`
- adapter HTTP: `gateway/integration_one_tok_api.go`
- usage proof materialization: `gateway/integration_usage.go`

## Verification

Primary verification commands:

```bash
go test ./... -count=1
```

in:

- `shared/`
- `gateway/`

Targeted callback and replay tests:

```bash
go test ./... -run 'TestDispatchPendingIntegrationCallbacks|TestHandleOneTokExecutionCallbackReplay' -count=1
```

## Review Focus

Review should focus on:

- core vs adapter boundary
- idempotency and durable store semantics
- callback ordering and replay behavior
- pause/resume lifecycle semantics
- usage proof correctness and limits
