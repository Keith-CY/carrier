# Log Rotation/Retention Policy and Disk Protection

> **Issue:** #83 · **Status:** Plan · **Track:** C2
>
> **Note:** This is a historical planning document and may diverge from current implementation.
> For current behavior and canonical references, use [`docs/current-architecture.md`](../current-architecture.md), [`docs/deployment.md`](../deployment.md), [`daemon/internal/lifecycle/`](../../daemon/internal/lifecycle/), and [`daemon/internal/logging/`](../../daemon/internal/logging/).

## Goals

- Define size-based and time-based log rotation policies.
- Specify compression and retention windows for rotated logs.
- Design protective behavior when disk space is critically low.
- Define artifact size-limit policies for diagnostic artifacts.

## Non-Goals

- Centralized log aggregation (e.g., shipping to ELK/Loki).
- Application-level structured logging format changes.
- Log encryption at rest.

---

## 1. Log Rotation Policy

### Configuration

```yaml
# carrier.yaml — logging section
logging:
  dir: /var/log/carrier
  rotation:
    max_size_mb: 50          # rotate when file exceeds this size
    max_age_days: 14         # delete rotated files older than this
    max_backups: 10          # max number of rotated files to keep
    compress: true           # gzip rotated files
    compress_delay_min: 5    # wait before compressing (avoid compressing active file)
  level: info                # debug | info | warn | error
```

### Size-Based Rotation

- When the active log file exceeds `max_size_mb`, it is closed and renamed with a timestamp suffix: `carrier.log` → `carrier-2026-02-14T08-30-00.log`
- A new `carrier.log` is created immediately.
- Rotation is atomic (rename + create) to avoid log loss.

### Time-Based Rotation

- Regardless of size, logs rotate at midnight UTC daily if the file is non-empty.
- This ensures predictable file boundaries for retention and debugging.

### Retention Window

| Rule | Default | Description |
|------|---------|-------------|
| `max_age_days` | 14 | Rotated files older than this are deleted |
| `max_backups` | 10 | Keep at most N rotated files; oldest deleted first |
| Effective retention | min(age, count) | Whichever limit is hit first triggers cleanup |

### Compression

- Rotated files are compressed with gzip after `compress_delay_min` minutes.
- Compressed files use `.gz` extension: `carrier-2026-02-14T08-30-00.log.gz`
- Typical compression ratio: 10:1 for text logs → 50 MB file becomes ~5 MB.

---

## 2. Disk Protection

### Monitoring

A background goroutine checks available disk space every 60 seconds on the partition containing the log directory.

### Thresholds and Behaviors

All threshold values are configurable via the deployment profile to accommodate different environments:

```yaml
disk_protection:
  normal_threshold_mb: 500       # above this → Normal (no action)
  warning_threshold_mb: 200      # 200–500 MB → Warning
  critical_threshold_mb: 100     # 100–200 MB → Critical
  recovery_target_mb: 200        # target free space after emergency cleanup
  warning_rotation_mb: 25        # rotation size during Warning state
```

**Default thresholds:**

| Available Space | Level | Action |
|----------------|-------|--------|
| > 500 MB | Normal | No action |
| 200–500 MB | Warning | Log warning; increase rotation frequency (rotate at 25 MB) |
| 100–200 MB | Critical | Switch to `error`-only logging; force-rotate and compress immediately |
| < 100 MB | Emergency | Stop all logging to disk; emit single stderr message; delete oldest rotated logs until > 200 MB free |

### Emergency Recovery

When emergency threshold is hit:
1. Delete compressed rotated logs oldest-first until space > 200 MB.
2. If still insufficient, delete uncompressed rotated logs.
3. If still insufficient, truncate active log file to 0 bytes.
4. Resume logging at `error` level only.
5. Emit a structured event to any configured alerting channel.

### State-Transition Policy

After entering a degraded state (Critical or Emergency), the system transitions back to normal operation as follows:

| Current State | Transition Condition | Target State |
|--------------|---------------------|-------------|
| Emergency | Free space recovers above `critical_threshold_mb` (default 100 MB) | Critical (`error`-only logging resumes to disk) |
| Critical | Free space recovers above `warning_threshold_mb` (default 200 MB) for 2 consecutive checks | Warning (normal log levels restored, increased rotation retained) |
| Warning | Free space recovers above `normal_threshold_mb` (default 500 MB) for 2 consecutive checks | Normal (default rotation settings restored) |

**Hysteresis:** Transitions to a less-severe state require the condition to hold for 2 consecutive monitoring intervals (2 × 60 s = 120 s) to prevent rapid oscillation near threshold boundaries. Transitions to a more-severe state are immediate.

**Operator override:** Operators can force a state transition via the `carrier log-level reset` command, which immediately re-evaluates disk space and sets the appropriate state.

### Safeguard: Never Delete Non-Log Files

The cleanup routine operates exclusively within the configured `logging.dir`. Path traversal or symlink following is explicitly prevented.

---

## 3. Diagnostic Artifact Size-Limit Policy

Diagnostic artifacts (crash dumps, heap profiles, goroutine dumps) generated by Carrier are subject to size limits to prevent disk exhaustion.

### Configuration

```yaml
logging:
  diagnostics:
    max_artifact_size_mb: 100    # max size per artifact
    max_total_size_mb: 500       # max total diagnostic storage
    retention_days: 7            # auto-delete after N days
    dir: /var/log/carrier/diag   # separate directory
```

### Behavior

| Condition | Action |
|-----------|--------|
| Single artifact > `max_artifact_size_mb` | Truncate and log warning |
| Total diag storage > `max_total_size_mb` | Delete oldest artifacts until under limit |
| Artifact older than `retention_days` | Delete on next cleanup sweep |
| Disk in Critical/Emergency state | Disable diagnostic artifact generation |

---

## 4. Implementation Notes

### Go Package Structure

```
daemon/internal/logmgr/
  ├── rotation.go       # rotation logic
  ├── diskcheck.go      # disk space monitoring
  ├── cleanup.go        # retention enforcement
  ├── config.go         # configuration types
  └── logmgr_test.go    # unit tests
```

### Dependencies

- Use `gopkg.in/natefinch/lumberjack.v2` or equivalent for rotation mechanics (or implement minimal version).
- Use `syscall.Statfs` for disk space checks on Linux.

---

## Acceptance Criteria

1. Log rotation triggers on both size and time boundaries.
2. Compressed rotated logs are retained within configured limits.
3. Disk protection engages at each threshold and recovers automatically.
4. Diagnostic artifacts respect size and retention limits.
5. No file operations occur outside the configured log directory.
6. All behaviors are configurable via `carrier.yaml`.

## Timeline Estimate

| Task | Estimate |
|------|----------|
| Rotation and compression implementation | 2 days |
| Disk monitoring and protection | 2 days |
| Diagnostic artifact limits | 1 day |
| Tests and documentation | 1 day |
| **Total** | **~6 days** |
