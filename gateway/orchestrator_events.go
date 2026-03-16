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
	switch execution.Status {
	case OrchestratorExecutionStatusCompleted, OrchestratorExecutionStatusPartialCompleted:
		if err := syncIntegrationUsageProofsByOrchestratorExecution(execution); err != nil {
			logOrchestratorPersistError("sync integration usage proofs", err)
		}
		if err := appendIntegrationEventByOrchestratorExecution(execution.ID, "execution.completed", map[string]interface{}{
			"status": execution.Status,
			"error":  execution.Error,
		}); err != nil {
			logOrchestratorPersistError("append integration completed event", err)
		}
	case OrchestratorExecutionStatusFailed, OrchestratorExecutionStatusRetryableFailed, OrchestratorExecutionStatusDeclined:
		if err := appendIntegrationEventByOrchestratorExecution(execution.ID, "execution.failed", map[string]interface{}{
			"status": execution.Status,
			"error":  execution.Error,
		}); err != nil {
			logOrchestratorPersistError("append integration failed event", err)
		}
	case OrchestratorExecutionStatusCancelled:
		if err := appendIntegrationEventByOrchestratorExecution(execution.ID, "execution.cancelled", map[string]interface{}{
			"status": execution.Status,
			"error":  execution.Error,
		}); err != nil {
			logOrchestratorPersistError("append integration cancelled event", err)
		}
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
