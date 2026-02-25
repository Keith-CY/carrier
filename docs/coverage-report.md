# Multi-Module Coverage Report

**Date:** 2026-02-25

## Current Totals

| Module | Total Coverage |
| --- | --- |
| `shared` | `100.0%` |
| `baseagent` | `100.0%` |
| `daemon` | `89.9%` |
| `gateway` | `91.5%` |

## Coverage Gate

From repository root:

```bash
make coverage-gate
```

`coverage-gate` currently enforces:

- `shared >= 100.0%`
- `baseagent >= 100.0%`
- `daemon >= 82.5%`
- `gateway >= 69.0%`

Strict mode (all modules must be `100%`):

```bash
COVERAGE_STRICT_100=1 make coverage-gate
```
