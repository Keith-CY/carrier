# 1h-Decomposition Issue Traversal and Resolution

- Snapshot date: 2026-02-17
- Query basis: open issues with label `1h-Decomposition`
- Open count at traversal time: 39
- Target PR strategy: **single aggregate PR**

## Resolution Classes (Single-PR)

1. **Direct code closure in this PR**
   - `#803` `/agents` response contract alignment
   - `#829` audit query/filter API (`actor/action/request_id/result`)
2. **Direct documentation closure in this PR**
   - `#856` hardening evidence checklist
   - `#858` final exit-gate pass/fail report + evidence index
3. **Evidence-backed closure in this PR (existing implementation + explicit evidence linkage)**
   - Remaining open `1h-Decomposition` issues listed below, with evidence centralized in:
     - `docs/plans/phase1-hardening-evidence-checklist.md`
     - `docs/plans/phase1-exit-gate-review.md`
     - `docs/daemon-api-contract.md`
     - `docs/audit-event-dictionary.md`

## Inventory (Traversed Issues)
- #858 [[Micro][T066][#87] Produce final Phase-1 Exit Gate report (pass/fail per criterion + evidence index).](https://github.com/Keith-CY/carrier/issues/858)
- #856 [[Micro][T064][#44] Build hardening evidence checklist (security/audit/reliability) with links to completed micro tasks.](https://github.com/Keith-CY/carrier/issues/856)
- #849 [[Micro][T057][#32] Emit structured triage/audit summary output for every failure-handling cycle.](https://github.com/Keith-CY/carrier/issues/849)
- #848 [[Micro][T056][#32] Implement FR-D-014 remote diagnosis package + explicit user-consent handshake.](https://github.com/Keith-CY/carrier/issues/848)
- #844 [[Micro][T052][#32] Define evidence collector interface (logs/exit/probes/trace inputs).](https://github.com/Keith-CY/carrier/issues/844)
- #843 [[Micro][T051][#84] Expose user-facing recovery status and aligned error codes.](https://github.com/Keith-CY/carrier/issues/843)
- #842 [[Micro][T050][#84] Define auto-recovery vs manual-recovery boundary and stop conditions.](https://github.com/Keith-CY/carrier/issues/842)
- #836 [[Micro][T044][#82] Add leak-regression tests covering command output/log/artifact paths.](https://github.com/Keith-CY/carrier/issues/836)
- #831 [[Micro][T039][#82] Define redaction rule set (allowlist/denylist/patterns for token/key/url).](https://github.com/Keith-CY/carrier/issues/831)
- #829 [[Micro][T037][#81] Add audit query/filter API or command (`actor/action/request_id/result`).](https://github.com/Keith-CY/carrier/issues/829)
- #828 [[Micro][T036][#81] Instrument attach/detach action audit events.](https://github.com/Keith-CY/carrier/issues/828)
- #827 [[Micro][T035][#81] Instrument duplicate/share action audit events.](https://github.com/Keith-CY/carrier/issues/827)
- #826 [[Micro][T034][#81] Instrument export action audit events.](https://github.com/Keith-CY/carrier/issues/826)
- #825 [[Micro][T033][#81] Instrument import action audit events.](https://github.com/Keith-CY/carrier/issues/825)
- #824 [[Micro][T032][#81] Define audit-event schema (`time/actor/action/target/result/request_id`).](https://github.com/Keith-CY/carrier/issues/824)
- #823 [[Micro][T031][#79] Standardize duplicate/share error code and user-message mapping.](https://github.com/Keith-CY/carrier/issues/823)
- #822 [[Micro][T030][#79] Implement share revoke path with explicit permission checks.](https://github.com/Keith-CY/carrier/issues/822)
- #821 [[Micro][T029][#79] Define and implement share visibility/owner semantics.](https://github.com/Keith-CY/carrier/issues/821)
- #820 [[Micro][T028][#79] Implement duplicate naming-conflict resolver behavior.](https://github.com/Keith-CY/carrier/issues/820)
- #819 [[Micro][T027][#79] Define duplicate flow contract (versioning + provenance fields).](https://github.com/Keith-CY/carrier/issues/819)
- #818 [[Micro][T026][#42] Implement logs/diagnose download token lifecycle (issue, consume-once, cleanup).](https://github.com/Keith-CY/carrier/issues/818)
- #817 [[Micro][T025][#42] Standardize structured command response mapping across providers (success/failure parity).](https://github.com/Keith-CY/carrier/issues/817)
- #816 [[Micro][T024][#42] Propagate `request_id` end-to-end (provider -> gateway -> daemon -> response).](https://github.com/Keith-CY/carrier/issues/816)
- #812 [[Micro][T020][#42] Normalize Discord command parser and unknown-command handling.](https://github.com/Keith-CY/carrier/issues/812)
- #806 [[Micro][T014][#30] Implement `/start` env validation + memory binding validation.](https://github.com/Keith-CY/carrier/issues/806)
- #805 [[Micro][T013][#30] Implement `/install` idempotent retry guard (safe re-entry semantics).](https://github.com/Keith-CY/carrier/issues/805)
- #803 [[Micro][T011][#30] Finalize `/agents` response contract fields and error codes.](https://github.com/Keith-CY/carrier/issues/803)
- #804 [[Micro][T012][#30] Implement `/install` precondition validation (env/binary/path checks).](https://github.com/Keith-CY/carrier/issues/804)
- #795 [[Micro][T003][#74] Lock Phase-1 runtime model decision record in issue comment.](https://github.com/Keith-CY/carrier/issues/795)
- #794 [[Micro][T002][#74] Write option matrix for each conflict (recommended option + downside).](https://github.com/Keith-CY/carrier/issues/794)
- #84 [[Plan][C3] Service Restart Recovery and Self-Healing Strategy](https://github.com/Keith-CY/carrier/issues/84)
- #82 [[Plan][C1] Unified Secret Redaction Policy (Daemon/Gateway/Artifact)](https://github.com/Keith-CY/carrier/issues/82)
- #81 [[Plan][B5] Memory Audit Log Model and Observability Completion](https://github.com/Keith-CY/carrier/issues/81)
- #79 [[Plan][B3] Memory Duplicate/Share Flows and Permission Constraints](https://github.com/Keith-CY/carrier/issues/79)
- #44 [[Task] W6.1 Security, Audit, and Reliability Hardening](https://github.com/Keith-CY/carrier/issues/44)
- #42 [[Task] W4.1 Gateway Three-Provider Integration and Session Routing Closure](https://github.com/Keith-CY/carrier/issues/42)
- #32 [[Task] W3.1 Base Agent fix and upgrade](https://github.com/Keith-CY/carrier/issues/32)
- #30 [[Task] W2.1 Implement daemon core lifecycle APIs](https://github.com/Keith-CY/carrier/issues/30)
- #17 [Release workflow follow-up: hardening package distribution](https://github.com/Keith-CY/carrier/issues/17)
