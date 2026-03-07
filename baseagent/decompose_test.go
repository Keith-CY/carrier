package baseagent

import (
	"strings"
	"testing"
)

func TestParseDecomposeTasks_ArrayPayload(t *testing.T) {
	raw := `[{"id":"task-1","input":"summarize logs","agentId":"picoclaw"},{"input":"triage incidents","agentId":"unknown"}]`
	tasks, err := parseDecomposeTasks(raw)
	if err != nil {
		t.Fatalf("parseDecomposeTasks failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != "task-1" || tasks[0].AgentID != "picoclaw" {
		t.Fatalf("unexpected task[0]: %+v", tasks[0])
	}
	if tasks[1].ID == "" || tasks[1].AgentID != "" {
		t.Fatalf("unexpected task[1]: %+v", tasks[1])
	}
}

func TestParseDecomposeTasks_ObjectPayload(t *testing.T) {
	raw := `{"tasks":[{"id":"a","input":"analyze errors","agentId":"zeroclaw"}]}`
	tasks, err := parseDecomposeTasks(raw)
	if err != nil {
		t.Fatalf("parseDecomposeTasks failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "a" || tasks[0].AgentID != "zeroclaw" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestParseDecomposeTasks_FencedJSON(t *testing.T) {
	raw := "```json\n[{\"input\":\"inspect backlog\"}]\n```"
	tasks, err := parseDecomposeTasks(raw)
	if err != nil {
		t.Fatalf("parseDecomposeTasks failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Input != "inspect backlog" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestParseDecomposeTasks_MultipleFencedBlocks(t *testing.T) {
	raw := strings.Join([]string{
		"```text",
		"not-json",
		"```",
		"noise",
		"```json",
		"[{\"input\":\"second block task\"}]",
		"```",
	}, "\n")
	tasks, err := parseDecomposeTasks(raw)
	if err != nil {
		t.Fatalf("parseDecomposeTasks failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Input != "second block task" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestParseDecomposeTasks_NoJSON(t *testing.T) {
	_, err := parseDecomposeTasks("not a json payload")
	if err == nil || !strings.Contains(err.Error(), "no JSON") {
		t.Fatalf("expected no JSON error, got %v", err)
	}
}
