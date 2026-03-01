package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"carrier/daemon/internal/memory"
)

func TestParseDurationEnvAndTrimPromptText(t *testing.T) {
	if got := parseDurationEnv("15m", time.Hour); got != 15*time.Minute {
		t.Fatalf("expected 15m, got %v", got)
	}
	if got := parseDurationEnv("invalid", time.Hour); got != time.Hour {
		t.Fatalf("expected fallback duration, got %v", got)
	}
	if got := parseDurationEnv("-5m", time.Hour); got != time.Hour {
		t.Fatalf("expected fallback for non-positive duration, got %v", got)
	}

	if got := trimPromptText("  hello\nworld  ", 100); got != "hello world" {
		t.Fatalf("unexpected trimPromptText result: %q", got)
	}
	if got := trimPromptText("abcdef", 3); got != "abc..." {
		t.Fatalf("unexpected truncated prompt text: %q", got)
	}
}

func TestBaseAgentDistillSummarizerShortCircuit(t *testing.T) {
	summarizer := baseAgentDistillSummarizer()

	out, err := summarizer(context.Background(), nil, 100)
	if err != nil {
		t.Fatalf("expected nil err for nil cluster, got %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output for nil cluster, got %q", out)
	}

	cluster := []memory.MemoryRecord{
		{ContentSummary: "   ", ContentRaw: "\n\t"},
		{ContentSummary: "", ContentRaw: "   "},
	}
	out, err = summarizer(context.Background(), cluster, 100)
	if err != nil {
		t.Fatalf("expected nil err when no usable lines, got %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty output when all records are blank, got %q", out)
	}
}

func TestStartBaseAgentDistillSchedulerEnableDisable(t *testing.T) {
	store := memory.NewStore(memory.WithRootDir(t.TempDir()))

	t.Run("nil store", func(t *testing.T) {
		if stop := startBaseAgentDistillScheduler(nil); stop != nil {
			t.Fatalf("expected nil stop function for nil store")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		t.Setenv("CARRIER_MEMORY_BASE_AGENT_DISTILL_ENABLED", "false")
		if stop := startBaseAgentDistillScheduler(store); stop != nil {
			t.Fatalf("expected nil stop function when scheduler disabled")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		t.Setenv("CARRIER_MEMORY_BASE_AGENT_DISTILL_ENABLED", "true")
		t.Setenv("CARRIER_MEMORY_BASE_AGENT_DISTILL_INTERVAL", "5m")
		t.Setenv("CARRIER_MEMORY_BASE_AGENT_INSTANCE_ID", "carrier.base.test")
		stop := startBaseAgentDistillScheduler(store)
		if stop == nil {
			t.Fatalf("expected non-nil stop function when scheduler enabled")
		}
		stop()
		stop()
	})
}

