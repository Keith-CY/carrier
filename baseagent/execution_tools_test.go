package baseagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubWebToolBackend struct {
	fetchText string
	fetchErr  error
	searchHit []WebSearchHit
	searchErr error
}

type stubExecutionSubagentSpawner struct {
	handle SubagentJobHandle
	err    error
}

func (b *stubWebToolBackend) Fetch(_ context.Context, _ string) (string, error) {
	if b.fetchErr != nil {
		return "", b.fetchErr
	}
	return b.fetchText, nil
}

func (b *stubWebToolBackend) Search(_ context.Context, _ string, _ int) ([]WebSearchHit, error) {
	if b.searchErr != nil {
		return nil, b.searchErr
	}
	return append([]WebSearchHit(nil), b.searchHit...), nil
}

func (s *stubExecutionSubagentSpawner) Spawn(_ context.Context, _ SubagentRequest) (SubagentJobHandle, error) {
	if s.err != nil {
		return SubagentJobHandle{}, s.err
	}
	return s.handle, nil
}

func TestExecutionToolRegistryFileRoundTrip(t *testing.T) {
	root := t.TempDir()
	registry := NewExecutionToolRegistry(root)

	writeResult := registry.Execute(context.Background(), "write_file", map[string]any{
		"path":    "notes/hello.txt",
		"content": "hello from baseagent",
	})
	if writeResult.IsError {
		t.Fatalf("write_file returned error: %+v", writeResult)
	}
	if len(writeResult.FilesTouched) != 1 || writeResult.FilesTouched[0] != filepath.Join(root, "notes", "hello.txt") {
		t.Fatalf("unexpected files_touched: %+v", writeResult.FilesTouched)
	}

	readResult := registry.Execute(context.Background(), "read_file", map[string]any{
		"path": "notes/hello.txt",
	})
	if readResult.IsError {
		t.Fatalf("read_file returned error: %+v", readResult)
	}
	if !strings.Contains(readResult.Output, "hello from baseagent") {
		t.Fatalf("unexpected read output: %q", readResult.Output)
	}

	editResult := registry.Execute(context.Background(), "edit_file", map[string]any{
		"path":     "notes/hello.txt",
		"old_text": "baseagent",
		"new_text": "tool loop",
	})
	if editResult.IsError {
		t.Fatalf("edit_file returned error: %+v", editResult)
	}

	listResult := registry.Execute(context.Background(), "list_dir", map[string]any{
		"path": "notes",
	})
	if listResult.IsError {
		t.Fatalf("list_dir returned error: %+v", listResult)
	}
	if !strings.Contains(listResult.Output, "hello.txt") {
		t.Fatalf("unexpected list output: %q", listResult.Output)
	}

	raw, err := os.ReadFile(filepath.Join(root, "notes", "hello.txt"))
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	if string(raw) != "hello from tool loop" {
		t.Fatalf("unexpected final file content: %q", string(raw))
	}
}

func TestExecutionToolRegistryAppendAndPagination(t *testing.T) {
	root := t.TempDir()
	registry := NewExecutionToolRegistry(root)

	writeResult := registry.Execute(context.Background(), "write_file", map[string]any{
		"path":    "notes/log.txt",
		"content": "line-1\nline-2",
	})
	if writeResult.IsError {
		t.Fatalf("write_file returned error: %+v", writeResult)
	}

	appendResult := registry.Execute(context.Background(), "append_file", map[string]any{
		"path":    "notes/log.txt",
		"content": "\nline-3\nline-4",
	})
	if appendResult.IsError {
		t.Fatalf("append_file returned error: %+v", appendResult)
	}

	readResult := registry.Execute(context.Background(), "read_file", map[string]any{
		"path":   "notes/log.txt",
		"offset": 1,
		"limit":  2,
	})
	if readResult.IsError {
		t.Fatalf("read_file returned error: %+v", readResult)
	}
	if readResult.Output != "line-2\nline-3" {
		t.Fatalf("unexpected paginated read output: %q", readResult.Output)
	}

	listResult := registry.Execute(context.Background(), "list_dir", map[string]any{
		"path":   "notes",
		"offset": 0,
		"limit":  1,
	})
	if listResult.IsError {
		t.Fatalf("list_dir returned error: %+v", listResult)
	}
	if listResult.Output != "log.txt" {
		t.Fatalf("unexpected paginated list output: %q", listResult.Output)
	}
}

