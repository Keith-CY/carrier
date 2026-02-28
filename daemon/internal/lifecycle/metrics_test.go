package lifecycle

import (
	"os"
	"testing"
)

func TestCollectAgentMetricsReturnsValidData(t *testing.T) {
	metrics, err := collectAgentMetrics(os.Getpid())
	if err != nil {
		t.Fatalf("collectAgentMetrics error: %v", err)
	}
	if metrics.CPUPercent < 0 {
		t.Fatalf("CPUPercent = %f, want >= 0", metrics.CPUPercent)
	}
	if metrics.MemoryRSS <= 0 {
		t.Fatalf("MemoryRSS = %d, want > 0", metrics.MemoryRSS)
	}
	if metrics.Uptime <= 0 {
		t.Fatalf("Uptime = %d, want > 0", metrics.Uptime)
	}
}
