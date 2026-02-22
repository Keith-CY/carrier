# Phase 1 Conflict Inventory (T001)

Status: Drafted for issue [#793](https://github.com/Keith-CY/carrier/issues/793)  
Scope: README, PRD, Product Design, PRD_extra (missing in repo)

## Inventory Rules

- Compare by **section + field**, not prose style.
- Mark each row as one of: `Conflict`, `Aligned`, `Missing Source`.
- For `Conflict`, include practical impact to implementation/acceptance.

## Source Files

- README: `README.md`
- PRD (source of truth): `docs/Agent_Installation_Platform_PRD.md`
- Product Design: `docs/Agent_Installation_Platform_Product_Design.md`
- PRD_extra: `docs/Agent_Installation_Platform_PRD_extra.md` (**not present in repository**)

## Conflict Inventory

| ID | Section | Field | README | PRD | Product Design | PRD_extra | Status | Notes |
|---|---|---|---|---|---|---|---|---|
| CI-001 | Runtime model | Supported runtime | local host (macOS/Linux), WSL2 | same | same | missing | Aligned | No model conflict among existing sources. |
| CI-002 | Lifecycle target | OpenClaw scope in Phase 1 | full lifecycle OpenClaw; others candidate-only | same | same | missing | Aligned | Catalog/README/PRD consistent. |
| CI-003 | Memory model | Classes | Per-Agent / Shared / Public | same | same | missing | Aligned | Terminology consistent. |
| CI-004 | Control surface | Mandatory GUI | no mandatory GUI (chat-first) | implied chat command flow | explicit no mandatory GUI | missing | Aligned | Product Design is explicit; others are consistent. |
| CI-005 | Gateway providers | Telegram/Discord/Feishu | listed | listed | listed | missing | Aligned | No provider list conflict. |
| CI-006 | Scope authority | Source-of-truth order | README says PRD > Implementation Plan > README | PRD says conflicts resolved by PRD | Design not marked as authority | missing | Aligned | Governance is coherent (PRD wins). |
| CI-007 | Phase-1 objective detail | Success metrics targets | not fully enumerated | explicit KPIs (TTFH, success rate) | north-star goals (10-minute setup) | missing | Aligned | Detail level differs, but no contradictory target. |
| CI-008 | Installation model | Runtime install types | not enumerated in README | local_binary / npm_cli / go_cli | same | missing | Aligned | README is summary-level only. |
| CI-009 | Diagnostics escalation | Remote diagnosis flow | mentions diagnose artifact + support | unresolved -> diagnose bundle + consent handoff | same | missing | Aligned | Same escalation shape across sources. |
| CI-010 | Baseline conflict source | PRD_extra participation | referenced by issue set | N/A | N/A | file absent | Missing Source | `docs/Agent_Installation_Platform_PRD_extra.md` is missing, so direct comparison cannot be completed. |

## Summary

- Current repository sources (README/PRD/Product Design) are **structurally aligned** for Phase 1 runtime, lifecycle, memory model, and gateway scope.
- The only hard blocker for full “4-doc conflict inventory” completion is **missing PRD_extra file**.

## Follow-up

1. Restore/add `docs/Agent_Installation_Platform_PRD_extra.md` (or archive rationale that it is intentionally removed) — see [#793](https://github.com/Keith-CY/carrier/issues/793).
2. Re-run this inventory with PRD_extra included and convert any confirmed differences into T002 option matrix inputs.
