package gateway

import (
	"sync"
	"time"
)

const (
	remoteOpHostCheck            = "host_check"
	remoteOpInstancesList        = "instances_list"
	remoteOpInstancesStatus      = "instances_status"
	remoteOpInstancesInstall     = "instances_install"
	remoteOpInstancesUninstall   = "instances_uninstall"
	remoteOpInstancesRepair      = "instances_repair"
	remoteOpInstancesLogs        = "instances_logs"
	remoteOpInstancesSync        = "instances_sync"
	remoteOpInstancesDiagnose    = "instances_diagnose"
	remoteOpInstancesReconcile   = "instances_reconcile"
	remoteOpInstancesRollback    = "instances_rollback"
	remoteOpConfigRead           = "config_read"
	remoteOpConfigPatch          = "config_patch"
	remoteOpSessionsList         = "sessions_list"
	remoteOpSessionArchive       = "session_archive"
	remoteOpSessionDelete        = "session_delete"
	remoteOpMemoryList           = "memory_list"
	remoteOpProfileTestRemote    = "provider_profile_test_remote"
	remoteOpProviderBindingApply = "provider_binding_apply"
	remoteOpRemoteChatStream     = "remote_chat_stream"
	remoteOpCodeAgentInstall     = "codeagent_install"
	remoteOpCodeAgentConfigure   = "codeagent_configure"
	remoteOpCodeAgentHealth      = "codeagent_health"
	remoteOpCodeAgentVersion     = "codeagent_version"
	remoteOpCodeAgentRun         = "codeagent_run"
)

type remoteOperationStats struct {
	Total          int64   `json:"total"`
	Success        int64   `json:"success"`
	Failure        int64   `json:"failure"`
	SuccessRate    float64 `json:"successRate"`
	AvgLatencyMs   int64   `json:"avgLatencyMs"`
	MinLatencyMs   int64   `json:"minLatencyMs"`
	MaxLatencyMs   int64   `json:"maxLatencyMs"`
	LatencyTotalMs int64   `json:"latencyTotalMs"`
}

type remoteRepairMetrics struct {
	Triggered   int64   `json:"triggered"`
	Success     int64   `json:"success"`
	Failure     int64   `json:"failure"`
	SuccessRate float64 `json:"successRate"`
}

type remoteChatMetrics struct {
	Total       int64   `json:"total"`
	Failure     int64   `json:"failure"`
	FailureRate float64 `json:"failureRate"`
}

type remoteRolloutStatus struct {
	State      string   `json:"state"`
	CanPromote bool     `json:"canPromote"`
	Reasons    []string `json:"reasons"`
}

type remoteMetricsSnapshot struct {
	Timestamp  string                          `json:"timestamp"`
	Operations map[string]remoteOperationStats `json:"operations"`
	Totals     remoteOperationStats            `json:"totals"`
	Repair     remoteRepairMetrics             `json:"repair"`
	ChatStream remoteChatMetrics               `json:"chatStream"`
	Rollout    remoteRolloutStatus             `json:"rollout"`
	Alerts     remoteAlertSummary              `json:"alerts"`
}

type remoteAlertSummary struct {
	Active bool   `json:"active"`
	Level  string `json:"level"`
	Count  int    `json:"count"`
}

type remoteMetricsCollector struct {
	mu         sync.Mutex
	operations map[string]remoteOperationStats
	repair     remoteRepairMetrics
}

func newRemoteMetricsCollector() *remoteMetricsCollector {
	return &remoteMetricsCollector{
		operations: make(map[string]remoteOperationStats),
	}
}

var remoteMetrics = newRemoteMetricsCollector()

func recordRemoteOperationMetric(class string, startedAt time.Time, err error) {
	if class == "" {
		return
	}
	remoteMetrics.recordOperation(class, err == nil, time.Since(startedAt))
}

