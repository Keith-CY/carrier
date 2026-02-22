package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFromContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithAgentName(ctx, "test-agent")
	ctx = WithRequestID(ctx, "req-123")
	ctx = WithOperation(ctx, "start")

	logger := FromContext(ctx, nil)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestFromContext_EmptyContext(t *testing.T) {
	logger := FromContext(context.Background(), nil)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestInit(t *testing.T) {
	t.Setenv("CARRIER_LOG_LEVEL", "DEBUG")
	logger := Init()
	if logger == nil {
		t.Fatal("expected non-nil logger from Init()")
	}
}

func TestInitDefaultLevel(t *testing.T) {
	t.Setenv("CARRIER_LOG_LEVEL", "")
	logger := Init()
	if logger == nil {
		t.Fatal("expected non-nil logger from Init() with default level")
	}
}

func TestNewWithLevel(t *testing.T) {
	logger := NewWithLevel(slog.LevelDebug)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}
