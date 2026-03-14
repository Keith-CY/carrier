package gateway

import "sync"

type orchestratorExecutionEvent struct {
	ExecutionID string `json:"executionId"`
	Status      string `json:"status"`
	Goal        string `json:"goal"`
	Error       string `json:"error,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

var orchestratorEventSubscribers = struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]chan orchestratorExecutionEvent
}{
	subs: map[int]chan orchestratorExecutionEvent{},
}

func publishOrchestratorExecutionEvent(execution OrchestratorExecution) {
	if !isOrchestratorExecutionTerminal(execution.Status) {
		return
	}
	evt := orchestratorExecutionEvent{
		ExecutionID: execution.ID,
		Status:      string(execution.Status),
		Goal:        execution.Goal,
		Error:       execution.Error,
		CompletedAt: execution.CompletedAt,
	}
	orchestratorEventSubscribers.mu.Lock()
	defer orchestratorEventSubscribers.mu.Unlock()
	for _, ch := range orchestratorEventSubscribers.subs {
		select {
		case ch <- evt:
		default:
		}
	}
}

func subscribeOrchestratorExecutionEvents() (int, chan orchestratorExecutionEvent) {
	orchestratorEventSubscribers.mu.Lock()
	defer orchestratorEventSubscribers.mu.Unlock()
	orchestratorEventSubscribers.nextID++
	id := orchestratorEventSubscribers.nextID
	ch := make(chan orchestratorExecutionEvent, 16)
	orchestratorEventSubscribers.subs[id] = ch
	return id, ch
}

func unsubscribeOrchestratorExecutionEvents(id int) {
	orchestratorEventSubscribers.mu.Lock()
	defer orchestratorEventSubscribers.mu.Unlock()
	ch, ok := orchestratorEventSubscribers.subs[id]
	if !ok {
		return
	}
	delete(orchestratorEventSubscribers.subs, id)
	close(ch)
}
