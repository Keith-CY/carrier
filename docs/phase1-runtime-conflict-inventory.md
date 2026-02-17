# Phase 1 Runtime Conflict Inventory

- Date: 2026-02-17
- Source issues: #74 #793

## Inventory Method

Compared runtime and terminology statements across:

- `docs/Agent_Installation_Platform_PRD.md`
- `docs/Agent_Installation_Platform_Implementation_Plan.md`
- `docs/Agent_Installation_Platform_Product_Design.md`
- `README.md`
- `docs/Agent_Installation_Platform_PRD_extra.md`

## Section-by-Section Inventory

| Document | Section | Field | Current Statement | Conflict | Resolution |
|---|---|---|---|---|---|
| `docs/Agent_Installation_Platform_PRD.md` | Scope / Runtime model | Runtime | Local host on macOS/Linux, WSL2 on Windows, no Docker | None after alignment | Keep as canonical baseline |
| `docs/Agent_Installation_Platform_Implementation_Plan.md` | Scope Baseline | Runtime | "Runtime: no Docker path" | Terminology previously shorter than PRD wording | Expanded wording and explicit ADR reference |
| `docs/Agent_Installation_Platform_Product_Design.md` | Design principles / Runtime strategy | Runtime | Native runtime first, Docker out of scope | None | Keep; cross-link to ADR added in entry docs |
| `README.md` | Current scope / Read order | Runtime + Source-of-truth | Local-first wording present but ADR link previously missing | Documentation authority chain was implicit | Added explicit source-of-truth chain and ADR link |
| `docs/Agent_Installation_Platform_PRD_extra.md` | Legacy supplemental notes | Runtime | File was missing in repository | Missing file caused ambiguity in issue references | Added archived note with runtime statements removed and pointer to ADR |

## Recommended Option and Downside Analysis

### Recommended Option

Adopt one runtime baseline for Phase 1:

- Local host (macOS/Linux) + WSL2 (Windows)
- No Docker in Phase 1
- ADR-driven change control for any runtime model update

### Downsides

1. No container parity path in Phase 1.
2. Operators must satisfy local toolchain prerequisites.
3. Some future docs or code proposals may require explicit rejection/deferral when they assume Docker.

### Why this option is still preferred

- Matches Phase 1 delivery objective (fast OpenClaw lifecycle validation).
- Keeps support matrix and test burden bounded.
- Reduces ambiguity for contributors and reviewers.