func TestExecutionToolRegistryExecBlocksDangerousCommand(t *testing.T) {
	registry := NewExecutionToolRegistry(t.TempDir())

	result := registry.Execute(context.Background(), "exec", map[string]any{
		"command": "rm -rf /",
	})
	if !result.IsError {
		t.Fatalf("expected blocked exec command, got %+v", result)
	}
	if !strings.Contains(strings.ToLower(result.Output), "denied") {
		t.Fatalf("unexpected exec output: %q", result.Output)
	}
}

func TestExecutionToolRegistryReadFileRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	leak := filepath.Join(workspace, "leak.txt")
	if err := os.Symlink(secret, leak); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	registry := NewExecutionToolRegistry(workspace)
	result := registry.Execute(context.Background(), "read_file", map[string]any{
		"path": "leak.txt",
	})
	if !result.IsError {
		t.Fatalf("expected symlink escape to be blocked, got %+v", result)
	}
	if !strings.Contains(strings.ToLower(result.Output), "workspace") &&
		!strings.Contains(strings.ToLower(result.Output), "escape") &&
		!strings.Contains(strings.ToLower(result.Output), "denied") {
		t.Fatalf("unexpected symlink escape output: %q", result.Output)
	}
}

func TestExecutionToolRegistryWriteFileRejectsSymlinkParentEscape(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	link := filepath.Join(workspace, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	registry := NewExecutionToolRegistry(workspace)
	result := registry.Execute(context.Background(), "write_file", map[string]any{
		"path":    "escape/payload.txt",
		"content": "should stay inside workspace",
	})
	if !result.IsError {
		t.Fatalf("expected symlink parent escape to be blocked, got %+v", result)
	}
	if _, err := os.Stat(filepath.Join(outside, "payload.txt")); err == nil {
		t.Fatal("write_file escaped workspace via symlink parent")
	}
}

func TestExecutionToolRegistryReadFileOffsetBeyondEOF(t *testing.T) {
	root := t.TempDir()
	registry := NewExecutionToolRegistry(root)

	writeResult := registry.Execute(context.Background(), "write_file", map[string]any{
		"path":    "notes/short.txt",
		"content": "line-1\nline-2",
	})
	if writeResult.IsError {
		t.Fatalf("write_file returned error: %+v", writeResult)
	}

	readResult := registry.Execute(context.Background(), "read_file", map[string]any{
		"path":   "notes/short.txt",
		"offset": 10,
		"limit":  2,
	})
	if readResult.IsError {
		t.Fatalf("read_file should return EOF marker, got error: %+v", readResult)
	}
	if readResult.Output != "[END OF FILE - no content at this offset]" {
		t.Fatalf("unexpected EOF read output: %q", readResult.Output)
	}
}

func TestExecutionToolRegistryExecTimeout(t *testing.T) {
	registry := NewExecutionToolRegistry(t.TempDir())

	result := registry.Execute(context.Background(), "exec", map[string]any{
		"command":         "sleep 2",
		"timeout_seconds": 1,
	})
	if !result.IsError {
		t.Fatalf("expected timeout to be reported as error, got %+v", result)
	}
	if !strings.Contains(strings.ToLower(result.Output), "timed out") {
		t.Fatalf("unexpected timeout output: %q", result.Output)
	}
}

func TestExecutionToolRegistryExecTruncatesLongOutput(t *testing.T) {
	registry := NewExecutionToolRegistry(t.TempDir())

	result := registry.Execute(context.Background(), "exec", map[string]any{
		"command": "python3 -c \"print('x' * 12000)\"",
	})
	if result.IsError {
		t.Fatalf("expected long output command to succeed, got %+v", result)
	}
	if len(result.Output) > 9000 {
		t.Fatalf("expected truncated output, got len=%d", len(result.Output))
	}
	if !strings.Contains(strings.ToLower(result.Output), "truncated") {
		t.Fatalf("expected truncation marker, got %q", result.Output)
	}
}

func TestExecutionToolRegistryEditFileValidationFailures(t *testing.T) {
	root := t.TempDir()
	registry := NewExecutionToolRegistry(root)

	writeResult := registry.Execute(context.Background(), "write_file", map[string]any{
		"path":    "notes/repeated.txt",
		"content": "test test test",
	})
	if writeResult.IsError {
		t.Fatalf("write_file returned error: %+v", writeResult)
	}

	t.Run("old text missing", func(t *testing.T) {
		result := registry.Execute(context.Background(), "edit_file", map[string]any{
			"path":     "notes/repeated.txt",
			"old_text": "missing",
			"new_text": "done",
		})
		if !result.IsError {
			t.Fatalf("expected missing old_text to fail, got %+v", result)
		}
		if !strings.Contains(result.Output, "not found") {
			t.Fatalf("unexpected missing old_text output: %q", result.Output)
		}
	})

	t.Run("multiple matches", func(t *testing.T) {
		result := registry.Execute(context.Background(), "edit_file", map[string]any{
			"path":     "notes/repeated.txt",
			"old_text": "test",
			"new_text": "done",
		})
		if !result.IsError {
			t.Fatalf("expected multiple matches to fail, got %+v", result)
		}
		if !strings.Contains(result.Output, "multiple") {
			t.Fatalf("unexpected multiple match output: %q", result.Output)
		}
	})
}

func TestExecutionToolRegistryExecReportsStderrAndExitCode(t *testing.T) {
	registry := NewExecutionToolRegistry(t.TempDir())

	result := registry.Execute(context.Background(), "exec", map[string]any{
		"command": "echo boom >&2; exit 7",
	})
	if !result.IsError {
		t.Fatalf("expected command failure, got %+v", result)
	}
	if result.ExitCode != 7 {
		t.Fatalf("unexpected exit code: %+v", result)
	}
	if !strings.Contains(result.Stderr, "boom") || !strings.Contains(result.Output, "boom") {
		t.Fatalf("expected stderr to be surfaced, got %+v", result)
	}
}

func TestExecutionToolRegistryDescriptors(t *testing.T) {
	registry := NewExecutionToolRegistry(t.TempDir())
	descriptors := registry.Descriptors()
	if len(descriptors) < 6 {
		t.Fatalf("expected core execution descriptors, got %d", len(descriptors))
	}

	seen := map[string]StructuredToolDescriptor{}
	for _, descriptor := range descriptors {
		seen[descriptor.Name] = descriptor
	}

	if seen["write_file"].Description == "" || seen["write_file"].Parameters == nil {
		t.Fatalf("write_file descriptor should include description and parameters: %+v", seen["write_file"])
	}
	if seen["append_file"].Description == "" || seen["append_file"].Parameters == nil {
		t.Fatalf("append_file descriptor should include description and parameters: %+v", seen["append_file"])
	}
	if seen["exec"].Description == "" || seen["exec"].Parameters == nil {
		t.Fatalf("exec descriptor should include description and parameters: %+v", seen["exec"])
	}
}

func TestExecutionToolsWebFetch(t *testing.T) {
	registry := NewExecutionToolRegistry(t.TempDir(), WithExecutionToolWebBackend(&stubWebToolBackend{
		fetchText: "Example Domain\nThis domain is for use in illustrative examples.",
	}))

	result := registry.Execute(context.Background(), "web_fetch", map[string]any{
		"url": "https://example.com/docs",
	})
	if result.IsError {
		t.Fatalf("expected web_fetch to succeed, got %+v", result)
	}
	if !strings.Contains(result.Output, "Example Domain") {
		t.Fatalf("unexpected web_fetch output: %q", result.Output)
	}
	if got, _ := result.Metadata["source_url"].(string); got != "https://example.com/docs" {
		t.Fatalf("expected source_url metadata, got %+v", result.Metadata)
	}
}

func TestExecutionToolsWebSearch(t *testing.T) {
	registry := NewExecutionToolRegistry(t.TempDir(), WithExecutionToolWebBackend(&stubWebToolBackend{
		searchHit: []WebSearchHit{
			{Title: "Carrier docs", URL: "https://example.com/carrier", Snippet: "Baseagent tools"},
			{Title: "Picoclaw docs", URL: "https://example.com/picoclaw", Snippet: "Reference implementation"},
		},
	}))

	result := registry.Execute(context.Background(), "web_search", map[string]any{
		"query": "carrier baseagent structured tools",
		"limit": 2,
	})
	if result.IsError {
		t.Fatalf("expected web_search to succeed, got %+v", result)
	}
	if !strings.Contains(result.Output, "Carrier docs") || !strings.Contains(result.Output, "Picoclaw docs") {
		t.Fatalf("unexpected web_search output: %q", result.Output)
	}
	hits, _ := result.Metadata["search_results"].([]WebSearchHit)
	if len(hits) != 2 || hits[0].URL != "https://example.com/carrier" {
		t.Fatalf("unexpected search_results metadata: %+v", result.Metadata)
	}
}

func TestExecutionToolsSendFile(t *testing.T) {
	root := t.TempDir()
	registry := NewExecutionToolRegistry(root)
	if err := os.WriteFile(filepath.Join(root, "artifact.log"), []byte("hello artifact"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	result := registry.Execute(context.Background(), "send_file", map[string]any{
		"path": "artifact.log",
	})
	if result.IsError {
		t.Fatalf("expected send_file to succeed, got %+v", result)
	}
	if !strings.Contains(result.Output, "artifact.log") {
		t.Fatalf("unexpected send_file output: %q", result.Output)
	}
	attachment, _ := result.Metadata["attachment"].(ExecutionAttachment)
	if attachment.Name != "artifact.log" || attachment.Path != filepath.Join(root, "artifact.log") {
		t.Fatalf("unexpected attachment metadata: %+v", result.Metadata)
	}
	if len(result.Attachments) != 1 {
		t.Fatalf("expected first-class attachment ref, got %+v", result)
	}
	if result.Attachments[0].Name != "artifact.log" || result.Attachments[0].Path != filepath.Join(root, "artifact.log") {
		t.Fatalf("unexpected attachment refs: %+v", result.Attachments)
	}
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected structured content block, got %+v", result)
	}
	if result.ContentBlocks[0].Type != "file" || result.ContentBlocks[0].Name != "artifact.log" {
		t.Fatalf("unexpected content blocks: %+v", result.ContentBlocks)
	}
	if result.ContentBlocks[0].AttachmentID == "" || result.ContentBlocks[0].AttachmentID != result.Attachments[0].ID {
		t.Fatalf("expected content block to link attachment, block=%+v attachment=%+v", result.ContentBlocks[0], result.Attachments[0])
	}
	if result.ContentBlocks[0].MediaType != result.Attachments[0].MediaType {
		t.Fatalf("expected content block media type to match attachment, block=%+v attachment=%+v", result.ContentBlocks[0], result.Attachments[0])
	}
}

func TestExecutionToolsSpawnSubagentIncludesDelegationMetadata(t *testing.T) {
	registry := NewExecutionToolRegistry(t.TempDir(), WithExecutionToolSubagentSpawner(&stubExecutionSubagentSpawner{
		handle: SubagentJobHandle{
			JobID:   "job-7",
			Status:  "running",
			Summary: "collect dependency graph",
		},
	}))

	result := registry.Execute(context.Background(), "spawn_subagent", map[string]any{
		"task": "collect dependency graph",
	})
	if result.IsError {
		t.Fatalf("expected spawn_subagent to succeed, got %+v", result)
	}
	if result.Metadata["job_id"] != "job-7" {
		t.Fatalf("expected job_id metadata, got %+v", result.Metadata)
	}
	if result.Metadata["job_status"] != "running" {
		t.Fatalf("expected job_status metadata, got %+v", result.Metadata)
	}
	if result.Metadata["job_summary"] != "collect dependency graph" {
		t.Fatalf("expected job_summary metadata, got %+v", result.Metadata)
	}
}

func TestNewSessionManagerWithStorageRoundTripsHistoryAndSummary(t *testing.T) {
	dir := t.TempDir()
	sessionKey := "cli:test"

	first := NewSessionManagerWithStorage(8, dir)
	first.AddMessage(sessionKey, "user", "hello")
	first.AddMessage(sessionKey, "assistant", "world")
	first.SetSummary(sessionKey, "short summary")

	second := NewSessionManagerWithStorage(8, dir)
	history := second.History(sessionKey)
	if len(history) != 2 {
		t.Fatalf("expected 2 persisted messages, got %d", len(history))
	}
	if history[0].Content != "hello" || history[1].Content != "world" {
		t.Fatalf("unexpected persisted history: %+v", history)
	}
	if got := second.Summary(sessionKey); got != "short summary" {
		t.Fatalf("unexpected persisted summary: %q", got)
	}
}

func TestNewSessionManagerWithStorageSkipsUnsafeKeys(t *testing.T) {
	dir := t.TempDir()

	first := NewSessionManagerWithStorage(8, dir)
	first.AddMessage(".", "user", "should not persist")

	second := NewSessionManagerWithStorage(8, dir)
	if history := second.History("."); len(history) != 0 {
		t.Fatalf("expected unsafe key to be skipped during persistence, got %+v", history)
	}
}

func TestNewSessionManagerWithStorageSkipsCorruptedJSONAndLoadsValidSessions(t *testing.T) {
	dir := t.TempDir()
	sessionKey := "cli:good"

	first := NewSessionManagerWithStorage(8, dir)
	first.AddMessage(sessionKey, "user", "hello")
	first.SetSummary(sessionKey, "valid summary")

	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write broken session file: %v", err)
	}

	second := NewSessionManagerWithStorage(8, dir)
	history := second.History(sessionKey)
	if len(history) != 1 || history[0].Content != "hello" {
		t.Fatalf("unexpected persisted history after corrupted file: %+v", history)
	}
	if got := second.Summary(sessionKey); got != "valid summary" {
		t.Fatalf("unexpected persisted summary after corrupted file: %q", got)
	}
}

func TestNewSessionManagerWithStorageReloadsCompactedSummary(t *testing.T) {
	dir := t.TempDir()
	sessionKey := "cli:compact"

	first := NewSessionManagerWithStorage(2, dir)
	first.AddMessage(sessionKey, "user", "one")
	first.AddMessage(sessionKey, "assistant", "two")
	first.AddMessage(sessionKey, "user", "three")
	first.AddMessage(sessionKey, "assistant", "four")

	second := NewSessionManagerWithStorage(2, dir)
	history := second.History(sessionKey)
	if len(history) != 2 {
		t.Fatalf("expected compacted history to persist with 2 messages, got %d", len(history))
	}
	summary := second.Summary(sessionKey)
	if !strings.Contains(summary, "[Compacted 1 earlier messages]") || !strings.Contains(summary, "- assistant: two") {
		t.Fatalf("expected compacted summary to persist, got %q", summary)
	}
}

func TestNewSessionManagerWithStorageTruncatesOversizedSummaryOnLoad(t *testing.T) {
	dir := t.TempDir()
	sessionKey := "cli:oversized"
	filename, ok := sessionStorageFilename(sessionKey)
	if !ok {
		t.Fatalf("expected valid filename for session key")
	}

	session := ConversationSession{
		Key:       sessionKey,
		Summary:   strings.Repeat("s", maxSessionSummaryBytes+128),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), raw, 0o600); err != nil {
		t.Fatalf("write oversized session file: %v", err)
	}

	reloaded := NewSessionManagerWithStorage(8, dir)
	summary := reloaded.Summary(sessionKey)
	if len(summary) > maxSessionSummaryBytes+3 {
		t.Fatalf("expected reloaded summary to be truncated, got length %d", len(summary))
	}
	if !strings.HasSuffix(summary, "...") {
		t.Fatalf("expected reloaded truncated summary to end with ellipsis, got %q", summary[len(summary)-10:])
	}
}

