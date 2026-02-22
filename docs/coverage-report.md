# Daemon Test Coverage Report

**Date:** 2026-02-14
**Total Coverage:** 75.7%

## Per-Package Coverage

```
carrier/daemon/internal/baseagent/types.go	0.0%
carrier/daemon/internal/catalog/manifests.go	0.0%
carrier/daemon/internal/memory/types.go	42.9%
carrier/daemon/internal/lifecycle/memory.go	43.7%
carrier/daemon/cmd/agentd/main.go	44.5%
carrier/daemon/internal/lifecycle/helpers.go	59.0%
carrier/daemon/internal/lifecycle/service.go	61.3%
carrier/daemon/internal/commandexec/runner.go	68.5%
carrier/daemon/internal/lifecycle/start_stop.go	68.5%
carrier/daemon/internal/manifest/load.go	72.7%
carrier/daemon/internal/lifecycle/upgrade.go	76.6%
carrier/daemon/internal/runtimecheck/checker.go	81.6%
carrier/daemon/internal/lifecycle/install.go	84.6%
carrier/daemon/internal/logging/logging.go	85.7%
carrier/daemon/internal/manifest/types.go	88.0%
carrier/daemon/internal/memory/store.go	92.5%
carrier/daemon/internal/runtimecheck/preflight.go	93.7%
carrier/daemon/internal/lifecycle/audit.go	95.2%
carrier/daemon/internal/catalog/catalog.go	95.8%
carrier/daemon/internal/baseagent/policy.go	97.1%
carrier/daemon/internal/memory/policy.go	97.4%
carrier/daemon/internal/config/config.go	97.8%
carrier/daemon/internal/redact/redact.go	98.6%
carrier/daemon/internal/health/health.go	100.0%
```

## Top Coverage Gaps

The following packages have the lowest coverage and would benefit from additional tests:

```
carrier/daemon/internal/baseagent/types.go	0.0%
carrier/daemon/internal/catalog/manifests.go	0.0%
carrier/daemon/internal/memory/types.go	42.9%
carrier/daemon/internal/lifecycle/memory.go	43.7%
carrier/daemon/cmd/agentd/main.go	44.5%
```

## How to Regenerate

```bash
./scripts/coverage.sh
```

The script also generates an HTML report at `daemon/coverage.html` for interactive browsing.

CI can optionally run this script and post coverage results as a PR comment.
