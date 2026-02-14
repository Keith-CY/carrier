// Package logging provides structured logging initialization using log/slog.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const (
	keyAgentName contextKey = "agent_name"
	keyRequestID contextKey = "request_id"
	keyOperation contextKey = "operation"
)

// Init initializes the default slog logger with the level configured via
// the CARRIER_LOG_LEVEL environment variable (DEBUG, INFO, WARN, ERROR).
// Defaults to INFO if unset or unrecognized.
func Init() *slog.Logger {
	level := ParseLevel(os.Getenv("CARRIER_LOG_LEVEL"))
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// NewWithLevel creates a logger at the specified level.
func NewWithLevel(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

// ParseLevel converts a string to slog.Level.
func ParseLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithAgentName returns a context with the agent name attached.
func WithAgentName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, keyAgentName, name)
}

// WithRequestID returns a context with the request ID attached.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// WithOperation returns a context with the operation name attached.
func WithOperation(ctx context.Context, op string) context.Context {
	return context.WithValue(ctx, keyOperation, op)
}

// FromContext extracts structured fields from context and returns a logger
// with those fields attached.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}
	if v, ok := ctx.Value(keyAgentName).(string); ok && v != "" {
		base = base.With("agent_name", v)
	}
	if v, ok := ctx.Value(keyRequestID).(string); ok && v != "" {
		base = base.With("request_id", v)
	}
	if v, ok := ctx.Value(keyOperation).(string); ok && v != "" {
		base = base.With("operation", v)
	}
	return base
}