func TestNewSessionManagerWithStorageRoundTripsPendingApproval(t *testing.T) {
	dir := t.TempDir()
	sessionKey := "cli:approval"
	requestedAt := time.Now().UTC().Round(0)
	expiresAt := requestedAt.Add(15 * time.Minute)

	first := NewSessionManagerWithStorage(8, dir)
	first.SetPendingApprovals(sessionKey, []*PendingToolApproval{
		{
			ID:          "approval-1",
			ToolName:    "exec",
			Arguments:   map[string]any{"command": "go test ./..."},
			RequestedAt: requestedAt,
			ExpiresAt:   expiresAt,
			Reason:      "Shell execution requires confirmation.",
			RuleID:      structuredPolicyRuleExecConfirmationNeeded,
		},
		{
			ID:          "approval-2",
			ToolName:    "agent_start",
			RequestedAt: requestedAt,
			ExpiresAt:   expiresAt,
			Reason:      "Agent lifecycle mutation requires confirmation.",
			RuleID:      structuredPolicyRuleAgentConfirmation,
		},
	})

	second := NewSessionManagerWithStorage(8, dir)
	pending := second.PendingApprovals(sessionKey)
	if len(pending) != 2 {
		t.Fatalf("expected pending approvals to persist, got %+v", pending)
	}
	if pending[0].ID != "approval-1" || pending[0].ToolName != "exec" {
		t.Fatalf("unexpected first pending approval metadata: %+v", pending[0])
	}
	if pending[0].RequestedAt.IsZero() || !pending[0].RequestedAt.Equal(requestedAt) || !pending[0].ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected first pending approval timestamps: %+v", pending[0])
	}
	if command, _ := pending[0].Arguments["command"].(string); command != "go test ./..." {
		t.Fatalf("unexpected first pending approval args: %+v", pending[0].Arguments)
	}
	if pending[0].RuleID != structuredPolicyRuleExecConfirmationNeeded || pending[0].Reason == "" {
		t.Fatalf("expected first pending approval audit metadata to persist, got %+v", pending[0])
	}
}