func (c *remoteMetricsCollector) recordOperation(class string, success bool, duration time.Duration) {
	if class == "" {
		return
	}
	latencyMs := duration.Milliseconds()
	if latencyMs < 0 {
		latencyMs = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	stats := c.operations[class]
	stats.Total++
	if success {
		stats.Success++
	} else {
		stats.Failure++
	}
	stats.LatencyTotalMs += latencyMs
	if stats.Total == 1 {
		stats.MinLatencyMs = latencyMs
		stats.MaxLatencyMs = latencyMs
	} else {
		if latencyMs < stats.MinLatencyMs {
			stats.MinLatencyMs = latencyMs
		}
		if latencyMs > stats.MaxLatencyMs {
			stats.MaxLatencyMs = latencyMs
		}
	}
	if stats.Total > 0 {
		stats.AvgLatencyMs = stats.LatencyTotalMs / stats.Total
		stats.SuccessRate = rate(stats.Success, stats.Total)
	}
	c.operations[class] = stats
}

func (c *remoteMetricsCollector) recordRepairAttempt(success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.repair.Triggered++
	if success {
		c.repair.Success++
	} else {
		c.repair.Failure++
	}
	c.repair.SuccessRate = rate(c.repair.Success, c.repair.Triggered)
}

func (c *remoteMetricsCollector) snapshot() remoteMetricsSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	ops := make(map[string]remoteOperationStats, len(c.operations))
	totals := remoteOperationStats{}
	for class, stats := range c.operations {
		ops[class] = stats
		totals.Total += stats.Total
		totals.Success += stats.Success
		totals.Failure += stats.Failure
		totals.LatencyTotalMs += stats.LatencyTotalMs
		if totals.Total == stats.Total {
			totals.MinLatencyMs = stats.MinLatencyMs
			totals.MaxLatencyMs = stats.MaxLatencyMs
		} else {
			if stats.MinLatencyMs < totals.MinLatencyMs {
				totals.MinLatencyMs = stats.MinLatencyMs
			}
			if stats.MaxLatencyMs > totals.MaxLatencyMs {
				totals.MaxLatencyMs = stats.MaxLatencyMs
			}
		}
	}
	if totals.Total > 0 {
		totals.AvgLatencyMs = totals.LatencyTotalMs / totals.Total
		totals.SuccessRate = rate(totals.Success, totals.Total)
	}

	chatStats := ops[remoteOpRemoteChatStream]
	chat := remoteChatMetrics{
		Total:       chatStats.Total,
		Failure:     chatStats.Failure,
		FailureRate: rate(chatStats.Failure, chatStats.Total),
	}
	rollout := evaluateRemoteRolloutStatus(totals, c.repair, chat)
	alerts := remoteAlertSummary{
		Active: len(rollout.Reasons) > 0,
		Level:  rollout.State,
		Count:  len(rollout.Reasons),
	}
	if !alerts.Active {
		alerts.Level = "none"
	}

	return remoteMetricsSnapshot{
		Timestamp:  nowTimestamp(),
		Operations: ops,
		Totals:     totals,
		Repair:     c.repair,
		ChatStream: chat,
		Rollout:    rollout,
		Alerts:     alerts,
	}
}

func (c *remoteMetricsCollector) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.operations = make(map[string]remoteOperationStats)
	c.repair = remoteRepairMetrics{}
}

func rate(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func evaluateRemoteRolloutStatus(totals remoteOperationStats, repair remoteRepairMetrics, chat remoteChatMetrics) remoteRolloutStatus {
	holdReasons := make([]string, 0, 4)
	canaryReasons := make([]string, 0, 4)

	if totals.Total >= 20 && totals.SuccessRate < 0.90 {
		holdReasons = append(holdReasons, "operation success rate below 90%")
	}
	if chat.Total >= 10 && chat.FailureRate >= 0.25 {
		holdReasons = append(holdReasons, "chat stream failure rate at or above 25%")
	}
	if repair.Triggered >= 5 && repair.SuccessRate < 0.70 {
		holdReasons = append(holdReasons, "repair success rate below 70%")
	}
	if totals.Total >= 10 && totals.MaxLatencyMs >= 5000 {
		holdReasons = append(holdReasons, "max latency above 5s")
	}
	if len(holdReasons) > 0 {
		return remoteRolloutStatus{
			State:      "hold",
			CanPromote: false,
			Reasons:    holdReasons,
		}
	}

	if totals.Total >= 5 && totals.SuccessRate < 0.98 {
		canaryReasons = append(canaryReasons, "operation success rate below 98%")
	}
	if chat.Total >= 2 && chat.FailureRate >= 0.20 {
		canaryReasons = append(canaryReasons, "chat stream failure rate at or above 20%")
	}
	if repair.Triggered >= 1 && repair.SuccessRate < 1 {
		canaryReasons = append(canaryReasons, "repair success rate below 100%")
	}
	if totals.Total >= 5 && totals.AvgLatencyMs >= 1500 {
		canaryReasons = append(canaryReasons, "average latency above 1.5s")
	}
	if len(canaryReasons) > 0 {
		return remoteRolloutStatus{
			State:      "canary",
			CanPromote: false,
			Reasons:    canaryReasons,
		}
	}

	return remoteRolloutStatus{
		State:      "healthy",
		CanPromote: true,
		Reasons:    []string{},
	}
}

func resetRemoteMetricsForTests() {
	remoteMetrics.reset()
}
