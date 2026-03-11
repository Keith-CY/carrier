package baseagent

import (
	"testing"
)

func TestSessionManagerPersistsStructuredToolMetadata(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionManagerWithStorage(8, dir)
	sm.AddStructuredToolMessage("cli:meta", StructuredToolMessage{
		Role:             "tool",
		Content:          "needs confirmation",
		ToolCallID:       "call-1",
		ToolName:         "exec",
		ToolResultStatus: ExecutionToolResultStatusAsk,
	})

	reloaded := NewSessionManagerWithStorage(8, dir)
	got := reloaded.StructuredHistory("cli:meta")
	if len(got) != 1 {
		t.Fatalf("expected 1 structured event, got %+v", got)
	}
	if got[0].ToolCallID != "call-1" || got[0].ToolName != "exec" || got[0].ToolResultStatus != ExecutionToolResultStatusAsk {
		t.Fatalf("unexpected structured history metadata: %+v", got[0])
	}
	if got[0].Content != "needs confirmation" {
		t.Fatalf("unexpected structured history content: %+v", got[0])
	}
}
