# codeagent

`codeagent` is the remote coding runtime module for Carrier.

Current scope in this phase:

- backend-neutral contract (`read_file`, `write_file`, `apply_patch`, `run_shell`, `run_shell_redirect`)
- strict policy evaluator (workspace boundaries + command classification)
- middleware-first orchestrator
- Codex and OpenCode adapters with resume-to-fresh fallback behavior
- transient error retry and lightweight cost estimation in normalized run envelope

Future phases can add deeper gateway/runtime lifecycle automation and richer cost accounting.
